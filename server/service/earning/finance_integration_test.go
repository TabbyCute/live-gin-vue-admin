package earning_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"tb_live_module/global"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	gm "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	lm "tb_live_module/model/live"
	sm "tb_live_module/model/social"
	sys "tb_live_module/model/system"
	wr "tb_live_module/model/wallet/request"
	es "tb_live_module/service/earning"
	svc "tb_live_module/service/gift"
	wallet "tb_live_module/service/wallet"
	"testing"
	"time"
)

var ctx = context.Background()
var admin = svc.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}
var reviewer = svc.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}
var financeAdmin = es.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}
var financeReviewer = es.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}

func ptr(s string) *string { return &s }
func giftFixture(t *testing.T, isMySQL bool) *svc.Service {
	t.Helper()
	var db *gorm.DB
	var e error
	if isMySQL {
		raw := os.Getenv("EARNING_TEST_MYSQL_DSN")
		if raw == "" {
			t.Skip("EARNING_TEST_MYSQL_DSN not configured")
		}
		cfg, e := driver.ParseDSN(raw)
		require.NoError(t, e)
		require.Equal(t, "127.0.0.1:13328", cfg.Addr)
		require.True(t, strings.HasPrefix(cfg.DBName, "earning_test_"))
		cfg.ParseTime = true
		db, e = gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
		// The fixture refuses every address except the dedicated local disposable container.
		var tables []string
		require.NoError(t, db.Raw("SHOW TABLES").Scan(&tables).Error)
		for _, table := range tables {
			require.NoError(t, db.Exec("DROP TABLE `"+table+"`").Error)
		}
	} else {
		db, e = gorm.Open(sqlite.Open("file:"+t.TempDir()+"/earning.db"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
	}
	pool, e := db.DB()
	require.NoError(t, e)
	if !isMySQL {
		pool.SetMaxOpenConns(1)
	} else {
		pool.SetMaxOpenConns(32)
	}
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&lm.LiveAccount{}, &lm.LiveAnchor{}, &lm.LiveRoom{}, &lm.LiveSession{}, &sm.SocialFollow{}, &sm.SocialBlock{}, &sys.SysUser{}, &sys.SysBaseMenu{}, &sys.SysAuthorityMenu{}, &sys.SysApi{}))
	require.NoError(t, initialize.MigrateWallet(db))
	require.NoError(t, initialize.MigrateGift(db))
	require.NoError(t, initialize.MigrateGift(db))
	require.NoError(t, db.Create(&sys.SysBaseMenu{Name: "liveManagement", Path: "liveManagement"}).Error)
	require.NoError(t, initialize.SyncGiftMetadata(db))
	require.NoError(t, initialize.SyncGiftMetadata(db))
	require.NoError(t, db.Create(&sys.SysAuthority{AuthorityId: 888, AuthorityName: "测试超级管理员"}).Error)
	for _, id := range []uint{1, 2, 3} {
		require.NoError(t, db.Create(&sys.SysUser{GVA_MODEL: global.GVA_MODEL{ID: id}, Username: fmt.Sprintf("admin%d", id), Enable: 1, AuthorityId: 888}).Error)
		require.NoError(t, db.Exec("INSERT INTO sys_user_authority(sys_user_id,sys_authority_authority_id) VALUES (?,888)", id).Error)
	}
	for _, id := range []uint{10, 11} {
		require.NoError(t, db.Create(&lm.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: fmt.Sprintf("100000%d", id), Phone: ptr(fmt.Sprintf("+601000000%d", id)), Username: fmt.Sprintf("user%d", id), Nickname: "Sender", Status: 1}).Error)
	}
	anchor := lm.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: 1}, UserId: 11, AnchorNo: "anchor01", Status: 1, ApplyStatus: 2, LivePermission: 1}
	require.NoError(t, db.Create(&anchor).Error)
	room := lm.LiveRoom{GVA_MODEL: global.GVA_MODEL{ID: 1}, RoomNo: "room0001", AnchorId: 1, Status: 1, LiveStatus: 2, CurrentSessionId: 1, Visibility: 1, StreamName: "teststream"}
	require.NoError(t, db.Create(&room).Error)
	session := lm.LiveSession{GVA_MODEL: global.GVA_MODEL{ID: 1}, SessionNo: "session01", RoomId: 1, AnchorId: 1, Status: 1}
	require.NoError(t, db.Create(&session).Error)
	w := &wallet.Service{DB: db}
	s := &svc.Service{DB: db, Wallet: w}
	o, e := w.Create(ctx, wallet.Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}, wr.CreateAdjustment{RequestID: "seed_wallet", ExternalOrderNo: "GIFT-SEED", AccountNo: "10000010", Amount: "1000", Reason: "测试钱包初始资金", EvidenceRef: "test:controlled-fixture"})
	require.NoError(t, e)
	_, e = w.Approve(ctx, wallet.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}, wr.ReviewAdjustment{OrderNo: o.OrderNo, Reason: "核验测试资金"})
	require.NoError(t, e)
	_, e = s.SaveCategory(ctx, admin, req.Category{CategoryNo: "category1", Name: "鲜花", Status: "enabled"})
	require.NoError(t, e)
	_, e = s.SaveItem(ctx, admin, item())
	require.NoError(t, e)
	s.Now = func() time.Time { return time.Now().Add(-time.Hour) }
	r, e := s.CreateRule(ctx, admin, er.Rule{Name: "测试分成", ScopeType: "global", AnchorShareBPS: "4000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(24 * time.Hour).UnixMilli()})
	require.NoError(t, e)
	_, e = s.ReviewRule(ctx, reviewer, er.Review{RuleNo: r.RuleNo, Approve: true})
	require.NoError(t, e)
	return s
}
func item() req.Item {
	return req.Item{GiftNo: "gift0001", CategoryNo: "category1", Name: "玫瑰", IconURL: "https://cdn.example.com/rose.png", AnimationType: "none", CoinPrice: "10", Status: "enabled", MaxQuantityPerOrder: "100", BenefitType: "none", BenefitStatus: "disabled"}
}

type payeeProvider struct {
	mu      sync.Mutex
	version string
	blocked bool
}

func (p *payeeProvider) ResolveInTx(tx *gorm.DB, anchor, ref, currency string) (es.Payee, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.blocked {
		return es.Payee{}, errors.New("provider unavailable")
	}
	return es.Payee{AnchorNo: anchor, Reference: ref, Version: p.version, Currency: currency, Method: "bank_card", Name: "受控测试实名", Masked: "****1234", IdentityVerified: true, RiskChecked: true, RiskFlags: []string{}, CheckedAt: time.Now().UnixMilli()}, nil
}
func fixture(t *testing.T, mysql bool) (*es.Service, *svc.Service, *payeeProvider) {
	t.Helper()
	g := giftFixture(t, mysql)
	db := g.DB
	require.NoError(t, initialize.MigrateEarning(db))
	require.NoError(t, initialize.MigrateEarning(db))
	require.NoError(t, initialize.MigratePayout(db))
	require.NoError(t, initialize.SyncEarningMetadata(db))
	require.NoError(t, initialize.SyncEarningMetadata(db))
	require.NoError(t, db.Model(&lm.LiveAnchor{}).Where("id=1").Update("withdraw_permission", 1).Error)
	g.EnrichEarnings = es.MarkGiftSettlementStatus
	p := &payeeProvider{version: "payee_v1"}
	s := &es.Service{DB: db, Payees: p, Sources: map[string]es.SourceVerifier{"gift": g.VerifyIncomeSources}}
	return s, g, p
}
func exchange(t *testing.T, s *es.Service, coin, minor string) em.SettlementRule {
	t.Helper()
	r, e := s.CreateExchangeRule(ctx, financeAdmin, er.ExchangeRule{RequestID: "exchange_" + coin + "_" + minor, Name: "测试兑换", SourceUnit: "COIN", Currency: "CNY", CoinUnits: coin, MinorUnits: minor, RoundingMode: "floor", EffectiveFrom: 1, EffectiveTo: time.Now().Add(24 * time.Hour).UnixMilli()})
	require.NoError(t, e)
	_, e = s.ReviewExchangeRule(ctx, financeAdmin, er.FinanceReview{No: r.RuleNo, Approve: true, Reason: "不能自己复核"})
	require.ErrorIs(t, e, es.ErrForbidden)
	r, e = s.ReviewExchangeRule(ctx, financeReviewer, er.FinanceReview{No: r.RuleNo, Approve: true, Reason: "受控测试规则确认"})
	require.NoError(t, e)
	return r
}
func send(t *testing.T, g *svc.Service, key, qty string) gm.Order {
	t.Helper()
	r, e := g.Send(ctx, 10, "127.0.0.1", "trace_income", req.Send{RoomNo: "room0001", GiftNo: "gift0001", Quantity: qty, RequestID: key})
	require.NoError(t, e)
	return r
}
func settle(t *testing.T, s *es.Service, rule, key string) em.Settlement {
	t.Helper()
	r, e := s.CreateSettlement(ctx, financeAdmin, er.CreateSettlement{RequestID: key, AnchorNo: "anchor01", RuleNo: rule, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()})
	require.NoError(t, e)
	r, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: r.SettlementNo, Approve: true, Reason: "受控结算复核"})
	require.NoError(t, e)
	return r
}
func account(t *testing.T, s *es.Service, available, frozen, debt int64) {
	t.Helper()
	var a em.Account
	require.NoError(t, s.DB.Where("anchor_id=1 AND currency='CNY'").Take(&a).Error)
	require.Equal(t, available, a.Available)
	require.Equal(t, frozen, a.Frozen)
	require.Equal(t, debt, a.Debt)
	require.Zero(t, a.Paid)
	r, e := s.CheckAccount(ctx, financeAdmin, er.FinanceQuery{AnchorNo: "anchor01", Currency: "CNY"})
	require.NoError(t, e)
	require.Equal(t, true, r["consistent"], r)
}
func apply(t *testing.T, s *es.Service, key, amount string) em.Withdrawal {
	t.Helper()
	r, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: key, Amount: amount, Currency: "CNY"})
	require.NoError(t, e)
	return r
}
func TestFinanceLifecycle(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "10", "1")
	send(t, g, "income_order_001", "100")
	st := settle(t, s, rule.RuleNo, "settle_first")
	require.Equal(t, int64(40), st.Amount)
	account(t, s, 40, 0, 0)
	w := apply(t, s, "withdraw_001", "15")
	account(t, s, 25, 15, 0)
	p, e := s.ReconcileWithdrawal(ctx, financeAdmin, er.Reconcile{WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	require.Equal(t, "passed", p.Status)
	result, e := s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "系统对账与资料核验通过"})
	require.NoError(t, e)
	require.Equal(t, "approved", result.Status)
	account(t, s, 25, 15, 0)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "系统对账与资料核验通过"})
	require.NoError(t, e)
	account(t, s, 25, 15, 0)
	rejected := apply(t, s, "withdraw_002", "20")
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: rejected.WithdrawalNo, Reason: "受控驳回测试"})
	require.NoError(t, e)
	account(t, s, 25, 15, 0)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: rejected.WithdrawalNo, Reason: "受控驳回测试"})
	require.NoError(t, e)
	account(t, s, 25, 15, 0)
	require.Error(t, s.DB.Model(&em.Withdrawal{}).Where("withdrawal_no=?", w.WithdrawalNo).Update("status", "paid").Error)
}
func TestWithdrawalDoesNotRequireIdentityCertification(t *testing.T) {
	s, g, _ := fixture(t, false)
	require.NoError(t, s.DB.Model(&lm.LiveAnchor{}).Where("id=1").Update("cert_status", 0).Error)
	require.NoError(t, s.DB.Model(&lm.LiveAccount{}).Where("id=11").Update("real_name_status", 0).Error)
	rule := exchange(t, s, "1", "1")
	send(t, g, "pending_payout_income", "10")
	settle(t, s, rule.RuleNo, "pending_payout_settle")
	w := apply(t, s, "pending_payout_apply", "5")
	require.Equal(t, "pa_0123456789abcdef0123456789abcdef", w.PayeeRef)
	require.Equal(t, "payee_v1", w.PayeeVersion)
	require.Equal(t, "bank_card", w.PayeeMethod)
	require.Equal(t, "****1234", w.AccountMasked)
	require.Equal(t, "pending_review", w.Status)
	account(t, s, 35, 5, 0)
	proof, e := s.ReconcileWithdrawal(ctx, financeAdmin, er.Reconcile{WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	require.Equal(t, "passed", proof.Status)
	w, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "账务与主播资格核验通过"})
	require.NoError(t, e)
	require.Equal(t, "approved", w.Status)
	account(t, s, 35, 5, 0)
}
func TestSettlementOverlapAndRefundDebt(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "3", "2")
	order := send(t, g, "income_order_001", "3")
	q := er.CreateSettlement{RequestID: "settlement_a", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()}
	first, e := s.CreateSettlement(ctx, financeAdmin, q)
	require.NoError(t, e)
	q.RequestID = "settlement_b"
	second, e := s.CreateSettlement(ctx, financeAdmin, q)
	require.NoError(t, e)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: first.SettlementNo, Approve: true, Reason: "第一次批准"})
	require.NoError(t, e)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: second.SettlementNo, Approve: true, Reason: "重叠结算"})
	require.ErrorIs(t, e, es.ErrConflict)
	account(t, s, 8, 0, 0)
	w := apply(t, s, "withdraw_all", "8")
	_, e = g.Refund(ctx, admin, req.Refund{OrderNo: order.OrderNo, RequestID: "refund_first", Quantity: "1", Reason: "第一部分退款"})
	require.NoError(t, e)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "存在未结算退款"})
	require.ErrorIs(t, e, es.ErrInvariant)
	adjustment := settle(t, s, rule.RuleNo, "settle_refund_one")
	require.Equal(t, int64(-3), adjustment.Amount)
	account(t, s, 0, 8, 3)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Reason: "退款后释放冻结并抵扣欠额"})
	require.NoError(t, e)
	account(t, s, 5, 0, 0)
	_, e = g.Refund(ctx, admin, req.Refund{OrderNo: order.OrderNo, RequestID: "refund_second", Quantity: "2", Reason: "剩余部分全退"})
	require.NoError(t, e)
	adjustment = settle(t, s, rule.RuleNo, "settle_refund_two")
	require.Equal(t, int64(-5), adjustment.Amount)
	account(t, s, 0, 0, 0)
}
func TestPayeeChangedAndMissingDependencyFailClosed(t *testing.T) {
	s, g, p := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	send(t, g, "income_order_001", "20")
	settle(t, s, rule.RuleNo, "settle_first")
	w := apply(t, s, "withdraw_001", "10")
	p.version = "payee_v2"
	_, e := s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "收款账户已经变更"})
	require.ErrorIs(t, e, es.ErrInvariant)
	var saved em.Withdrawal
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&saved).Error)
	require.Equal(t, "pending_review", saved.Status)
	require.Equal(t, "abnormal", saved.ReconciliationStatus)
	s.Payees = nil
	_, e = s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: "disabled_apply", Amount: "1", Currency: "CNY"})
	require.ErrorIs(t, e, es.ErrDisabled)
	_, e = s.WithdrawalDetail(ctx, 10, nil, w.WithdrawalNo)
	require.Error(t, e)
	_, e = s.List(ctx, 11, nil, "income", er.FinanceQuery{})
	require.ErrorIs(t, e, es.ErrForbidden)
	require.NoError(t, s.DB.Model(&lm.LiveAnchor{}).Where("id=1").Update("withdraw_permission", 0).Error)
	_, e = s.ReconcileWithdrawal(ctx, financeAdmin, er.Reconcile{WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
}
func TestConversionBoundaries(t *testing.T) {
	v, e := es.Convert(math.MaxInt64, math.MaxInt64, math.MaxInt64)
	require.NoError(t, e)
	require.Equal(t, int64(math.MaxInt64), v)
	_, e = es.Convert(math.MaxInt64, 1, 2)
	require.ErrorIs(t, e, es.ErrInvalid)
	_, e = es.Convert(1, 0, 1)
	require.ErrorIs(t, e, es.ErrInvalid)
	_, e = es.Convert(-1, 1, 1)
	require.ErrorIs(t, e, es.ErrInvalid)
}

func TestMySQLFinanceConcurrentMoneyAndReview(t *testing.T) {
	s, g, _ := fixture(t, true)
	rule := exchange(t, s, "1", "1")
	send(t, g, "mysql_income_order", "100")
	st, e := s.CreateSettlement(ctx, financeAdmin, er.CreateSettlement{RequestID: "mysql_settlement", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()})
	require.NoError(t, e)
	const workers = 24
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: st.SettlementNo, Approve: true, Reason: "并发同一复核"})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	account(t, s, 400, 0, 0)
	errs = make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: "same_withdrawal", Amount: "100", Currency: "CNY"})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	account(t, s, 300, 100, 0)
	errs = make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: fmt.Sprintf("different_%04d", i), Amount: "100", Currency: "CNY"})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, es.ErrInsufficient)
		}
	}
	require.Equal(t, 3, success)
	account(t, s, 0, 400, 0)
	var w em.Withdrawal
	require.NoError(t, s.DB.Where("request_id='same_withdrawal'").Take(&w).Error)
	errs = make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: i%2 == 0, Reason: "同一轮竞争复核"})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			require.ErrorIs(t, e, es.ErrConflict)
		}
	}
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&w).Error)
	if w.Status == "approved" {
		account(t, s, 0, 400, 0)
	} else {
		require.Equal(t, "rejected", w.Status)
		account(t, s, 100, 300, 0)
	}
	require.NoError(t, initialize.ValidateEarningSchema(s.DB))
}
func TestFinanceCorruptionBlocksApprovalAndPersistsSnapshot(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	send(t, g, "income_corruption", "20")
	settle(t, s, rule.RuleNo, "settle_corruption")
	w := apply(t, s, "withdraw_corrupt", "10")
	require.Error(t, s.DB.Model(&em.AccountLedger{}).Where("business_no=?", w.WithdrawalNo).Update("business_no", "wd_wrong").Error)
	require.NoError(t, s.DB.Exec("DROP TRIGGER trg_anchor_withdrawable_ledger_update").Error)
	require.NoError(t, s.DB.Model(&em.AccountLedger{}).Where("business_no=?", w.WithdrawalNo).Update("business_no", "wd_wrong").Error)
	_, e := s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "损坏账务必须禁止通过"})
	require.ErrorIs(t, e, es.ErrInvariant)
	var saved em.Withdrawal
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&saved).Error)
	require.Equal(t, "pending_review", saved.Status)
	require.Equal(t, "abnormal", saved.ReconciliationStatus)
	var proof em.Reconciliation
	require.NoError(t, s.DB.Where("reconciliation_no=?", saved.ReconciliationNo).Take(&proof).Error)
	require.Equal(t, "abnormal", proof.Status)
	require.Error(t, s.DB.Model(&proof).Update("snapshot", "{}").Error)
	require.Error(t, s.DB.Delete(&proof).Error)
}
func TestFinanceRollbackIdempotencyAndProviderIsolation(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	send(t, g, "income_atomicity", "20")
	st := settle(t, s, rule.RuleNo, "settle_atomicity")
	account(t, s, 80, 0, 0)
	fail := errors.New("injected audit write failure")
	require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("test:earning_failure", func(db *gorm.DB) {
		if db.Statement.Table == "anchor_finance_audit" {
			db.AddError(fail)
		}
	}))
	_, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: "atomic_withdrawal", Amount: "10", Currency: "CNY"})
	require.ErrorIs(t, e, fail)
	require.NoError(t, s.DB.Callback().Create().Remove("test:earning_failure"))
	account(t, s, 80, 0, 0)
	w := apply(t, s, "atomic_withdrawal", "10")
	again := apply(t, s, "atomic_withdrawal", "10")
	require.Equal(t, w.WithdrawalNo, again.WithdrawalNo)
	_, e = s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: "atomic_withdrawal", Amount: "11", Currency: "CNY"})
	require.ErrorIs(t, e, es.ErrConflict)
	_, e = s.ReviewSettlement(ctx, financeAdmin, er.FinanceReview{No: st.SettlementNo, Approve: true, Reason: "受控结算复核"})
	require.ErrorIs(t, e, es.ErrConflict)
	_, e = s.List(ctx, 11, &es.Actor{ID: 3, AuthorityID: 999}, "withdrawal", er.FinanceQuery{})
	require.ErrorIs(t, e, es.ErrForbidden)
	require.NoError(t, s.DB.Where("authority_id=888 AND path='/earning/withdrawal/review'").Delete(&em.AdminPermission{}).Error)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "撤权立即生效"})
	require.ErrorIs(t, e, es.ErrForbidden)
}
func TestGenericSourcesAndBackfill(t *testing.T) {
	s, g, _ := fixture(t, false)
	send(t, g, "gift_source_backfill", "10")
	require.NoError(t, initialize.BackfillGiftIncome(s.DB))
	require.NoError(t, initialize.BackfillGiftIncome(s.DB))
	var count int64
	require.NoError(t, s.DB.Model(&em.Income{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
	input := es.IncomeInput{AnchorID: 1, SourceType: "guard", SourceNo: "controlled_guard_order", SourceEntryNo: "controlled_guard_income", Unit: "COIN", Amount: 50, GrossAmount: 100, RuleNo: "guard_test_rule", ShareBPS: 5000, CreatedAt: time.Now().Add(-time.Minute).UnixMilli(), SourceHash: strings.Repeat("a", 64)}
	require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error { _, e := es.AccrueInTx(tx, input); return e }))
	require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error { _, e := es.AccrueInTx(tx, input); return e }))
	rule := exchange(t, s, "1", "1")
	q := er.CreateSettlement{RequestID: "multi_source_settle", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()}
	_, e := s.CreateSettlement(ctx, financeAdmin, q)
	require.ErrorIs(t, e, es.ErrSource)
	// Test-only provider stands for an independently verified guard purchase; production registers no guard source.
	s.Sources["guard"] = func(tx *gorm.DB, facts []em.Income) error {
		for _, f := range facts {
			if f.SourceNo != input.SourceNo || f.SourceEntryNo != input.SourceEntryNo || f.Amount != input.Amount || f.SourceHash != input.SourceHash {
				return es.ErrInvariant
			}
		}
		return nil
	}
	st, e := s.CreateSettlement(ctx, financeAdmin, q)
	require.NoError(t, e)
	require.Equal(t, int64(90), st.Amount)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: st.SettlementNo, Approve: true, Reason: "多收益来源同账务结算"})
	require.NoError(t, e)
	account(t, s, 90, 0, 0)
}

