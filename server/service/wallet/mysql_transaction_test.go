package wallet_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tb_live_module/initialize"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	svc "tb_live_module/service/wallet"
)

func movement(no, amount string) svc.MovementInput {
	return svc.MovementInput{AccountID: 10, Amount: amount, BusinessType: "test_shop", BusinessNo: no, ActorType: "user", ActorID: "10", ActorIP: "127.0.0.1", TraceID: "mysql_trace", Reason: "服务端核价测试"}
}
func fundMovement(t *testing.T, s *svc.Service, amount string) {
	r := grant("mysql_movement_seed")
	r.Amount = amount
	o, e := s.Create(context.Background(), creator, r)
	require.NoError(t, e)
	_, e = s.Approve(context.Background(), reviewer, review(o.OrderNo))
	require.NoError(t, e)
}
func TestMySQLMovementConcurrentDebitRefund(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	fundMovement(t, s, "100")
	for _, e := range parallel(48, func(int) error { _, e := s.Debit(ctx, movement("same-order", "20")); return e }) {
		require.NoError(t, e)
	}
	assertCount(t, s.DB, &model.WalletTransaction{}, 1)
	errs := parallel(40, func(i int) error { _, e := s.Debit(ctx, movement(fmt.Sprintf("compete-%d", i), "10")); return e })
	success := 0
	for _, e := range errs {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, svc.ErrInsufficient)
		}
	}
	require.Equal(t, 8, success)
	d, e := s.Debit(ctx, movement("same-order", "20"))
	require.NoError(t, e)
	errs = parallel(40, func(i int) error {
		r := movement(fmt.Sprintf("refund-%d", i), "1")
		r.OriginalNo = d.TransactionNo
		_, e := s.Refund(ctx, r)
		return e
	})
	success = 0
	for _, e := range errs {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, svc.ErrRefundLimit)
		}
	}
	require.Equal(t, 20, success)
	b, e := s.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "20", b.AvailableBalance)
	c, e := s.Check(ctx, creator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 1000})
	require.NoError(t, e)
	require.True(t, c.Consistent, c.Issues)
	var refunds []model.WalletTransaction
	require.NoError(t, s.DB.Where("kind='refund'").Find(&refunds).Error)
	r := movement(refunds[0].BusinessNo, "1")
	r.OriginalNo = d.TransactionNo
	for _, e := range parallel(32, func(int) error { _, e := s.Refund(ctx, r); return e }) {
		require.NoError(t, e)
	}
	assertCount(t, s.DB, &model.WalletTransaction{}, 29)
}
func TestMySQLMovementFailureImmutableAndUnknownCommit(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	fundMovement(t, s, "100")
	for _, table := range []string{"wallet_transaction", "wallet_ledger", "wallet_audit_log"} {
		require.NoError(t, s.DB.Exec("CREATE TRIGGER test_fail_move BEFORE INSERT ON "+table+" FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'").Error)
		_, e := s.Debit(ctx, movement("rollback", "40"))
		require.Error(t, e)
		b, e := s.Balance(ctx, 10, "")
		require.NoError(t, e)
		require.Equal(t, "100", b.AvailableBalance)
		assertCount(t, s.DB, &model.WalletTransaction{}, 0)
		require.NoError(t, s.DB.Exec("DROP TRIGGER test_fail_move").Error)
	}
	d, e := s.Debit(ctx, movement("rollback", "40"))
	require.NoError(t, e)
	require.NoError(t, s.DB.Exec("CREATE TRIGGER test_fail_refund BEFORE INSERT ON wallet_audit_log FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected refund failure'").Error)
	r := movement("refund-rollback", "40")
	r.OriginalNo = d.TransactionNo
	_, e = s.Refund(ctx, r)
	require.Error(t, e)
	tr, e := s.Trace(ctx, creator, req.TraceQuery{No: d.TransactionNo})
	require.NoError(t, e)
	require.Equal(t, "0", tr.Transaction.RefundedAmount)
	require.NoError(t, s.DB.Exec("DROP TRIGGER test_fail_refund").Error)
	for _, col := range []string{"transaction_no", "business_no", "business_type", "account_no", "reason", "actor_ip", "actor_type", "actor_id", "trace_id", "original_no", "ledger_no", "currency_code"} {
		require.Error(t, s.DB.Exec("UPDATE wallet_transaction SET "+col+"=CONCAT("+col+",' ') WHERE transaction_no=?", d.TransactionNo).Error, col)
	}
	for _, col := range []string{"id", "amount", "account_id", "created_at"} {
		require.Error(t, s.DB.Exec("UPDATE wallet_transaction SET "+col+"="+col+"+1 WHERE transaction_no=?", d.TransactionNo).Error, col)
	}
	require.Error(t, s.DB.Exec("DELETE FROM wallet_transaction").Error)
	require.Error(t, s.DB.Exec("UPDATE wallet_transaction SET refunded_amount=41").Error)
	pool, e := s.DB.DB()
	require.NoError(t, e)
	loss := &commitLossPool{DB: pool}
	db := s.DB.Session(&gorm.Session{NewDB: true})
	db.Statement.ConnPool = loss
	client := &svc.Service{DB: db}
	loss.armed.Store(true)
	_, e = client.Refund(ctx, r)
	require.ErrorIs(t, e, errCommitResponseLost)
	_, e = s.Refund(ctx, r)
	require.NoError(t, e)
	assertCount(t, s.DB, &model.WalletTransaction{}, 2)
	b, e := s.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "100", b.AvailableBalance)
}
func TestMySQLMovementCrossWalletAndAtomicity(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	fundMovement(t, s, "100")
	r := grant("another_wallet")
	r.AccountNo = "10000011"
	r.Amount = "100"
	o, e := s.Create(ctx, creator, r)
	require.NoError(t, e)
	_, e = s.Approve(ctx, reviewer, review(o.OrderNo))
	require.NoError(t, e)
	errs := parallel(2, func(i int) error {
		r := movement("global-business", "10")
		r.AccountID += uint64(i)
		r.ActorID = fmt.Sprint(r.AccountID)
		_, e := s.Debit(ctx, r)
		return e
	})
	success := 0
	for _, e := range errs {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, svc.ErrConflict)
		}
	}
	require.Equal(t, 1, success)
	failure := errors.New("business failure")
	e = s.DB.Transaction(func(tx *gorm.DB) error {
		_, e := s.DebitInTx(tx, movement("atomic-business", "10"))
		if e != nil {
			return e
		}
		return failure
	})
	require.ErrorIs(t, e, failure)
	assertCount(t, s.DB, &model.WalletTransaction{}, 1)
	var original model.WalletTransaction
	require.NoError(t, s.DB.Take(&original).Error)
	wrong := movement("wrong-user-refund", "1")
	wrong.AccountID = 21 - original.AccountID
	wrong.ActorID = fmt.Sprint(wrong.AccountID)
	wrong.OriginalNo = original.TransactionNo
	_, e = s.Refund(ctx, wrong)
	require.ErrorIs(t, e, svc.ErrConflict)
	for _, no := range []string{"10000010", "10000011"} {
		c, e := s.Check(ctx, creator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: no}})
		require.NoError(t, e)
		require.True(t, c.Consistent, c.Issues)
	}
}

