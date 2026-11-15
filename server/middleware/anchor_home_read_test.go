package middleware

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"tb_live_module/global"
	"testing"
)

func TestAnchorHomeReadLimitRejectsSpoofedForwardingAndIsAtomic(t *testing.T) {
	oldConfig, oldRedis := global.GVA_CONFIG, global.GVA_REDIS
	global.GVA_CONFIG.System.UseRedis = false
	global.GVA_CONFIG.LiveAccountAuth.TrustedProxies = nil
	global.GVA_REDIS = nil
	t.Cleanup(func() { global.GVA_CONFIG = oldConfig; global.GVA_REDIS = oldRedis })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AnchorHomeReadLimit("json", 10))
	r.GET("/home", func(c *gin.Context) { c.Status(204) })
	var success, limited atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/home", nil)
			req.RemoteAddr = "192.0.2.1:1234"
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			switch rec.Code {
			case 204:
				success.Add(1)
			case 429:
				limited.Add(1)
				if rec.Header().Get("Retry-After") == "" {
					t.Error("missing retry-after")
				}
			default:
				t.Errorf("status %d", rec.Code)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, int64(10), success.Load())
	require.Equal(t, int64(40), limited.Load())
	req := httptest.NewRequest(http.MethodGet, "/home", nil)
	req.RemoteAddr = "192.0.2.2:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, 204, rec.Code)
}

func TestAnchorHomeReadLimitRequiredRedisFailsClosed(t *testing.T) {
	oldConfig, oldRedis := global.GVA_CONFIG, global.GVA_REDIS
	global.GVA_CONFIG.System.UseRedis = true
	global.GVA_REDIS = nil
	t.Cleanup(func() { global.GVA_CONFIG = oldConfig; global.GVA_REDIS = oldRedis })
	r := gin.New()
	r.Use(AnchorHomeReadLimit("media", 10))
	r.GET("/photo", func(c *gin.Context) { t.Error("handler ran during required Redis outage") })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/photo", nil))
	require.Equal(t, 503, rec.Code)
}
