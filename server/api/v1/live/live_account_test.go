package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tb_live_module/config"
	"tb_live_module/global"
	commonRes "tb_live_module/model/common/response"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestClientLoginReturnsTokenWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
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

	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}))
	_, err = accountService.Register(liveReq.Register{Username: "demo", Password: "password123"})
	require.NoError(t, err)

	router := gin.New()
	router.POST("/login", (&UserApi{}).Login)
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"demo","password":"password123"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, recorder.Header().Values("Set-Cookie"))
	var body commonRes.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, commonRes.SUCCESS, body.Code)
	data, ok := body.Data.(map[string]interface{})
	require.True(t, ok)
	require.NotEmpty(t, data["token"])
}
