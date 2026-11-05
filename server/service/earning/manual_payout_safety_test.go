package earning_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	em "tb_live_module/model/earning"
	pm "tb_live_module/model/payout"
	pr "tb_live_module/model/payout/request"
	pres "tb_live_module/model/payout/response"
	es "tb_live_module/service/earning"
	ps "tb_live_module/service/payout"
	"testing"
	"time"
)

type manualRig struct {
	s              *es.Service
	p              *ps.Service
	payee, company string
	counter        int
}

var payoutReviewer = ps.Actor{ID: 2, AuthorityID: 888}

func newManualRig(t *testing.T, real bool) *manualRig {
	s, p, no := payoutFixture(t, real)
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	store, e := ps.OpenEvidenceStore(dir)
	require.NoError(t, e)
	t.Cleanup(func() { store.Close() })
	p.EvidenceStore = store
	company, e := p.CreateChannelAccount(ctx, payoutAdmin, "trace_test", pr.CreateChannelAccount{RequestID: "company_setup", ChannelCode: "manual_bank", AccountName: "ISOLATED COMPANY", InstitutionName: "ISOLATED BANK", Currency: "CNY", Account: "TESTBANK200002", SingleLimit: "10000", DailyLimit: "100000"})
	require.NoError(t, e)
	return &manualRig{s: s, p: p, payee: no, company: company.ChannelAccountNo}
}
func (r *manualRig) key() string { r.counter++; return fmt.Sprintf("manual_test_%04d", r.counter) }
func (r *manualRig) order(t *testing.T) pres.Order {
	w := payoutWithdrawal(t, r.s, r.payee, r.key(), true)
	o, e := r.p.CreateOrder(ctx, payoutAdmin, "trace_test", pr.CreateOrder{RequestID: r.key(), WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	return o
}
func (r *manualRig) start(t *testing.T, o pres.Order, retry bool) pres.ActionResult {
	at, e := r.p.StartOrder(ctx, payoutAdmin, "trace_test", pr.StartOrder{RequestID: r.key(), PayoutNo: o.PayoutNo, ChannelAccountNo: r.company, Reason: "隔离测试领取"}, retry)
	require.NoError(t, e)
	return at
}
func (r *manualRig) evidence(t *testing.T, at string, actor ps.Actor) pres.ActionResult {
	v, e := r.p.UploadEvidence(ctx, actor, "trace_test", pr.UploadEvidence{RequestID: r.key(), AttemptNo: at, MIME: "image/png", Content: testReceipt(t)})
	require.NoError(t, e)
	return v
}
func (r *manualRig) submit(t *testing.T, at string, ref string) pr.SubmitAttempt {
	ev := r.evidence(t, at, payoutAdmin)
	q := pr.SubmitAttempt{RequestID: r.key(), AttemptNo: at, PaymentReference: ref, PaymentTime: time.Now().UnixMilli(), EvidenceNo: ev.EvidenceNo, OperatorNote: "隔离测试支付证明", ConfirmRealTransfer: true}
	_, e := r.p.SubmitAttempt(ctx, payoutAdmin, "trace_test", q)
	require.NoError(t, e)
	return q
}
func (r *manualRig) review(t *testing.T, at, decision string) pr.ReviewAttempt {
	q := pr.ReviewAttempt{RequestID: r.key(), AttemptNo: at, Reason: "独立核验测试记录", ConfirmExternalResult: true}
	_, e := r.p.ReviewAttempt(ctx, payoutReviewer, "trace_test", q, decision)
	require.NoError(t, e)
	return q
}
func (r *manualRig) balance(t *testing.T) em.Account {
	var a em.Account
	require.NoError(t, r.s.DB.First(&a).Error)
	return a
}

func TestManualPayoutBoundaries(t *testing.T)      { manualBoundaries(t, false) }
func TestMySQLManualPayoutBoundaries(t *testing.T) { manualBoundaries(t, true) }
func manualBoundaries(t *testing.T, real bool) {
	r := newManualRig(t, real)
	o := r.order(t)
	initial := r.balance(t)
	start := pr.StartOrder{RequestID: r.key(), PayoutNo: o.PayoutNo, ChannelAccountNo: r.company, Reason: "检查领取权限"}
	_, e := r.p.StartOrder(ctx, ps.Actor{ID: 99, AuthorityID: 888}, "trace", start, false)
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	_, e = r.p.ChangeChannelAccount(ctx, payoutAdmin, "trace", pr.ChannelAccountAction{RequestID: r.key(), ChannelAccountNo: r.company, Reason: "停用测试账户"}, false)
	require.NoError(t, e)
	_, e = r.p.StartOrder(ctx, payoutAdmin, "trace", start, false)
	require.ErrorIs(t, e, ps.ErrChannelLimit)
	_, e = r.p.ChangeChannelAccount(ctx, payoutAdmin, "trace", pr.ChannelAccountAction{RequestID: r.key(), ChannelAccountNo: r.company, Reason: "重新启用账户"}, true)
	require.NoError(t, e)
	at := r.start(t, o, false)
	require.Equal(t, initial, r.balance(t))
	_, e = r.p.GrantReveal(ctx, payoutReviewer, "trace", pr.RevealGrant{RequestID: r.key(), TargetNo: o.PayoutNo, Reason: "不是领取人"}, false)
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	detail, e := r.p.OrderDetail(ctx, payoutAdmin, o.PayoutNo)
	require.NoError(t, e)
	raw, _ := json.Marshal(detail)
	require.NotContains(t, string(raw), "6222021234567890123")
	require.NotContains(t, string(raw), "private-payee-name")
	require.NotContains(t, string(raw), "encryptedPayload")
	require.True(t, detail.AllowedActions["order/release"])
	_, e = r.p.UnpaidAttempt(ctx, payoutAdmin, "trace", pr.UnpaidAction{RequestID: r.key(), AttemptNo: at.AttemptNo, Reason: "尚未支付释放", ConfirmNoTransfer: true}, false)
	require.NoError(t, e)
	second := r.start(t, o, false)
	require.NotEqual(t, at.AttemptNo, second.AttemptNo)
	_, e = r.p.MarkBankSubmitted(ctx, payoutAdmin, "trace", pr.AttemptAction{RequestID: r.key(), AttemptNo: second.AttemptNo, Reason: "已经提交银行"})
	require.NoError(t, e)
	_, e = r.p.UnpaidAttempt(ctx, payoutAdmin, "trace", pr.UnpaidAction{RequestID: r.key(), AttemptNo: second.AttemptNo, Reason: "不允许错误释放", ConfirmNoTransfer: true}, false)
	require.ErrorIs(t, e, ps.ErrOrderState)
	submitted := r.submit(t, second.AttemptNo, "ISOLATED-REFERENCE-CASE-A")
	_, e = r.p.SubmitAttempt(ctx, payoutAdmin, "trace", submitted)
	require.NoError(t, e)
	q := pr.CancelOrder{RequestID: r.key(), PayoutNo: o.PayoutNo, Reason: "待复核不能取消", ConfirmNoTransfer: true}
	_, e = r.p.CancelOrder(ctx, payoutAdmin, "trace", q)
	require.ErrorIs(t, e, ps.ErrOrderState)
	r.review(t, second.AttemptNo, "unknown")
	require.Equal(t, initial, r.balance(t))
	_, e = r.p.StartOrder(ctx, payoutAdmin, "trace", pr.StartOrder{RequestID: r.key(), PayoutNo: o.PayoutNo, ChannelAccountNo: r.company, Reason: "未知不可重试"}, true)
	require.ErrorIs(t, e, ps.ErrOrderState)
	_, e = r.p.CancelOrder(ctx, payoutAdmin, "trace", q)
	require.ErrorIs(t, e, ps.ErrOrderState)
	_, e = r.p.ReviewAttempt(ctx, payoutReviewer, "trace", pr.ReviewAttempt{RequestID: r.key(), AttemptNo: second.AttemptNo, Reason: "普通失败不得绕过调查", ConfirmExternalResult: true}, "failure")
	require.ErrorIs(t, e, ps.ErrOrderState)
	_, e = r.p.ResolveAttempt(ctx, payoutReviewer, "trace", pr.ResolveAttempt{RequestID: r.key(), AttemptNo: second.AttemptNo, Decision: "failure", Reason: "必须有新调查", ConfirmExternalResult: true})
	require.ErrorIs(t, e, ps.ErrInvestigation)
	fresh := r.evidence(t, second.AttemptNo, payoutReviewer)
	investigation, e := r.p.InvestigateOrder(ctx, payoutReviewer, "trace", pr.Investigate{RequestID: r.key(), PayoutNo: o.PayoutNo, EvidenceNo: fresh.EvidenceNo, Note: "隔离测试银行明确未支付证明"})
	require.NoError(t, e)
	_, e = r.p.ResolveAttempt(ctx, payoutReviewer, "trace", pr.ResolveAttempt{RequestID: r.key(), AttemptNo: second.AttemptNo, Decision: "failure", InvestigationNo: investigation.InvestigationNo, Reason: "经独立调查确认未支付", ConfirmExternalResult: true})
	require.NoError(t, e)
	require.Equal(t, initial, r.balance(t))
	third := r.start(t, o, true)
	require.NotEqual(t, second.AttemptNo, third.AttemptNo)
	thirdEv := r.evidence(t, third.AttemptNo, payoutAdmin)
	duplicate := pr.SubmitAttempt{RequestID: r.key(), AttemptNo: third.AttemptNo, PaymentReference: " isolated-reference-case-a ", PaymentTime: time.Now().UnixMilli(), EvidenceNo: thirdEv.EvidenceNo, OperatorNote: "同账户旧流水不能复用", ConfirmRealTransfer: true}
	_, e = r.p.SubmitAttempt(ctx, payoutAdmin, "trace", duplicate)
	require.Error(t, e)
	duplicate.RequestID = r.key()
	duplicate.PaymentReference = "00000000-0000-4000-8000-000000000000"
	_, e = r.p.SubmitAttempt(ctx, payoutAdmin, "trace", duplicate)
	require.ErrorIs(t, e, ps.ErrOrderInvalid)
	duplicate.RequestID = r.key()
	duplicate.PaymentReference = "ISOLATED-REFERENCE-CASE-B"
	_, e = r.p.SubmitAttempt(ctx, payoutAdmin, "trace", duplicate)
	require.NoError(t, e)
	r.review(t, third.AttemptNo, "failure")
	require.Equal(t, initial, r.balance(t))
	q.RequestID = r.key()
	q.Reason = "全部尝试均确认未付款"
	_, e = r.p.CancelOrder(ctx, payoutAdmin, "trace", q)
	require.NoError(t, e)
	after := r.balance(t)
	require.Equal(t, initial.Available+5, after.Available)
	require.Zero(t, after.Frozen)
	require.Zero(t, after.Paid)
	require.Error(t, r.s.DB.Model(&pm.Attempt{}).Where("attempt_no=?", third.AttemptNo).Update("status", "processing").Error)
	require.Error(t, r.s.DB.Where("payout_no=?", o.PayoutNo).Delete(&pm.Order{}).Error)
	require.NoError(t, r.s.CheckPayoutFinanceInTx(r.s.DB, o.BusinessNo))
}

func TestManualPayoutUnknownSuccessAfterPayeeDisabled(t *testing.T) {
	r := newManualRig(t, false)
	o := r.order(t)
	at := r.start(t, o, false)
	r.submit(t, at.AttemptNo, "ISOLATED-UNKNOWN-SUCCESS")
	r.review(t, at.AttemptNo, "unknown")
	_, e := r.p.Change(ctx, 11, "trace", r.payee, true)
	require.NoError(t, e)
	fresh := r.evidence(t, at.AttemptNo, payoutReviewer)
	investigation, e := r.p.InvestigateOrder(ctx, payoutReviewer, "trace", pr.Investigate{RequestID: r.key(), PayoutNo: o.PayoutNo, EvidenceNo: fresh.EvidenceNo, Note: "独立获取银行最终支付成功资料"})
	require.NoError(t, e)
	_, e = r.p.ResolveAttempt(ctx, payoutReviewer, "trace", pr.ResolveAttempt{RequestID: r.key(), AttemptNo: at.AttemptNo, Decision: "success", InvestigationNo: investigation.InvestigationNo, Reason: "已发生的真实支付须完成记账", ConfirmExternalResult: true})
	require.NoError(t, e)
	require.EqualValues(t, 5, r.balance(t).Paid)
	d, e := r.p.OrderDetail(ctx, payoutReviewer, o.PayoutNo)
	require.NoError(t, e)
	require.True(t, d.Consistent)
	require.Equal(t, "disabled", d.CurrentPayee.State)
}

func TestManualPayoutEveryWriteRollback(t *testing.T)      { manualWriteRollback(t, false) }
func TestMySQLManualPayoutEveryWriteRollback(t *testing.T) { manualWriteRollback(t, true) }
func manualWriteRollback(t *testing.T, real bool) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel_%v", cancel), func(t *testing.T) {
			r := newManualRig(t, real)
			o := r.order(t)
			var at pres.ActionResult
			if !cancel {
				at = r.start(t, o, false)
				r.submit(t, at.AttemptNo, "ISOLATED-FAULT-TEST")
			}
			before := r.balance(t)
			tables := []string{"payout_order", "payout_attempt", "payout_status_history", "payout_attempt_event", "payout_audit_log", "payout_manual_request", "anchor_withdrawable_ledger", "anchor_finance_audit", "payout_terminal_event", "payout_reconciliation"}
			counts := map[string]int64{}
			for _, table := range tables {
				var n int64
				require.NoError(t, r.s.DB.Table(table).Count(&n).Error)
				counts[table] = n
			}
			injected := errors.New("isolated write fault")
			var writes atomic.Int64
			var failAt atomic.Int64
			callback := func(tx *gorm.DB) {
				if writes.Add(1) == failAt.Load() {
					tx.AddError(injected)
				}
			}
			require.NoError(t, r.s.DB.Callback().Create().Before("gorm:create").Register("manual_test_fault", callback))
			require.NoError(t, r.s.DB.Callback().Update().Before("gorm:update").Register("manual_test_fault", callback))
			defer r.s.DB.Callback().Create().Remove("manual_test_fault")
			defer r.s.DB.Callback().Update().Remove("manual_test_fault")
			for point := int64(1); point <= 40; point++ {
				writes.Store(0)
				failAt.Store(point)
				var e error
				if cancel {
					_, e = r.p.CancelOrder(ctx, payoutAdmin, "trace", pr.CancelOrder{RequestID: "fault_cancel", PayoutNo: o.PayoutNo, Reason: "验证取消的事务原子性", ConfirmNoTransfer: true})
				} else {
					_, e = r.p.ReviewAttempt(ctx, payoutReviewer, "trace", pr.ReviewAttempt{RequestID: "fault_review", AttemptNo: at.AttemptNo, Reason: "验证成功事务原子性", ConfirmExternalResult: true}, "success")
				}
				if e == nil {
					require.Greater(t, point, int64(8))
					t.Logf("injected failure at every one of %d writes; no partial financial facts", point-1)
					return
				}
				require.ErrorIs(t, e, injected, "write %d", point)
				require.Equal(t, before, r.balance(t))
				for _, table := range tables {
					var n int64
					require.NoError(t, r.s.DB.Table(table).Count(&n).Error)
					require.Equal(t, counts[table], n, "%s write %d", table, point)
				}
			}
			t.Fatal("write fault coverage did not reach commit")
		})
	}
}