func TestFinanceAmountOverflowAndAtomicSettlement(t *testing.T) {
	s, _, _ := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	s.Sources["charge"] = func(*gorm.DB, []em.Income) error { return nil }
	accrue := func(key string, amount int64) {
		require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error {
			_, e := es.AccrueInTx(tx, es.IncomeInput{AnchorID: 1, SourceType: "charge", SourceNo: key, SourceEntryNo: key, Unit: "COIN", Amount: amount, GrossAmount: amount, RuleNo: "controlled_max_rule", ShareBPS: 10000, CreatedAt: time.Now().Add(-time.Minute).UnixMilli(), SourceHash: strings.Repeat("b", 64)})
			return e
		}))
	}
	accrue("maximum_income", math.MaxInt64)
	settle(t, s, rule.RuleNo, "settle_maximum")
	account(t, s, math.MaxInt64, 0, 0)
	accrue("overflow_income", 1)
	st, e := s.CreateSettlement(ctx, financeAdmin, er.CreateSettlement{RequestID: "settle_overflow", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()})
	require.NoError(t, e)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: st.SettlementNo, Approve: true, Reason: "溢出必须全部回滚"})
	require.ErrorIs(t, e, es.ErrInvalid)
	account(t, s, math.MaxInt64, 0, 0)
	var n int64
	require.NoError(t, s.DB.Model(&em.Allocation{}).Where("settlement_no=?", st.SettlementNo).Count(&n).Error)
	require.Zero(t, n)
	w := apply(t, s, "withdraw_maximum", "9223372036854775807")
	account(t, s, 0, math.MaxInt64, 0)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Reason: "极限值释放验证"})
	require.NoError(t, e)
	account(t, s, math.MaxInt64, 0, 0)
}
func TestSettlementFailureAtEachMoneyWriteRollsBack(t *testing.T) {
	for _, table := range []string{"anchor_settlement_allocation", "anchor_withdrawable_ledger", "anchor_settlement_rounding", "anchor_finance_audit"} {
		t.Run(table, func(t *testing.T) {
			s, g, _ := fixture(t, false)
			rule := exchange(t, s, "1", "1")
			send(t, g, "rollback_income_001", "10")
			st, e := s.CreateSettlement(ctx, financeAdmin, er.CreateSettlement{RequestID: "rollback_settlement", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()})
			require.NoError(t, e)
			injected := errors.New("injected financial write failure")
			require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("test:settlement_failure", func(db *gorm.DB) {
				if db.Statement.Table == table {
					db.AddError(injected)
				}
			}))
			_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: st.SettlementNo, Approve: true, Reason: "故障注入"})
			require.ErrorIs(t, e, injected)
			require.NoError(t, s.DB.Callback().Create().Remove("test:settlement_failure"))
			for _, m := range []any{&em.Account{}, &em.AccountLedger{}, &em.Allocation{}, &em.RoundingBalance{}} {
				var n int64
				require.NoError(t, s.DB.Model(m).Count(&n).Error)
				require.Zero(t, n)
			}
			require.NoError(t, s.DB.Where("settlement_no=?", st.SettlementNo).Take(&st).Error)
			require.Equal(t, "pending", st.Status)
		})
	}
}
func TestGiftIncomeFailureRollsBackWallet(t *testing.T) {
	s, g, _ := fixture(t, false)
	injected := errors.New("income append failed")
	require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("test:income_failure", func(db *gorm.DB) {
		if db.Statement.Table == "anchor_income_fact" {
			db.AddError(injected)
		}
	}))
	_, e := g.Send(ctx, 10, "127.0.0.1", "trace_fail", req.Send{RoomNo: "room0001", GiftNo: "gift0001", Quantity: "10", RequestID: "failed_income_gift"})
	require.ErrorIs(t, e, injected)
	require.NoError(t, s.DB.Callback().Create().Remove("test:income_failure"))
	balance, e := g.Wallet.(*wallet.Service).Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "1000", balance.AvailableBalance)
	for _, m := range []any{&em.Income{}, &em.Ledger{}, &gm.Order{}} {
		var n int64
		require.NoError(t, s.DB.Model(m).Count(&n).Error)
		require.Zero(t, n)
	}
}
func TestExchangeMissingAndBoundaries(t *testing.T) {
	s, g, _ := fixture(t, false)
	send(t, g, "missing_rule_order", "1")
	_, e := s.CreateSettlement(ctx, financeAdmin, er.CreateSettlement{RequestID: "missing_rule_settle", AnchorNo: "anchor01", RuleNo: "missing_rule", PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()})
	require.ErrorIs(t, e, es.ErrExchange)
	rule := exchange(t, s, "1", "1")
	for _, v := range []string{"0", "-1", "01", "1.0", "1e3", "9223372036854775808"} {
		_, e = s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: "bad_amount_request", Currency: "CNY", Amount: v})
		require.ErrorIs(t, e, es.ErrInvalid)
	}
	r, e := s.CreateExchangeRule(ctx, financeAdmin, er.ExchangeRule{RequestID: "overlapping_exchange", Name: "重叠兑换", SourceUnit: "COIN", Currency: "CNY", CoinUnits: "2", MinorUnits: "1", RoundingMode: "floor", EffectiveFrom: 1, EffectiveTo: rule.EffectiveTo})
	require.NoError(t, e)
	_, e = s.ReviewExchangeRule(ctx, financeReviewer, er.FinanceReview{No: r.RuleNo, Approve: true, Reason: "不能重叠"})
	require.ErrorIs(t, e, es.ErrExchange)
	_, e = s.List(ctx, 11, &financeAdmin, "settlement", er.FinanceQuery{Sort: "amount; DROP TABLE anchor_settlement"})
	require.ErrorIs(t, e, es.ErrInvalid)
}

