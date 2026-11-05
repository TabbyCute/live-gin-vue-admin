package wallet

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
)

func debitInput(no, amount string) MovementInput {
	return MovementInput{AccountID: 10, Amount: amount, BusinessType: "test_shop", BusinessNo: no, ActorType: "user", ActorID: "10", ActorIP: "127.0.0.1", TraceID: "trace_001", Reason: "核验业务订单扣款"}
}
func seedMovement(t *testing.T, s *Service, amount string) {
	t.Helper()
	o, e := s.Create(context.Background(), testCreator, grant("movement_seed", amount))
	require.NoError(t, e)
	_, e = s.Approve(context.Background(), testReviewer, review(o.OrderNo))
	require.NoError(t, e)
}
func refundInput(no, amount, original string) MovementInput {
	r := debitInput(no, amount)
	r.OriginalNo = original
	r.ActorType = "system"
	r.ActorID = "order_worker"
	r.Reason = "业务订单消费退款"
	return r
}

func TestMovementLifecycleTraceAndPagination(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	seedMovement(t, s, "100")
	r := debitInput("purchase-1", "80")
	d, e := s.Debit(ctx, r)
	require.NoError(t, e)
	amount(t, s, "20")
	r.ActorIP = "::1"
	r.TraceID = "retry_trace"
	again, e := s.Debit(ctx, r)
	require.NoError(t, e)
	require.Equal(t, d, again)
	r.Amount = "81"
	_, e = s.Debit(ctx, r)
	require.ErrorIs(t, e, ErrConflict)
	for i := 0; i < 60; i++ {
		_, e = s.Refund(ctx, refundInput(fmt.Sprintf("refund-%d", i), "1", d.TransactionNo))
		require.NoError(t, e)
	}
	amount(t, s, "80")
	trace, e := s.Trace(ctx, testCreator, req.TraceQuery{No: d.LedgerNo})
	require.NoError(t, e)
	require.Len(t, trace.Refunds, 50)
	require.True(t, trace.HasMore)
	require.Equal(t, "60", trace.Transaction.RefundedAmount)
	require.Equal(t, "用户 [ 10000010 ]", trace.Ledger.OperatorDisplay)
	page, e := s.Trace(ctx, testCreator, req.TraceQuery{No: d.TransactionNo, AfterRefundID: trace.NextRefundID})
	require.NoError(t, e)
	require.Len(t, page.Refunds, 10)
	require.False(t, page.HasMore)
	tr, e := s.Trace(ctx, testCreator, req.TraceQuery{BusinessType: "test_shop", BusinessNo: "refund-0", Kind: "refund"})
	require.NoError(t, e)
	require.Equal(t, d.TransactionNo, tr.Original.TransactionNo)
	require.Equal(t, d.LedgerNo, tr.Ledger.OriginalLedgerNo)
	require.Len(t, tr.Audit, 1)
	_, e = s.Refund(ctx, refundInput("too-much", "21", d.TransactionNo))
	require.ErrorIs(t, e, ErrRefundLimit)
	_, e = s.Refund(ctx, refundInput("remainder", "20", d.TransactionNo))
	require.NoError(t, e)
	amount(t, s, "100")
	_, e = s.Refund(ctx, refundInput("one-more", "1", d.TransactionNo))
	require.ErrorIs(t, e, ErrRefundLimit)
	_, e = s.Refund(ctx, refundInput("refund-refund", "1", tr.Transaction.TransactionNo))
	require.ErrorIs(t, e, ErrConflict)
	query := req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 7}
	checked := 0
	for i := 0; i < 20; i++ {
		c, e := s.Check(ctx, testReviewer, query)
		require.NoError(t, e)
		require.True(t, c.Consistent, c.Issues)
		checked += c.CheckedTransactions
		if !c.HasMore {
			break
		}
		query.AfterVersion = c.NextVersion
		query.AfterOrderID = c.NextOrderID
		query.AfterTransactionID = c.NextTransactionID
	}
	require.Equal(t, 62, checked)
}
func TestMovementValidationAndState(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	_, e := s.Debit(ctx, debitInput("unopened", "1"))
	require.ErrorIs(t, e, ErrInsufficient)
	count(t, s.DB, &model.WalletAccount{}, 0)
	seedMovement(t, s, "10")
	for _, a := range []string{"0", "-1", "1.0", "1e2", "01", "9223372036854775808"} {
		_, e = s.Debit(ctx, debitInput("invalid", a))
		require.ErrorIs(t, e, ErrInvalid)
	}
	for _, mutate := range []func(*MovementInput){func(r *MovementInput) { r.AccountID = 0 }, func(r *MovementInput) { r.ActorID = "11" }, func(r *MovementInput) { r.ActorType = "unknown" }, func(r *MovementInput) { r.BusinessType = "TEST" }, func(r *MovementInput) { r.BusinessNo = " bad" }, func(r *MovementInput) { r.TraceID = "" }, func(r *MovementInput) { r.ActorIP = "forwarded" }, func(r *MovementInput) { r.CurrencyCode = "USD" }} {
		r := debitInput("bad", "1")
		mutate(&r)
		_, e = s.Debit(ctx, r)
		require.ErrorIs(t, e, ErrInvalid)
	}
	d, e := s.Debit(ctx, debitInput("valid", "10"))
	require.NoError(t, e)
	_, e = s.Debit(ctx, debitInput("overdraft", "1"))
	require.ErrorIs(t, e, ErrInsufficient)
	_, e = s.SetStatus(ctx, testReviewer, req.AccountStatus{AccountNo: "10000010", Status: "disabled", Reason: "冻结测试钱包"})
	require.NoError(t, e)
	_, e = s.Refund(ctx, refundInput("disabled-refund", "1", d.TransactionNo))
	require.ErrorIs(t, e, ErrDisabled)
	_, e = s.Debit(ctx, debitInput("valid", "10"))
	require.NoError(t, e) // 已提交幂等确认不被后续停用阻断。
	_, e = s.SetStatus(ctx, testReviewer, req.AccountStatus{AccountNo: "10000010", Status: "normal", Reason: "恢复测试钱包"})
	require.NoError(t, e)
	require.NoError(t, s.DB.Exec("UPDATE live_account SET status=2, deleted_at=CURRENT_TIMESTAMP WHERE id=10").Error)
	_, e = s.Refund(ctx, refundInput("closed-account-refund", "10", d.TransactionNo))
	require.NoError(t, e)
	_, e = s.Debit(ctx, debitInput("closed-account-debit", "1"))
	require.ErrorIs(t, e, ErrAccount)
	_, e = s.DebitInTx(s.DB, debitInput("no-tx", "1"))
	require.ErrorIs(t, e, ErrInvariant)
}
func TestMovementAtomicBusinessRollback(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	seedMovement(t, s, "100")
	failure := errors.New("business entitlement failed")
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		_, e := s.DebitInTx(tx, debitInput("atomic", "40"))
		require.NoError(t, e)
		return failure
	})
	require.ErrorIs(t, e, failure)
	amount(t, s, "100")
	count(t, s.DB, &model.WalletTransaction{}, 0)
	d, e := s.Debit(ctx, debitInput("atomic", "40"))
	require.NoError(t, e)
	e = s.DB.Transaction(func(tx *gorm.DB) error {
		_, e := s.RefundInTx(tx, refundInput("atomic-refund", "40", d.TransactionNo))
		require.NoError(t, e)
		return failure
	})
	require.ErrorIs(t, e, failure)
	amount(t, s, "60")
	trace, e := s.Trace(ctx, testCreator, req.TraceQuery{No: d.TransactionNo})
	require.NoError(t, e)
	require.Equal(t, "0", trace.Transaction.RefundedAmount)
	_, e = s.Refund(ctx, refundInput("atomic-refund", "40", d.TransactionNo))
	require.NoError(t, e)
	amount(t, s, "100")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, e = s.Debit(cancelled, debitInput("cancelled", "1"))
	require.ErrorIs(t, e, context.Canceled)
}
func TestMovementExtremeAndCorruption(t *testing.T) {
	for _, corruption := range []string{"DELETE FROM wallet_ledger WHERE business_type='CONSUME'", "DELETE FROM wallet_audit_log WHERE action='debit'", "UPDATE wallet_transaction SET refunded_amount=1 WHERE kind='debit'", "UPDATE wallet_transaction SET amount=2 WHERE kind='debit'"} {
		t.Run(corruption, func(t *testing.T) {
			s := fixture(t)
			seedMovement(t, s, "100")
			r := debitInput("corruption", "1")
			_, e := s.Debit(context.Background(), r)
			require.NoError(t, e)
			require.NoError(t, s.DB.Exec(corruption).Error)
			_, e = s.Debit(context.Background(), r)
			require.Error(t, e)
			c, e := s.Check(context.Background(), testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
			require.NoError(t, e)
			require.False(t, c.Consistent)
		})
	}
	s := fixture(t)
	s.Config.MaxGrantAmount = math.MaxInt64
	s.Config.AccountDailyLimit = math.MaxInt64
	s.Config.OperatorDailyLimit = math.MaxInt64
	seedMovement(t, s, strconv.FormatInt(math.MaxInt64, 10))
	d, e := s.Debit(context.Background(), debitInput("max-debit", strconv.FormatInt(math.MaxInt64, 10)))
	require.NoError(t, e)
	amount(t, s, "0")
	_, e = s.Refund(context.Background(), refundInput("max-refund", strconv.FormatInt(math.MaxInt64, 10), d.TransactionNo))
	require.NoError(t, e)
	amount(t, s, strconv.FormatInt(math.MaxInt64, 10))
}
