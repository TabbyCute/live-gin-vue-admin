package wallet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/global"
	liveReq "tb_live_module/model/live/request"
	systemReq "tb_live_module/model/system/request"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	svc "tb_live_module/service/wallet"
)

func TestWalletInvariantProducesStructuredAlertLog(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	previous := global.GVA_LOG
	global.GVA_LOG = zap.New(core)
	t.Cleanup(func() { global.GVA_LOG = previous })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/approve", strings.NewReader(`{"evidenceRef":"private-proof"}`))
	c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{ID: 1, AuthorityId: 888}})
	respond(c, nil, svc.ErrInvariant)
	require.Equal(t, 1, logs.Len())
	require.Equal(t, "wallet invariant violated", logs.All()[0].Message)
	fields := logs.All()[0].ContextMap()
	require.EqualValues(t, 47011, fields["code"])
	require.EqualValues(t, 1, fields["operatorId"])
	require.Len(t, fields, 3)
}

func apiFixture(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/api.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.Exec("CREATE TABLE sys_users(id INTEGER PRIMARY KEY,nick_name VARCHAR(64),enable INT,authority_id INT,deleted_at datetime)").Error)
	require.NoError(t, db.Exec("CREATE TABLE live_account(id INTEGER PRIMARY KEY,account_no VARCHAR(32),phone VARCHAR(20),status INT,deleted_at datetime)").Error)
	require.NoError(t, db.Exec("INSERT INTO sys_users(id,nick_name,enable,authority_id) VALUES(1,'Admin',1,888),(2,'Reviewer',1,888)").Error)
	require.NoError(t, db.Exec("INSERT INTO live_account(id,account_no,phone,status) VALUES(10,'10000010','+60100000010',1),(11,'10000011','+60100000011',1)").Error)
	require.NoError(t, db.AutoMigrate(model.Models()...))
	old := walletService
	walletService = &svc.Service{DB: db}
	t.Cleanup(func() { walletService = old })
	egn := gin.New()
	egn.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{ID: 1, AuthorityId: 888}})
		c.Set("app-claims", &liveReq.AppClaims{AccountID: 10})
		c.Next()
	})
	a := WalletAdminApi{}
	egn.POST("/create", a.Create)
	egn.POST("/approve", a.Approve)
	egn.GET("/detail", a.Detail)
	egn.GET("/balance", (&WalletApi{}).Balance)
	egn.GET("/ledger", (&WalletApi{}).Ledger)
	return egn
}
func call(egn *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Forwarded-For", "8.8.8.8")
	egn.ServeHTTP(w, r)
	return w
}
func TestWalletStrictInputAndIdentity(t *testing.T) {
	e := apiFixture(t)
	valid := `{"requestId":"api_key_001","externalOrderNo":"BANK-001","evidenceRef":"upload:proof","accountNo":"10000010","amount":"100","reason":"运营赠送测试"}`
	for _, body := range []string{
		strings.Replace(valid, `"externalOrderNo":"BANK-001",`, "", 1),
		strings.Replace(valid, `"externalOrderNo":"BANK-001",`, `"sourceType":"alipay","externalOrderNo":"BANK-001",`, 1),
	} {
		require.Contains(t, call(e, "POST", "/create", body).Body.String(), `"code":47001`)
	}
	require.Contains(t, call(e, "POST", "/create", strings.Replace(valid, `"evidenceRef":"upload:proof",`, "", 1)).Body.String(), `"code":47008`)
	for _, body := range []string{
		strings.Replace(valid, `"100"`, `100`, 1),
		strings.Replace(valid, `"amount":"100"`, `"amount":"100","amount":"999"`, 1),
		strings.Replace(valid, `"amount":"100"`, `"amount":"100","AMOUNT":"999"`, 1),
		strings.TrimSuffix(valid, "}") + `,"createdBy":2}`,
		strings.TrimSuffix(valid, "}") + `,"status":"credited"}`,
		valid + `{}`, `null`, `[]`, strings.Replace(valid, "运营赠送测试", strings.Repeat("x", 20000), 1),
	} {
		w := call(e, "POST", "/create", body)
		require.Contains(t, w.Body.String(), `"code":47001`, body[:min(100, len(body))])
	}
	for _, path := range []string{"/balance?accountNo=10000011", "/balance?accountId=11", "/ledger?accountNo=10000011", "/ledger?limit=20&limit=100", "/ledger?limit=-1", "/ledger?cursor=invalid", "/balance?currencyCode=USD"} {
		w := call(e, "GET", path, "")
		require.Contains(t, w.Body.String(), `"code":47001`, path)
	}
	w := call(e, "POST", "/create", valid)
	require.Contains(t, w.Body.String(), `"code":0`)
	var response struct {
		Data struct {
			OrderNo   string `json:"orderNo"`
			CreatedBy string `json:"createdBy"`
			CreatedIP string `json:"createdIp"`
		}
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "1", response.Data.CreatedBy)
	require.Equal(t, "127.0.0.1", response.Data.CreatedIP)
	w = call(e, "POST", "/approve", `{"orderNo":"`+response.Data.OrderNo+`","reason":"顶级管理员确认入账"}`)
	require.Contains(t, w.Body.String(), `"code":0`)
	w = call(e, "GET", "/balance", "")
	require.Contains(t, w.Body.String(), `"availableBalance":"100"`)
	w = call(e, "GET", "/ledger", "")
	for _, private := range []string{"operatorId", "operatorDisplay", "reason", "evidenceRef", "sourceType", "externalOrderNo", "createdBy", "walletAccountId", "accountId", "accountNo", "phone"} {
		require.NotContains(t, w.Body.String(), `"`+private+`"`)
	}
}
func TestWalletBodyAndQueryRejectMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.POST("/", func(c *gin.Context) {
		var r req.CreateAdjustment
		if bindBody(c, &r) {
			c.Status(204)
		}
	})
	for _, body := range []string{`{"requestId":`, `{"requestId":"x",}`, `{"requestId":null}`, `{} trailing`, `1`, `"x"`} {
		w := call(e, "POST", "/", body)
		if body == `{"requestId":null}` {
			require.Equal(t, 204, w.Code)
		} else {
			require.Contains(t, w.Body.String(), `"code":47001`)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "text/plain")
	e.ServeHTTP(w, r)
	require.Contains(t, w.Body.String(), `"code":47001`)
}
func FuzzWalletJSON(f *testing.F) {
	gin.SetMode(gin.TestMode)
	for _, s := range []string{`{}`, `null`, `{"amount":"1"}`, `{"amount":"1","amount":"2"}`, `{"createdBy":1}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var r req.CreateAdjustment
		_ = bindBody(c, &r)
	})
}