func TestMySQLFinanceLoad(t *testing.T) {
	if os.Getenv("EARNING_RUN_LOAD") != "1" {
		t.Skip("EARNING_RUN_LOAD=1 enables the bounded local load scenario")
	}
	s, g, _ := fixture(t, true)
	payout, payeeNo := realPayees(t, s)
	rule := exchange(t, s, "1", "1")
	w := g.Wallet.(*wallet.Service)
	topup, e := w.Create(ctx, wallet.Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}, wr.CreateAdjustment{RequestID: "load_topup_request", ExternalOrderNo: "EARNING-LOAD-TOPUP", AccountNo: "10000010", Amount: "10000", Reason: "本地受控压力测试资金", EvidenceRef: "test:local-earning-load"})
	require.NoError(t, e)
	_, e = w.Approve(ctx, wallet.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}, wr.ReviewAdjustment{OrderNo: topup.OrderNo, Reason: "本地受控压力测试复核"})
	require.NoError(t, e)
	for i := 0; i < 500; i++ {
		send(t, g, fmt.Sprintf("load_income_%04d", i), "1")
	}
	started := time.Now()
	settle(t, s, rule.RuleNo, "load_settlement")
	settlementMs := time.Since(started).Milliseconds()
	account(t, s, 2000, 0, 0)
	const workers = 16
	const rounds = 4
	latencies := make(chan int64, workers*rounds)
	errs := make(chan error, workers*rounds)
	var wg sync.WaitGroup
	started = time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				at := time.Now()
				bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
				_, e := s.ApplyWithdrawal(bounded, 11, er.ApplyWithdrawal{PayeeAccountNo: payeeNo, RequestID: fmt.Sprintf("load_withdraw_%03d_%03d", worker, j), Amount: "1", Currency: "CNY"})
				cancel()
				latencies <- time.Since(at).Milliseconds()
				errs <- e
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(started)
	close(latencies)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	values := []int64{}
	for v := range latencies {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	account(t, s, 1936, 64, 0)
	report := map[string]any{"databasePayee": true, "incomeFacts": 500, "concurrency": workers, "withdrawals": workers * rounds, "elapsedMs": elapsed.Milliseconds(), "p50Ms": values[len(values)/2], "p95Ms": values[len(values)*95/100], "maxMs": values[len(values)-1], "settlementCreateAndApproveMs": settlementMs, "allMoneyChecksPassed": true}
	b, _ := json.Marshal(report)
	t.Log("EARNING_LOAD_RESULT", string(b))
	require.Less(t, values[len(values)*95/100], int64(8000), "local p95 must fit the HTTP budget")
	measurePayoutLoad(t, s, payout)
}

