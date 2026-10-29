package wallet

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/config"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
)

var testCreator = Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}
var testReviewer = Actor{ID: 2, AuthorityID: 888, IP: "::1"}

func fixture(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/wallet.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.Exec("CREATE TABLE sys_users (id INTEGER PRIMARY KEY, nick_name VARCHAR(64), enable INTEGER, authority_id INTEGER, deleted_at datetime)").Error)
	require.NoError(t, db.Exec("CREATE TABLE sys_user_authority (sys_user_id INTEGER, sys_authority_authority_id INTEGER)").Error)
	require.NoError(t, db.Exec("CREATE TABLE live_account (id INTEGER PRIMARY KEY, account_no VARCHAR(32), phone VARCHAR(20), status INTEGER, deleted_at datetime)").Error)
	require.NoError(t, db.Exec("INSERT INTO sys_users(id,nick_name,enable,authority_id) VALUES (1,'Admin',1,888),(2,'Reviewer',1,888),(3,'Creator',1,888),(4,'Frozen',2,888),(5,'Other',1,999)").Error)
	require.NoError(t, db.Exec("INSERT INTO live_account(id,account_no,phone,status) VALUES (10,'10000010','+60100000010',1),(11,'10000011','+60100000011',1),(12,'10000012','+60100000012',2)").Error)
	require.NoError(t, db.AutoMigrate(model.Models()...))
	return &Service{DB: db, Now: func() time.Time { return time.Date(2026, 10, 28, 23, 59, 0, 0, time.UTC) }}
}
func grant(key, amount string) req.CreateAdjustment {
	return req.CreateAdjustment{RequestID: key, ExternalOrderNo: key, AccountNo: "10000010", Amount: amount, Reason: "运营活动奖励", EvidenceRef: "upload:private-proof-1"}
}
func review(no string) req.ReviewAdjustment {
	return req.ReviewAdjustment{OrderNo: no, Reason: "已核对活动凭证"}
}
func count(t *testing.T, db *gorm.DB, m any, want int64) {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(m).Count(&n).Error)
	require.Equal(t, want, n)
}
func amount(t *testing.T, s *Service, want string) {
	t.Helper()
	b, e := s.Balance(context.Background(), 10, "")
	require.NoError(t, e)
	require.Equal(t, want, b.AvailableBalance)
}

func TestGrantIdempotencyAuditAndPrivacy(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	creator := Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}
	amount(t, s, "0")
	count(t, s.DB, &model.WalletAccount{}, 0)
	o, err := s.Create(ctx, creator, grant("request_001", "100"))
	require.NoError(t, err)
	require.Equal(t, model.StatusPending, o.Status)
	amount(t, s, "0")
	same, err := s.Create(ctx, creator, grant("request_001", "100"))
	require.NoError(t, err)
	require.Equal(t, o.OrderNo, same.OrderNo)
	_, err = s.Create(ctx, creator, grant("request_001", "101"))
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.Approve(ctx, creator, review(o.OrderNo))
	require.ErrorIs(t, err, ErrSelfReview)
	paid, err := s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	require.Equal(t, model.StatusCredited, paid.Status)
	require.NotEmpty(t, paid.LedgerNo)
	_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	_, err = s.Approve(ctx, testCreator, review(o.OrderNo))
	require.ErrorIs(t, err, ErrConflict)
	_, err = s.Reject(ctx, testReviewer, review(o.OrderNo))
	require.ErrorIs(t, err, ErrConflict)
	amount(t, s, "100")
	count(t, s.DB, &model.WalletLedger{}, 1)
	count(t, s.DB, &model.WalletAuditLog{}, 3)
	count(t, s.DB, &model.WalletDailyQuota{}, 3)
	list, err := s.Ledger(ctx, 10, req.LedgerSearch{})
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	require.Empty(t, list.List[0].Reason)
	require.Empty(t, list.List[0].OperatorID)
	admin, err := s.AdminLedger(ctx, testCreator, req.LedgerSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, err)
	require.Equal(t, "2", admin.List[0].OperatorID)
	require.Equal(t, "Reviewer [ 2 ]", admin.List[0].OperatorDisplay)
	detail, err := s.Detail(ctx, testCreator, o.OrderNo)
	require.NoError(t, err)
	require.Len(t, detail.Audit, 2)
	require.Equal(t, "Creator [ 3 ]", detail.Order.CreatedByDisplay)
	require.Equal(t, "Reviewer [ 2 ]", detail.Order.ApprovedByDisplay)
	require.Equal(t, "Reviewer [ 2 ]", detail.Ledger.OperatorDisplay)
	require.Equal(t, "Creator [ 3 ]", detail.Audit[0].OperatorDisplay)
	require.Equal(t, "0", detail.Ledger.BalanceBefore)
	require.Equal(t, "100", detail.Ledger.BalanceAfter)
	check, err := s.Check(ctx, testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, err)
	require.True(t, check.Consistent)
	require.Equal(t, 1, check.Checked)
	byPhone, err := s.AdminBalance(ctx, testCreator, req.AccountQuery{Phone: "+60100000010"})
	require.NoError(t, err)
	require.Equal(t, "10000010", byPhone.AccountNo)
	require.Equal(t, "+60100000010", byPhone.Phone)
	require.Equal(t, "100", byPhone.AvailableBalance)
	phoneLedger, err := s.AdminLedger(ctx, testCreator, req.LedgerSearch{AccountQuery: req.AccountQuery{Phone: "+60100000010"}})
	require.NoError(t, err)
	require.Len(t, phoneLedger.List, 1)
	orders, err := s.Orders(ctx, testCreator, req.OrderSearch{Phone: "+60100000010"})
	require.NoError(t, err)
	require.EqualValues(t, 1, orders.Total)
}

