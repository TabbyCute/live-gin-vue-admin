package earning_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sort"
	"strings"
	"sync"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	pm "tb_live_module/model/payout"
	pr "tb_live_module/model/payout/request"
	sys "tb_live_module/model/system"
	es "tb_live_module/service/earning"
	ps "tb_live_module/service/payout"
	"testing"
	"time"
)

var payoutAdmin = ps.Actor{ID: 3, AuthorityID: 888}

func payoutFixture(t *testing.T, real bool) (*es.Service, *ps.Service, string) {
	t.Helper()
	s, g, _ := fixture(t, real)
	p, no := realPayees(t, s)
	p.FinancialGuard = s.ValidatePayoutInTx
	p.FinancialLock = s.LockPayoutFinanceInTx
	p.FinancialCheck = s.CheckPayoutFinanceInTx
	p.FinancialFinalize = s.FinalizePayoutInTx
	require.NoError(t, initialize.SyncPayoutMetadata(s.DB))
	require.NoError(t, initialize.SyncPayoutMetadata(s.DB))
	var menus []sys.SysBaseMenu
	require.NoError(t, s.DB.Where("name IN ?", []string{"payoutCenter", "payoutOrders"}).Order("id").Find(&menus).Error)
	require.Len(t, menus, 2)
	require.Zero(t, menus[0].ParentId)
	require.Equal(t, menus[0].ID, menus[1].ParentId)
	var permissions int64
	require.NoError(t, s.DB.Model(&em.AdminPermission{}).Where("path LIKE '/payout/order/%'").Count(&permissions).Error)
	require.EqualValues(t, 3, permissions, "menu synchronization must not implicitly grant manual payment rights")
	for _, op := range ps.ManualOperations {
		var existing int64
		require.NoError(t, s.DB.Model(&em.AdminPermission{}).Where("path=? AND method=?", "/payout/"+op.Path, op.Method).Count(&existing).Error)
		require.Zero(t, existing)
		// Explicit grants belong to this isolated test fixture, never migration.
		require.NoError(t, s.DB.Create(&em.AdminPermission{AuthorityID: 888, Path: "/payout/" + op.Path, Method: op.Method}).Error)
	}
	rule := exchange(t, s, "1", "1")
	send(t, g, "payout_order_income", "100")
	settle(t, s, rule.RuleNo, "payout_order_settle")
	return s, p, no
}
func payoutWithdrawal(t *testing.T, s *es.Service, no, key string, approve bool) em.Withdrawal {
	t.Helper()
	w, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{RequestID: key, Amount: "5", Currency: "CNY", PayeeAccountNo: no})
	require.NoError(t, e)
	if approve {
		w, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "核验冻结与真实绑定"})
		require.NoError(t, e)
	}
	return w
}
func TestPayoutOrderLifecycle(t *testing.T)      { payoutOrderLifecycle(t, false) }
func TestMySQLPayoutOrderLifecycle(t *testing.T) { payoutOrderLifecycle(t, true) }
func TestMySQLPayoutSchemaDrift(t *testing.T) {
	s, _, _ := payoutFixture(t, true)
	for _, drift := range []struct{ change, restore, issue string }{
		{"ALTER TABLE payout_order MODIFY currency varchar(9) NOT NULL", "ALTER TABLE payout_order MODIFY currency varchar(8) NOT NULL", "column definition"},
		{"ALTER TABLE payout_order DROP CHECK ck_payout_amount, ADD CONSTRAINT ck_payout_amount CHECK(amount>=0)", "ALTER TABLE payout_order DROP CHECK ck_payout_amount, ADD CONSTRAINT ck_payout_amount CHECK(amount>0)", "check definition"},
		{"ALTER TABLE payout_owner_lock DROP PRIMARY KEY, ADD PRIMARY KEY(owner_no,owner_type)", "ALTER TABLE payout_owner_lock DROP PRIMARY KEY, ADD PRIMARY KEY(owner_type,owner_no)", "primary key"},
	} {
		require.NoError(t, s.DB.Exec(drift.change).Error)
		require.ErrorContains(t, initialize.ValidatePayoutSchema(s.DB), drift.issue)
		require.NoError(t, s.DB.Exec(drift.restore).Error)
		require.NoError(t, initialize.ValidatePayoutSchema(s.DB))
	}
}
func payoutOrderLifecycle(t *testing.T, real bool) {
	s, p, no := payoutFixture(t, real)
	pending := payoutWithdrawal(t, s, no, "pending_request", false)
	_, e := p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: "pending_order", WithdrawalNo: pending.WithdrawalNo})
	require.ErrorIs(t, e, ps.ErrOrderState)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: pending.WithdrawalNo, Reason: "测试驳回"})
	require.NoError(t, e)
	_, e = p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: "rejected_order", WithdrawalNo: pending.WithdrawalNo})
	require.ErrorIs(t, e, ps.ErrOrderState)
	w := payoutWithdrawal(t, s, no, "approved_request", true)
	second := payoutWithdrawal(t, s, no, "second_approved", true)
	var before em.Account
	require.NoError(t, s.DB.First(&before).Error)
	r := pr.CreateOrder{RequestID: "submit_order_001", WithdrawalNo: w.WithdrawalNo}
	_, e = p.CreateOrder(ctx, ps.Actor{ID: 10, AuthorityID: 888}, "trace", r)
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	_, e = p.CreateOrder(ctx, ps.Actor{ID: 3, AuthorityID: 999}, "trace", r)
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	o, e := p.CreateOrder(ctx, payoutAdmin, "trace", r)
	require.NoError(t, e)
	require.Equal(t, "ready", o.Status)
	require.Equal(t, w.WithdrawalNo, o.BusinessNo)
	replay, e := p.CreateOrder(ctx, payoutAdmin, "trace", r)
	require.NoError(t, e)
	require.Equal(t, o, replay)
	r.RequestID = "second_submit_key"
	replay, e = p.CreateOrder(ctx, ps.Actor{ID: 1}, "trace", r)
	require.NoError(t, e)
	require.Equal(t, o, replay)
	r.WithdrawalNo = second.WithdrawalNo
	_, e = p.CreateOrder(ctx, payoutAdmin, "trace", r)
	require.ErrorIs(t, e, ps.ErrOrderConflict)
	var stored pm.Order
	require.NoError(t, s.DB.Where("payout_no=?", o.PayoutNo).Take(&stored).Error)
	require.NoError(t, p.VerifyOrder(stored))
	require.NoError(t, p.ValidateOrderKeys(s.DB))
	var binding pm.PayeeAccount
	require.NoError(t, s.DB.Where("payee_account_no=?", no).Take(&binding).Error)
	require.NotEqual(t, binding.EncryptedPayload, stored.PayeeSnapshotEncrypted)
	for field, value := range map[string]any{"amount": stored.Amount + 1, "status": "processing", "business_no": second.WithdrawalNo, "payee_snapshot_encrypted": "replacement ciphertext"} {
		copy := stored
		require.ErrorContains(t, s.DB.Model(&copy).Update(field, value).Error, "immutable")
	}
	invalid := stored
	invalid.ID = 0
	invalid.PayoutNo = "po_ffffffffffffffffffffffffffffffff"
	invalid.RequestID = "non_ready_insert"
	invalid.BusinessNo = second.WithdrawalNo
	invalid.Status = "processing"
	require.ErrorContains(t, s.DB.Create(&invalid).Error, "manual payout immutable invariant")
	require.Error(t, s.DB.Delete(&stored).Error)
	for _, m := range []any{&pm.OrderRequest{}, &pm.StatusHistory{}, &pm.OrderAudit{}} {
		require.ErrorContains(t, s.DB.Model(m).Where("payout_no=?", o.PayoutNo).Update("created_at", stored.CreatedAt+1).Error, "immutable")
		require.Error(t, s.DB.Where("payout_no=?", o.PayoutNo).Delete(m).Error)
	}
	detail, e := p.OrderDetail(ctx, payoutAdmin, o.PayoutNo)
	require.NoError(t, e)
	require.True(t, detail.Consistent)
	require.Len(t, detail.History, 1)
	require.Len(t, detail.Audit, 1)
	b, e := json.Marshal(detail)
	require.NoError(t, e)
	for _, secret := range []string{"6222021234567890123", "private-payee-name", stored.PayeeSnapshotEncrypted, "encryptionKeyID", "accountFingerprint", "applicantUserId"} {
		require.NotContains(t, string(b), secret)
	}
	page, e := p.ListOrders(ctx, payoutAdmin, pr.OrderSearch{WithdrawalNo: w.WithdrawalNo, AnchorNo: w.AnchorNo, Currency: "CNY", PayeeMethod: "bank_card", Status: "ready", From: o.CreatedAt, To: o.CreatedAt + 1})
	require.NoError(t, e)
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.List, 1)
	page, e = p.ListOrders(ctx, payoutAdmin, pr.OrderSearch{From: o.CreatedAt + 1})
	require.NoError(t, e)
	require.Zero(t, page.Total)
	page, e = p.ListOrders(ctx, payoutAdmin, pr.OrderSearch{Status: "succeeded"})
	require.NoError(t, e)
	require.Zero(t, page.Total)
	list, e := s.List(ctx, 0, &financeAdmin, "withdrawal", er.FinanceQuery{No: w.WithdrawalNo})
	require.NoError(t, e)
	require.Equal(t, o.PayoutNo, (*list.List.(*[]em.Withdrawal))[0].PayoutNo)
	var after em.Account
	require.NoError(t, s.DB.First(&after).Error)
	require.Equal(t, before, after)
	var unchanged em.Withdrawal
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&unchanged).Error)
	require.Equal(t, w, unchanged)
	_, e = p.Change(ctx, 11, "trace", no, true)
	require.NoError(t, e)
	_, e = p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: "disabled_order", WithdrawalNo: second.WithdrawalNo})
	require.ErrorIs(t, e, ps.ErrOrderState)
	replay, e = p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: "submit_order_001", WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	require.Equal(t, o, replay)
	detail, e = p.OrderDetail(ctx, payoutAdmin, o.PayoutNo)
	require.NoError(t, e)
	require.True(t, detail.Consistent) // Internal ledger remains valid after a binding is disabled.
	require.False(t, detail.AllowedActions["order/start"])
	require.Equal(t, "disabled", detail.CurrentPayee.State)
	require.NoError(t, initialize.MigratePayout(s.DB))
	require.NoError(t, initialize.ValidatePayoutSchema(s.DB))
}
func TestPayoutOrderFailures(t *testing.T) {
	s, p, no := payoutFixture(t, false)
	w := payoutWithdrawal(t, s, no, "failure_withdrawal", true)
	r := pr.CreateOrder{RequestID: "failure_order", WithdrawalNo: w.WithdrawalNo}
	var before em.Account
	require.NoError(t, s.DB.First(&before).Error)
	for _, table := range []string{"payout_order", "payout_status_history", "payout_audit_log", "payout_order_request"} {
		injected := errors.New("injected write failure")
		require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("order:fail", func(tx *gorm.DB) {
			if tx.Statement.Table == table {
				tx.AddError(injected)
			}
		}))
		_, e := p.CreateOrder(ctx, payoutAdmin, "trace", r)
		require.ErrorIs(t, e, injected)
		require.NoError(t, s.DB.Callback().Create().Remove("order:fail"))
		for _, m := range []any{&pm.Order{}, &pm.StatusHistory{}, &pm.OrderAudit{}, &pm.OrderRequest{}} {
			var n int64
			require.NoError(t, s.DB.Model(m).Count(&n).Error)
			require.Zero(t, n)
		}
	}
	for _, table := range []string{"anchor_withdrawal_order", "anchor_withdrawable_ledger", "anchor_reconciliation"} {
		injected := errors.New("injected database read failure")
		require.NoError(t, s.DB.Callback().Query().Before("gorm:query").Register("order:read-fail", func(tx *gorm.DB) {
			if tx.Statement.Table == table {
				tx.AddError(injected)
			}
		}))
		_, err := p.CreateOrder(ctx, payoutAdmin, "trace", r)
		require.ErrorIs(t, err, ps.ErrOrderUnavailable)
		require.NoError(t, s.DB.Callback().Query().Remove("order:read-fail"))
		var n int64
		require.NoError(t, s.DB.Model(&pm.Order{}).Count(&n).Error)
		require.Zero(t, n)
	}
	// Read-side corruption simulates unavailable/invalid financial evidence without
	// bypassing immutable database protections in production code.
	for _, mutate := range []func(*gorm.DB){
		func(tx *gorm.DB) {
			if v, ok := tx.Statement.Dest.(*em.Withdrawal); ok {
				v.ReconciliationStatus = "abnormal"
			}
		},
		func(tx *gorm.DB) {
			if v, ok := tx.Statement.Dest.(*em.Withdrawal); ok {
				v.PayeeRef = "payout_center_pending"
			}
		},
		func(tx *gorm.DB) {
			if v, ok := tx.Statement.Dest.(*em.Withdrawal); ok {
				v.PayeeVersion = "2"
			}
		},
		func(tx *gorm.DB) {
			if v, ok := tx.Statement.Dest.(*em.AccountLedger); ok {
				v.ChangeAmount++
			}
		},
		func(tx *gorm.DB) {
			if v, ok := tx.Statement.Dest.(*em.Account); ok {
				v.Frozen = 0
			}
		},
	} {
		require.NoError(t, s.DB.Callback().Query().After("gorm:query").Register("order:corrupt", mutate))
		_, e := p.CreateOrder(ctx, payoutAdmin, "trace", r)
		require.ErrorIs(t, e, ps.ErrOrderState)
		require.NoError(t, s.DB.Callback().Query().Remove("order:corrupt"))
	}
	original := p.Crypto
	p.Crypto = nil
	_, e := p.CreateOrder(ctx, payoutAdmin, "trace", r)
	require.ErrorIs(t, e, ps.ErrOrderUnavailable)
	p.Crypto = original
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, e = p.CreateOrder(cancelled, payoutAdmin, "trace", r)
	require.Error(t, e)
	var after em.Account
	require.NoError(t, s.DB.First(&after).Error)
	require.Equal(t, before, after)
	order, e := p.CreateOrder(ctx, payoutAdmin, "trace", r)
	require.NoError(t, e)
	originalGuard := p.FinancialCheck
	p.FinancialCheck = func(*gorm.DB, string) error {
		return ps.ErrOrderUnavailable
	}
	_, e = p.OrderDetail(ctx, payoutAdmin, order.PayoutNo)
	require.ErrorIs(t, e, ps.ErrOrderUnavailable)
	p.FinancialCheck = originalGuard
}
func TestMySQLPayoutOrderConcurrent(t *testing.T) {
	s, p, no := payoutFixture(t, true)
	w := payoutWithdrawal(t, s, no, "concurrent_withdrawal", true)
	var before em.Account
	require.NoError(t, s.DB.First(&before).Error)
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	nos := make(chan string, 32)
	start := time.Now()
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent_%08d", i)
			if i < 16 {
				key = "concurrent_same_key"
			}
			o, e := p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: key, WithdrawalNo: w.WithdrawalNo})
			errs <- e
			nos <- o.PayoutNo
		}(i)
	}
	wg.Wait()
	close(errs)
	close(nos)
	for e := range errs {
		require.NoError(t, e)
	}
	unique := map[string]bool{}
	for no := range nos {
		unique[no] = true
	}
	require.Len(t, unique, 1)
	var after em.Account
	require.NoError(t, s.DB.First(&after).Error)
	require.Equal(t, before, after)
	for _, m := range []any{&pm.Order{}, &pm.StatusHistory{}, &pm.OrderAudit{}} {
		var n int64
		require.NoError(t, s.DB.Model(m).Count(&n).Error)
		require.EqualValues(t, 1, n)
	}
	t.Logf("PAYOUT_CONCURRENT_RESULT workers=32 elapsedMs=%d uniqueOrders=1 unchangedBalances=true", time.Since(start).Milliseconds())
	// Same global requestId racing across different business keys cannot change meaning.
	a := payoutWithdrawal(t, s, no, "compete_request_a", true)
	b := payoutWithdrawal(t, s, no, "compete_request_b", true)
	out := make(chan error, 2)
	for _, w := range []em.Withdrawal{a, b} {
		wg.Add(1)
		go func(w em.Withdrawal) {
			defer wg.Done()
			_, e := p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: "same_global_request", WithdrawalNo: w.WithdrawalNo})
			out <- e
		}(w)
	}
	wg.Wait()
	close(out)
	wins := 0
	for e := range out {
		if e == nil {
			wins++
		} else {
			require.ErrorIs(t, e, ps.ErrOrderConflict)
		}
	}
	require.Equal(t, 1, wins)
}
func TestMySQLPayoutDisableRace(t *testing.T) {
	for i := 0; i < 4; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, p, no := payoutFixture(t, true)
			w := payoutWithdrawal(t, s, no, "disable_race", true)
			var wg sync.WaitGroup
			wg.Add(2)
			var createErr, disableErr error
			go func() {
				defer wg.Done()
				_, createErr = p.CreateOrder(ctx, payoutAdmin, "trace", pr.CreateOrder{RequestID: "race_order_key", WithdrawalNo: w.WithdrawalNo})
			}()
			go func() { defer wg.Done(); _, disableErr = p.Change(ctx, 11, "trace", no, true) }()
			wg.Wait()
			require.NoError(t, disableErr)
			if createErr != nil {
				require.ErrorIs(t, createErr, ps.ErrOrderState)
			}
			var a em.Account
			require.NoError(t, s.DB.First(&a).Error)
			require.EqualValues(t, 5, a.Frozen)
			require.Zero(t, a.Paid)
		})
	}
}

