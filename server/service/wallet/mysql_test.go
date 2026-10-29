package wallet_test

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/config"
	"tb_live_module/initialize"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	res "tb_live_module/model/wallet/response"
	svc "tb_live_module/service/wallet"
)

var creator = svc.Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}
var reviewer = svc.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}

func mysqlFixture(t *testing.T) *svc.Service {
	t.Helper()
	dsn := os.Getenv("WALLET_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires isolated local MySQL; see docs/wallet/testing.md")
	}
	c, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.Equal(t, "tcp", c.Net)
	require.Equal(t, "127.0.0.1:13326", c.Addr)
	require.True(t, strings.HasPrefix(c.DBName, "wallet_test_"))
	c.DBName = ""
	c.ParseTime = true
	admin, e := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	dbname := fmt.Sprintf("wallet_test_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE DATABASE "+dbname+" CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
	t.Cleanup(func() {
		require.NoError(t, admin.Exec("DROP DATABASE "+dbname).Error)
		pool, _ := admin.DB()
		pool.Close()
	})
	c.DBName = dbname
	db, e := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(24)
	pool.SetMaxIdleConns(24)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.Exec("CREATE TABLE sys_users(id BIGINT UNSIGNED PRIMARY KEY, nick_name VARCHAR(64), enable INT, authority_id BIGINT UNSIGNED, deleted_at datetime(3)) ENGINE=InnoDB").Error)
	require.NoError(t, db.Exec("CREATE TABLE sys_user_authority(sys_user_id BIGINT UNSIGNED,sys_authority_authority_id BIGINT UNSIGNED)").Error)
	require.NoError(t, db.Exec("CREATE TABLE live_account(id BIGINT UNSIGNED PRIMARY KEY,account_no VARCHAR(32) UNIQUE,phone VARCHAR(20) UNIQUE,status INT,deleted_at datetime(3)) ENGINE=InnoDB").Error)
	require.NoError(t, db.Exec("INSERT INTO sys_users(id,nick_name,enable,authority_id) VALUES (1,'Admin',1,888),(2,'Reviewer',1,888),(3,'Creator',1,888)").Error)
	require.NoError(t, db.Exec("INSERT INTO live_account(id,account_no,phone,status) VALUES (10,'10000010','+60100000010',1),(11,'10000011','+60100000011',1)").Error)
	require.NoError(t, initialize.MigrateWallet(db))
	require.NoError(t, initialize.MigrateWallet(db))
	return &svc.Service{DB: db}
}
func grant(key string) req.CreateAdjustment {
	return req.CreateAdjustment{RequestID: key, ExternalOrderNo: key, AccountNo: "10000010", Amount: "10", Reason: "并发测试充值", EvidenceRef: "upload:mysql-proof"}
}
func review(no string) req.ReviewAdjustment {
	return req.ReviewAdjustment{OrderNo: no, Reason: "核对并发测试"}
}

