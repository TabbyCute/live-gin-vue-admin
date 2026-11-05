package payout

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	req "tb_live_module/model/payout/request"
	"testing"
)

func body(t *testing.T, s string) bool {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(s))
	c.Request.Header.Set("Content-Type", "application/json")
	var r req.CreatePayee
	return bindBody(c, &r)
}
func TestPayeeStrictJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.True(t, body(t, `{"requestId":"create001","method":"bank_card","currency":"CNY","account":"6222021234567890123","accountName":"test","bankCode":"ICBC"}`))
	for _, s := range []string{`{"ownerNo":"someone"}`, `{"anchorNo":"someone"}`, `{"version":1}`, `{"verified":true}`, `{"account":"a","account":"b"}`, `{"account":"a","Account":"b"}`, `{"Account":"a"}`, `{"account":null}`, `{"account":{"nested":"value"}}`, `{"account":123}`, `[]`, `null`, `{} {}`, `{"account":"` + string([]byte{0xff}) + `"}`, strings.Repeat(" ", 8193)} {
		require.False(t, body(t, s))
	}
}
func FuzzPayeeJSON(f *testing.F) {
	f.Add(`{"account":"masked"}`)
	f.Add(`{"account":null}`)
	gin.SetMode(gin.TestMode)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 9000 {
			t.Skip()
		}
		if body(t, s) {
			var r req.CreatePayee
			require.NoError(t, json.Unmarshal([]byte(s), &r))
		}
	})
}
func FuzzOrderJSON(f *testing.F) {
	f.Add(`{"requestId":"order_key","withdrawalNo":"wd_0123456789abcdef0123456789abcdef"}`)
	f.Add(`{"status":"succeeded"}`)
	gin.SetMode(gin.TestMode)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 9000 {
			t.Skip()
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(s))
		c.Request.Header.Set("Content-Type", "application/json")
		var r req.CreateOrder
		if bindBody(c, &r) {
			var raw map[string]any
			require.NoError(t, json.Unmarshal([]byte(s), &raw))
			for key := range raw {
				require.Contains(t, []string{"requestId", "withdrawalNo"}, key)
			}
		}
	})
}