func TestMySQLMovementIncrementalMigration(t *testing.T) {
	s := mysqlFixture(t)
	fundMovement(t, s, "100")
	require.NoError(t, s.DB.Migrator().DropTable(&model.WalletTransaction{}))
	require.NoError(t, initialize.MigrateWalletTransactions(s.DB))
	require.NoError(t, initialize.MigrateWalletTransactions(s.DB))
	assertCount(t, s.DB, &model.WalletLedger{}, 1)
	_, e := s.Debit(context.Background(), movement("new-table-debit", "1"))
	require.NoError(t, e)
}

func TestMySQLMovementInTxOldSnapshot(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	fundMovement(t, s, "100")
	// 业务方先读订单/钱包，默认RR建立快照后，另一个请求已完成记账。
	tx := s.DB.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	var old model.WalletAccount
	require.NoError(t, tx.Take(&old).Error)
	d, e := s.Debit(ctx, movement("snapshot-debit", "50"))
	require.NoError(t, e)
	r := movement("snapshot-refund", "10")
	r.OriginalNo = d.TransactionNo
	first, e := s.Refund(ctx, r)
	require.NoError(t, e)
	replayed, e := s.RefundInTx(tx, r)
	require.NoError(t, e)
	require.Equal(t, first, replayed)
	r.BusinessNo, r.Amount = "snapshot-remainder", "40"
	_, e = s.RefundInTx(tx, r)
	require.NoError(t, e)
	require.NoError(t, tx.Commit().Error)
	b, e := s.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "100", b.AvailableBalance)
	// 第二个旧快照必须拒绝超额退款，不能依靠碰巧触发尾链异常。
	tx = s.DB.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	defer tx.Rollback()
	_, e = s.DebitInTx(tx, movement("snapshot-new-debit", "1"))
	require.NoError(t, e)
	r.BusinessNo, r.Amount = "snapshot-excess", "1"
	_, e = s.RefundInTx(tx, r)
	require.ErrorIs(t, e, svc.ErrRefundLimit)
	require.NoError(t, tx.Rollback().Error)
	assertCount(t, s.DB, &model.WalletTransaction{}, 3)
}