func TestMySQLWalletAdminLookupAndOperatorDisplay(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, creator, grant("mysql_phone_lookup"))
	require.NoError(t, err)
	_, err = s.Approve(ctx, reviewer, review(o.OrderNo))
	require.NoError(t, err)

	balance, err := s.AdminBalance(ctx, creator, req.AccountQuery{Phone: "+60100000010"})
	require.NoError(t, err)
	require.Equal(t, "10000010", balance.AccountNo)
	require.Equal(t, "+60100000010", balance.Phone)
	require.Equal(t, "10", balance.AvailableBalance)
	_, err = s.AdminBalance(ctx, creator, req.AccountQuery{AccountNo: "10000010", Phone: "+60100000011"})
	require.ErrorIs(t, err, svc.ErrAccount)

	// 已软删除的管理员仍需在历史账务中显示昵称和ID，不能丢失审计主体。
	require.NoError(t, s.DB.Exec("UPDATE sys_users SET deleted_at = NOW(3) WHERE id = 2").Error)
	ledger, err := s.AdminLedger(ctx, creator, req.LedgerSearch{AccountQuery: req.AccountQuery{Phone: "+60100000010"}})
	require.NoError(t, err)
	require.Len(t, ledger.List, 1)
	require.Equal(t, "Reviewer [ 2 ]", ledger.List[0].OperatorDisplay)
	page, err := s.Orders(ctx, creator, req.OrderSearch{Phone: "+60100000010"})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	orders := page.List.([]res.Order)
	require.Equal(t, "Admin [ 1 ]", orders[0].CreatedByDisplay)
	require.Equal(t, "Reviewer [ 2 ]", orders[0].ApprovedByDisplay)
	detail, err := s.Detail(ctx, creator, o.OrderNo)
	require.NoError(t, err)
	require.Equal(t, "Reviewer [ 2 ]", detail.Ledger.OperatorDisplay)
	require.Equal(t, "Reviewer [ 2 ]", detail.Audit[len(detail.Audit)-1].OperatorDisplay)
	check, err := s.Check(ctx, creator, req.CheckSearch{AccountQuery: req.AccountQuery{Phone: "+60100000010"}})
	require.NoError(t, err)
	require.True(t, check.Consistent, check.Issues)
}
func parallel(n int, fn func(int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; errs[i] = fn(i) }(i)
	}
	close(start)
	wg.Wait()
	return errs
}
func assertCount(t *testing.T, db *gorm.DB, m any, n int64) {
	t.Helper()
	var actual int64
	require.NoError(t, db.Model(m).Count(&actual).Error)
	require.Equal(t, n, actual)
}
func TestMySQLWalletConcurrency(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	errs := parallel(24, func(i int) error { _, e := s.Create(ctx, creator, grant("same_key_001")); return e })
	for _, e := range errs {
		require.NoError(t, e)
	}
	assertCount(t, s.DB, &model.WalletAdjustmentOrder{}, 1)
	o, e := s.Create(ctx, creator, grant("same_key_001"))
	require.NoError(t, e)
	// 固定顶级超级管理员ID=1允许审核自己创建的订单；并发重试仍只能记一笔账。
	errs = parallel(24, func(i int) error { _, e := s.Approve(ctx, creator, review(o.OrderNo)); return e })
	for _, e := range errs {
		require.NoError(t, e)
	}
	assertCount(t, s.DB, &model.WalletLedger{}, 1)
	assertCount(t, s.DB, &model.WalletAuditLog{}, 3)
	// 不同业务单并发入账同一钱包：不能覆盖余额、跳号或生成重复流水。
	orders := make([]string, 40)
	for i := range orders {
		order, e := s.Create(ctx, creator, grant(fmt.Sprintf("different_%04d", i)))
		require.NoError(t, e)
		orders[i] = order.OrderNo
	}
	errs = parallel(len(orders), func(i int) error { _, e := s.Approve(ctx, reviewer, review(orders[i])); return e })
	for _, e := range errs {
		require.NoError(t, e)
	}
	b, e := s.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "410", b.AvailableBalance)
	require.Equal(t, "41", b.Version)
	c, e := s.Check(ctx, creator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 1000})
	require.NoError(t, e)
	require.True(t, c.Consistent, c.Issues)
	// 同一原单并发冲正申请只有一张生效；同key并发应全部返回同一单。
	r := req.ReverseAdjustment{OrderNo: o.OrderNo, RequestID: "reverse_same", Reason: "测试错误发放冲正"}
	errs = parallel(24, func(i int) error { _, e := s.Reverse(ctx, creator, r); return e })
	for _, e := range errs {
		require.NoError(t, e)
	}
	rev, e := s.Reverse(ctx, creator, r)
	require.NoError(t, e)
	errs = parallel(24, func(i int) error { _, e := s.Approve(ctx, reviewer, review(rev.OrderNo)); return e })
	for _, e := range errs {
		require.NoError(t, e)
	}
	b, e = s.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "400", b.AvailableBalance)
	assertCount(t, s.DB, &model.WalletLedger{}, 42)
	for _, sql := range []string{"UPDATE wallet_ledger SET reason='changed' LIMIT 1", "DELETE FROM wallet_ledger LIMIT 1", "UPDATE wallet_audit_log SET reason='changed' LIMIT 1", "DELETE FROM wallet_adjustment_order LIMIT 1"} {
		require.Error(t, s.DB.Exec(sql).Error)
	}
}
func TestMySQLWalletReviewRaceAndQuota(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	s.Config = config.Wallet{MaxGrantAmount: 10, AccountDailyLimit: 30, OperatorDailyLimit: 100}
	orders := make([]string, 10)
	for i := range orders {
		o, e := s.Create(ctx, creator, grant(fmt.Sprintf("quota_%04d", i)))
		require.NoError(t, e)
		orders[i] = o.OrderNo
	}
	errs := parallel(10, func(i int) error { _, e := s.Approve(ctx, reviewer, review(orders[i])); return e })
	success := 0
	for _, e := range errs {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, svc.ErrLimit)
		}
	}
	require.Equal(t, 3, success)
	b, e := s.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "30", b.AvailableBalance)
	assertCount(t, s.DB, &model.WalletLedger{}, 3)
	s.Config = config.Wallet{}
	o, e := s.Create(ctx, creator, grant("race_review_1"))
	require.NoError(t, e)
	errs = parallel(2, func(i int) error {
		if i == 0 {
			_, e := s.Approve(ctx, reviewer, review(o.OrderNo))
			return e
		}
		_, e := s.Reject(ctx, svc.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}, review(o.OrderNo))
		return e
	})
	successes := 0
	for _, e := range errs {
		if e == nil {
			successes++
		} else {
			require.ErrorIs(t, e, svc.ErrConflict)
		}
	}
	require.Equal(t, 1, successes)
	d, e := s.Detail(ctx, creator, o.OrderNo)
	require.NoError(t, e)
	require.Len(t, d.Audit, 2)
	check, e := s.Check(ctx, creator, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, e)
	require.True(t, check.Consistent, check.Issues)
}
func TestMySQLWalletPlansAndLoad(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	for i := 0; i < 80; i++ {
		o, e := s.Create(ctx, creator, grant(fmt.Sprintf("load_%04d", i)))
		require.NoError(t, e)
		_, e = s.Approve(ctx, reviewer, review(o.OrderNo))
		require.NoError(t, e)
	}
	// 加入其他钱包历史，验证真实选择性；小表80行时MySQL全表扫描可能反而更便宜。
	seed := make([]model.WalletLedger, 20000)
	for i := range seed {
		seed[i] = model.WalletLedger{LedgerNo: fmt.Sprintf("seed_%d", i), WalletAccountID: 2, WalletVersion: int64(i + 1), ChangeAmount: 1, BalanceBefore: int64(i), BalanceAfter: int64(i + 1), BusinessType: "ADMIN_GRANT", BusinessNo: fmt.Sprintf("seed_order_%d", i), OperatorID: 1, Reason: "plan fixture"}
	}
	require.NoError(t, s.DB.CreateInBatches(seed, 500).Error)
	require.NoError(t, s.DB.Exec("ANALYZE TABLE wallet_account,wallet_ledger,wallet_adjustment_order,wallet_daily_quota").Error)
	for _, query := range []string{
		"SELECT * FROM wallet_ledger WHERE wallet_account_id=1 AND wallet_version<70 ORDER BY wallet_version DESC LIMIT 21",
		"SELECT * FROM wallet_ledger WHERE wallet_account_id=1 ORDER BY wallet_version DESC LIMIT 21",
		"SELECT * FROM wallet_ledger WHERE wallet_account_id=1 AND wallet_version>20 ORDER BY wallet_version LIMIT 101",
		"SELECT id FROM wallet_adjustment_order WHERE created_by=1 AND status='pending' LIMIT 100",
		"SELECT * FROM wallet_account WHERE account_id=10 AND currency_code='COIN'",
		"SELECT * FROM wallet_adjustment_order WHERE source_type='manual' AND external_order_no='LOAD_0001'",
	} {
		var rows []map[string]any
		require.NoError(t, s.DB.Raw("EXPLAIN "+query).Scan(&rows).Error)
		for _, r := range rows {
			require.NotEqual(t, "ALL", fmt.Sprint(r["type"]), fmt.Sprint(r))
		}
		t.Logf("EXPLAIN %s => %v", query, rows)
	}
	if os.Getenv("WALLET_RUN_LOAD") != "1" {
		return
	}
	const n = 400
	durations := make([]time.Duration, n)
	errs := make([]error, n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	start := time.Now()
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				at := time.Now()
				_, errs[i] = s.Ledger(ctx, 10, req.LedgerSearch{Limit: 20})
				durations[i] = time.Since(at)
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start)
	for _, e := range errs {
		require.NoError(t, e)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	t.Logf("wallet ledger read requests=%d concurrency=16 QPS=%.2f p95=%s p99=%s errors=0", n, float64(n)/elapsed.Seconds(), durations[n*95/100-1], durations[n*99/100-1])
}

func TestMySQLWalletRollbackTimeoutAndReconnect(t *testing.T) {
	s := mysqlFixture(t)
	ctx := context.Background()
	o, e := s.Create(ctx, creator, grant("rollback_mysql"))
	require.NoError(t, e)
	// 在余额、额度、流水与订单均已写入后让审计写失败，验证真实InnoDB整笔回滚。
	require.NoError(t, s.DB.Exec("CREATE TRIGGER test_wallet_audit_failure BEFORE INSERT ON wallet_audit_log FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected audit failure'").Error)
	_, e = s.Approve(ctx, reviewer, review(o.OrderNo))
	require.Error(t, e)
	assertCount(t, s.DB, &model.WalletLedger{}, 0)
	assertCount(t, s.DB, &model.WalletDailyQuota{}, 0)
	assertCount(t, s.DB, &model.WalletAccount{}, 0)
	require.NoError(t, s.DB.Exec("DROP TRIGGER test_wallet_audit_failure").Error)
	_, e = s.Approve(ctx, reviewer, review(o.OrderNo))
	require.NoError(t, e)
	next, e := s.Create(ctx, creator, grant("timeout_mysql"))
	require.NoError(t, e)
	blocker := s.DB.Begin()
	require.NoError(t, blocker.Error)
	require.NoError(t, blocker.Exec("SELECT id FROM wallet_account WHERE account_id = 10 FOR UPDATE").Error)
	timeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	_, e = s.Approve(timeout, reviewer, review(next.OrderNo))
	cancel()
	require.Error(t, e)
	require.NoError(t, blocker.Rollback().Error)
	assertCount(t, s.DB, &model.WalletLedger{}, 1)
	_, e = s.Approve(ctx, reviewer, review(next.OrderNo))
	require.NoError(t, e)
	// 重建连接池和Service，持久化幂等不依赖进程内锁或缓存。
	dsn := s.DB.Dialector.(*mysql.Dialector).Config.DSN
	pool, _ := s.DB.DB()
	require.NoError(t, pool.Close())
	reopened, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool2, _ := reopened.DB()
	defer pool2.Close()
	fresh := &svc.Service{DB: reopened}
	_, e = fresh.Approve(ctx, reviewer, review(next.OrderNo))
	require.NoError(t, e)
	b, e := fresh.Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, "20", b.AvailableBalance)
	assertCount(t, reopened, &model.WalletLedger{}, 2)
}