func TestTopSuperAdminCanReviewOwnOrder(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	topAdmin := Actor{ID: 1, AuthorityID: 999, IP: "127.0.0.1"} // 不受钱包角色列表限制。
	o, err := s.Create(ctx, testCreator, grant("super_self_approve", "100"))
	require.NoError(t, err)
	paid, err := s.Approve(ctx, topAdmin, review(o.OrderNo))
	require.NoError(t, err)
	require.Equal(t, model.StatusCredited, paid.Status)
	require.Equal(t, "1", paid.CreatedBy)
	require.Equal(t, "1", paid.ApprovedBy)
	_, err = s.Approve(ctx, topAdmin, review(o.OrderNo))
	require.NoError(t, err)
	check, err := s.Check(ctx, testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, err)
	require.True(t, check.Consistent, check.Issues)
	count(t, s.DB, &model.WalletLedger{}, 1)
	var quota model.WalletDailyQuota
	require.NoError(t, s.DB.Where("subject_type = ? AND subject_id = ?", "operator", uint64(1)).Take(&quota).Error)
	require.EqualValues(t, 100, quota.Amount)

	rejectedOrder, err := s.Create(ctx, testCreator, grant("super_self_reject", "10"))
	require.NoError(t, err)
	rejected, err := s.Reject(ctx, topAdmin, review(rejectedOrder.OrderNo))
	require.NoError(t, err)
	require.Equal(t, model.StatusRejected, rejected.Status)
	require.Equal(t, "1", rejected.CreatedBy)
	require.Equal(t, "1", rejected.RejectedBy)
}

