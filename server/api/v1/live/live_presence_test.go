package live

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	req "tb_live_module/model/live/request"
	"testing"
)

func TestPresenceStrictJSON(t *testing.T) {
	for _, raw := range []string{`{"roomNo":"x","requestId":"request_1"}`, `{"requestId":"request_1","roomNo":"x"}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(raw))
		var v req.PresenceOpen
		require.True(t, presenceBind(c, &v))
	}
	for _, raw := range []string{`{"roomNo":"x","roomNo":"y","requestId":"request_1"}`, `{"roomNo":"x","requestId":"request_1"} {}`, `{"roomNo":"x","RequestId":"request_1"}`, `{"roomNo":"x","requestId":"request_1","viewerId":1}`, `{"roomNo":null,"requestId":"request_1"}`, `null`, `[]`, `{}`, `{"roomNo":"` + strings.Repeat("x", 4096) + `","requestId":"request_1"}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(raw))
		var v req.PresenceOpen
		require.False(t, presenceBind(c, &v), raw[:min(len(raw), 150)])
	}
	for _, query := range []string{"roomNo=a&roomNo=b", "roomNo=a&viewerId=1", "roomNo=a&limit=x"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		_, ok := presenceQuery(c)
		require.False(t, ok)
	}
}
func FuzzPresenceJSON(f *testing.F) {
	f.Add(`{"presenceToken":"x","sequence":"1","playing":true,"visible":true}`)
	f.Add(`{"sequence":"1","sequence":"2"}`)
	f.Fuzz(func(t *testing.T, raw string) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(raw))
		var v req.PresenceHeartbeat
		_ = presenceBind(c, &v)
	})
}
