package earning

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	req "tb_live_module/model/earning/request"
	"testing"
)

func TestStrictClientInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `{"requestId":"withdraw_request_001","currency":"CNY","amount":"100"}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(valid))
	c.Request.Header.Set("Content-Type", "application/json")
	var accepted req.ApplyWithdrawal
	require.True(t, bindBody(c, &accepted))
	require.Equal(t, req.ApplyWithdrawal{RequestID: "withdraw_request_001", Currency: "CNY", Amount: "100"}, accepted)
	for _, field := range []string{`"payeeVersion":"1"`, `"account":"6222021234567890123"`, `"accountName":"private"`, `"riskChecked":true`, `"PayeeAccountNo":"pa_0123456789abcdef0123456789abcdef"`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"requestId":"withdraw_001","currency":"CNY","amount":"1",`+field+`}`))
		c.Request.Header.Set("Content-Type", "application/json")
		var r req.ApplyWithdrawal
		require.False(t, bindBody(c, &r))
	}

	for _, body := range []string{`{"requestId":"request01","currency":"CNY","amount":"1","payeeRef":"client_value"}`, `{"roomNo":"room0001","giftNo":"gift0001","quantity":"1","requestId":"request01","price":"1"}`, `{"quantity":"1","quantity":"2"}`, `{"quantity":"1","Quantity":"2"}`, `{"quantity":1}`, `{} {}`, `null`, `{"anchorId":1}`, `{"sessionId":1}`, `{"amount":1}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var r req.ApplyWithdrawal
		require.False(t, bindBody(c, &r), body)
	}
}
func TestQueryPollution(t *testing.T) {
	for _, query := range []string{"?sort=id;DROP+TABLE+x", "?page=1&page=2", "?accountId=10"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/"+query, nil)
		var r req.FinanceQuery
		if strings.Contains(query, "page=") || strings.Contains(query, "accountId=") {
			require.False(t, bindQuery(c, &r))
		}
	}
}
func FuzzEarningJSON(f *testing.F) {
	f.Add(`{"roomNo":"room0001","giftNo":"gift0001","quantity":"1","requestId":"request01"}`)
	f.Add(`{"quantity":"1","quantity":"2"}`)
	gin.SetMode(gin.TestMode)
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 65536 {
			t.Skip()
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var r req.ApplyWithdrawal
		bindBody(c, &r)
	})
}