func TestGiftDetailShowsDerivedSettlementStatus(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	order := send(t, g, "display_settlement_status", "1")
	before, e := g.Detail(ctx, admin, req.Query{No: order.OrderNo})
	require.NoError(t, e)
	require.True(t, before.Consistent)
	require.Equal(t, "pending", before.Earnings[0].Status)
	settle(t, s, rule.RuleNo, "display_settlement")
	after, e := g.Detail(ctx, admin, req.Query{No: order.OrderNo})
	require.NoError(t, e)
	require.True(t, after.Consistent)
	require.Equal(t, "settled", after.Earnings[0].Status)
	var fact em.Ledger
	require.NoError(t, s.DB.Where("gift_order_no=?", order.OrderNo).Take(&fact).Error)
	require.Equal(t, "pending", fact.Status)
}

func TestFractionalIncomeSurvivesSplitSettlementsAndRefunds(t *testing.T) {
	for _, split := range []bool{true, false} {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			s, g, _ := fixture(t, false)
			rule := exchange(t, s, "10", "1")
			orders := []gm.Order{}
			for i := 0; i < 3; i++ {
				orders = append(orders, send(t, g, fmt.Sprintf("small_order_%d", i), "1"))
				if split {
					settle(t, s, rule.RuleNo, fmt.Sprintf("small_settle_%d", i))
				}
			}
			if !split {
				settle(t, s, rule.RuleNo, "small_settle_all")
			}
			account(t, s, 1, 0, 0)
			var remainder em.RoundingBalance
			require.NoError(t, s.DB.Where("anchor_id=1").Take(&remainder).Error)
			require.Equal(t, int64(2), remainder.Remainder)
			for i, o := range orders {
				_, e := g.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, RequestID: fmt.Sprintf("small_refund_%d", i), Quantity: "1", Reason: "小额退款余数守恒"})
				require.NoError(t, e)
				settle(t, s, rule.RuleNo, fmt.Sprintf("refund_settle_%d", i))
			}
			account(t, s, 0, 0, 0)
			require.NoError(t, s.DB.Where("anchor_id=1").Take(&remainder).Error)
			require.Zero(t, remainder.Remainder)
		})
	}
}
func TestRoundingRejectsStaleIndependentPreview(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "10", "1")
	base := time.Now().Add(-time.Hour).UnixMilli()
	g.Now = func() time.Time { return time.UnixMilli(base) }
	send(t, g, "round_preview_a", "1")
	g.Now = func() time.Time { return time.UnixMilli(base + 1) }
	send(t, g, "round_preview_b", "1")
	makePreview := func(key string, from, to int64) em.Settlement {
		st, e := s.CreateSettlement(ctx, financeAdmin, er.CreateSettlement{RequestID: key, AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: from, PeriodEnd: to})
		require.NoError(t, e)
		return st
	}
	a := makePreview("round_settle_a", base, base+1)
	b := makePreview("round_settle_b", base+1, base+2)
	_, e := s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: a.SettlementNo, Approve: true, Reason: "批准第一份"})
	require.NoError(t, e)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: b.SettlementNo, Approve: true, Reason: "旧余数预览失效"})
	require.ErrorIs(t, e, es.ErrConflict)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: b.SettlementNo, Approve: false, Reason: "重新生成结算"})
	require.NoError(t, e)
	fresh := makePreview("round_settle_new", base+1, base+2)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: fresh.SettlementNo, Approve: true, Reason: "新余数预览通过"})
	require.NoError(t, e)
	account(t, s, 0, 0, 0)
	var remainder em.RoundingBalance
	require.NoError(t, s.DB.Where("anchor_id=1").Take(&remainder).Error)
	require.Equal(t, int64(8), remainder.Remainder)
	require.Error(t, s.DB.Model(&em.RoundingBalance{}).Where("id=?", remainder.ID).Update("remainder", 0).Error)
}
func TestBackfillMissingHistoricalGiftFacts(t *testing.T) {
	s, g, _ := fixture(t, false)
	o := send(t, g, "historical_income", "3")
	_, e := g.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, RequestID: "historical_refund", Quantity: "1", Reason: "历史退款"})
	require.NoError(t, e)
	// Only this temporary SQLite fixture removes its mirror, simulating a pre-migration database.
	var triggers []string
	require.NoError(t, s.DB.Raw("SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='anchor_income_fact'").Scan(&triggers).Error)
	for _, name := range triggers {
		require.NoError(t, s.DB.Exec("DROP TRIGGER "+name).Error)
	}
	require.NoError(t, s.DB.Exec("DELETE FROM anchor_income_fact").Error)
	require.Error(t, initialize.ValidateIncomeCoverage(s.DB))
	require.NoError(t, initialize.MigrateEarning(s.DB))
	require.NoError(t, initialize.BackfillGiftIncome(s.DB))
	require.NoError(t, initialize.BackfillGiftIncome(s.DB))
	require.NoError(t, initialize.ValidateIncomeCoverage(s.DB))
	var n int64
	require.NoError(t, s.DB.Model(&em.Income{}).Count(&n).Error)
	require.Equal(t, int64(2), n)
	rule := exchange(t, s, "3", "2")
	st := settle(t, s, rule.RuleNo, "historical_settle")
	require.Equal(t, int64(5), st.Amount)
	account(t, s, 5, 0, 0)
}