func TestMySQLManualPayoutConcurrent(t *testing.T) {
	r := newManualRig(t, true)
	o := r.order(t)
	var wg sync.WaitGroup
	var claimed atomic.Int64
	attempts := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, e := r.p.StartOrder(ctx, payoutAdmin, "trace", pr.StartOrder{RequestID: fmt.Sprintf("claim_race_%d", i), PayoutNo: o.PayoutNo, ChannelAccountNo: r.company, Reason: "并发领取隔离测试"}, false)
			if e == nil {
				claimed.Add(1)
				attempts <- v.AttemptNo
			}
		}(i)
	}
	wg.Wait()
	close(attempts)
	require.EqualValues(t, 1, claimed.Load())
	at := <-attempts
	r.submit(t, at, "ISOLATED-CONCURRENT-SUCCESS")
	var succeeded, cancelled atomic.Int64
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := r.p.ReviewAttempt(ctx, payoutReviewer, "trace", pr.ReviewAttempt{RequestID: fmt.Sprintf("success_race_%d", i), AttemptNo: at, Reason: "并发复核隔离测试", ConfirmExternalResult: true}, "success")
			if e == nil {
				succeeded.Add(1)
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := r.p.CancelOrder(ctx, payoutAdmin, "trace", pr.CancelOrder{RequestID: fmt.Sprintf("cancel_race_%d", i), PayoutNo: o.PayoutNo, Reason: "取消成功竞态隔离测试", ConfirmNoTransfer: true})
			if e == nil {
				cancelled.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 1, succeeded.Load())
	require.Zero(t, cancelled.Load())
	a := r.balance(t)
	require.EqualValues(t, 5, a.Paid)
	require.Zero(t, a.Frozen)
	var count int64
	require.NoError(t, r.s.DB.Model(&em.AccountLedger{}).Where("entry_type='withdrawal_paid'").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestManualPayoutRevealAndEvidenceSecurity(t *testing.T) {
	r := newManualRig(t, false)
	o := r.order(t)
	at := r.start(t, o, false)
	grant, e := r.p.GrantReveal(ctx, payoutAdmin, "trace", pr.RevealGrant{RequestID: r.key(), TargetNo: o.PayoutNo, Reason: "测试一次性查看"}, false)
	require.NoError(t, e)
	r.p.Now = func() time.Time { return time.UnixMilli(grant.ExpiresAt + 1) }
	_, e = r.p.ConsumeReveal(ctx, payoutAdmin, "trace", pr.RevealConsume{RequestID: r.key(), GrantNo: grant.GrantNo, Token: grant.Token}, false)
	require.ErrorIs(t, e, ps.ErrReveal)
	r.p.Now = nil
	for _, v := range []pr.UploadEvidence{{MIME: "image/svg+xml", Content: "PHN2Zz48L3N2Zz4="}, {MIME: "image/png", Content: "<script>secret</script>"}, {MIME: "image/png", Content: strings.Repeat("A", 3<<20)}} {
		v.RequestID = r.key()
		v.AttemptNo = at.AttemptNo
		_, e = r.p.UploadEvidence(ctx, payoutAdmin, "trace", v)
		require.Error(t, e)
	}
	proof := r.evidence(t, at.AttemptNo, payoutAdmin)
	_, _, e = r.p.ReadEvidence(ctx, ps.Actor{ID: 99, AuthorityID: 888}, "trace", pr.ReadEvidence{RequestID: r.key(), EvidenceNo: proof.EvidenceNo, Reason: "越权读取被拒绝"})
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	var evidence pm.Evidence
	require.NoError(t, r.s.DB.Where("evidence_no=?", proof.EvidenceNo).Take(&evidence).Error)
	require.Error(t, r.p.EvidenceStore.Put(ctx, "../escape", []byte("private")))
	require.Error(t, r.p.EvidenceStore.Put(ctx, evidence.ObjectReference, []byte("overwrite")))
	require.NoError(t, r.p.EvidenceStore.Remove(evidence.ObjectReference))
	q := pr.SubmitAttempt{RequestID: r.key(), AttemptNo: at.AttemptNo, PaymentReference: "ISOLATED-MISSING-PROOF", PaymentTime: time.Now().UnixMilli(), EvidenceNo: proof.EvidenceNo, OperatorNote: "丢失凭证必须阻止提交", ConfirmRealTransfer: true}
	_, e = r.p.SubmitAttempt(ctx, payoutAdmin, "trace", q)
	require.ErrorIs(t, e, ps.ErrEvidence)
	require.Error(t, r.s.DB.Model(&pm.ChannelAccount{}).Where("channel_account_no=?", r.company).Update("encrypted_account", "tamper").Error)
	require.Error(t, r.s.DB.Model(&pm.Evidence{}).Where("evidence_no=?", proof.EvidenceNo).Update("object_reference", "../escape").Error)
}

func TestManualPayoutLimitsPermissionsAndRootException(t *testing.T) {
	r := newManualRig(t, false)
	o := r.order(t)
	create := func(currency, account, single, daily string) string {
		c, e := r.p.CreateChannelAccount(ctx, payoutAdmin, "trace", pr.CreateChannelAccount{RequestID: r.key(), ChannelCode: "manual_bank", AccountName: "ISOLATED LIMIT TEST", InstitutionName: "ISOLATED BANK", Currency: currency, Account: account, SingleLimit: single, DailyLimit: daily})
		require.NoError(t, e)
		return c.ChannelAccountNo
	}
	usd := create("USD", "USDTEST0001", "100", "100")
	small := create("CNY", "SMALLTEST0001", "1", "100")
	limited := create("CNY", "LIMITTEST0001", "5", "5")
	for _, company := range []string{usd, small} {
		_, e := r.p.StartOrder(ctx, payoutAdmin, "trace", pr.StartOrder{RequestID: r.key(), PayoutNo: o.PayoutNo, ChannelAccountNo: company, Reason: "币种或单笔限额不符合"}, false)
		require.ErrorIs(t, e, ps.ErrChannelLimit)
	}
	old := r.company
	r.company = limited
	at := r.start(t, o, false)
	_, e := r.p.UnpaidAttempt(ctx, payoutAdmin, "trace", pr.UnpaidAction{RequestID: r.key(), AttemptNo: at.AttemptNo, Reason: "尚未打款释放测试", ConfirmNoTransfer: true}, false)
	require.NoError(t, e)
	_, e = r.p.StartOrder(ctx, payoutAdmin, "trace", pr.StartOrder{RequestID: r.key(), PayoutNo: o.PayoutNo, ChannelAccountNo: limited, Reason: "释放不退回UTC日额度"}, false)
	require.ErrorIs(t, e, ps.ErrChannelLimit)
	r.company = old
	root := ps.Actor{ID: 1}
	at, e = r.p.StartOrder(ctx, root, "trace", pr.StartOrder{RequestID: r.key(), PayoutNo: o.PayoutNo, ChannelAccountNo: old, Reason: "超级root领取测试"}, false)
	require.NoError(t, e)
	proof := r.evidence(t, at.AttemptNo, root)
	_, e = r.p.SubmitAttempt(ctx, root, "trace", pr.SubmitAttempt{RequestID: r.key(), AttemptNo: at.AttemptNo, PaymentReference: "ISOLATED-ROOT-EXCEPTION", PaymentTime: time.Now().UnixMilli(), EvidenceNo: proof.EvidenceNo, OperatorNote: "超级root测试回单", ConfirmRealTransfer: true})
	require.NoError(t, e)
	_, e = r.p.ReviewAttempt(ctx, root, "trace", pr.ReviewAttempt{RequestID: r.key(), AttemptNo: at.AttemptNo, Reason: "超级root同人例外测试", ConfirmExternalResult: true}, "success")
	require.NoError(t, e)
	var audit pm.OrderAudit
	require.NoError(t, r.s.DB.Where("payout_no=? AND action='attempt.review_success'", o.PayoutNo).Take(&audit).Error)
	require.Contains(t, audit.Snapshot, `"superRootException":true`)
	var permission em.AdminPermission
	require.NoError(t, r.s.DB.Where("authority_id=888 AND path='/payout/order/reveal-payee'").Take(&permission).Error)
	require.NoError(t, r.s.DB.Delete(&permission).Error)
	another := r.order(t)
	r.start(t, another, false)
	_, e = r.p.GrantReveal(ctx, payoutAdmin, "trace", pr.RevealGrant{RequestID: r.key(), TargetNo: another.PayoutNo, Reason: "缺独立查看权限"}, false)
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	caps, e := r.p.Capabilities(ctx, payoutAdmin)
	require.NoError(t, e)
	require.False(t, caps["order/reveal-payee"])
	caps, e = r.p.Capabilities(ctx, root)
	require.NoError(t, e)
	require.True(t, caps["order/reveal-payee"])
}

func TestMySQLManualPayoutSameRequestRace(t *testing.T) {
	r := newManualRig(t, true)
	o := r.order(t)
	q := pr.StartOrder{RequestID: "same_request_race", PayoutNo: o.PayoutNo, ChannelAccountNo: r.company, Reason: "同请求并发重试"}
	var wg sync.WaitGroup
	results := make(chan pres.ActionResult, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := r.p.StartOrder(ctx, payoutAdmin, "trace", q, false)
			results <- v
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	no := ""
	for v := range results {
		if no == "" {
			no = v.AttemptNo
		}
		require.Equal(t, no, v.AttemptNo)
	}
	q.Reason = "同请求篡改参数"
	_, e := r.p.StartOrder(ctx, payoutAdmin, "trace", q, false)
	require.ErrorIs(t, e, ps.ErrOrderConflict)
}

func TestMySQLManualPayoutLoad(t *testing.T) {
	if os.Getenv("EARNING_RUN_LOAD") != "1" {
		t.Skip("finite local load disabled")
	}
	r := newManualRig(t, true)
	samples := map[string][]float64{}
	measure := func(name string, fn func()) {
		start := time.Now()
		fn()
		samples[name] = append(samples[name], float64(time.Since(start).Microseconds())/1000)
	}
	started := time.Now()
	for i := 0; i < 20; i++ {
		var o pres.Order
		var at pres.ActionResult
		measure("withdrawal_and_ready", func() { o = r.order(t) })
		measure("claim", func() { at = r.start(t, o, false) })
		measure("upload_and_submit", func() { r.submit(t, at.AttemptNo, fmt.Sprintf("ISOLATED-LOAD-%03d", i)) })
		measure("review_and_accounting", func() { r.review(t, at.AttemptNo, "success") })
		measure("detail_with_internal_check", func() {
			d, e := r.p.OrderDetail(ctx, payoutReviewer, o.PayoutNo)
			require.NoError(t, e)
			require.True(t, d.Consistent)
		})
	}
	report := map[string]any{"orders": 20, "seconds": time.Since(started).Seconds(), "paid": r.balance(t).Paid, "frozen": r.balance(t).Frozen, "database": "isolated local MySQL", "externalBankCalls": 0}
	for name, v := range samples {
		sort.Float64s(v)
		report[name] = map[string]float64{"p50_ms": v[len(v)/2], "p95_ms": v[(len(v)*95-1)/100], "max_ms": v[len(v)-1]}
		require.Less(t, v[len(v)-1], float64(8000), name)
	}
	b, _ := json.Marshal(report)
	t.Log("PAYOUT_MANUAL_LOAD " + string(b))
}
