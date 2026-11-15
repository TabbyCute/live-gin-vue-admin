package anchorhome

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"net/http/httptest"
	"strings"
	"tb_live_module/global"
	svc "tb_live_module/service/anchorhome"
	"testing"
)

func TestCommandLogCorrelatesAuditWithoutPayloadOrUntrustedTrace(t *testing.T) {
	old := global.GVA_LOG
	core, logs := observer.New(zap.InfoLevel)
	global.GVA_LOG = zap.New(core)
	t.Cleanup(func() { global.GVA_LOG = old })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/album/review", strings.NewReader(`{"reason":"SECRET_REASON"}`))
	c.Request.Header.Set("Authorization", "Bearer SECRET_TOKEN")
	c.Request.Header.Set("X-Request-ID", "UNTRUSTED_TRACE")
	albumCommandTrace(c, "review_request_123")
	albumRespond(c, nil, svc.ErrVersion)
	entries := logs.All()
	require.Len(t, entries, 2)
	require.Equal(t, "review_request_123", entries[0].ContextMap()["requestId"])
	require.Equal(t, "version_conflict", entries[0].ContextMap()["outcome"])
	require.NotEmpty(t, entries[0].ContextMap()["traceId"])
	require.Equal(t, entries[0].ContextMap()["traceId"], entries[1].ContextMap()["traceId"])
	for _, entry := range entries {
		raw, _ := json.Marshal(entry.ContextMap())
		for _, secret := range []string{"SECRET_REASON", "SECRET_TOKEN", "UNTRUSTED_TRACE"} {
			require.NotContains(t, string(raw), secret)
		}
	}
	logs.TakeAll()
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/album/review", nil)
	albumCommandTrace(c, "invalid\nSECRET")
	albumRespond(c, struct{}{}, nil)
	require.Empty(t, logs.All())
}
