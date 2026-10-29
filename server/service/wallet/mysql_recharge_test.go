package wallet_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/initialize"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	svc "tb_live_module/service/wallet"
)

func TestMySQLRechargeCrossAdminBusinessDedup(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	const n = 48
	for i := 0; i < n; i++ {
		require.NoError(t, s.DB.Exec("INSERT INTO sys_users(id,enable,authority_id) VALUES (?,1,888)", i+100).Error)
	}
	// 独立Service实例、不同管理员/请求ID/金额/用户，同一凭证只有一张申请。
	errs := parallel(n, func(i int) error {
		instance := &svc.Service{DB: s.DB}
		r := grant(fmt.Sprintf("cross_admin_%03d", i))
		r.ExternalOrderNo = "bank-cross-admin-001"
		if i%2 == 0 {
			r.ExternalOrderNo = strings.ToUpper(r.ExternalOrderNo)
			r.AccountNo = "10000011"
			r.Amount = "20"
		}
		_, err := instance.Create(ctx, svc.Actor{ID: uint64(i + 100), AuthorityID: 888, IP: "127.0.0.1"}, r)
		return err
	})
	success := 0
	for _, err := range errs {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, svc.ErrDuplicateSource)
		}
	}
	require.Equal(t, 1, success)
	assertCount(t, s.DB, &model.WalletAdjustmentOrder{}, 1)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 1)
	var o model.WalletAdjustmentOrder
	require.NoError(t, s.DB.Take(&o).Error)
	errs = parallel(n, func(i int) error { _, err := s.Approve(ctx, reviewer, review(o.OrderNo)); return err })
	for _, err := range errs {
		require.NoError(t, err)
	}
	assertCount(t, s.DB, &model.WalletLedger{}, 1)
	assertCount(t, s.DB, &model.WalletAccount{}, 1)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 3)
	b, err := s.Balance(ctx, o.AccountID, "")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprint(o.Amount), b.AvailableBalance)
	// 直接绕开Service也不能再次插入相同业务键。
	o.ID, o.OrderNo, o.RequestID, o.CreatedBy = 0, "wa_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "raw_duplicate", 1
	require.Error(t, s.DB.Create(&o).Error)
}

func TestMySQLRechargeConcurrentFirstOpenAndDisable(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	const n = 32
	orders := make([]string, n)
	for i := range orders {
		o, err := s.Create(ctx, creator, grant(fmt.Sprintf("first_open_%03d", i)))
		require.NoError(t, err)
		orders[i] = o.OrderNo
	}
	errs := parallel(n*2, func(i int) error {
		if i < n {
			_, err := s.Approve(ctx, reviewer, review(orders[i]))
			return err
		}
		check, err := s.Check(ctx, reviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 1000})
		if err == nil && !check.Consistent {
			return fmt.Errorf("concurrent opening reconciliation: %v", check.Issues)
		}
		return err
	})
	for _, err := range errs {
		require.NoError(t, err)
	}
	assertCount(t, s.DB, &model.WalletAccount{}, 1)
	assertCount(t, s.DB, &model.WalletLedger{}, n)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 2*n+1)
	var opened int64
	require.NoError(t, s.DB.Model(&model.WalletAuditLog{}).Where("action='wallet_open'").Count(&opened).Error)
	require.EqualValues(t, 1, opened)
	for i := range orders {
		o, err := s.Create(ctx, creator, grant(fmt.Sprintf("disable_race_%03d", i)))
		require.NoError(t, err)
		orders[i] = o.OrderNo
	}
	errs = parallel(n+1, func(i int) error {
		if i == n {
			_, err := s.SetStatus(ctx, reviewer, req.AccountStatus{AccountNo: "10000010", Status: model.AccountDisabled, Reason: "竞争中停用钱包"})
			return err
		}
		_, err := s.Approve(ctx, reviewer, review(orders[i]))
		return err
	})
	credited := n
	for i, err := range errs {
		if i == n {
			require.NoError(t, err)
		} else if err == nil {
			credited++
		} else {
			require.ErrorIs(t, err, svc.ErrDisabled)
		}
	}
	b, err := s.Balance(ctx, 10, "")
	require.NoError(t, err)
	require.Equal(t, model.AccountDisabled, b.Status)
	require.Equal(t, fmt.Sprint(credited*10), b.AvailableBalance)
	require.Equal(t, fmt.Sprint(credited), b.Version)
	assertCount(t, s.DB, &model.WalletLedger{}, int64(credited))
	check, err := s.Check(ctx, reviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 1000})
	require.NoError(t, err)
	require.True(t, check.Consistent, check.Issues)
}

