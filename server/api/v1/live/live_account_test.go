package live

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	previousCode := global.LiveAccountSuperCode()
	global.SetLiveAccountSuperCode("123456")
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "admin-signing-key-for-tests"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "app-signing-key-for-tests", ExpiresTime: "1h", Issuer: "live-app-tests"}
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.GVA_LOG = previousLog
		global.GVA_CONFIG.JWT = previousJWT
		global.GVA_CONFIG.JWTApp = previousAppJWT
		global.SetLiveAccountSuperCode(previousCode)
	})

	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}, &liveModel.LiveAccountNumberSequence{}))
	_, err = accountService.Register(liveReq.Register{Phone: "+60123456789", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)

	router := gin.New()
	router.POST("/login", (&UserApi{}).Login)
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"phone":"+60123456789","password":"password123"}`))
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
	for _, legacy := range []string{
		`{"username":"u1145781","password":"password123"}`,
		`{"phone":"+60123456789","smsCode":"123456"}`,
	} {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(legacy))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.Equal(t, commonRes.ERROR, body.Code)
	}
}

func TestSMSPlaceholderWaitsWithoutDatabaseOrRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/sms/send", (&UserApi{}).SendSMS)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(`{"phone":"+60123456789","purpose":"register"}`))
	request.Header.Set("Content-Type", "application/json")
	start := time.Now()
	router.ServeHTTP(recorder, request)
	require.GreaterOrEqual(t, time.Since(start), 2*time.Second)
	var body commonRes.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, commonRes.SUCCESS, body.Code)
	require.Equal(t, "ok", body.Msg)
	require.Empty(t, body.Data)
}

func TestSMSPlaceholderCancellationAndInvalidPurpose(t *testing.T) {
	router := gin.New()
	router.POST("/sms/send", (&UserApi{}).SendSMS)
	request := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(`{"phone":"+60123456789"}`))
	ctx, cancel := context.WithCancel(request.Context())
	request = request.WithContext(ctx)
	cancel()
	recorder := httptest.NewRecorder()
	start := time.Now()
	router.ServeHTTP(recorder, request)
	require.Less(t, time.Since(start), time.Second)
	require.Empty(t, recorder.Body.String())
	for _, body := range []string{
		`{"phone":"+60123456789","purpose":"reset_password"}`,
		`{"phone":"13812345678","purpose":"register"}`,
	} {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
		router.ServeHTTP(recorder, request)
		var response commonRes.Response
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Equal(t, commonRes.ERROR, response.Code)
	}
}
