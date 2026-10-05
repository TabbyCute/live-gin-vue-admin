package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tb_live_module/global"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestBindAndLogSRSHookPreservesRawBodyAndUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const rawBody = `{"server_id":"srs-1","action":"on_publish","stream":"1500001","param":"?pt=raw-token","custom_field":{"value":7}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/app/live/hook/publish", strings.NewReader(rawBody))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.10:4567"
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request

	core, logs := observer.New(zapcore.InfoLevel)
	previousLog, previousLive := global.GVA_LOG, global.GVA_CONFIG.Live
	global.GVA_LOG = zap.New(core)
	global.GVA_CONFIG.Live.LogSRSHookRawBody = true
	t.Cleanup(func() {
		global.GVA_LOG = previousLog
		global.GVA_CONFIG.Live = previousLive
	})

	req, err := bindAndLogSRSHook(context, "publish")
	require.NoError(t, err)
	require.Equal(t, "on_publish", req.Action)
	require.Equal(t, "?pt=raw-token", req.Param)
	require.JSONEq(t, `{"value":7}`, string(req.Extra["custom_field"]))

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "SRS Hook 原始参数", entries[0].Message)
	fields := entries[0].ContextMap()
	require.Equal(t, "publish", fields["hook"])
	require.Equal(t, "/v1/app/live/hook/publish", fields["path"])
	require.Equal(t, "192.0.2.10", fields["clientIp"])
	require.Equal(t, "application/json", fields["contentType"])
	require.Equal(t, rawBody, fields["rawBody"])
}

func TestBindAndLogSRSHookLogsMalformedRawBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const rawBody = `{"action":"on_unpublish"`
	request := httptest.NewRequest(http.MethodPost, "/v1/app/live/hook/unpublish", strings.NewReader(rawBody))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request

	core, logs := observer.New(zapcore.InfoLevel)
	previousLog, previousLive := global.GVA_LOG, global.GVA_CONFIG.Live
	global.GVA_LOG = zap.New(core)
	global.GVA_CONFIG.Live.LogSRSHookRawBody = true
	t.Cleanup(func() {
		global.GVA_LOG = previousLog
		global.GVA_CONFIG.Live = previousLive
	})

	_, err := bindAndLogSRSHook(context, "unpublish")
	require.Error(t, err)
	require.Len(t, logs.All(), 1)
	require.Equal(t, rawBody, logs.All()[0].ContextMap()["rawBody"])
}

func TestSRSHookRejectsOversizedBodyWithHTTP413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousLive := global.GVA_CONFIG.Live
	global.GVA_CONFIG.Live.SRSHookMaxBodyBytes = 128
	t.Cleanup(func() { global.GVA_CONFIG.Live = previousLive })

	router := gin.New()
	router.POST("/hook/publish", (&HookApi{}).Publish)
	body := `{"action":"on_publish","stream":"1500001","padding":"` + strings.Repeat("x", 256) + `"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hook/publish", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.Equal(t, 7, result.Code)
	require.Equal(t, "回调请求体过大", result.Msg)
}

func TestLiveStatsHookRejectsOversizedBodyWithHTTP413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousLive := global.GVA_CONFIG.Live
	global.GVA_CONFIG.Live.SRSHookMaxBodyBytes = 128
	t.Cleanup(func() { global.GVA_CONFIG.Live = previousLive })

	router := gin.New()
	router.POST("/hook/session/stats", (&HookApi{}).UpdateStats)
	body := `{"sessionNo":"session-1","padding":"` + strings.Repeat("x", 256) + `"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hook/session/stats", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestBindAndLogSRSHookTruncatesRawLogIndependently(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rawBody := `{"action":"on_publish","stream":"1500001","custom":"` + strings.Repeat("中", 100) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/app/live/hook/publish", strings.NewReader(rawBody))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request

	core, logs := observer.New(zapcore.InfoLevel)
	previousLog, previousLive := global.GVA_LOG, global.GVA_CONFIG.Live
	global.GVA_LOG = zap.New(core)
	global.GVA_CONFIG.Live.LogSRSHookRawBody = true
	global.GVA_CONFIG.Live.SRSHookMaxBodyBytes = 1024
	global.GVA_CONFIG.Live.SRSHookLogMaxBodyBytes = 64
	t.Cleanup(func() {
		global.GVA_LOG = previousLog
		global.GVA_CONFIG.Live = previousLive
	})

	req, err := bindAndLogSRSHook(context, "publish")
	require.NoError(t, err)
	require.Contains(t, string(req.Extra["custom"]), "中")
	fields := logs.All()[0].ContextMap()
	require.EqualValues(t, len(rawBody), fields["rawBodyBytes"])
	require.Equal(t, true, fields["rawBodyTruncated"])
	logged, ok := fields["rawBody"].(string)
	require.True(t, ok)
	require.Less(t, len(logged), len(rawBody))
	require.True(t, strings.HasSuffix(logged, "...[truncated]"))
}

func TestSRSHookBodyAndLogLimitsHaveHardCaps(t *testing.T) {
	previousLive := global.GVA_CONFIG.Live
	global.GVA_CONFIG.Live.SRSHookMaxBodyBytes = 1 << 30
	global.GVA_CONFIG.Live.SRSHookLogMaxBodyBytes = 1 << 30
	t.Cleanup(func() { global.GVA_CONFIG.Live = previousLive })

	require.Equal(t, hardSRSHookMaxBodyBytes, liveSRSHookMaxBodyBytes())
	require.Equal(t, hardSRSHookLogMaxBodyBytes, liveSRSHookLogMaxBodyBytes())
}

func TestSRSHookTokenFailsClosedWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SRS_HOOK_TOKEN", "high-entropy-test-token")
	router := gin.New()
	router.POST("/hook/publish", (&HookApi{}).Publish)
	body := `{"action":"on_publish","stream":"1500001","stream_id":"stream-1","param":"?pt=x"}`

	unauthorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hook/publish", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(unauthorized, request)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	wrong := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/hook/publish?hook_token=wrong", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wrong, request)
	require.Equal(t, http.StatusUnauthorized, wrong.Code)
}