func TestMySQLMovementMixedStatusAndCancellation(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	fundMovement(t, s, "1000")
	d, e := s.Debit(ctx, movement("mixed-original", "200"))
	require.NoError(t, e)
	for _, e := range parallel(64, func(i int) error {
		r := movement(fmt.Sprintf("mixed-%d", i), "1")
		if i%2 == 0 {
			_, e := s.Debit(ctx, r)
			return e
		}
		r.OriginalNo = d.TransactionNo
		_, e := s.Refund(ctx, r)
		return e
	}) {
		require.NoError(t, e)
	}
	b, e := s.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "800", b.AvailableBalance)
	// 等待钱包锁期间取消必须退出，不能随后后台继续扣钱。
	blocker := s.DB.Begin()
	require.NoError(t, blocker.Error)
	defer blocker.Rollback()
	require.NoError(t, blocker.Exec("SELECT id FROM wallet_account WHERE account_id=10 FOR UPDATE").Error)
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, e = s.Debit(short, movement("cancelled-debit", "10"))
	require.ErrorIs(t, e, context.DeadlineExceeded)
	require.NoError(t, blocker.Rollback().Error)
	assertCount(t, s.DB, &model.WalletTransaction{}, 65)
	// 停用与扣款争用同一行锁，只允许停用提交前的合法扣款。
	errs := parallel(33, func(i int) error {
		if i == 32 {
			_, e := s.SetStatus(ctx, reviewer, req.AccountStatus{AccountNo: "10000010", Status: model.AccountDisabled, Reason: "并发停用测试"})
			return e
		}
		_, e := s.Debit(ctx, movement(fmt.Sprintf("disable-%d", i), "1"))
		return e
	})
	require.NoError(t, errs[32])
	for _, e := range errs[:32] {
		if e != nil {
			require.ErrorIs(t, e, svc.ErrDisabled)
		}
	}
	_, e = s.Debit(ctx, movement("after-disabled", "1"))
	require.ErrorIs(t, e, svc.ErrDisabled)
	r := movement("refund-after-disabled", "1")
	r.OriginalNo = d.TransactionNo
	_, e = s.Refund(ctx, r)
	require.ErrorIs(t, e, svc.ErrDisabled)
	_, e = s.Debit(ctx, movement("mixed-original", "200"))
	require.NoError(t, e, "成功交易查询式重试不因后续停用失效")
	c, e := s.Check(ctx, creator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 1000})
	require.NoError(t, e)
	require.True(t, c.Consistent, c.Issues)
}

func TestMySQLMovementQueryPlans(t *testing.T) {
	s := mysqlFixture(t)
	// 仅用于查询计划选择性的隔离数据，不伪装成已对账的业务事实。
	rows := make([]model.WalletTransaction, 10000)
	for i := range rows {
		rows[i] = model.WalletTransaction{TransactionNo: fmt.Sprintf("plan_%d", i), Kind: model.TransactionRefund, BusinessType: "test_shop", BusinessNo: fmt.Sprintf("plan_order_%d", i), AccountID: uint64(20 + i%100), AccountNo: fmt.Sprint(20 + i%100), CurrencyCode: "COIN", Amount: 1, OriginalNo: fmt.Sprintf("parent_%d", i%100), LedgerNo: fmt.Sprintf("ledger_%d", i), RequestHash: "plan", ActorType: "system", ActorID: "test", ActorIP: "127.0.0.1", TraceID: "plan", Reason: "query plan fixture"}
	}
	require.NoError(t, s.DB.CreateInBatches(rows, 500).Error)
	require.NoError(t, s.DB.Exec("ANALYZE TABLE wallet_transaction").Error)
	for _, query := range []string{
		"SELECT * FROM wallet_transaction WHERE kind='refund' AND business_type='test_shop' AND business_no='plan_order_50'",
		"SELECT * FROM wallet_transaction WHERE original_no='parent_50' AND id>100 ORDER BY id LIMIT 51",
		"SELECT * FROM wallet_transaction WHERE account_id=70 AND currency_code='COIN' AND id>100 ORDER BY id LIMIT 101",
		"SELECT SUM(amount),COUNT(*) FROM wallet_transaction WHERE original_no='parent_50'",
	} {
		var plan []map[string]any
		require.NoError(t, s.DB.Raw("EXPLAIN "+query).Scan(&plan).Error)
		for _, r := range plan {
			require.NotEqual(t, "ALL", fmt.Sprint(r["type"]))
			require.NotContains(t, fmt.Sprint(r["Extra"]), "filesort")
		}
		t.Logf("EXPLAIN %s => %v", query, plan)
	}
}