func TestMySQLRechargeImmutableCoreAndLateFailure(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, creator, grant("immutable_order"))
	require.NoError(t, err)
	for _, col := range []string{"order_no", "request_id", "request_hash", "source_type", "external_order_no", "kind", "account_no", "currency_code", "reason", "evidence_ref", "original_order_no", "created_ip"} {
		expr := fmt.Sprintf("CONCAT(%s,' ')", col)
		// CHAR(64)尾随空格被MySQL折叠，不构成实际变化；改一个有效字符。
		if col == "request_hash" {
			expr = "CONCAT('x',SUBSTRING(request_hash,2))"
		}
		require.ErrorContains(t, s.DB.Exec(fmt.Sprintf("UPDATE wallet_adjustment_order SET %s=%s WHERE order_no=?", col, expr), o.OrderNo).Error, "wallet record is immutable", col)
	}
	for _, col := range []string{"id", "account_id", "amount", "created_by", "created_at"} {
		require.ErrorContains(t, s.DB.Exec(fmt.Sprintf("UPDATE wallet_adjustment_order SET %s=%s+1 WHERE order_no=?", col, col), o.OrderNo).Error, "wallet record is immutable", col)
	}
	require.NoError(t, s.DB.Exec("UPDATE wallet_adjustment_order SET amount=amount WHERE order_no=?", o.OrderNo).Error)
	// 分别让开户审计、订单状态更新、最终审核审计失败，涵盖事务的前中后段。
	for _, failure := range []struct{ name, table, event, condition string }{
		{"test_fail_open", "wallet_audit_log", "INSERT", "NEW.action='wallet_open'"},
		{"test_fail_order", "wallet_adjustment_order", "UPDATE", "NEW.status='credited'"},
		{"test_fail_approval", "wallet_audit_log", "INSERT", "NEW.action='approve_and_credit'"},
	} {
		require.NoError(t, s.DB.Exec(fmt.Sprintf("CREATE TRIGGER %s BEFORE %s ON %s FOR EACH ROW BEGIN IF %s THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'; END IF; END", failure.name, failure.event, failure.table, failure.condition)).Error)
		_, err = s.Approve(ctx, reviewer, review(o.OrderNo))
		require.Error(t, err)
		assertCount(t, s.DB, &model.WalletAccount{}, 0)
		assertCount(t, s.DB, &model.WalletLedger{}, 0)
		assertCount(t, s.DB, &model.WalletDailyQuota{}, 0)
		assertCount(t, s.DB, &model.WalletAuditLog{}, 1)
		var stored model.WalletAdjustmentOrder
		require.NoError(t, s.DB.Where("order_no=?", o.OrderNo).Take(&stored).Error)
		require.Equal(t, model.StatusPending, stored.Status)
		require.Empty(t, stored.LedgerNo)
		require.NoError(t, s.DB.Exec("DROP TRIGGER "+failure.name).Error)
	}
	_, err = s.Approve(ctx, reviewer, review(o.OrderNo))
	require.NoError(t, err)
	assertCount(t, s.DB, &model.WalletLedger{}, 1)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 3)
}

var errCommitResponseLost = errors.New("test: commit response lost after durable MySQL commit")

