package gift

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	req "tb_live_module/model/gift/request"
	"testing"
)

func TestStrictClientInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{"roomNo":"room0001","giftNo":"gift0001","quantity":"1","requestId":"request01","price":"1"}`, `{"quantity":"1","quantity":"2"}`, `{"quantity":"1","Quantity":"2"}`, `{"quantity":1}`, `{} {}`, `null`, `{"anchorId":1}`, `{"sessionId":1}`, `{"amount":"1"}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var r req.Send
		require.False(t, bindBody(c, &r), body)
	}
}
func TestQueryPollution(t *testing.T) {
	for _, query := range []string{"?sort=id;DROP+TABLE+x", "?page=1&page=2", "?accountId=10"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/"+query, nil)
		var r req.Query
		if strings.Contains(query, "page=") || strings.Contains(query, "accountId=") {
			require.False(t, bindQuery(c, &r))
		}
	}
}
func FuzzGiftJSON(f *testing.F) {
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
		var r req.Send
		bindBody(c, &r)
	})
}
