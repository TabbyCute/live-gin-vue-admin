package earning_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"sync"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	lm "tb_live_module/model/live"
	pm "tb_live_module/model/payout"
	pr "tb_live_module/model/payout/request"
	es "tb_live_module/service/earning"
	ps "tb_live_module/service/payout"
	"testing"
	"time"
)

func realPayees(t *testing.T, s *es.Service) (*ps.Service, string) {
	t.Helper()
	require.NoError(t, initialize.MigratePayout(s.DB))
	c, e := ps.NewCrypto("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32))))
	require.NoError(t, e)
	p := &ps.Service{DB: s.DB, Crypto: c}
	s.Payees = es.DatabasePayeeProvider{Accounts: p}
	a, e := p.Create(ctx, 11, "trace_fixture", pr.CreatePayee{RequestID: "payee_fixture", Method: "bank_card", Currency: "CNY", Account: "6222021234567890123", AccountName: "private-payee-name", BankCode: "ICBC"})
	require.NoError(t, e)
	return p, a.PayeeAccountNo
}
func TestRealPayeeWithdrawal(t *testing.T)      { realWithdrawal(t, false) }
func TestMySQLRealPayeeWithdrawal(t *testing.T) { realWithdrawal(t, true) }
func realWithdrawal(t *testing.T, real bool) {
	s, g, _ := fixture(t, real)
	p, no := realPayees(t, s)
	rule := exchange(t, s, "1", "1")
	send(t, g, "real_payee_income", "10")
	settle(t, s, rule.RuleNo, "real_payee_settle")
	r := er.ApplyWithdrawal{RequestID: "real_payee_apply", Amount: "5", Currency: "CNY", PayeeAccountNo: no}
	// Ownership, currency and disabled state all fail before freezing.
	other, e := p.Create(ctx, 11, "trace", pr.CreatePayee{RequestID: "currency_other", Method: "alipay", Currency: "USD", Account: "private@example.test", AccountName: "private-name"})
	require.NoError(t, e)
	r.PayeeAccountNo = other.PayeeAccountNo
	_, e = s.ApplyWithdrawal(ctx, 11, r)
	require.ErrorIs(t, e, es.ErrState)
	require.NoError(t, s.DB.Create(&lm.LiveAnchor{UserId: 10, AnchorNo: "another_anchor", Status: 1, ApplyStatus: 2}).Error)
	foreign, e := p.Create(ctx, 10, "trace", pr.CreatePayee{RequestID: "foreign_payee", Method: "alipay", Currency: "CNY", Account: "foreign@example.test", AccountName: "private-name"})
	require.NoError(t, e)
	r.PayeeAccountNo = foreign.PayeeAccountNo
	_, e = s.ApplyWithdrawal(ctx, 11, r)
	require.ErrorIs(t, e, es.ErrState)
	r.PayeeAccountNo = no
	require.NoError(t, s.DB.Model(&lm.LiveAnchor{}).Where("id=1").Update("cert_status", 0).Error)
	require.NoError(t, s.DB.Model(&lm.LiveAccount{}).Where("id=11").Update("real_name_status", 0).Error)
	w, e := s.ApplyWithdrawal(ctx, 11, r)
	require.NoError(t, e)
	require.Equal(t, no, w.PayeeRef)
	require.Equal(t, "1", w.PayeeVersion)
	require.Equal(t, "****0123", w.AccountMasked)
	account(t, s, 35, 5, 0)
	page, e := s.List(ctx, 0, &financeAdmin, "withdrawal", er.FinanceQuery{PayeeAccountNo: no})
	require.NoError(t, e)
	require.EqualValues(t, 1, page.Total)
	_, e = s.List(ctx, 11, nil, "withdrawal", er.FinanceQuery{PayeeAccountNo: no})
	require.ErrorIs(t, e, es.ErrInvalid)
	copy, e := s.ApplyWithdrawal(ctx, 11, r)
	require.NoError(t, e)
	require.Equal(t, w.WithdrawalNo, copy.WithdrawalNo)
	r.PayeeAccountNo = other.PayeeAccountNo
	_, e = s.ApplyWithdrawal(ctx, 11, r)
	require.ErrorIs(t, e, es.ErrConflict)
	r.PayeeAccountNo = no
	r.Amount = "6"
	_, e = s.ApplyWithdrawal(ctx, 11, r)
	require.ErrorIs(t, e, es.ErrConflict)
	r.Amount = "5"
	// Default changes are metadata, never a sensitive version change.
	second, e := p.Create(ctx, 11, "trace", pr.CreatePayee{RequestID: "default_second", Method: "wechat", Currency: "CNY", Account: "openid_01234567890123456789", AccountName: "private-name", IdentifierType: "openid", AppID: "wx0123456789abcdef"})
	require.NoError(t, e)
	_, e = p.Change(ctx, 11, "trace", second.PayeeAccountNo, false)
	require.NoError(t, e)
	proof, e := s.ReconcileWithdrawal(ctx, financeAdmin, er.Reconcile{WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	require.Equal(t, "passed", proof.Status)
	_, e = p.Change(ctx, 11, "trace", no, true)
	require.NoError(t, e)
	copy, e = s.ApplyWithdrawal(ctx, 11, r)
	require.NoError(t, e)
	require.Equal(t, w.WithdrawalNo, copy.WithdrawalNo) // result recovery is still allowed
	r.RequestID = "after_disable"
	_, e = s.ApplyWithdrawal(ctx, 11, r)
	require.ErrorIs(t, e, es.ErrState)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "已停用阻止审核"})
	require.ErrorIs(t, e, es.ErrInvariant)
	var saved em.Withdrawal
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&saved).Error)
	require.Equal(t, w.PayeeRef, saved.PayeeRef)
	require.Equal(t, w.PayeeVersion, saved.PayeeVersion)
	require.Equal(t, w.AccountMasked, saved.AccountMasked)
	require.Equal(t, "pending_review", saved.Status)
	detail, e := s.WithdrawalDetail(ctx, 0, &financeAdmin, w.WithdrawalNo)
	require.NoError(t, e)
	b, e := json.Marshal(detail)
	require.NoError(t, e)
	require.Contains(t, string(b), `"state":"disabled"`)
	require.NotContains(t, string(b), "6222021234567890123")
	require.NotContains(t, string(b), "private-payee-name")
	require.NotContains(t, string(b), "encryptedPayload")
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Reason: "释放冻结后重新申请"})
	require.NoError(t, e)
	account(t, s, 40, 0, 0)
}
func TestRealPayeeAuditRollbackAndHistoricalPlaceholder(t *testing.T) {
	s, g, _ := fixture(t, false)
	p, no := realPayees(t, s)
	rule := exchange(t, s, "1", "1")
	send(t, g, "rollback_income", "10")
	settle(t, s, rule.RuleNo, "rollback_settle")
	r := er.ApplyWithdrawal{RequestID: "rollback_payee", Amount: "5", Currency: "CNY", PayeeAccountNo: no}
	fail := errors.New("injected after freeze")
	require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("test:payout_failure", func(db *gorm.DB) {
		if db.Statement.Table == "anchor_finance_audit" {
			db.AddError(fail)
		}
	}))
	_, e := s.ApplyWithdrawal(ctx, 11, r)
	require.ErrorIs(t, e, fail)
	require.NoError(t, s.DB.Callback().Create().Remove("test:payout_failure"))
	account(t, s, 40, 0, 0)
	// Controlled old-version writer fixture creates a historical placeholder fact;
	// production code cannot create or mutate this snapshot.
	require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("test:legacy_writer", func(db *gorm.DB) {
		if w, ok := db.Statement.Dest.(*em.Withdrawal); ok {
			w.PayeeRef = "payout_center_pending"
			w.PayeeVersion = "payout_center_pending_v1"
			w.PayeeMethod = "payout_center"
			w.AccountName = "待出款中心核验"
			w.AccountMasked = "待出款中心核验***"
		}
	}))
	w, e := s.ApplyWithdrawal(ctx, 11, r)
	require.NoError(t, e)
	require.NoError(t, s.DB.Callback().Create().Remove("test:legacy_writer"))
	require.NoError(t, initialize.MigratePayout(s.DB))
	_, e = p.Change(ctx, 11, "trace", no, false)
	require.NoError(t, e)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "历史占位单必须阻止"})
	require.ErrorIs(t, e, es.ErrInvariant)
	var saved em.Withdrawal
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&saved).Error)
	require.Equal(t, "payout_center_pending", saved.PayeeRef)
	require.Equal(t, "pending_review", saved.Status)
	account(t, s, 35, 5, 0)
}
func TestMySQLPayeeDisableWithdrawalRace(t *testing.T) {
	s, g, _ := fixture(t, true)
	p, _ := realPayees(t, s)
	rule := exchange(t, s, "1", "1")
	send(t, g, "race_income_payee", "100")
	settle(t, s, rule.RuleNo, "race_settle_payee")
	var applied int64
	for i := 0; i < 16; i++ {
		a, e := p.Create(ctx, 11, "trace", pr.CreatePayee{RequestID: fmt.Sprintf("race_payee_%04d", i), Method: "bank_card", Currency: "CNY", Account: fmt.Sprintf("62220212345678%05d", i), AccountName: "private-name", BankCode: "ICBC"})
		require.NoError(t, e)
		bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var applyErr, disableErr error
		r := er.ApplyWithdrawal{RequestID: fmt.Sprintf("race_withdraw_%04d", i), Amount: "1", Currency: "CNY", PayeeAccountNo: a.PayeeAccountNo}
		go func() { defer wg.Done(); <-start; _, applyErr = s.ApplyWithdrawal(bounded, 11, r) }()
		go func() {
			defer wg.Done()
			<-start
			_, disableErr = p.Change(bounded, 11, "trace", a.PayeeAccountNo, true)
		}()
		close(start)
		wg.Wait()
		cancel()
		require.NoError(t, disableErr)
		if applyErr == nil {
			applied++
		} else {
			require.ErrorIs(t, applyErr, es.ErrState)
		}
		r.RequestID += "_later"
		_, e = s.ApplyWithdrawal(ctx, 11, r)
		require.ErrorIs(t, e, es.ErrState)
	}
	account(t, s, 400-applied, applied, 0)
	var count int64
	require.NoError(t, s.DB.Model(&pm.PayeeAccount{}).Where("status='disabled'").Count(&count).Error)
	require.EqualValues(t, 16, count)
}