type commitLossPool struct {
	*sql.DB
	armed  atomic.Bool
	failed atomic.Int32
}
type commitLossTx struct {
	*sql.Tx
	pool *commitLossPool
}

func (p *commitLossPool) BeginTx(ctx context.Context, opt *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opt)
	if err != nil {
		return nil, err
	}
	return &commitLossTx{Tx: tx, pool: p}, nil
}
func (t *commitLossTx) Commit() error {
	if err := t.Tx.Commit(); err != nil {
		return err
	}
	if t.pool.armed.Swap(false) {
		t.pool.failed.Add(1)
		return errCommitResponseLost
	}
	return nil
}

func TestMySQLRechargeUnknownCommitOutcome(t *testing.T) {
	s := mysqlFixture(t)
	pool, err := s.DB.DB()
	require.NoError(t, err)
	loss := &commitLossPool{DB: pool}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: loss}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	client := &svc.Service{DB: db}
	ctx := context.Background()
	loss.armed.Store(true)
	_, err = client.Create(ctx, creator, grant("lost_commit_create"))
	require.ErrorIs(t, err, errCommitResponseLost) // 不能把未知提交结果内部盲目重放。
	require.EqualValues(t, 1, loss.failed.Load())
	assertCount(t, s.DB, &model.WalletAdjustmentOrder{}, 1)
	confirmed, err := s.Create(ctx, creator, grant("lost_commit_create"))
	require.NoError(t, err)
	loss.armed.Store(true)
	_, err = client.Approve(ctx, reviewer, review(confirmed.OrderNo))
	require.ErrorIs(t, err, errCommitResponseLost)
	require.EqualValues(t, 2, loss.failed.Load())
	// 新实例查询/重试已提交事实；换管理员、请求ID仍不能再次充值。
	fresh := &svc.Service{DB: s.DB}
	_, err = fresh.Approve(ctx, reviewer, review(confirmed.OrderNo))
	require.NoError(t, err)
	r := grant("lost_commit_create")
	r.RequestID = "lost_retry_new_key"
	_, err = fresh.Create(ctx, reviewer, r)
	require.ErrorIs(t, err, svc.ErrDuplicateSource)
	assertCount(t, s.DB, &model.WalletAdjustmentOrder{}, 1)
	assertCount(t, s.DB, &model.WalletLedger{}, 1)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 3)
	b, err := fresh.Balance(ctx, 10, "")
	require.NoError(t, err)
	require.Equal(t, "10", b.AvailableBalance)
}

