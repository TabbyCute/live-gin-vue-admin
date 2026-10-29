package wallet_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tb_live_module/initialize"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
)

type snapshotProbe struct{}

// 暂停真实SELECT后的读事务，并在它结束前完成审核，验证不持资金锁和快照一致性。
func TestMySQLWalletReadSnapshotDoesNotBlockCredit(t *testing.T) {
	for _, scenario := range []string{"first_open", "existing_wallet", "order_detail"} {
		t.Run(scenario, func(t *testing.T) {
			s := mysqlFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if scenario == "existing_wallet" {
				first, err := s.Create(ctx, creator, grant("snapshot_first"))
				require.NoError(t, err)
				_, err = s.Approve(ctx, reviewer, review(first.OrderNo))
				require.NoError(t, err)
			}
			o, err := s.Create(ctx, creator, grant("snapshot_pending"))
			require.NoError(t, err)
			paused, resume := make(chan struct{}), make(chan struct{})
			var pauseOnce, resumeOnce sync.Once
			defer resumeOnce.Do(func() { close(resume) })
			table := "wallet_account"
			if scenario == "order_detail" {
				table = "wallet_adjustment_order"
			}
			require.NoError(t, s.DB.Callback().Query().After("gorm:query").Register("test_snapshot_pause", func(tx *gorm.DB) {
				if tx.Statement.Context.Value(snapshotProbe{}) != true || tx.Statement.Table != table {
					return
				}
				pauseOnce.Do(func() {
					close(paused)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}))
			t.Cleanup(func() { s.DB.Callback().Query().Remove("test_snapshot_pause") })
			done := make(chan error, 1)
			go func() {
				readCtx := context.WithValue(ctx, snapshotProbe{}, true)
				if scenario == "order_detail" {
					detail, e := s.Detail(readCtx, reviewer, o.OrderNo)
					if e == nil && (detail.Order.Status != model.StatusPending || detail.Ledger != nil || len(detail.Audit) != 1) {
						e = fmt.Errorf("mixed order snapshot: %+v", detail)
					}
					done <- e
					return
				}
				check, e := s.Check(readCtx, reviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
				version := "0"
				if scenario == "existing_wallet" {
					version = "1"
				}
				if e == nil && (!check.Consistent || check.SnapshotVersion != version) {
					e = fmt.Errorf("mixed check snapshot: %+v", check)
				}
				done <- e
			}()
			select {
			case <-paused:
			case <-ctx.Done():
				t.Fatal("snapshot did not start")
			}
			writeCtx, stop := context.WithTimeout(ctx, 2*time.Second)
			_, err = s.Approve(writeCtx, reviewer, review(o.OrderNo))
			stop()
			resumeOnce.Do(func() { close(resume) })
			require.NoError(t, err, "read transaction must not block approval")
			select {
			case err = <-done:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("snapshot did not finish")
			}
			check, err := s.Check(ctx, reviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
			require.NoError(t, err)
			require.True(t, check.Consistent, check.Issues)
		})
	}
}

func TestMySQLWalletConcurrentMigrationAndValidationCancellation(t *testing.T) {
	s := mysqlFixture(t)
	// 只删除fixture生成的隔离测试库内钱包表，模拟多个进程同时首次部署。
	for _, m := range model.Models() {
		require.NoError(t, s.DB.Migrator().DropTable(m))
	}
	for _, err := range parallel(8, func(int) error { return initialize.MigrateWallet(s.DB) }) {
		require.NoError(t, err)
	}
	require.NoError(t, initialize.ValidateWalletSchema(s.DB))
	// DDL之后结构校验失败也必须释放锁，第二次连接可以重试，且不自动吞掉多余列。
	require.NoError(t, s.DB.Exec("ALTER TABLE wallet_audit_log ADD COLUMN migration_probe INT").Error)
	require.ErrorContains(t, initialize.MigrateWallet(s.DB), "wallet column set incompatible")
	require.NoError(t, s.DB.Exec("ALTER TABLE wallet_audit_log DROP COLUMN migration_probe").Error)
	require.NoError(t, initialize.MigrateWallet(s.DB))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, initialize.ValidateWalletSchema(s.DB.WithContext(ctx)), context.Canceled)
	require.ErrorIs(t, initialize.MigrateWallet(s.DB.WithContext(ctx)), context.Canceled)
	// 失败路径必须释放连接锁，后续迁移可以继续执行。
	require.NoError(t, initialize.MigrateWallet(s.DB))
}

func TestMySQLWalletSchemaQueryBudgetAndErrors(t *testing.T) {
	s := mysqlFixture(t)
	queries := 0
	failChecks := false
	require.NoError(t, s.DB.Callback().Row().Before("gorm:row").Register("test_schema_query", func(tx *gorm.DB) {
		queries++
		if failChecks && strings.Contains(tx.Statement.SQL.String(), "TABLE_CONSTRAINTS") {
			tx.AddError(context.DeadlineExceeded)
		}
	}))
	t.Cleanup(func() { s.DB.Callback().Row().Remove("test_schema_query") })
	require.NoError(t, initialize.ValidateWalletSchema(s.DB))
	require.Equal(t, 6, queries, "validation must stay bounded independent of the number of fields/checks")
	failChecks = true
	err := initialize.ValidateWalletSchema(s.DB)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "query wallet checks")
	require.NotContains(t, err.Error(), "missing")
}
