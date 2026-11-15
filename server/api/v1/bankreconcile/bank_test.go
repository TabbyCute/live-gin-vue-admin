package bankreconcile

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	req "tb_live_module/model/bankreconcile/request"
	"testing"
)

func TestStrictBankJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`null`, `[]`, `{"requestId":"first-key","requestId":"other-key"}`, `{"requestId":null}`, `{"RequestID":"alias-key"}`, `{"requestId":"first-key","unexpected":true}`, `{"requestId":"first-key"} {}`, `{"requestId":2}`, strings.Repeat("x", (1<<20)+1)} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var input req.Import
		require.False(t, bind(c, &input), body[:min(len(body), 80)])
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"requestId":"valid-key","csv":"abc"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	var input req.Import
	require.True(t, bind(c, &input))
}
func TestBankQueryAllowlist(t *testing.T) {
	for _, raw := range []string{"page=1&page=2", "account=secret", "page=%ZZ", strings.Repeat("x", 513)} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Request.URL.RawQuery = raw
		require.False(t, query(c, "page", "pageSize"))
	}
}
func FuzzBankStrictJSON(f *testing.F) {
	for _, seed := range []string{`{}`, `null`, `{"requestId":"duplicate","requestId":"other"}`, `{"csv":"=1+1"}`} {
		f.Add(seed)
	}
	gin.SetMode(gin.TestMode)
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 1<<20 {
			t.Skip()
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var input req.Import
		_ = bind(c, &input)
	})
}