func TestMySQLWalletStrictSchemaValidation(t *testing.T) {
	for _, change := range []struct{ sql, column string }{
		{"ALTER TABLE wallet_adjustment_order MODIFY amount INT NOT NULL", "amount"},
		{"ALTER TABLE wallet_account MODIFY created_at BIGINT NULL", "created_at"},
		{"ALTER TABLE wallet_account MODIFY version BIGINT NOT NULL DEFAULT 1", "version"},
		{"ALTER TABLE wallet_adjustment_order MODIFY evidence_ref VARCHAR(499) NOT NULL DEFAULT ''", "evidence_ref"},
		{"ALTER TABLE wallet_adjustment_order MODIFY external_order_no VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT ''", "external_order_no"},
		{"ALTER TABLE wallet_adjustment_order MODIFY created_by BIGINT NOT NULL", "created_by"},
		{"ALTER TABLE wallet_account ALTER CHECK ck_wallet_account_balance NOT ENFORCED", "ck_wallet_account_balance"},
		{"ALTER TABLE wallet_account DROP CHECK ck_wallet_account_balance, ADD CONSTRAINT ck_wallet_account_balance CHECK(1=1)", "ck_wallet_account_balance"},
		{"ALTER TABLE wallet_adjustment_order DROP CHECK ck_wallet_order_status", "ck_wallet_order_status"},
		{"ALTER TABLE wallet_adjustment_order DROP INDEX uk_wallet_order_source, ADD INDEX uk_wallet_order_source(source_type,external_order_no)", "uk_wallet_order_source"},
		{"ALTER TABLE wallet_ledger DROP INDEX uk_wallet_ledger_business, ADD UNIQUE INDEX uk_wallet_ledger_business(business_no,business_type)", "uk_wallet_ledger_business"},
		{"ALTER TABLE wallet_ledger DROP INDEX uk_wallet_ledger_business, ADD UNIQUE INDEX uk_wallet_ledger_business(business_type,business_no(10))", "uk_wallet_ledger_business"},
		{"ALTER TABLE wallet_ledger ALTER INDEX uk_wallet_ledger_business INVISIBLE", "uk_wallet_ledger_business"},
		{"ALTER TABLE wallet_daily_quota DROP PRIMARY KEY, ADD PRIMARY KEY(id,day)", "wallet_daily_quota"},
	} {
		t.Run(change.column+fmt.Sprint(len(change.sql)), func(t *testing.T) {
			s := mysqlFixture(t)
			require.NoError(t, s.DB.Exec(change.sql).Error)
			require.ErrorContains(t, initialize.ValidateWalletSchema(s.DB), change.column)
		})
	}
	s := mysqlFixture(t)
	require.NoError(t, s.DB.Exec("DROP TRIGGER trg_wallet_order_core_immutable").Error)
	require.NoError(t, s.DB.Exec("CREATE TRIGGER trg_wallet_order_core_immutable BEFORE UPDATE ON wallet_adjustment_order FOR EACH ROW SET @wallet_noop=1").Error)
	require.ErrorContains(t, initialize.ValidateWalletSchema(s.DB), "trg_wallet_order_core_immutable")
}

func TestMySQLWalletLegacyMigration(t *testing.T) {
	s := mysqlFixture(t)
	// 按旧模型全部字段创建无source列的真实历史表，验证非空表升级与重跑。
	require.NoError(t, s.DB.Exec("DROP TABLE wallet_adjustment_order").Error)
	st := &gorm.Statement{DB: s.DB}
	require.NoError(t, st.Parse(&model.WalletAdjustmentOrder{}))
	var definitions []string
	for _, field := range st.Schema.Fields {
		if field.DBName == "source_type" || field.DBName == "external_order_no" {
			continue
		}
		definitions = append(definitions, "`"+field.DBName+"` "+s.DB.Migrator().FullDataTypeOf(field).SQL)
	}
	definitions = append(definitions, "PRIMARY KEY(id)")
	require.NoError(t, s.DB.Exec("CREATE TABLE wallet_adjustment_order ("+strings.Join(definitions, ",")+") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin").Error)
	for i, kind := range []string{model.KindGrant, model.KindGrant, model.KindReverse} {
		o := model.WalletAdjustmentOrder{OrderNo: fmt.Sprintf("wa_%032x", i+1), RequestID: fmt.Sprintf("legacy_%08d", i), RequestHash: strings.Repeat("a", 64), Kind: kind, AccountID: 10, AccountNo: "10000010", CurrencyCode: "COIN", Amount: 10, Reason: "历史申请记录", Status: model.StatusPending, CreatedBy: 1, CreatedIP: "127.0.0.1"}
		require.NoError(t, s.DB.Omit("SourceType", "ExternalOrderNo").Create(&o).Error)
	}
	require.NoError(t, initialize.MigrateWallet(s.DB))
	require.NoError(t, initialize.MigrateWallet(s.DB))
	var migrated []model.WalletAdjustmentOrder
	require.NoError(t, s.DB.Order("id").Find(&migrated).Error)
	require.Len(t, migrated, 3)
	for i, o := range migrated {
		require.Equal(t, o.OrderNo, o.ExternalOrderNo)
		if i == 2 {
			require.Equal(t, model.SourceReversal, o.SourceType)
		} else {
			require.Equal(t, model.SourceLegacy, o.SourceType)
		}
	}
	_, err := s.Approve(context.Background(), reviewer, review(migrated[0].OrderNo))
	require.ErrorIs(t, err, svc.ErrLegacyOrder)
	_, err = s.Reject(context.Background(), reviewer, review(migrated[0].OrderNo))
	require.NoError(t, err)
	assertCount(t, s.DB, &model.WalletLedger{}, 0)
	assertCount(t, s.DB, &model.WalletAccount{}, 0)
	// 新凭证仍可正常创建，旧单不能通过SQL补造凭证后直接审核。
	_, err = s.Create(context.Background(), creator, grant("after_migration"))
	require.NoError(t, err)
	require.Error(t, s.DB.Exec("UPDATE wallet_adjustment_order SET source_type='manual', external_order_no='FABRICATED' WHERE id=?", migrated[1].ID).Error)
}

