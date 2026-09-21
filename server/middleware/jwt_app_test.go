package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tb_live_module/config"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	"tb_live_module/utils"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func setupAppAuthTest(t *testing.T) (string, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}))
	account := liveModel.LiveAccount{Username: "demo", Password: "hash", Nickname: "demo", Status: liveModel.LiveAccountStatusNormal}
	require.NoError(t, db.Create(&account).Error)

	previousDB := global.GVA_DB
	previousLog := global.GVA_LOG
	previousJWT := global.GVA_CONFIG.JWT
	previousAppJWT := global.GVA_CONFIG.JWTApp
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "admin-signing-key-for-tests"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "app-signing-key-for-tests", ExpiresTime: "1h", Issuer: "live-app-tests"}
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.GVA_LOG = previousLog
		global.GVA_CONFIG.JWT = previousJWT
		global.GVA_CONFIG.JWTApp = previousAppJWT
	})

	token, _, err := utils.NewAppJWT().CreateToken(account.ID, account.Username)
	require.NoError(t, err)
	router := gin.New()
	router.GET("/private", JWTAppAuth(), func(c *gin.Context) {
		accountID, ok := GetAppAccountID(c)
		require.True(t, ok)
		require.Equal(t, account.ID, accountID)
		c.Status(http.StatusNoContent)
	})
	return token, router
}

func TestJWTAppAuthOnlyAcceptsAuthorizationBearerToken(t *testing.T) {
	token, router := setupAppAuthTest(t)

	xTokenRequest := httptest.NewRequest(http.MethodGet, "/private", nil)
	xTokenRequest.Header.Set("x-token", token)
	xTokenResponse := httptest.NewRecorder()
	router.ServeHTTP(xTokenResponse, xTokenRequest)
	require.Equal(t, http.StatusUnauthorized, xTokenResponse.Code)

	cookieRequest := httptest.NewRequest(http.MethodGet, "/private", nil)
	cookieRequest.AddCookie(&http.Cookie{Name: "x-token", Value: token})
	cookieResponse := httptest.NewRecorder()
	router.ServeHTTP(cookieResponse, cookieRequest)
	require.Equal(t, http.StatusUnauthorized, cookieResponse.Code)

	bearerRequest := httptest.NewRequest(http.MethodGet, "/private", nil)
	bearerRequest.Header.Set("Authorization", "Bearer "+token)
	bearerResponse := httptest.NewRecorder()
	router.ServeHTTP(bearerResponse, bearerRequest)
	require.Equal(t, http.StatusNoContent, bearerResponse.Code)
	require.Empty(t, bearerResponse.Header().Values("Set-Cookie"))
}