func TestWithdrawalResponseLossQueryIsOwnerScoped(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	send(t, g, "query_income_001", "10")
	settle(t, s, rule.RuleNo, "query_settle_001")
	w := apply(t, s, "query_request_001", "5")
	out, e := s.AppWithdrawalResult(ctx, 11, er.AppWithdrawalResult{RequestID: w.RequestID})
	require.NoError(t, e)
	body, e := json.Marshal(out)
	require.NoError(t, e)
	require.Contains(t, string(body), w.WithdrawalNo)
	require.NotContains(t, string(body), "payeeRef")
	require.NoError(t, s.DB.Create(&lm.LiveAnchor{UserId: 10, AnchorNo: "other_anchor", Status: 1}).Error)
	_, e = s.AppWithdrawalResult(ctx, 10, er.AppWithdrawalResult{RequestID: w.RequestID})
	require.ErrorIs(t, e, es.ErrNotFound)
	_, e = s.AppWithdrawalResult(ctx, 11, er.AppWithdrawalResult{RequestID: w.RequestID, No: w.WithdrawalNo})
	require.ErrorIs(t, e, es.ErrInvalid)
	_, e = s.AppWithdrawalResult(ctx, 11, er.AppWithdrawalResult{})
	require.ErrorIs(t, e, es.ErrInvalid)
	account(t, s, 35, 5, 0)
}

