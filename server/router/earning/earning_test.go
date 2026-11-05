package earning

import (
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/songzhibin97/gkit/cache/local_cache"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"strings"
	"tb_live_module/config"
	"tb_live_module/global"
	"tb_live_module/middleware"
	model "tb_live_module/model/earning"
	live "tb_live_module/model/live"
	"tb_live_module/model/system"
	sr "tb_live_module/model/system/request"
	svc "tb_live_module/service/earning"
	"tb_live_module/utils"
	"testing"
	"time"
)

func TestEarningJWTIsolationAndForgery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/http.db"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	require.NoError(t, db.AutoMigrate(&live.LiveAccount{}, &system.SysUser{}, &model.SettlementRule{}, &model.Income{}, &model.Allocation{}, &model.Account{}, &live.LiveAnchor{}, &model.AdminPermission{}, &model.Withdrawal{}))
	require.NoError(t, db.Exec("INSERT INTO sys_users(id,username,enable,authority_id) VALUES (1,'admin',1,888),(2,'reader',1,999)").Error)
	require.NoError(t, db.Exec("INSERT INTO sys_user_authority(sys_user_id,sys_authority_authority_id) VALUES (1,888),(2,999)").Error)
	require.NoError(t, db.Create(&system.SysAuthority{AuthorityId: 888, AuthorityName: "test admin"}).Error)
	require.NoError(t, db.Create(&system.SysAuthority{AuthorityId: 999, AuthorityName: "test reader"}).Error)
	require.NoError(t, db.Create(&model.AdminPermission{AuthorityID: 888, Path: "/earning/exchange/list", Method: "GET"}).Error)
	phone := "+60100000010"
	user := live.LiveAccount{Username: "gift_http_user", AccountNo: "10000010", Phone: &phone, Status: 1, TokenVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	oldDB, oldLog, oldConfig, oldCache, oldSvc := global.GVA_DB, global.GVA_LOG, global.GVA_CONFIG, global.BlackCache, *svc.Default
	require.NoError(t, db.Create(&live.LiveAnchor{AnchorNo: "http_anchor", UserId: uint64(user.ID), Status: 1, ApplyStatus: 2}).Error)
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	global.BlackCache = local_cache.NewCache(local_cache.SetDefaultExpire(time.Hour))
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "gift-admin-test-signing-key-only", Issuer: "gift-admin", ExpiresTime: "1h", BufferTime: "1m"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "gift-app-test-signing-key-only", Issuer: "gift-app", ExpiresTime: "1h"}
	*svc.Default = svc.Service{DB: db}
	defer func() {
		global.GVA_DB = oldDB
		global.GVA_LOG = oldLog
		global.GVA_CONFIG = oldConfig
		global.BlackCache = oldCache
		*svc.Default = oldSvc
	}()
	j := utils.NewJWT()
	adminToken, e := j.CreateToken(j.CreateClaims(sr.BaseClaims{ID: 1, AuthorityId: 888, Username: "admin"}))
	require.NoError(t, e)
	ordinary, e := j.CreateToken(j.CreateClaims(sr.BaseClaims{ID: 2, AuthorityId: 999, Username: "reader"}))
	require.NoError(t, e)
	appToken, _, e := utils.NewAppJWT().CreateToken(user.ID, user.Username, 1)
	require.NoError(t, e)
	engine := gin.New()
	RouterGroup{}.InitEarningRouter(engine.Group("/api/v1/app"))
	group := engine.Group("/api/v1/admin")
	group.Use(middleware.JWTAuth())
	RouterGroup{}.InitEarningAdminRouter(group)
	call := func(method, path, body, header, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:3210"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(header, token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	// A fresh HTTP router exercises requestId recovery without consuming the later limiter probe.
	require.NoError(t, db.Create(&model.Withdrawal{WithdrawalNo: "wd_http_result", RequestID: "http_request_001", AnchorID: 1, Amount: 1, Currency: "CNY", Status: "pending_review", ReconciliationStatus: "pending", PayeeRef: "private_provider_reference"}).Error)
	recoveryEngine := gin.New()
	RouterGroup{}.InitEarningRouter(recoveryEngine.Group("/api/v1/app"))
	for _, tc := range []struct{ query, want string }{{"requestId=http_request_001", `"code":0`}, {"requestId=http_request_001&no=wd_http_result", `"code":49001`}, {"requestId=http_request_001&anchorId=1", `"code":49001`}} {
		r := httptest.NewRequest("GET", "/api/v1/app/earning/withdrawal/detail?"+tc.query, nil)
		r.Header.Set("Authorization", "Bearer "+appToken)
		w := httptest.NewRecorder()
		recoveryEngine.ServeHTTP(w, r)
		require.Contains(t, w.Body.String(), tc.want)
		require.NotContains(t, w.Body.String(), "private_provider_reference")
	}
	for _, r := range engine.Routes() {
		require.Equal(t, http.StatusUnauthorized, call(r.Method, r.Path, "{}", "", "").Code, r.Path)
	}
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/app/earning/overview", "", "Authorization", "Bearer "+adminToken).Code)
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/admin/earning/exchange/list", "", "x-token", appToken).Code)
	require.Contains(t, call("GET", "/api/v1/admin/earning/exchange/list", "", "x-token", ordinary).Body.String(), `"code":49002`)
	require.Contains(t, call("GET", "/api/v1/app/earning/overview", "", "Authorization", "Bearer "+appToken).Body.String(), `"code":0`)
	for _, field := range []string{`"anchorId":1`, `"status":"approved"`, `"payeeVersion":"forged"`, `"payeeRef":"client_must_not_choose_payee"`} {
		body := `{"amount":"1","currency":"CNY","requestId":"request01",` + field + `}`
		require.Contains(t, call("POST", "/api/v1/app/earning/withdrawal/apply", body, "Authorization", "Bearer "+appToken).Body.String(), `"code":49001`)
	}
	for _, path := range []string{"/api/v1/admin/earning/withdrawal/paid", "/api/v1/admin/earning/withdrawal/dispatch"} {
		require.Equal(t, 404, call("POST", path, "{}", "x-token", adminToken).Code)
	}
	require.Equal(t, 404, call("POST", "/api/v1/app/earning/withdrawal/review", "{}", "Authorization", "Bearer "+appToken).Code)
	require.Equal(t, 400, call("POST", "/api/v1/app/earning/withdrawal/apply", strings.Repeat("x", 65537), "Authorization", "Bearer "+appToken).Code)
	rejected := 0
	for i := 0; i < 25; i++ {
		if call("GET", "/api/v1/app/earning/overview", "", "Authorization", "Bearer "+appToken).Code == 429 {
			rejected++
		}
	}
	require.Positive(t, rejected)
}
func TestGuardCapacityDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	entered, release := make(chan struct{}), make(chan struct{})
	engine.GET("/", guard(1, 30*time.Millisecond), func(c *gin.Context) { close(entered); <-release })
	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	<-entered
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, 503, w.Code)
	close(release)
	<-done
}