func TestReverseApprovalAndRejectedReservation(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, e := s.Create(ctx, testCreator, grant("reverse_base", "400"))
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, e)
	r := req.ReverseAdjustment{RequestID: "reverse_001", OrderNo: o.OrderNo, Reason: "奖励发放对象错误"}
	rev, e := s.Reverse(ctx, testReviewer, r)
	require.NoError(t, e)
	amount(t, s, "400")
	_, e = s.Approve(ctx, testReviewer, review(rev.OrderNo))
	require.ErrorIs(t, e, ErrSelfReview)
	dup, e := s.Reverse(ctx, testReviewer, r)
	require.NoError(t, e)
	require.Equal(t, rev.OrderNo, dup.OrderNo)
	r.RequestID = "reverse_002"
	_, e = s.Reverse(ctx, testReviewer, r)
	require.ErrorIs(t, e, ErrConflict)
	_, e = s.Reject(ctx, testCreator, review(rev.OrderNo))
	require.NoError(t, e)
	_, e = s.Reject(ctx, testCreator, review(rev.OrderNo))
	require.NoError(t, e)
	rev, e = s.Reverse(ctx, testReviewer, r)
	require.NoError(t, e)
	_, e = s.Approve(ctx, testCreator, review(rev.OrderNo))
	require.NoError(t, e)
	_, e = s.Approve(ctx, testCreator, review(rev.OrderNo))
	require.NoError(t, e)
	amount(t, s, "0")
	count(t, s.DB, &model.WalletLedger{}, 2)
	detail, e := s.Detail(ctx, testCreator, o.OrderNo)
	require.NoError(t, e)
	require.Equal(t, model.StatusReversed, detail.Order.Status)
	detail, e = s.Detail(ctx, testCreator, rev.OrderNo)
	require.NoError(t, e)
	require.Equal(t, "-400", detail.Ledger.ChangeAmount)
	require.NotEmpty(t, detail.Ledger.OriginalLedgerNo)
	var qs []model.WalletDailyQuota
	require.NoError(t, s.DB.Find(&qs).Error)
	for _, q := range qs {
		require.EqualValues(t, 400, q.Amount)
	}
	r.RequestID = "reverse_003"
	_, e = s.Reverse(ctx, testReviewer, r)
	require.ErrorIs(t, e, ErrConflict)
	r.OrderNo = rev.OrderNo
	_, e = s.Reverse(ctx, testReviewer, r)
	require.ErrorIs(t, e, ErrConflict)
	check, e := s.Check(ctx, testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 1})
	require.NoError(t, e)
	require.True(t, check.Consistent)
	require.True(t, check.HasMore)
	check, e = s.Check(ctx, testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, AfterVersion: check.NextVersion, AfterOrderID: check.NextOrderID})
	require.NoError(t, e)
	require.True(t, check.Consistent)
	require.False(t, check.HasMore)
}

func TestValidationAndPermissionBoundaries(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	for _, v := range []string{"0", "-1", "1.1", "01", "+1", "1e3", " 1", "1 ", "9223372036854775808", "9999999999999999999999999"} {
		t.Run(v, func(t *testing.T) {
			_, e := s.Create(ctx, testCreator, grant("invalid_001", v))
			require.ErrorIs(t, e, ErrInvalid)
		})
	}
	r := grant("invalid_002", "1")
	r.CurrencyCode = "USD"
	_, e := s.Create(ctx, testCreator, r)
	require.ErrorIs(t, e, ErrInvalid)
	r = grant("invalid_003", "1")
	r.Reason = " \n"
	_, e = s.Create(ctx, testCreator, r)
	require.ErrorIs(t, e, ErrInvalid)
	r = grant("invalid_004", "100000")
	r.EvidenceRef = ""
	_, e = s.Create(ctx, testCreator, r)
	require.ErrorIs(t, e, ErrEvidence)
	_, e = s.Create(ctx, testCreator, grant("invalid_005", "1000001"))
	require.ErrorIs(t, e, ErrLimit)
	for _, a := range []Actor{{}, {ID: 4, AuthorityID: 888, IP: "127.0.0.1"}, {ID: 5, AuthorityID: 888, IP: "127.0.0.1"}, {ID: 999, AuthorityID: 888, IP: "127.0.0.1"}} {
		_, e = s.Create(ctx, a, grant("invalid_006", "1"))
		require.ErrorIs(t, e, ErrForbidden)
	}
	r = grant("disabled_001", "1")
	r.AccountNo = "10000012"
	_, e = s.Create(ctx, testCreator, r)
	require.ErrorIs(t, e, ErrAccount)
	r.AccountNo = "99999999"
	_, e = s.Create(ctx, testCreator, r)
	require.ErrorIs(t, e, ErrAccount)
	_, e = s.Balance(ctx, 12, "")
	require.ErrorIs(t, e, ErrAccount)
	_, e = s.Ledger(ctx, 10, req.LedgerSearch{AccountQuery: req.AccountQuery{AccountNo: "10000011"}})
	require.ErrorIs(t, e, ErrInvalid)
	_, e = s.AdminBalance(ctx, testCreator, req.AccountQuery{Phone: "60100000010"})
	require.ErrorIs(t, e, ErrInvalid)
	_, e = s.AdminBalance(ctx, testCreator, req.AccountQuery{AccountNo: "10000010", Phone: "+60100000011"})
	require.ErrorIs(t, e, ErrAccount)
	count(t, s.DB, &model.WalletAdjustmentOrder{}, 0)
	// 独立申请、复核角色及数据库撤权。
	s.Config = config.Wallet{ReadAuthorities: []uint{777}, CreateAuthorities: []uint{888}, ReviewAuthorities: []uint{999}}
	require.NoError(t, s.DB.Exec("UPDATE sys_users SET authority_id=999 WHERE id=2").Error)
	o, e := s.Create(ctx, testCreator, grant("split_role_1", "1"))
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.ErrorIs(t, e, ErrForbidden)
	reviewer := testReviewer
	reviewer.AuthorityID = 999
	_, e = s.Approve(ctx, reviewer, review(o.OrderNo))
	require.NoError(t, e)
	require.NoError(t, s.DB.Exec("UPDATE sys_users SET enable=2 WHERE id=2").Error)
	_, e = s.Approve(ctx, reviewer, review(o.OrderNo))
	require.ErrorIs(t, e, ErrForbidden)
}

