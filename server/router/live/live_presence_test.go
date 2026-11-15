package live

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"tb_live_module/config"
	"tb_live_module/global"
	systemReq "tb_live_module/model/system/request"
	svc "tb_live_module/service/live"
	"tb_live_module/utils"
	"testing"
)

func TestPresenceAdminCannotUseMenuOrAPPIdentity(t *testing.T) {
	g := gin.New()
	root := g.Group("/admin")
	root.Use(func(c *gin.Context) {
		c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{ID: 2, AuthorityId: 9528}})
		c.Next()
	})
	new(PresenceRouter).InitPresenceAdminRouter(root)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/admin/live/presence/stats", strings.NewReader(`{"roomNos":["room"],"sessionNos":[]}`))
	g.ServeHTTP(w, r)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}
func TestPresenceHTTPAuthenticationAndBudgets(t *testing.T) {
	addr := os.Getenv("PRESENCE_TEST_REDIS")
	if addr == "" {
		t.Skip("isolated Redis required")
	}
	require.True(t, strings.HasPrefix(addr, "127.0.0.1:"))
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15, ContextTimeoutEnabled: true})
	defer rdb.Close()
	require.NoError(t, rdb.Ping(context.Background()).Err())
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, e)
	pool, _ := db.DB()
	defer pool.Close()
	s := svc.DefaultPresence
	oldDB, oldRedis, oldKey := s.DB, s.Redis, s.Key
	s.DB = db
	s.Redis = rdb
	s.Key = []byte(strings.Repeat("http-presence-tests", 3))
	defer func() { s.DB = oldDB; s.Redis = oldRedis; s.Key = oldKey }()
	oldConfig := global.GVA_CONFIG
	defer func() { global.GVA_CONFIG = oldConfig }()
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: strings.Repeat("app-key-", 5), Issuer: "presence-tests", ExpiresTime: "1h"}
	global.GVA_CONFIG.JWT.SigningKey = "admin-key"
	token, _, e := utils.NewAppJWT().CreateToken(601, "member", 1)
	require.NoError(t, e)
	g := gin.New()
	new(PresenceRouter).InitPresenceRouter(g.Group("/api/v1/app/live"))
	call := func(path, body, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/app/live/"+path, strings.NewReader(body))
		r.Header.Set("Authorization", auth)
		r.RemoteAddr = "192.0.2.44:1234"
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		return w
	}
	require.Equal(t, http.StatusForbidden, call("presence/open", `{"roomNo":"room","requestId":"request_1"}`, "").Code)
	require.Equal(t, http.StatusUnauthorized, call("presence/open", `{"roomNo":"room","requestId":"request_1"}`, "Bearer bad").Code)
	require.Equal(t, http.StatusBadRequest, call("presence/open", `{"roomNo":"room","requestId":"request_1","accountId":1}`, "Bearer "+token).Code)
	require.Equal(t, http.StatusBadRequest, call("presence/open", strings.Repeat("x", 4097), "Bearer "+token).Code)
	guest := call("presence/guest", `{}`, "")
	require.Equal(t, http.StatusOK, guest.Code)
	require.Contains(t, guest.Body.String(), "guestToken")
	// Redis failure must not be reported as heartbeat/like success or expose internals.
	s.Redis = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer s.Redis.Close()
	w := call("room/like", `{"presenceToken":"private-secret","requestId":"request_1","sequence":"1","count":"1"}`, "Bearer "+token)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.NotContains(t, w.Body.String(), "127.0.0.1")
	require.NotContains(t, w.Body.String(), "private-secret")
}
