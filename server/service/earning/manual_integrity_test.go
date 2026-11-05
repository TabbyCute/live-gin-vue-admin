package earning_test

import (
	"encoding/base64"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	gr "tb_live_module/model/gift/request"
	pm "tb_live_module/model/payout"
	pr "tb_live_module/model/payout/request"
	ps "tb_live_module/service/payout"
	"testing"
)

func TestManualPayoutCorruptionDetected(t *testing.T) {
	cases := map[string]string{
		"account paid":            "UPDATE anchor_withdrawable_account SET paid=paid+1",
		"paid ledger chain":       "UPDATE anchor_withdrawable_ledger SET paid_before=paid_before+1,paid_after=paid_after+1 WHERE entry_type='withdrawal_paid'",
		"attempt amount":          "UPDATE payout_attempt SET amount=amount+1",
		"independent reviewer":    "UPDATE payout_attempt SET reviewer_id=operator_id",
		"terminal amount":         "UPDATE payout_terminal_event SET amount=amount+1",
		"missing history":         "DELETE FROM payout_status_history WHERE sequence=2",
		"missing review audit":    "DELETE FROM payout_audit_log WHERE action='attempt.review_success'",
		"missing request receipt": "DELETE FROM payout_manual_request WHERE action='attempt/review-success'",
		"missing attempt event":   "DELETE FROM payout_attempt_event WHERE version=2",
	}
	for name, mutation := range cases {
		t.Run(name, func(t *testing.T) {
			r := newManualRig(t, false)
			o := r.order(t)
			at := r.start(t, o, false)
			r.submit(t, at.AttemptNo, "ISOLATED-CORRUPTION-TEST")
			r.review(t, at.AttemptNo, "success")
			// Deliberate DBA-style corruption is confined to this disposable SQLite DB.
			var guards []string
			require.NoError(t, r.s.DB.Raw("SELECT name FROM sqlite_master WHERE type='trigger'").Scan(&guards).Error)
			for _, guard := range guards {
				require.NoError(t, r.s.DB.Exec("DROP TRIGGER `"+guard+"`").Error)
			}
			require.NoError(t, r.s.DB.Exec(mutation).Error)
			v, e := r.p.RunOrderReconciliation(ctx, payoutAdmin, "trace_corruption", pr.OrderAction{RequestID: r.key(), PayoutNo: o.PayoutNo, Reason: "隔离测试识别损坏事实"})
			require.NoError(t, e)
			var report pm.OrderReconciliation
			require.NoError(t, r.s.DB.Where("reconciliation_no=?", v.ReconciliationNo).Take(&report).Error)
			require.Equal(t, "abnormal", report.Status, report.Snapshot)
		})
	}
}
func TestManualPayoutPrivateKeyAndDTOBoundaries(t *testing.T) {
	r := newManualRig(t, false)
	o := r.order(t)
	at := r.start(t, o, false)
	r.evidence(t, at.AttemptNo, payoutAdmin)
	require.NoError(t, r.p.ValidateManualKeys(r.s.DB))
	detail, e := r.p.OrderDetail(ctx, payoutAdmin, o.PayoutNo)
	require.NoError(t, e)
	b, e := json.Marshal(detail)
	require.NoError(t, e)
	for _, secret := range []string{"6222021234567890123", "private-payee-name", "TESTBANK200002", "encryptedAccount", "encryptionKeyId", "objectReference", "accountFingerprint"} {
		require.NotContains(t, string(b), secret)
	}
	crypto := r.p.Crypto
	r.p.Crypto = nil
	require.ErrorIs(t, r.p.ValidateManualKeys(r.s.DB), ps.ErrCrypto)
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("z", 32)))
	fp := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32)))
	for _, id := range []string{"other", "test"} {
		r.p.Crypto, e = ps.NewCrypto(id, map[string]string{id: key}, fp)
		require.NoError(t, e)
		require.Error(t, r.p.ValidateManualKeys(r.s.DB))
	}
	r.p.Crypto = crypto
	store := r.p.EvidenceStore
	r.p.EvidenceStore = nil
	require.Error(t, r.p.ValidateManualKeys(r.s.DB))
	r.p.EvidenceStore = store
}
func TestManualPayoutCancellationPaysRefundDebtFirst(t *testing.T) {
	s, g, _ := fixture(t, false)
	p, payee := realPayees(t, s)
	p.FinancialGuard = s.ValidatePayoutInTx
	p.FinancialLock = s.LockPayoutFinanceInTx
	p.FinancialCheck = s.CheckPayoutFinanceInTx
	p.FinancialFinalize = s.FinalizePayoutInTx
	rule := exchange(t, s, "3", "2")
	gift := send(t, g, "manual_debt_gift", "3")
	settle(t, s, rule.RuleNo, "manual_debt_settlement")
	w, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{RequestID: "manual_debt_withdrawal", Amount: "8", Currency: "CNY", PayeeAccountNo: payee})
	require.NoError(t, e)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "真实来源冻结核验"})
	require.NoError(t, e)
	root := ps.Actor{ID: 1}
	o, e := p.CreateOrder(ctx, root, "trace", pr.CreateOrder{RequestID: "manual_debt_order", WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	_, e = g.Refund(ctx, admin, gr.Refund{OrderNo: gift.OrderNo, RequestID: "manual_debt_refund", Quantity: "1", Reason: "已冻结后部分退款"})
	require.NoError(t, e)
	settle(t, s, rule.RuleNo, "manual_debt_adjustment")
	account(t, s, 0, 8, 3)
	_, e = p.CancelOrder(ctx, root, "trace", pr.CancelOrder{RequestID: "manual_debt_cancel", PayoutNo: o.PayoutNo, Reason: "确认未支付后优先抵扣欠款", ConfirmNoTransfer: true})
	require.NoError(t, e)
	account(t, s, 5, 0, 0)
	var a em.Account
	require.NoError(t, s.DB.First(&a).Error)
	require.Zero(t, a.Paid)
	require.NoError(t, s.CheckPayoutFinanceInTx(s.DB, w.WithdrawalNo))
}
