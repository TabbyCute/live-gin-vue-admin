package wallet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/songzhibin97/gkit/cache/local_cache"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/config"
	"tb_live_module/global"
	"tb_live_module/middleware"
	live "tb_live_module/model/live"
	"tb_live_module/model/system"
	systemReq "tb_live_module/model/system/request"
	model "tb_live_module/model/wallet"
	svc "tb_live_module/service/wallet"
	"tb_live_module/utils"
)

func TestWalletRoutesJWTIsolationAndEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/e2e.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	require.NoError(t, db.AutoMigrate(append(model.Models(), &live.LiveAccount{}, &system.SysUser{}, &system.SysOperationRecord{})...))
	require.NoError(t, db.Exec("INSERT INTO sys_users(id,enable,authority_id) VALUES (1,1,888),(2,1,888),(3,1,999)").Error)
	phone := "+60100000010"
	user := live.LiveAccount{AccountNo: "10000010", Username: "wallet_e2e", Phone: &phone, PhoneVerifiedAt: time.Now().UnixMilli(), Status: 1, TokenVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	oldDB, oldLog, oldConfig, oldCache, oldService := global.GVA_DB, global.GVA_LOG, global.GVA_CONFIG, global.BlackCache, *svc.Default
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	global.BlackCache = local_cache.NewCache(local_cache.SetDefaultExpire(time.Hour))
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "wallet-admin-unit-test-signing-key", Issuer: "wallet-admin", ExpiresTime: "1h", BufferTime: "1m"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "wallet-app-unit-test-signing-key", Issuer: "wallet-app", ExpiresTime: "1h"}
	*svc.Default = svc.Service{DB: db}
	defer func() {
		global.GVA_DB = oldDB
		global.GVA_LOG = oldLog
		global.GVA_CONFIG = oldConfig
		global.BlackCache = oldCache
		*svc.Default = oldService
	}()
	token := func(id uint, role uint) string {
		j := utils.NewJWT()
		v, e := j.CreateToken(j.CreateClaims(systemReq.BaseClaims{ID: id, AuthorityId: role, Username: "test"}))
		require.NoError(t, e)
		return v
	}
	adminToken, reviewToken, ordinaryToken := token(1, 888), token(2, 888), token(3, 999)
	appToken, _, e := utils.NewAppJWT().CreateToken(user.ID, user.Username, 1)
	require.NoError(t, e)
	engine := gin.New()
	new(WalletRouter).InitWalletRouter(engine.Group("/api/v1/app"))
	admin := engine.Group("/api/v1/admin")
	admin.Use(middleware.JWTAuth())
	new(WalletAdminRouter).InitWalletAdminRouter(admin)
	call := func(method, path, body, header, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:3210"
		r.Header.Set("Content-Type", "application/json")
		if header != "" {
			r.Header.Set(header, token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	require.Len(t, engine.Routes(), 12)
	for _, route := range engine.Routes() {
		w := call(route.Method, route.Path, `{}`, "", "")
		require.Equal(t, http.StatusUnauthorized, w.Code, route.Path)
		require.NotEqual(t, "DELETE", route.Method)
	}
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/app/wallet/balance", "", "x-token", adminToken).Code)
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/app/wallet/balance", "", "Authorization", "Bearer "+adminToken).Code)
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/admin/wallet/account/detail?accountNo=10000010", "", "x-token", appToken).Code)
	valid := `{"requestId":"http_flow_001","externalOrderNo":"BANK-HTTP-001","evidenceRef":"upload:proof","accountNo":"10000010","amount":"250","reason":"完整链路测试"}`
	require.Contains(t, call("POST", "/api/v1/admin/wallet/account/status", `{"accountNo":"10000010","status":"normal","reason":"不能隐式开户"}`, "x-token", reviewToken).Body.String(), `"code":47014`)
	require.Contains(t, call("POST", "/api/v1/admin/wallet/adjustment/create", valid, "x-token", ordinaryToken).Body.String(), `"code":47002`)
	created := call("POST", "/api/v1/admin/wallet/adjustment/create", valid, "x-token", adminToken)
	require.Contains(t, created.Body.String(), `"code":0`)
	require.Contains(t, call("POST", "/api/v1/admin/wallet/adjustment/create", valid, "x-token", reviewToken).Body.String(), `"code":47013`)
	require.Contains(t, call("POST", "/api/v1/admin/wallet/adjustment/create", strings.Replace(valid, "http_flow_001", "http_flow_002", 1), "x-token", reviewToken).Body.String(), `"code":47013`)
	var result struct {
		Data struct {
			OrderNo string `json:"orderNo"`
		}
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &result))
	approve := `{"orderNo":"` + result.Data.OrderNo + `","reason":"核对通过测试"}`
	require.Contains(t, call("POST", "/api/v1/admin/wallet/adjustment/approve", approve, "x-token", reviewToken).Body.String(), `"code":0`)
	balanceResponse := call("GET", "/api/v1/app/wallet/balance", "", "Authorization", "Bearer "+appToken)
	require.Contains(t, balanceResponse.Body.String(), `"availableBalance":"250"`)
	require.Equal(t, "no-store", balanceResponse.Header().Get("Cache-Control"))
	require.Contains(t, call("GET", "/api/v1/admin/wallet/adjustment/detail?orderNo="+result.Data.OrderNo, "", "x-token", adminToken).Body.String(), `"approve_and_credit"`)
	for _, path := range []string{
		"/api/v1/admin/wallet/account/detail?accountNo=10000010",
		"/api/v1/admin/wallet/account/detail?phone=%2B60100000010",
		"/api/v1/admin/wallet/ledger/list?accountNo=10000010",
		"/api/v1/admin/wallet/ledger/list?phone=%2B60100000010",
		"/api/v1/admin/wallet/adjustment/list?status=credited",
		"/api/v1/admin/wallet/adjustment/list?phone=%2B60100000010",
		"/api/v1/admin/wallet/adjustment/list?externalOrderNo=bank-http-001",
		"/api/v1/admin/wallet/account/check?accountNo=10000010",
		"/api/v1/admin/wallet/account/check?phone=%2B60100000010",
	} {
		response := call("GET", path, "", "x-token", adminToken)
		require.Contains(t, response.Body.String(), `"code":0`, path)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"), path)
	}
	require.Contains(t, call("POST", "/api/v1/admin/wallet/account/status", `{"accountNo":"10000010","status":"disabled","reason":"临时风险核查"}`, "x-token", reviewToken).Body.String(), `"code":0`)
	reverseBody := `{"requestId":"http_reverse_001","orderNo":"` + result.Data.OrderNo + `","reason":"错误奖励发放冲正"}`
	reversed := call("POST", "/api/v1/admin/wallet/adjustment/reverse", reverseBody, "x-token", adminToken)
	require.Contains(t, reversed.Body.String(), `"code":0`)
	var reverseResult struct {
		Data struct {
			OrderNo string `json:"orderNo"`
		}
	}
	require.NoError(t, json.Unmarshal(reversed.Body.Bytes(), &reverseResult))
	reverseReview := `{"orderNo":"` + reverseResult.Data.OrderNo + `","reason":"核对冲正申请"}`
	require.Contains(t, call("POST", "/api/v1/admin/wallet/adjustment/approve", reverseReview, "x-token", reviewToken).Body.String(), `"code":47009`)
	require.Contains(t, call("POST", "/api/v1/admin/wallet/adjustment/reject", reverseReview, "x-token", reviewToken).Body.String(), `"code":0`)
	require.Contains(t, call("POST", "/api/v1/admin/wallet/account/status", `{"accountNo":"10000010","status":"normal","reason":"风险核查结束"}`, "x-token", reviewToken).Body.String(), `"code":0`)
	require.Contains(t, call("GET", "/api/v1/app/wallet/ledger", "", "Authorization", "Bearer "+appToken).Body.String(), `"changeAmount":"250"`)
	// JWT尚未过期，但数据库中的管理员被冻结后必须立即拒绝。
	require.NoError(t, db.Exec("UPDATE sys_users SET enable=2 WHERE id=2").Error)
	require.Contains(t, call("POST", "/api/v1/admin/wallet/adjustment/approve", approve, "x-token", reviewToken).Body.String(), `"code":47002`)
	require.NoError(t, db.Model(&live.LiveAccount{}).Where("id = ?", user.ID).Update("token_version", 2).Error)
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/v1/app/wallet/balance", "", "Authorization", "Bearer "+appToken).Code)
}
