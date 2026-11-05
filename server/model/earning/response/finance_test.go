package response

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	model "tb_live_module/model/earning"
	"testing"
)

func TestPublicFinanceDTOAllowlist(t *testing.T) {
	w := model.Withdrawal{WithdrawalNo: "wd_public", RequestID: "request_public", Amount: 9007199254740993, Currency: "CNY", AccountMasked: "****1234", AccountName: "private-real-name", PayeeRef: "pa_public", ReviewReason: "internal-risk-note", ReconciliationNo: "internal-proof", ReviewedBy: 42}
	b, e := json.Marshal(PublicWithdrawal(w))
	require.NoError(t, e)
	text := string(b)
	for _, v := range []string{"private-real-name", "internal-risk-note", "internal-proof", "reviewedBy", "anchorId", "requestHash", "reconciliation"} {
		require.NotContains(t, text, v)
	}
	require.True(t, strings.Contains(text, `"amount":"9007199254740993"`))
}
