package gift

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
	model "tb_live_module/model/gift"
	live "tb_live_module/model/live"
	"tb_live_module/model/system"
	sr "tb_live_module/model/system/request"
	svc "tb_live_module/service/gift"
	"tb_live_module/utils"
	"testing"
	"time"
)

func TestGiftJWTIsolationAndForgery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/http.db"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	require.NoError(t, db.AutoMigrate(&live.LiveAccount{}, &system.SysUser{}, &model.Category{}, &model.Control{}, &model.AdminPermission{}))
	require.NoError(t, db.Exec("INSERT INTO sys_users(id,username,enable,authority_id) VALUES (1,'admin',1,888),(2,'reader',1,999)").Error)
	require.NoError(t, db.Exec("INSERT INTO sys_user_authority(sys_user_id,sys_authority_authority_id) VALUES (1,888),(2,999)").Error)
	require.NoError(t, db.Create(&system.SysAuthority{AuthorityId: 888, AuthorityName: "test admin"}).Error)
	require.NoError(t, db.Create(&system.SysAuthority{AuthorityId: 999, AuthorityName: "test reader"}).Error)
	require.NoError(t, db.Create(&model.Control{ID: 1, Version: 1}).Error)
	phone := "+60100000010"
	user := live.LiveAccount{Username: "gift_http_user", AccountNo: "10000010", Phone: &phone, Status: 1, TokenVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	oldDB, oldLog, oldConfig, oldCache, oldSvc := global.GVA_DB, global.GVA_LOG, global.GVA_CONFIG, global.BlackCache, *svc.Default
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
	RouterGroup{}.InitGiftRouter(engine.Group("/api/v1/app"))
	group := engine.Group("/api/v1/admin")
	group.Use(middleware.JWTAuth())
	RouterGroup{}.InitGiftAdminRouter(group)
	call := func(method, path, body, header, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:3210"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(header, token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	for _, r := range engine.Routes() {
		require.Equal(t, http.StatusUnauthorized, call(r.Method, r.Path, "{}", "", "").Code, r.Path)
	}
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/app/gift/category/list", "", "Authorization", "Bearer "+adminToken).Code)
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/admin/gift/category/list", "", "x-token", appToken).Code)
	require.Contains(t, call("GET", "/api/v1/admin/gift/category/list", "", "x-token", ordinary).Body.String(), `"code":48002`)
	require.Contains(t, call("GET", "/api/v1/app/gift/category/list", "", "Authorization", "Bearer "+appToken).Body.String(), `"code":0`)
	for _, field := range []string{`"price":"1"`, `"anchorId":1`, `"sessionId":1`, `"amount":"1"`} {
		body := `{"roomNo":"room0001","giftNo":"gift0001","quantity":"1","requestId":"request01",` + field + `}`
		require.Contains(t, call("POST", "/api/v1/app/gift/send", body, "Authorization", "Bearer "+appToken).Body.String(), `"code":48001`)
	}
	require.Equal(t, 404, call("POST", "/api/v1/app/gift/refund", "{}", "Authorization", "Bearer "+appToken).Code)
	require.Equal(t, 400, call("POST", "/api/v1/app/gift/send", strings.Repeat("x", 65537), "Authorization", "Bearer "+appToken).Code)
	rejected := 0
	for i := 0; i < 25; i++ {
		if call("GET", "/api/v1/app/gift/category/list", "", "Authorization", "Bearer "+appToken).Code == 429 {
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
