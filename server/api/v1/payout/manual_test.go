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

func manualJSON(s string, out any, limit int64) bool {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(s))
	c.Request.Header.Set("Content-Type", "application/json")
	return bindBodySized(c, out, limit)
}
func TestManualStrictJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.True(t, manualJSON(`{"requestId":"manual_key","attemptNo":"pat_test","paymentReference":"BANK-123","paymentTime":123,"evidenceNo":"pev_test","operatorNote":"review","confirmRealTransfer":true}`, &req.SubmitAttempt{}, 8192))
	for _, raw := range []string{`{"amount":"5"}`, `{"currency":"CNY"}`, `{"account":"full-private-account"}`, `{"reviewerId":"1"}`, `{"status":"succeeded"}`, `{"requestId":"a","requestId":"b"}`, `{"RequestID":"b"}`, `{"paymentTime":"123"}`, `{"evidenceNo":null}`, `{"confirmRealTransfer":1}`, `{} {}`, `[]`, strings.Repeat(" ", 8193)} {
		require.False(t, manualJSON(raw, &req.SubmitAttempt{}, 8192), raw[:min(len(raw), 100)])
	}
	require.False(t, manualJSON(`{"content":"a","content":"b"}`, &req.UploadEvidence{}, 3<<20))
	require.False(t, manualJSON(`{"content":"a","path":"../../public"}`, &req.UploadEvidence{}, 3<<20))
	require.False(t, manualJSON(`{"grantNo":"a","token":"b","targetNo":"other"}`, &req.RevealConsume{}, 8192))
}
func FuzzManualPayoutJSON(f *testing.F) {
	f.Add(`{"requestId":"manual_key","attemptNo":"pat_test","confirmRealTransfer":true}`)
	f.Add(`{"status":"succeeded"}`)
	gin.SetMode(gin.TestMode)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 9000 {
			t.Skip()
		}
		var r req.SubmitAttempt
		if manualJSON(s, &r, 8192) {
			var m map[string]any
			require.NoError(t, json.Unmarshal([]byte(s), &m))
			for k := range m {
				require.Contains(t, []string{"requestId", "attemptNo", "paymentReference", "paymentTime", "evidenceNo", "operatorNote", "confirmRealTransfer"}, k)
			}
		}
	})
}