func TestMySQLRechargeCreationAndAccountCancellationRace(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, creator, grant("cancel_race"))
	require.NoError(t, err)
	errs := parallel(2, func(i int) error {
		if i == 0 {
			_, e := s.Approve(ctx, reviewer, review(o.OrderNo))
			return e
		}
		return s.DB.Exec("UPDATE live_account SET status=2, deleted_at=NOW(3) WHERE id=10").Error
	})
	require.NoError(t, errs[1])
	if errs[0] != nil {
		require.True(t, errors.Is(errs[0], svc.ErrAccount))
		assertCount(t, s.DB, &model.WalletAccount{}, 0)
	} else {
		assertCount(t, s.DB, &model.WalletAccount{}, 1)
		assertCount(t, s.DB, &model.WalletLedger{}, 1)
	}
	// 注销完成之后的新请求和状态操作永远不会重新开户。
	_, err = s.Create(ctx, creator, grant("after_cancel"))
	require.ErrorIs(t, err, svc.ErrAccount)
	_, err = s.SetStatus(ctx, reviewer, req.AccountStatus{AccountNo: "10000010", Status: model.AccountNormal, Reason: "注销后启用测试"})
	require.ErrorIs(t, err, svc.ErrAccount)
}

func TestMySQLRechargeCreationAndStatusAuditRollback(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	require.NoError(t, s.DB.Exec("CREATE TRIGGER test_create_audit_fail BEFORE INSERT ON wallet_audit_log FOR EACH ROW BEGIN IF NEW.action='create' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='create audit failure'; END IF; END").Error)
	_, err := s.Create(ctx, creator, grant("create_rollback"))
	require.Error(t, err)
	assertCount(t, s.DB, &model.WalletAdjustmentOrder{}, 0)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 0)
	require.NoError(t, s.DB.Exec("DROP TRIGGER test_create_audit_fail").Error)
	// 失败的事务没有占用资金号，另一管理员可以按同一真实凭证正常申请。
	o, err := s.Create(ctx, reviewer, grant("create_rollback"))
	require.NoError(t, err)
	_, err = s.Approve(ctx, creator, review(o.OrderNo))
	require.NoError(t, err)
	require.NoError(t, s.DB.Exec("CREATE TRIGGER test_status_audit_fail BEFORE INSERT ON wallet_audit_log FOR EACH ROW BEGIN IF NEW.action='status_disabled' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='status audit failure'; END IF; END").Error)
	r := req.AccountStatus{AccountNo: "10000010", Status: model.AccountDisabled, Reason: "测试停用审计失败"}
	_, err = s.SetStatus(ctx, reviewer, r)
	require.Error(t, err)
	b, err := s.Balance(ctx, 10, "")
	require.NoError(t, err)
	require.Equal(t, model.AccountNormal, b.Status)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 3)
	require.NoError(t, s.DB.Exec("DROP TRIGGER test_status_audit_fail").Error)
	_, err = s.SetStatus(ctx, reviewer, r)
	require.NoError(t, err)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 4)
}
