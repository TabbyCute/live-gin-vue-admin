package social

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"tb_live_module/config"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	systemReq "tb_live_module/model/system/request"
	"tb_live_module/utils"
	"testing"
	"time"
)

func TestAppAuthenticationQueryHasRequestDeadline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pool, _ := db.DB()
	defer pool.Close()
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}))
	oldDB, oldLog, oldConfig := global.GVA_DB, global.GVA_LOG, global.GVA_CONFIG
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "social-app-deadline-test-key", ExpiresTime: "1h", Issuer: "social-tests"}
	global.GVA_CONFIG.JWT.SigningKey = "different-admin-key"
	t.Cleanup(func() { global.GVA_DB = oldDB; global.GVA_LOG = oldLog; global.GVA_CONFIG = oldConfig })
	token, _, err := utils.NewAppJWT().CreateToken(42, "test", 1)
	require.NoError(t, err)
	observed := false
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:auth-budget", func(tx *gorm.DB) {
		deadline, ok := tx.Statement.Context.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
		observed = true
		tx.AddError(context.DeadlineExceeded)
	}))
	engine := gin.New()
	new(SocialRouter).InitSocialRouter(engine.Group("/api/v1/app"))
	r := httptest.NewRequest(http.MethodGet, "/api/v1/app/social/following/list", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, r)
	require.True(t, observed)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestSocialRoutesRejectAnonymousAndOnlyGetPost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	new(SocialRouter).InitSocialRouter(engine.Group("/api/v1/app"))
	removed := httptest.NewRecorder()
	engine.ServeHTTP(removed, httptest.NewRequest(http.MethodGet, "/api/v1/app/social/favorite/list", nil))
	require.Equal(t, http.StatusNotFound, removed.Code)
	for _, route := range engine.Routes() {
		require.Contains(t, []string{"GET", "POST"}, route.Method)
		r := httptest.NewRecorder()
		request := httptest.NewRequest(route.Method, route.Path, strings.NewReader(`{"accountNo":"10000001","userId":1}`))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(r, request)
		require.Equal(t, http.StatusUnauthorized, r.Code, route.Path)
	}
}
func TestSocialAdminAllRoutesRequireAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1/admin")
	group.Use(func(c *gin.Context) {
		c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{AuthorityId: 9528}})
		c.Next()
	})
	new(SocialAdminRouter).InitSocialAdminRouter(group)
	for _, route := range engine.Routes() {
		r := httptest.NewRecorder()
		engine.ServeHTTP(r, httptest.NewRequest(route.Method, route.Path, nil))
		require.Equal(t, http.StatusUnauthorized, r.Code, route.Path)
	}
}
