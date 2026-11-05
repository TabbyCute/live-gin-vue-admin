package payout_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/songzhibin97/gkit/cache/local_cache"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"strings"
	"tb_live_module/config"
	"tb_live_module/global"
	"tb_live_module/initialize"
	"tb_live_module/middleware"
	em "tb_live_module/model/earning"
	live "tb_live_module/model/live"
	system "tb_live_module/model/system"
	sr "tb_live_module/model/system/request"
	router "tb_live_module/router/payout"
	svc "tb_live_module/service/payout"
	"tb_live_module/utils"
	"testing"
	"time"
)

func TestPayoutHTTPIsolationAndLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/http.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	require.NoError(t, db.AutoMigrate(&live.LiveAccount{}, &live.LiveAnchor{}))
	require.NoError(t, initialize.MigratePayout(db))
	u := live.LiveAccount{AccountNo: "http_user", Username: "http_user", Status: 1, TokenVersion: 1}
	require.NoError(t, db.Create(&u).Error)
	require.NoError(t, db.Create(&live.LiveAnchor{UserId: uint64(u.ID), AnchorNo: "http_anchor", Status: 1, ApplyStatus: 2}).Error)
	oldDB, oldLog, oldConfig, oldCache, oldSvc := global.GVA_DB, global.GVA_LOG, global.GVA_CONFIG, global.BlackCache, *svc.Default
	core, logs := observer.New(zap.DebugLevel)
	global.GVA_DB = db
	global.GVA_LOG = zap.New(core)
	global.BlackCache = local_cache.NewCache(local_cache.SetDefaultExpire(time.Hour))
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "payout-test-admin-key", Issuer: "payout-admin", ExpiresTime: "1h", BufferTime: "1m"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "payout-test-app-key", Issuer: "payout-app", ExpiresTime: "1h"}
	c, e := svc.NewCrypto("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32))))
	require.NoError(t, e)
	*svc.Default = svc.Service{DB: db, Crypto: c}
	t.Cleanup(func() {
		global.GVA_DB = oldDB
		global.GVA_LOG = oldLog
		global.GVA_CONFIG = oldConfig
		global.BlackCache = oldCache
		*svc.Default = oldSvc
	})
	appToken, _, e := utils.NewAppJWT().CreateToken(u.ID, u.Username, 1)
	require.NoError(t, e)
	j := utils.NewJWT()
	adminToken, e := j.CreateToken(j.CreateClaims(sr.BaseClaims{ID: 1, AuthorityId: 888, Username: "admin"}))
	require.NoError(t, e)
	engine := gin.New()
	engine.Use(middleware.GinRecovery(true))
	router.RouterGroup{}.InitPayoutRouter(engine.Group("/api/v1/app"))
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	for _, route := range engine.Routes() {
		require.Equal(t, http.StatusUnauthorized, call(route.Method, route.Path, "{}", "").Code)
	}
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/app/payout/payee/list", "", adminToken).Code)
	for _, suffix := range []string{"edit", "delete", "decrypt", "verify", "paid"} {
		require.Equal(t, 404, call("POST", "/api/v1/app/payout/payee/"+suffix, "{}", appToken).Code)
	}
	fullAccount := "6222021234567890123"
	for _, forged := range []string{`"ownerNo":"other"`, `"anchorNo":"other"`, `"version":"1"`, `"isDefault":true`} {
		w := call("POST", "/api/v1/app/payout/payee/create", `{"requestId":"http_create","account":"`+fullAccount+`",`+forged+`}`, appToken)
		require.Contains(t, w.Body.String(), `"code":49101`)
		require.NotContains(t, w.Body.String(), fullAccount)
	}
	w := call("POST", "/api/v1/app/payout/payee/create", `{"requestId":"http_create","method":"bank_card","currency":"CNY","account":"`+fullAccount+`","accountName":"private-name","bankCode":"ICBC"}`, appToken)
	require.Contains(t, w.Body.String(), `"code":0`)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.NotEmpty(t, w.Header().Get("X-Request-ID"))
	for _, secret := range []string{fullAccount, "private-name", "encryptedPayload", "accountFingerprint", "ownerNo", "\"id\""} {
		require.NotContains(t, w.Body.String(), secret)
	}
	require.Equal(t, 400, call("POST", "/api/v1/app/payout/payee/create", strings.Repeat("x", 8193), appToken).Code)
	require.Contains(t, call("GET", "/api/v1/app/payout/payee/list?ownerNo=other", "", appToken).Body.String(), `"code":49101`)
	throttled := false
	for i := 0; i < 20; i++ {
		if call("GET", "/api/v1/app/payout/payee/list", "", appToken).Code == 429 {
			throttled = true
		}
	}
	require.True(t, throttled)
	// Administrative payout surfaces require an admin JWT and exact DB permission.
	require.NoError(t, db.AutoMigrate(&system.SysUser{}, &em.AdminPermission{}))
	require.NoError(t, db.Create(&system.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 1}, Username: "root", Enable: 1}).Error)
	adminEngine := gin.New()
	adminEngine.Use(middleware.GinRecovery(true))
	adminGroup := adminEngine.Group("/api/v1/admin")
	router.RouterGroup{}.InitPayoutAdminRouter(adminGroup)
	expectedRoutes := map[string]bool{"GET /api/v1/admin/payout/capabilities": true}
	for _, op := range append([]svc.Operation{{Path: "order/create", Method: "POST"}, {Path: "order/list", Method: "GET"}, {Path: "order/detail", Method: "GET"}}, svc.ManualOperations...) {
		expectedRoutes[op.Method+" /api/v1/admin/payout/"+op.Path] = true
	}
	require.Len(t, adminEngine.Routes(), len(expectedRoutes))
	for _, route := range adminEngine.Routes() {
		require.True(t, expectedRoutes[route.Method+" "+route.Path], "unexpected public payment capability")
		for _, token := range []string{"", appToken} {
			r := httptest.NewRequest(route.Method, route.Path, strings.NewReader("{}"))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("x-token", token)
			w := httptest.NewRecorder()
			adminEngine.ServeHTTP(w, r)
			require.Equal(t, http.StatusUnauthorized, w.Code, route.Path)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		}
		if route.Method == "POST" {
			// Independent limiter per case: this assertion tests body rejection;
			// rate-limit saturation is exercised separately above.
			isolated := gin.New()
			router.RouterGroup{}.InitPayoutAdminRouter(isolated.Group("/api/v1/admin"))
			r := httptest.NewRequest(route.Method, route.Path, strings.NewReader(`{"requestId":"forged_http_input","status":"succeeded","amount":"1","account":"`+fullAccount+`"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("x-token", adminToken)
			w := httptest.NewRecorder()
			isolated.ServeHTTP(w, r)
			require.Contains(t, w.Body.String(), `"code":49201`, route.Path)
			require.NotContains(t, w.Body.String(), fullAccount)
		}
	}
	adminCall := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/admin/payout/order/"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("x-token", token)
		w := httptest.NewRecorder()
		adminEngine.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"list", "detail?payoutNo=po_0123456789abcdef0123456789abcdef"} {
		require.Equal(t, 401, adminCall("GET", path, "", appToken).Code)
		require.Equal(t, 401, adminCall("GET", path, "", "").Code)
	}
	w = adminCall("GET", "list", "", adminToken)
	require.Contains(t, w.Body.String(), `"code":0`)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.NotEmpty(t, w.Header().Get("X-Request-ID"))
	require.Equal(t, 400, adminCall("GET", "list", "", strings.Repeat("x", 8193)).Code)
	for _, forged := range []string{`"status":"succeeded"`, `"account":"` + fullAccount + `"`, `"amount":"1"`, `"payeeVersion":"2"`} {
		w = adminCall("POST", "create", `{"requestId":"order_key",`+forged+`}`, adminToken)
		require.Contains(t, w.Body.String(), `"code":49201`)
		require.NotContains(t, w.Body.String(), fullAccount)
	}
	for _, query := range []string{"anchorId=1", "keyword=ignored"} {
		w = adminCall("GET", "list?"+query, "", adminToken)
		require.Contains(t, w.Body.String(), `"code":49201`)
	}
	for _, path := range []string{"succeeded", "paid", "dispatch", "attempt", "delete", "decrypt", "mock-success", "callback"} {
		require.Equal(t, 404, adminCall("POST", path, "{}", adminToken).Code)
	}
	otherToken, e := j.CreateToken(j.CreateClaims(sr.BaseClaims{ID: 99, AuthorityId: 888, Username: "absent"}))
	require.NoError(t, e)
	w = adminCall("GET", "list", "", otherToken)
	require.Contains(t, w.Body.String(), `"code":49202`)
	// Even a panic containing material and forged headers must not reach recovery logs.
	engine.GET("/api/v1/app/payout/panic", func(c *gin.Context) { panic(fullAccount) })
	require.Equal(t, 500, call("GET", "/api/v1/app/payout/panic?account="+fullAccount, "", fullAccount).Code)
	encoded, e := json.Marshal(logs.All())
	require.NoError(t, e)
	require.NotContains(t, string(encoded), fullAccount)
	require.NotContains(t, string(encoded), "private-name")
	var output bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &output
	defer func() { gin.DefaultWriter = old }()
	debugEngine := gin.New()
	debugEngine.Use(middleware.FinancialSafeLogger())
	debugEngine.GET("/api/v1/app/payout/log", func(c *gin.Context) { c.Status(400) })
	debugEngine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/app/payout/log?account="+fullAccount, nil))
	require.NotContains(t, output.String(), fullAccount)
}
