package eventdelivery

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"tb_live_module/global"
	"testing"
)

func TestJWTRejectionAndBudgetAreNoStore(t *testing.T) {
	oldLog := global.GVA_LOG
	global.GVA_LOG = zap.NewNop()
	defer func() { global.GVA_LOG = oldLog }()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RouterGroup{}.InitEventDeliveryRouter(r.Group("/"))
	for _, tc := range []struct{ method, path, body string }{{"GET", "/event-delivery/list", ""}, {"POST", "/event-delivery/retry", strings.Repeat("x", (64<<10)+1)}} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		result := httptest.NewRecorder()
		r.ServeHTTP(result, req)
		require.Equal(t, "no-store", result.Header().Get("Cache-Control"))
		if len(tc.body) > 64<<10 {
			require.Equal(t, http.StatusBadRequest, result.Code)
		} else {
			require.Contains(t, result.Body.String(), "code")
		}
		require.NotContains(t, result.Body.String(), "eventId")
	}
}