func TestSettlementFactBoundaryAndBatchRecovery(t *testing.T) {
	s, _, _ := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	base := time.Now().Add(-time.Hour).UnixMilli()
	s.Sources["charge"] = func(*gorm.DB, []em.Income) error { return nil } // Controlled test source only.
	require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < 1001; i++ {
			key := fmt.Sprintf("boundary_income_%04d", i)
			_, e := es.AccrueInTx(tx, es.IncomeInput{AnchorID: 1, SourceType: "charge", SourceNo: key, SourceEntryNo: key, Unit: "COIN", Amount: 1, GrossAmount: 1, ShareBPS: 10000, RuleNo: "boundary_test_rule", CreatedAt: base + int64(i), SourceHash: strings.Repeat("c", 64)})
			if e != nil {
				return e
			}
		}
		return nil
	}))
	q := er.CreateSettlement{RequestID: "boundary_settlement", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: base, PeriodEnd: base + 1001}
	_, e := s.CreateSettlement(ctx, financeAdmin, q)
	require.ErrorIs(t, e, es.ErrLimit)
	var n int64
	require.NoError(t, s.DB.Model(&em.Allocation{}).Count(&n).Error)
	require.Zero(t, n)
	q.PeriodEnd = base + 1000
	st, e := s.CreateSettlement(ctx, financeAdmin, q)
	require.NoError(t, e)
	require.Equal(t, int64(1000), st.ItemCount)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: st.SettlementNo, Approve: true, Reason: "上限内原子结算"})
	require.NoError(t, e)
	settle(t, s, rule.RuleNo, "boundary_remaining")
	account(t, s, 1001, 0, 0)
}
func TestRefundRetainsOriginalExchangeRateAfterRuleChange(t *testing.T) {
	s, g, _ := fixture(t, false)
	old := exchange(t, s, "10", "1")
	o := send(t, g, "old_rate_income", "3")
	settle(t, s, old.RuleNo, "old_rate_settle")
	account(t, s, 1, 0, 0)
	s.Now = func() time.Time { return time.UnixMilli(old.EffectiveTo + 1) }
	next, e := s.CreateExchangeRule(ctx, financeAdmin, er.ExchangeRule{RequestID: "replacement_exchange", Name: "后续不同兑换比例", SourceUnit: "COIN", Currency: "CNY", CoinUnits: "1", MinorUnits: "2", RoundingMode: "floor", EffectiveFrom: old.EffectiveTo, EffectiveTo: old.EffectiveTo + 86400000})
	require.NoError(t, e)
	next, e = s.ReviewExchangeRule(ctx, financeReviewer, er.FinanceReview{No: next.RuleNo, Approve: true, Reason: "新比例复核"})
	require.NoError(t, e)
	_, e = g.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, RequestID: "old_rate_refund", Quantity: "3", Reason: "切换规则后退款"})
	require.NoError(t, e)
	adjustment := settle(t, s, next.RuleNo, "old_rate_reversal")
	require.Equal(t, int64(-1), adjustment.Amount)
	account(t, s, 0, 0, 0)
	var items []em.SettlementItem
	require.NoError(t, s.DB.Where("settlement_no=?", adjustment.SettlementNo).Find(&items).Error)
	require.Len(t, items, 1)
	require.Equal(t, old.RuleNo, items[0].RuleNo)
}

