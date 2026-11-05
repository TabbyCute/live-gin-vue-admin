package gift_test

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net/http/httptest"
	"strings"
	"tb_live_module/config"
	"tb_live_module/global"
	sysReq "tb_live_module/model/system/request"
	gr "tb_live_module/router/guard"
	rr "tb_live_module/router/ranking"
	"tb_live_module/service/guard"
	"tb_live_module/service/ranking"
	"tb_live_module/utils"
	"testing"
)

func TestEngagementHTTPIsolation(t *testing.T) {
	oldGuardReady, oldRankingReady := global.GuardReady.Swap(true), global.RankingReady.Swap(true)
	t.Cleanup(func() { global.GuardReady.Store(oldGuardReady); global.RankingReady.Store(oldRankingReady) })
	s, gs, rs, _ := engagementFixture(t, false)
	_, e := send(s, "http_guard_flow1", "1")
	require.NoError(t, e)
	_, e = s.ProcessBenefits(ctx, 20)
	require.NoError(t, e)
	drain(t, rs)
	oldDB, oldConfig, oldLog, oldGuard, oldRank := global.GVA_DB, global.GVA_CONFIG, global.GVA_LOG, *guard.Default, *ranking.Default
	global.GVA_DB = s.DB
	global.GVA_LOG = zap.NewNop()
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "engagement-test-only", Issuer: "engagement-test", ExpiresTime: "1h"}
	*guard.Default = *gs
	*ranking.Default = *rs
	t.Cleanup(func() {
		global.GVA_DB = oldDB
		global.GVA_CONFIG = oldConfig
		global.GVA_LOG = oldLog
		*guard.Default = oldGuard
		*ranking.Default = oldRank
	})
	token, _, e := utils.NewAppJWT().CreateToken(10, "user10", 1)
	require.NoError(t, e)
	other, _, e := utils.NewAppJWT().CreateToken(11, "user11", 1)
	require.NoError(t, e)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	gr.RouterGroup{}.InitGuardRouter(r.Group("/api/v1/app"))
	rr.RouterGroup{}.InitRankingRouter(r.Group("/api/v1/app"))
	adminGroup := r.Group("/api/v1/admin")
	adminGroup.Use(func(c *gin.Context) { c.Set("claims", &sysReq.CustomClaims{BaseClaims: sysReq.BaseClaims{ID: 999}}) })
	gr.RouterGroup{}.InitGuardAdminRouter(adminGroup)
	rr.RouterGroup{}.InitRankingAdminRouter(adminGroup)
	call := func(method, path, body, token string) (int, map[string]any, string) {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		out := map[string]any{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out, w.Body.String()
	}
	for _, path := range []string{"/guard/plan/list", "/guard/status?anchorNo=anchor01", "/guard/mine", "/guard/anchor/members?anchorNo=anchor01", "/ranking/gift?scopeType=anchor&scopeNo=anchor01", "/ranking/wealth", "/ranking/charm"} {
		status, _, _ := call("GET", "/api/v1/app"+path, "", "")
		require.Equal(t, 401, status)
	}
	_, out, raw := call("GET", "/api/v1/app/guard/mine", "", token)
	require.Equal(t, float64(0), out["code"])
	for _, field := range []string{"userId", "anchorId", "password", "wallet", "sourceNo", "planSnapshot", "projection", "ledgerNo", "email", "phone"} {
		require.NotContains(t, raw, `"`+field+`"`)
	}
	_, out, _ = call("GET", "/api/v1/app/guard/mine", "", other)
	require.Equal(t, "0", out["data"].(map[string]any)["total"])
	for _, path := range []string{"/guard/mine?userId=11", "/guard/mine?page=1&page=2", "/ranking/wealth?pageSize=101", "/ranking/wealth?period=all&scopeNo=bad", "/ranking/wealth?sort=DROP", "/ranking/wealth?period=week&periodKey=2026-11-01"} {
		_, out, _ = call("GET", "/api/v1/app"+path, "", token)
		require.Equal(t, float64(49001), out["code"], path)
	}
	_, out, raw = call("GET", "/api/v1/app/ranking/wealth?period=all", "", token)
	require.Equal(t, float64(0), out["code"])
	for _, field := range []string{"id", "userId", "anchorShareBps", "revenueRuleNo", "walletLedgerNo", "sourceId"} {
		require.NotContains(t, raw, `"`+field+`"`)
	}
	_, out, _ = call("GET", "/api/v1/admin/guard/plan/list", "", token)
	require.Equal(t, float64(49002), out["code"])
	for _, body := range []string{`{"requestId":"admin_retry1","requestId":"admin_retry2","number":"fake"}`, `{"requestId":"admin_retry1","number":"fake","duration":100}`, `{"requestId":"admin_retry1","number":"fake"} {}`} {
		_, out, _ = call("POST", "/api/v1/admin/guard/task/retry", body, token)
		require.Equal(t, float64(49001), out["code"])
	}
	status, _, _ := call("POST", "/api/v1/app/guard/purchase", `{}`, token)
	require.Equal(t, 404, status)
	status, _, _ = call("POST", "/api/v1/admin/guard/task/retry", strings.Repeat("x", 65537), token)
	require.Equal(t, 400, status)
}
