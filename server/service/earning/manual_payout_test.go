package earning_test

import (
	"bytes"
	"encoding/base64"
	"github.com/stretchr/testify/require"
	"image"
	"image/png"
	"os"
	em "tb_live_module/model/earning"
	pm "tb_live_module/model/payout"
	pr "tb_live_module/model/payout/request"
	ps "tb_live_module/service/payout"
	"testing"
	"time"
)

// Only isolated fixtures produce these simulated receipts; no production mock
// route, bank call or external transfer is involved in this accounting test.
func testReceipt(t *testing.T) string {
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	return base64.StdEncoding.EncodeToString(b.Bytes())
}
func TestManualPayoutGiftToPaid(t *testing.T)      { manualGiftToPaid(t, false) }
func TestMySQLManualPayoutGiftToPaid(t *testing.T) { manualGiftToPaid(t, true) }
func manualGiftToPaid(t *testing.T, real bool) {
	s, p, payee := payoutFixture(t, real)
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	store, e := ps.OpenEvidenceStore(dir)
	require.NoError(t, e)
	t.Cleanup(func() { store.Close() })
	p.EvidenceStore = store
	company, e := p.CreateChannelAccount(ctx, payoutAdmin, "trace", pr.CreateChannelAccount{RequestID: "company_create", ChannelCode: "manual_bank", AccountName: "ISOLATED TEST COMPANY", InstitutionName: "TEST BANK", Currency: "CNY", Account: "TESTBANK100001", SingleLimit: "100000", DailyLimit: "1000000"})
	require.NoError(t, e)
	w := payoutWithdrawal(t, s, payee, "manual_withdrawal", true)
	order, e := p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: "manual_order", WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	claim := pr.StartOrder{RequestID: "manual_start", PayoutNo: order.PayoutNo, ChannelAccountNo: company.ChannelAccountNo, Reason: "隔离测试领取"}
	at, e := p.StartOrder(ctx, payoutAdmin, "trace", claim, false)
	require.NoError(t, e)
	again, e := p.StartOrder(ctx, payoutAdmin, "trace", claim, false)
	require.NoError(t, e)
	require.Equal(t, at, again)
	grant, e := p.GrantReveal(ctx, payoutAdmin, "trace", pr.RevealGrant{RequestID: "reveal_grant", TargetNo: order.PayoutNo, Reason: "隔离测试核验资料"}, false)
	require.NoError(t, e)
	reveal := pr.RevealConsume{RequestID: "reveal_consume", GrantNo: grant.GrantNo, Token: grant.Token}
	clear, e := p.ConsumeReveal(ctx, payoutAdmin, "trace", reveal, false)
	require.NoError(t, e)
	require.Equal(t, "6222021234567890123", clear.Account)
	_, e = p.ConsumeReveal(ctx, payoutAdmin, "trace", reveal, false)
	require.ErrorIs(t, e, ps.ErrReveal)
	evidence, e := p.UploadEvidence(ctx, payoutAdmin, "trace", pr.UploadEvidence{RequestID: "evidence_upload", AttemptNo: at.AttemptNo, MIME: "image/png", Content: testReceipt(t)})
	require.NoError(t, e)
	_, e = p.UnpaidAttempt(ctx, payoutAdmin, "trace", pr.UnpaidAction{RequestID: "unsafe_release", AttemptNo: at.AttemptNo, Reason: "已存在凭证禁止释放", ConfirmNoTransfer: true}, false)
	require.ErrorIs(t, e, ps.ErrOrderState)
	submitted, e := p.SubmitAttempt(ctx, payoutAdmin, "trace", pr.SubmitAttempt{RequestID: "manual_submit", AttemptNo: at.AttemptNo, PaymentReference: "ISOLATED-TEST-BANK-001", PaymentTime: time.Now().UnixMilli(), EvidenceNo: evidence.EvidenceNo, OperatorNote: "隔离测试模拟银行结果", ConfirmRealTransfer: true})
	require.NoError(t, e)
	require.Equal(t, "pending_confirmation", submitted.Status)
	review := pr.ReviewAttempt{RequestID: "manual_review", AttemptNo: at.AttemptNo, Reason: "隔离测试独立确认结果", ConfirmExternalResult: true}
	_, e = p.ReviewAttempt(ctx, payoutAdmin, "trace", review, "success")
	require.ErrorIs(t, e, ps.ErrSelfReview)
	paid, e := p.ReviewAttempt(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "trace", review, "success")
	require.NoError(t, e)
	require.Equal(t, "succeeded", paid.Status)
	_, e = p.ReviewAttempt(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "trace", review, "success")
	require.NoError(t, e)
	var account em.Account
	require.NoError(t, s.DB.First(&account).Error)
	require.EqualValues(t, 5, account.Paid)
	require.Zero(t, account.Frozen)
	var final em.Withdrawal
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&final).Error)
	require.Equal(t, "paid", final.Status)
	var ledger em.AccountLedger
	require.NoError(t, s.DB.Where("business_no=? AND entry_type='withdrawal_paid'", w.WithdrawalNo).Take(&ledger).Error)
	require.EqualValues(t, 0, ledger.PaidBefore)
	require.EqualValues(t, 5, ledger.PaidAfter)
	var event pm.TerminalEvent
	require.NoError(t, s.DB.Where("payout_no=?", order.PayoutNo).Take(&event).Error)
	require.Equal(t, ledger.LedgerNo, event.LedgerNo)
	require.NoError(t, s.CheckPayoutFinanceInTx(s.DB, w.WithdrawalNo))
	_, e = p.RunOrderReconciliation(ctx, payoutAdmin, "trace", pr.OrderAction{RequestID: "manual_reconcile", PayoutNo: order.PayoutNo, Reason: "核验完整资金链"})
	require.NoError(t, e)
	var reports []pm.OrderReconciliation
	require.NoError(t, s.DB.Where("payout_no=?", order.PayoutNo).Find(&reports).Error)
	require.Len(t, reports, 2)
	for _, r := range reports {
		require.Equal(t, "passed", r.Status, r.Snapshot)
	}
	require.Error(t, s.DB.Model(&pm.Order{}).Where("payout_no=?", order.PayoutNo).Update("status", "ready").Error)
	require.Error(t, s.DB.Model(&em.Account{}).Where("id=?", account.ID).Update("paid", 0).Error)
	t.Logf("TRACE isolated gift→income→settlement→withdrawal %s→payout %s→attempt %s→evidence %s→ledger %s→terminal %s; paid=5, frozen=0", w.WithdrawalNo, order.PayoutNo, at.AttemptNo, evidence.EvidenceNo, ledger.LedgerNo, event.EventNo)
}