func TestQuotaRolloverAndStoppedWallet(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	s.Config = config.Wallet{MaxGrantAmount: 100, AccountDailyLimit: 150, OperatorDailyLimit: 200, EvidenceThreshold: 100}
	o, e := s.Create(ctx, testCreator, grant("limits_001", "100"))
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, e)
	second, e := s.Create(ctx, testCreator, grant("limits_002", "100"))
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(second.OrderNo))
	require.ErrorIs(t, e, ErrLimit)
	amount(t, s, "100")
	s.Now = func() time.Time { return time.Date(2026, 10, 29, 0, 0, 1, 0, time.UTC) }
	_, e = s.SetStatus(ctx, testReviewer, req.AccountStatus{AccountNo: "10000010", Status: model.AccountDisabled, Reason: "风险调查暂停"})
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(second.OrderNo))
	require.ErrorIs(t, e, ErrDisabled)
	_, e = s.SetStatus(ctx, testReviewer, req.AccountStatus{AccountNo: "10000010", Status: model.AccountNormal, Reason: "风险调查结束"})
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(second.OrderNo))
	require.NoError(t, e)
	amount(t, s, "200")
	r := grant("limits_003", "100")
	r.AccountNo = "10000011"
	third, e := s.Create(ctx, testCreator, r)
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(third.OrderNo))
	require.NoError(t, e)
	r.RequestID = "limits_004"
	r.ExternalOrderNo = "limits_004"
	fourth, e := s.Create(ctx, testCreator, r)
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(fourth.OrderNo))
	require.ErrorIs(t, e, ErrLimit)
}

func TestTransactionalFailuresRollBackEverything(t *testing.T) {
	for _, table := range []string{"wallet_ledger", "wallet_audit_log", "wallet_daily_quota"} {
		t.Run(table, func(t *testing.T) {
			s := fixture(t)
			ctx := context.Background()
			o, e := s.Create(ctx, testCreator, grant("rollback_001", "10"))
			require.NoError(t, e)
			require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("test:fail", func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(errors.New("injected storage failure"))
				}
			}))
			_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
			require.Error(t, e)
			require.NoError(t, s.DB.Callback().Create().Remove("test:fail"))
			amount(t, s, "0")
			count(t, s.DB, &model.WalletLedger{}, 0)
			count(t, s.DB, &model.WalletDailyQuota{}, 0)
			count(t, s.DB, &model.WalletAuditLog{}, 1)
			d, e := s.Detail(ctx, testCreator, o.OrderNo)
			require.NoError(t, e)
			require.Equal(t, model.StatusPending, d.Order.Status)
			_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
			require.NoError(t, e)
			amount(t, s, "10")
		})
	}
	s := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := s.Create(ctx, testCreator, grant("cancelled_1", "10"))
	require.ErrorIs(t, e, context.Canceled)
	count(t, s.DB, &model.WalletAdjustmentOrder{}, 0)
}

