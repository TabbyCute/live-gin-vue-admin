package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"tb_live_module/global"
	liveReq "tb_live_module/model/live/request"
)

func TestRequestTraceAndRecoveryNeverLogSecrets(t *testing.T) {
	oldLog, oldDB := global.GVA_LOG, global.GVA_DB
	t.Cleanup(func() { global.GVA_LOG, global.GVA_DB = oldLog, oldDB })
	core, logs := observer.New(zap.InfoLevel)
	global.GVA_LOG, global.GVA_DB = zap.New(core), nil
	r := gin.New()
	r.Use(RequestObservability(), GinRecovery(true))
	r.GET("/account/:account", func(c *gin.Context) {
		require.Equal(t, RequestID(c), RequestID(c))
		panic("private-panic-password")
	})
	q := httptest.NewRequest("GET", "/account/private-account?token=private-query", nil)
	q.Header.Set("Authorization", "Bearer private-token")
	q.Header.Set("Cookie", "session=private-cookie")
	q.Header.Set("X-Request-ID", "private-injected-trace")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	require.Equal(t, 500, w.Code)
	require.True(t, strings.HasPrefix(w.Header().Get("X-Request-ID"), "rt_"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.NotEmpty(t, logs.All())
	for _, entry := range logs.All() {
		require.NotContains(t, fmt.Sprint(entry.ContextMap()), "private-")
	}
	last := logs.All()[logs.Len()-1].ContextMap()
	require.Equal(t, w.Header().Get("X-Request-ID"), last["traceId"])
	require.Equal(t, "/account/:account", last["route"])
	require.EqualValues(t, 500, last["status"])
	logs.TakeAll()
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/private-unmatched-path?secret=private-query", nil))
	require.Equal(t, "[unmatched]", logs.All()[0].ContextMap()["route"])
}

func TestPrivateAccountGuardCapacityAndBodyLimits(t *testing.T) {
	oldRedis, oldConfig := global.GVA_REDIS, global.GVA_CONFIG
	t.Cleanup(func() { global.GVA_REDIS, global.GVA_CONFIG = oldRedis, oldConfig })
	global.GVA_REDIS = nil
	global.GVA_CONFIG.System.UseRedis = false
	g := NewAccountAuthGuard()
	r := gin.New()
	r.POST("/change", func(c *gin.Context) {
		c.Set(appClaimsContextKey, &liveReq.AppClaims{AccountID: 7})
	}, g.PrivateHandler("password", 2), func(c *gin.Context) { c.Status(204) })
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		q := httptest.NewRequest("POST", "/change", strings.NewReader(`{}`))
		q.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i))
		r.ServeHTTP(w, q)
		if i < 2 {
			require.Equal(t, 204, w.Code)
		} else {
			require.Equal(t, 429, w.Code)
			require.NotEmpty(t, w.Header().Get("Retry-After"))
		}
	}
	for _, chunked := range []bool{false, true} {
		g = NewAccountAuthGuard()
		r = gin.New()
		r.POST("/change", func(c *gin.Context) { c.Set(appClaimsContextKey, &liveReq.AppClaims{AccountID: 7}) }, g.PrivateHandler("password", 5), func(c *gin.Context) { t.Error("oversized body reached handler") })
		q := httptest.NewRequest("POST", "/change", strings.NewReader(strings.Repeat("x", accountAuthMaxBody+1)))
		if chunked {
			q.ContentLength = -1
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		require.Empty(t, g.authSlots)
	}
}

func TestCORSAllowAllNeverExposesCredentials(t *testing.T) {
	r := gin.New()
	r.Use(Cors())
	r.GET("/", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	q := httptest.NewRequest("GET", "/", nil)
	q.Header.Set("Origin", "https://attacker.example")
	r.ServeHTTP(w, q)
	require.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSStrictPreservesSameOriginAndRejectsForgedProxy(t *testing.T) {
	restoreCorsConfig(t)
	global.GVA_CONFIG.Cors.Mode = "strict-whitelist"
	global.GVA_CONFIG.Cors.Whitelist = nil
	r := gin.New()
	r.Use(CorsByRules())
	r.POST("/", func(c *gin.Context) { c.Status(204) })
	for _, origin := range []string{"http://example.com", "https://attacker.example", "http://example.com.attacker.example", "http://example.com@attacker.example"} {
		q := httptest.NewRequest("POST", "http://example.com/", nil)
		q.Header.Set("Origin", origin)
		q.Header.Set("X-Forwarded-Host", "attacker.example")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if origin == "http://example.com" {
			require.Equal(t, 204, w.Code)
		} else {
			require.Equal(t, 403, w.Code)
		}
	}
}
