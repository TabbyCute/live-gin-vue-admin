package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tb_live_module/global"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func testAccountAuthGuard(t *testing.T) *AccountAuthGuard {
	t.Helper()
	previous := global.GVA_REDIS
	global.GVA_REDIS = nil
	t.Cleanup(func() { global.GVA_REDIS = previous })
	return &AccountAuthGuard{
		key: []byte("auth-limit-test-key"), entries: make(map[string]accountAuthLimitEntry),
		authSlots: make(chan struct{}, 1), smsSlots: make(chan struct{}, 4),
	}
}

func TestAccountAuthLocalLimitsAreAtomicAndExpire(t *testing.T) {
	guard := testAccountAuthGuard(t)
	now := time.Now()
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if guard.allowLocal("one-phone", 10, time.Minute, now) == 0 {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 10, allowed.Load())
	require.Zero(t, guard.allowLocal("one-phone", 10, time.Minute, now.Add(time.Minute)))
	for i := 0; i < accountAuthMaxLimitEntries; i++ {
		guard.allowLocal(fmt.Sprint(i), 1, time.Minute, now.Add(time.Minute))
	}
	require.Len(t, guard.entries, accountAuthMaxLimitEntries)
	require.Positive(t, guard.allowLocal("overflow", 1, time.Minute, now.Add(time.Minute)))
	require.Zero(t, guard.allowLocal("after-expiry", 1, time.Minute, now.Add(2*time.Minute)))
}

func TestAccountAuthBoundsRequestsAndRateLimitsPhone(t *testing.T) {
	guard := testAccountAuthGuard(t)
	router := gin.New()
	router.POST("/login", guard.Handler("login", 100, 1, time.Minute), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func(body, remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	valid := `{"phone":"+60123456789","password":"password123"}`
	require.Equal(t, http.StatusNoContent, request(valid, "192.0.2.1:1234").Code)
	limited := request(valid, "192.0.2.2:1234")
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.NotEmpty(t, limited.Header().Get("Retry-After"))
	require.Equal(t, "no-store", limited.Header().Get("Cache-Control"))
	require.Equal(t, http.StatusBadRequest, request(`{"username":"demo"}`, "192.0.2.1:1234").Code)
	require.Equal(t, http.StatusBadRequest, request(valid+valid, "192.0.2.1:1234").Code)
	require.Equal(t, http.StatusRequestEntityTooLarge, request(strings.Repeat("x", accountAuthMaxBody+1), "192.0.2.1:1234").Code)
	for key := range guard.entries {
		require.NotContains(t, key, "+60123456789")
		require.NotContains(t, key, "192.0.2.1")
	}
}

func TestAccountAuthTrustsOnlyConfiguredProxies(t *testing.T) {
	guard := testAccountAuthGuard(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.RemoteAddr = "192.0.2.1:4321"
	c.Request.Header.Set("X-Forwarded-For", "203.0.113.5, 198.51.100.10")
	require.Equal(t, "192.0.2.1", guard.clientIP(c))
	guard.trustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")}
	require.Equal(t, "198.51.100.10", guard.clientIP(c), "忽略攻击者伪造的更左侧地址")
	guard.trustedProxies = append(guard.trustedProxies, netip.MustParsePrefix("198.51.100.10/32"))
	require.Equal(t, "203.0.113.5", guard.clientIP(c))
	c.Request.Header.Set("X-Forwarded-For", "invalid")
	require.Equal(t, "192.0.2.1", guard.clientIP(c))
}

type unavailableAccountRedis struct{ redis.UniversalClient }

func (unavailableAccountRedis) EvalSha(context.Context, string, []string, ...interface{}) *redis.Cmd {
	return redis.NewCmdResult(nil, errors.New("unavailable"))
}

func TestAccountAuthFailsClosedWhenRedisIsRequiredOrUnavailable(t *testing.T) {
	guard := testAccountAuthGuard(t)
	guard.requireRedis = true
	for _, client := range []redis.UniversalClient{nil, unavailableAccountRedis{}} {
		global.GVA_REDIS = client
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		require.False(t, guard.allow(c, "register:ip", "192.0.2.1", 10, time.Minute))
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		require.Empty(t, guard.entries)
	}
}

func TestAccountAuthBoundsConcurrentPasswordWork(t *testing.T) {
	guard := testAccountAuthGuard(t)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	router := gin.New()
	router.POST("/login", guard.Handler("login", 20, 10, time.Minute), func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(http.StatusNoContent)
	})
	first := httptest.NewRecorder()
	go func() {
		defer close(finished)
		router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"phone":"+60123456789"}`)))
	}()
	<-entered
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"phone":"+60123456780"}`)))
	close(release)
	<-finished
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	require.Equal(t, http.StatusNoContent, first.Code)
	require.Empty(t, guard.authSlots)
}