func TestOverflowInsufficientAndAuditDetection(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, e := s.Create(ctx, testCreator, grant("overflow_001", "10"))
	require.NoError(t, e)
	require.NoError(t, s.DB.Create(&model.WalletAccount{AccountID: 10, CurrencyCode: model.CurrencyCOIN, AvailableBalance: math.MaxInt64 - 5, Version: 1, Status: model.AccountNormal}).Error)
	require.NoError(t, s.DB.Create(&model.WalletLedger{LedgerNo: "overflow_seed", WalletAccountID: 1, WalletVersion: 1, ChangeAmount: math.MaxInt64 - 5, BalanceAfter: math.MaxInt64 - 5, BusinessType: "ADMIN_GRANT", BusinessNo: "overflow_seed", OperatorID: 2, Reason: "overflow fixture"}).Error)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.ErrorIs(t, e, ErrLimit)
	// 独立新fixture测试其他不变量，不能删除账务事实。
	s = fixture(t)
	o, e = s.Create(ctx, testCreator, grant("overflow_002", "10"))
	require.NoError(t, e)
	require.NoError(t, s.DB.Create(&model.WalletAccount{AccountID: 10, CurrencyCode: "COIN", Status: "normal"}).Error)
	require.NoError(t, s.DB.Model(&model.WalletAccount{}).Where("account_id=10").Updates(map[string]any{"available_balance": 0, "version": math.MaxInt64}).Error)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.ErrorIs(t, e, ErrInvariant)
	require.NoError(t, s.DB.Model(&model.WalletAccount{}).Where("account_id=10").Updates(map[string]any{"version": 0, "frozen_balance": 1}).Error)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.ErrorIs(t, e, ErrInvariant)
	require.NoError(t, s.DB.Model(&model.WalletAccount{}).Where("account_id=10").Update("frozen_balance", 0).Error)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, e)
	rev, e := s.Reverse(ctx, testCreator, req.ReverseAdjustment{RequestID: "insufficient_1", OrderNo: o.OrderNo, Reason: "错误入账冲正"})
	require.NoError(t, e)
	require.NoError(t, s.DB.Model(&model.WalletAccount{}).Where("account_id=10").Update("available_balance", 9).Error)
	// 模拟后续消费模块产生的合法扣减尾流水，测试未来消费后的余额不足冲正。
	require.NoError(t, s.DB.Create(&model.WalletLedger{LedgerNo: "future_debit", WalletAccountID: 1, WalletVersion: 2, ChangeAmount: -1, BalanceBefore: 10, BalanceAfter: 9, BusinessType: "FUTURE_DEBIT", BusinessNo: "future_debit", OperatorID: 2, Reason: "future consumption fixture"}).Error)
	require.NoError(t, s.DB.Model(&model.WalletAccount{}).Where("account_id=10").Update("version", 2).Error)
	_, e = s.Approve(ctx, testReviewer, review(rev.OrderNo))
	require.ErrorIs(t, e, ErrInsufficient)
	count(t, s.DB, &model.WalletLedger{}, 2)
	require.NoError(t, s.DB.Model(&model.WalletAccount{}).Where("account_id=10").Update("available_balance", 8).Error)
	check, e := s.Check(ctx, testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, e)
	require.False(t, check.Consistent)
	require.Contains(t, check.Issues, "wallet_tail_mismatch")
}

func TestPaginationAndLargeInteger(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	s.Config = config.Wallet{MaxGrantAmount: math.MaxInt64, AccountDailyLimit: math.MaxInt64, OperatorDailyLimit: math.MaxInt64, EvidenceThreshold: math.MaxInt64}
	values := []string{"9007199254740993", "1", "2"}
	for i, v := range values {
		o, e := s.Create(ctx, testCreator, grant(fmt.Sprintf("paging_%03d", i), v))
		require.NoError(t, e)
		_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
		require.NoError(t, e)
	}
	amount(t, s, "9007199254740996")
	first, e := s.Ledger(ctx, 10, req.LedgerSearch{Limit: 2})
	require.NoError(t, e)
	require.True(t, first.HasMore)
	require.Len(t, first.List, 2)
	second, e := s.Ledger(ctx, 10, req.LedgerSearch{Limit: 2, Cursor: first.NextCursor})
	require.NoError(t, e)
	require.False(t, second.HasMore)
	require.Len(t, second.List, 1)
	require.NotEqual(t, first.List[1].LedgerNo, second.List[0].LedgerNo)
	_, e = s.Ledger(ctx, 11, req.LedgerSearch{Cursor: first.NextCursor})
	require.ErrorIs(t, e, ErrInvalid)
	_, e = s.Ledger(ctx, 10, req.LedgerSearch{Limit: 101})
	require.ErrorIs(t, e, ErrInvalid)
	r := req.OrderSearch{}
	r.Page = math.MaxInt
	r.PageSize = 100
	_, e = s.Orders(ctx, testCreator, r)
	require.ErrorIs(t, e, ErrInvalid)
	page, e := s.Orders(ctx, testCreator, req.OrderSearch{})
	require.NoError(t, e)
	require.EqualValues(t, 3, page.Total)
}
func TestImmutableModelHooks(t *testing.T) {
	s := fixture(t)
	require.ErrorIs(t, s.DB.Model(&model.WalletLedger{}).Where("id=1").Update("reason", "changed").Error, model.ErrImmutable)
	require.ErrorIs(t, s.DB.Where("id=1").Delete(&model.WalletLedger{}).Error, model.ErrImmutable)
	require.ErrorIs(t, s.DB.Model(&model.WalletAuditLog{}).Where("id=1").Update("reason", "changed").Error, model.ErrImmutable)
	require.ErrorIs(t, s.DB.Where("id=1").Delete(&model.WalletAdjustmentOrder{}).Error, model.ErrImmutable)
}
func FuzzAmount(f *testing.F) {
	for _, s := range []string{"1", "0", "-1", "01", "1.0", "9223372036854775807", "9223372036854775808", "9007199254740993"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		v, e := parseAmount(input)
		if e == nil {
			require.Greater(t, v, int64(0))
			require.Equal(t, input, strconv.FormatInt(v, 10))
		}
	})
}