func TestPayoutOrderInputBoundaries(t *testing.T) {
	s, p, _ := payoutFixture(t, false)
	_ = s
	for _, r := range []pr.CreateOrder{{RequestID: "short", WithdrawalNo: "wd_x"}, {RequestID: strings.Repeat("x", 65), WithdrawalNo: "wd_" + strings.Repeat("a", 32)}, {RequestID: "valid_key", WithdrawalNo: "payout_center_pending"}} {
		_, e := p.CreateOrder(ctx, payoutAdmin, "trace", r)
		require.ErrorIs(t, e, ps.ErrOrderInvalid)
	}
}

func measurePayoutLoad(t *testing.T, s *es.Service, p *ps.Service) {
	t.Helper()
	require.NoError(t, initialize.SyncPayoutMetadata(s.DB))
	p.FinancialGuard = s.ValidatePayoutInTx
	var withdrawals []em.Withdrawal
	require.NoError(t, s.DB.Order("id ASC").Find(&withdrawals).Error)
	require.Len(t, withdrawals, 64)
	for _, w := range withdrawals {
		_, e := s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "本地负载审核"})
		require.NoError(t, e)
	}
	var before em.Account
	require.NoError(t, s.DB.First(&before).Error)
	jobs := make(chan em.Withdrawal, 64)
	for _, w := range withdrawals {
		jobs <- w
	}
	close(jobs)
	durations := make(chan int64, 64)
	errs := make(chan error, 64)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range jobs {
				at := time.Now()
				bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
				_, e := p.CreateOrder(bounded, payoutAdmin, "load_trace", pr.CreateOrder{RequestID: "load_" + w.WithdrawalNo, WithdrawalNo: w.WithdrawalNo})
				cancel()
				errs <- e
				durations <- time.Since(at).Milliseconds()
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(durations)
	for e := range errs {
		require.NoError(t, e)
	}
	values := []int64{}
	for v := range durations {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	var after em.Account
	require.NoError(t, s.DB.First(&after).Error)
	require.Equal(t, before, after)
	var n int64
	require.NoError(t, s.DB.Model(&pm.Order{}).Where("status='ready'").Count(&n).Error)
	require.EqualValues(t, 64, n)
	report := map[string]any{"incomeFacts": 500, "concurrency": 16, "readyOrders": 64, "elapsedMs": time.Since(start).Milliseconds(), "p50Ms": values[32], "p95Ms": values[60], "maxMs": values[63], "unchangedBalances": true}
	b, _ := json.Marshal(report)
	t.Log("PAYOUT_LOAD_RESULT", string(b))
	require.Less(t, values[60], int64(8000))
}