type payeeFunc func(*gorm.DB, string, string, string) (es.Payee, error)

func (f payeeFunc) ResolveInTx(c *gorm.DB, a, r, u string) (es.Payee, error) { return f(c, a, r, u) }
func TestPayeeTrustBoundaryFailsBeforeFreezing(t *testing.T) {
	s, g, p := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	send(t, g, "provider_income", "10")
	settle(t, s, rule.RuleNo, "provider_settle")
	cases := []struct {
		name   string
		mutate func(*es.Payee)
	}{
		{"foreign_anchor", func(p *es.Payee) { p.AnchorNo = "other" }}, {"foreign_ref", func(p *es.Payee) { p.Reference = "other" }}, {"wrong_currency", func(p *es.Payee) { p.Currency = "USD" }},
		{"unchecked_risk", func(p *es.Payee) { p.RiskChecked = false }}, {"risk_flag", func(p *es.Payee) { p.RiskFlags = []string{"manual_hold"} }},
		{"expired", func(p *es.Payee) { p.CheckedAt = time.Now().Add(-time.Minute).UnixMilli() }}, {"future", func(p *es.Payee) { p.CheckedAt = time.Now().Add(time.Minute).UnixMilli() }},
		{"changed_future", func(p *es.Payee) { p.ChangedAt = p.CheckedAt + 1 }}, {"unmasked", func(p *es.Payee) { p.Masked = "1234567890" }}, {"empty_version", func(p *es.Payee) { p.Version = "" }}, {"control_character", func(p *es.Payee) { p.Method = "bank\n" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s.Payees = payeeFunc(func(c *gorm.DB, a, r, u string) (es.Payee, error) {
				v, e := p.ResolveInTx(c, a, r, u)
				tc.mutate(&v)
				return v, e
			})
			_, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: "provider_" + tc.name, Amount: "1", Currency: "CNY"})
			require.ErrorIs(t, e, es.ErrState)
		})
	}
	var n int64
	require.NoError(t, s.DB.Model(&em.Withdrawal{}).Count(&n).Error)
	require.Zero(t, n)
	account(t, s, 40, 0, 0)
	s.Payees = p
	w := apply(t, s, "risk_review_apply", "5")
	s.Payees = payeeFunc(func(c *gorm.DB, a, r, u string) (es.Payee, error) {
		v, e := p.ResolveInTx(c, a, r, u)
		v.RiskFlags = []string{"manual_hold"}
		return v, e
	})
	_, e := s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "风险变化阻止审核"})
	require.ErrorIs(t, e, es.ErrInvariant)
	var proof em.Reconciliation
	require.NoError(t, s.DB.Where("withdrawal_no=?", w.WithdrawalNo).Take(&proof).Error)
	require.Equal(t, "abnormal", proof.Status)
	require.Contains(t, proof.Snapshot, "manual_hold")
	account(t, s, 35, 5, 0)
}
func TestAnchorOwnershipChangedDuringProviderCallCannotSpendOldAccount(t *testing.T) {
	s, g, p := fixture(t, false)
	rule := exchange(t, s, "1", "1")
	send(t, g, "ownership_income", "10")
	settle(t, s, rule.RuleNo, "ownership_settle")
	s.Payees = payeeFunc(func(c *gorm.DB, a, r, u string) (es.Payee, error) {
		require.NoError(t, c.Model(&lm.LiveAnchor{}).Where("id=1").Update("user_id", 10).Error)
		require.NoError(t, c.Create(&lm.LiveAnchor{UserId: 11, AnchorNo: "new_owned_anchor", Status: 1, ApplyStatus: 2, CertStatus: 2, WithdrawPermission: 1}).Error)
		return p.ResolveInTx(c, a, r, u)
	})
	_, e := s.ApplyWithdrawal(ctx, 11, er.ApplyWithdrawal{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: "changed_ownership", Amount: "1", Currency: "CNY"})
	require.ErrorIs(t, e, es.ErrConflict)
	var n int64
	require.NoError(t, s.DB.Model(&em.Withdrawal{}).Count(&n).Error)
	require.Zero(t, n)
	account(t, s, 40, 0, 0)
}