func TestReconciliationDetectsOrderWithoutLedger(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, e := s.Create(ctx, testCreator, grant("missing_ledger", "10"))
	require.NoError(t, e)
	require.NoError(t, s.DB.Model(&model.WalletAdjustmentOrder{}).Where("order_no = ?", o.OrderNo).Updates(map[string]any{"status": model.StatusCredited, "approved_by": 2, "ledger_no": "missing"}).Error)
	c, e := s.Check(ctx, testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, e)
	require.False(t, c.Consistent)
	require.Contains(t, c.Issues, "order_ledger:"+o.OrderNo)
	// 即使单号、金额和审核人吻合，流水落在另一钱包也必须报警。
	other := model.WalletAccount{AccountID: 11, CurrencyCode: "COIN", Status: "normal", AvailableBalance: 10, Version: 1}
	require.NoError(t, s.DB.Create(&other).Error)
	require.NoError(t, s.DB.Model(&model.WalletAdjustmentOrder{}).Where("order_no = ?", o.OrderNo).Updates(map[string]any{"approved_at": 123, "credited_at": 123}).Error)
	require.NoError(t, s.DB.Create(&model.WalletLedger{LedgerNo: "missing", WalletAccountID: other.ID, WalletVersion: 1, ChangeAmount: 10, BalanceAfter: 10, BusinessType: model.BusinessGrant, BusinessNo: o.OrderNo, OperatorID: 2, CreatedAt: 123, Reason: "cross-wallet fixture"}).Error)
	c, e = s.Check(ctx, testCreator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, e)
	require.False(t, c.Consistent)
	require.Contains(t, c.Issues, "order_ledger:"+o.OrderNo)
}

func TestPendingCapAndAccountLifecycle(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		_, e := s.Create(ctx, testCreator, grant(fmt.Sprintf("pending_%03d", i), "1"))
		require.NoError(t, e)
	}
	_, e := s.Create(ctx, testCreator, grant("pending_100", "1"))
	require.ErrorIs(t, e, ErrLimit)
	first, e := s.Create(ctx, testCreator, grant("pending_000", "1"))
	require.NoError(t, e)
	_, e = s.Reject(ctx, testReviewer, review(first.OrderNo))
	require.NoError(t, e)
	next, e := s.Create(ctx, testCreator, grant("pending_100", "1"))
	require.NoError(t, e)
	require.NoError(t, s.DB.Exec("UPDATE live_account SET status=4 WHERE id=10").Error)
	_, e = s.Approve(ctx, testReviewer, review(next.OrderNo))
	require.ErrorIs(t, e, ErrAccount)
	_, e = s.Reject(ctx, testReviewer, review(next.OrderNo))
	require.NoError(t, e)
	_, e = s.AdminBalance(ctx, testReviewer, req.AccountQuery{AccountNo: "10000010"})
	require.NoError(t, e)
	_, e = s.Balance(ctx, 10, "")
	require.ErrorIs(t, e, ErrAccount)
}
