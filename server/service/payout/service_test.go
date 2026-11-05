package payout_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"sync"
	"tb_live_module/global"
	"tb_live_module/initialize"
	live "tb_live_module/model/live"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	svc "tb_live_module/service/payout"
	"testing"
	"time"
)

var ctx = context.Background()

func fixture(t *testing.T, real bool) *svc.Service {
	t.Helper()
	var db *gorm.DB
	var e error
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}
	if real {
		raw := os.Getenv("PAYOUT_TEST_MYSQL_DSN")
		if raw == "" {
			t.Skip("PAYOUT_TEST_MYSQL_DSN not configured")
		}
		dsn, e := driver.ParseDSN(raw)
		require.NoError(t, e)
		require.Equal(t, "127.0.0.1:13329", dsn.Addr)
		require.Equal(t, "payout_test_delivery", dsn.DBName)
		db, e = gorm.Open(mysql.Open(raw), cfg)
		require.NoError(t, e)
		var tables []string
		require.NoError(t, db.Raw("SHOW TABLES").Scan(&tables).Error)
		for _, table := range tables {
			require.NoError(t, db.Exec("DROP TABLE `"+table+"`").Error)
		}
	} else {
		db, e = gorm.Open(sqlite.Open("file:"+t.TempDir()+"/payout.db"), cfg)
		require.NoError(t, e)
	}
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(32)
	if !real {
		pool.SetMaxOpenConns(1)
	}
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&live.LiveAccount{}, &live.LiveAnchor{}))
	require.NoError(t, initialize.MigratePayout(db))
	require.NoError(t, initialize.MigratePayout(db))
	require.NoError(t, initialize.ValidatePayoutSchema(db))
	for i := uint(1); i <= 2; i++ {
		require.NoError(t, db.Create(&live.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: i}, AccountNo: fmt.Sprintf("user%d", i), Username: fmt.Sprintf("user%d", i), Status: 1}).Error)
		require.NoError(t, db.Create(&live.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: i}, UserId: uint64(i), AnchorNo: fmt.Sprintf("anchor%d", i), Status: 1, ApplyStatus: 2}).Error)
	}
	c, e := svc.NewCrypto("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32))))
	require.NoError(t, e)
	return &svc.Service{DB: db, Crypto: c}
}
func request(key string) req.CreatePayee {
	return req.CreatePayee{RequestID: key, Method: "bank_card", Currency: "CNY", Account: "6222021234567890123", AccountName: "测试姓名", BankCode: "ICBC"}
}
func TestPayeeIsolationEncryptionAndImmutableHistory(t *testing.T)      { exercise(t, false) }
func TestMySQLPayeeIsolationEncryptionAndImmutableHistory(t *testing.T) { exercise(t, true) }
func exercise(t *testing.T, real bool) {
	s := fixture(t, real)
	r := request("create_first")
	a, e := s.Create(ctx, 1, "trace_create", r)
	require.NoError(t, e)
	require.True(t, a.IsDefault)
	page, e := s.List(ctx, 2, req.PayeeSearch{})
	require.NoError(t, e)
	require.Empty(t, page.List)
	_, e = s.Change(ctx, 2, "trace", a.PayeeAccountNo, true)
	require.ErrorIs(t, e, svc.ErrState)
	_, e = s.Change(ctx, 2, "trace", a.PayeeAccountNo, false)
	require.ErrorIs(t, e, svc.ErrState)
	_, e = s.Create(ctx, 0, "trace", r)
	require.ErrorIs(t, e, svc.ErrForbidden)
	retry, e := s.Create(ctx, 1, "trace_retry", r)
	require.NoError(t, e)
	require.Equal(t, a, retry)
	r.AccountName = "changed"
	_, e = s.Create(ctx, 1, "trace", r)
	require.ErrorIs(t, e, svc.ErrConflict)
	r = request("duplicate_key")
	_, e = s.Create(ctx, 1, "trace", r)
	require.ErrorIs(t, e, svc.ErrConflict)
	r.Account = "6222021234567890456"
	b, e := s.Create(ctx, 1, "trace_second", r)
	require.NoError(t, e)
	_, e = s.Change(ctx, 1, "trace_default", b.PayeeAccountNo, false)
	require.NoError(t, e)
	var stored model.PayeeAccount
	require.NoError(t, s.DB.Where("payee_account_no=?", a.PayeeAccountNo).Take(&stored).Error)
	require.NotContains(t, stored.EncryptedPayload, "6222021234567890123")
	require.NoError(t, s.VerifyBinding(stored))
	before := stored.EncryptedPayload
	_, e = s.Change(ctx, 1, "trace_disable", a.PayeeAccountNo, true)
	require.NoError(t, e)
	_, e = s.Change(ctx, 1, "trace_default", a.PayeeAccountNo, false)
	require.ErrorIs(t, e, svc.ErrState)
	require.NoError(t, s.DB.Where("payee_account_no=?", a.PayeeAccountNo).Take(&stored).Error)
	require.Equal(t, before, stored.EncryptedPayload)
	require.Error(t, s.DB.Delete(&stored).Error)
	require.Error(t, s.DB.Model(&stored).Update("masked_account", "****0000").Error)
	require.Error(t, s.DB.Model(&model.PayeeAccount{}).Where("payee_account_no=?", b.PayeeAccountNo).Update("version", 2).Error)
	page, e = s.List(ctx, 1, req.PayeeSearch{})
	require.NoError(t, e)
	require.Len(t, page.List, 2)
	var audits []model.Audit
	require.NoError(t, s.DB.Find(&audits).Error)
	require.Len(t, audits, 5)
	encoded, e := json.Marshal(page)
	require.NoError(t, e)
	for _, secret := range []string{r.Account, r.AccountName, stored.AccountFingerprint, stored.EncryptedPayload, "encryptedPayload", "accountFingerprint", "ownerNo", "encryptionKeyID"} {
		require.NotContains(t, string(encoded), secret)
		for _, a := range audits {
			require.NotContains(t, a.Snapshot, secret)
		}
	}
	require.NoError(t, initialize.MigratePayout(s.DB))
	require.NoError(t, initialize.ValidatePayoutSchema(s.DB))
	var after model.PayeeAccount
	require.NoError(t, s.DB.First(&after, stored.ID).Error)
	require.Equal(t, before, after.EncryptedPayload)
	require.Equal(t, "****0123", after.MaskedAccount)
	require.Equal(t, "disabled", after.Status)
}
func TestPayeeRollbackAndTimeout(t *testing.T) {
	s := fixture(t, false)
	require.NoError(t, s.DB.Exec("CREATE TRIGGER inject_audit_failure BEFORE INSERT ON payout_payee_audit BEGIN SELECT RAISE(ABORT,'injected'); END").Error)
	_, e := s.Create(ctx, 1, "trace", request("failed_create"))
	require.Error(t, e)
	var n int64
	require.NoError(t, s.DB.Model(&model.PayeeAccount{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.DB.Exec("DROP TRIGGER inject_audit_failure").Error)
	a, e := s.Create(ctx, 1, "trace", request("failed_create"))
	require.NoError(t, e)
	require.NoError(t, s.DB.Exec("CREATE TRIGGER inject_audit_failure BEFORE INSERT ON payout_payee_audit BEGIN SELECT RAISE(ABORT,'injected'); END").Error)
	_, e = s.Change(ctx, 1, "trace", a.PayeeAccountNo, true)
	require.Error(t, e)
	page, e := s.List(ctx, 1, req.PayeeSearch{})
	require.NoError(t, e)
	require.Equal(t, "active", page.List[0].Status)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, e = s.Create(cancelled, 1, "trace", request("cancelled_create"))
	require.Error(t, e)
}

func TestPayeeKeyConfigurationAndLimits(t *testing.T) {
	s := fixture(t, false)
	_, e := s.Create(ctx, 1, "trace", request("key_fixture"))
	require.NoError(t, e)
	require.NoError(t, s.ValidateKeys(s.DB))
	bad, e := svc.NewCrypto("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	require.NoError(t, e)
	require.Error(t, (&svc.Service{Crypto: bad}).ValidateKeys(s.DB))
	for i := 1; i < svc.MaxAccounts; i++ {
		r := request(fmt.Sprintf("capacity_%04d", i))
		r.Account = fmt.Sprintf("622202123456789%04d", i)
		_, e = s.Create(ctx, 1, "trace", r)
		require.NoError(t, e)
	}
	r := request("capacity_overflow")
	r.Account = "6222021234567890999"
	_, e = s.Create(ctx, 1, "trace", r)
	require.ErrorIs(t, e, svc.ErrLimit)
	page, e := s.List(ctx, 1, req.PayeeSearch{})
	require.NoError(t, e)
	require.EqualValues(t, 100, page.Total)
	require.Len(t, page.List, 20)
	s.Crypto = nil
	_, e = s.Create(ctx, 2, "trace", r)
	require.ErrorIs(t, e, svc.ErrCrypto)
	var n int64
	require.NoError(t, s.DB.Model(&model.PayeeAccount{}).Where("owner_no='anchor2'").Count(&n).Error)
	require.Zero(t, n)
}

func TestMySQLPayeeLockDeadline(t *testing.T) {
	s := fixture(t, true)
	_, e := s.Create(ctx, 1, "trace", request("lock_fixture"))
	require.NoError(t, e)
	locked, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- s.DB.Transaction(func(tx *gorm.DB) error {
			if e := svc.LockOwner(tx, "anchor1"); e != nil {
				return e
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	bounded, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	r := request("lock_timeout")
	r.Account = "6222021234567890456"
	_, e = s.Create(bounded, 1, "trace", r)
	cancel()
	close(release)
	require.Error(t, e)
	require.NoError(t, <-finished)
	var n int64
	require.NoError(t, s.DB.Model(&model.PayeeAccount{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
	_, e = s.Create(ctx, 1, "trace_retry", r)
	require.NoError(t, e)
}
func TestMySQLPayeeConcurrentUniqueness(t *testing.T) {
	s := fixture(t, true)
	const concurrency = 32
	errs := make(chan error, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Create(ctx, 1, "trace", request(fmt.Sprintf("concurrent_%04d", i)))
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
			require.ErrorIs(t, e, svc.ErrConflict)
		}
	}
	require.Equal(t, 1, success)
	var accounts []string
	for i := 0; i < 8; i++ {
		r := request(fmt.Sprintf("many_%04d", i))
		r.Account = fmt.Sprintf("622202123456789%04d", i)
		a, e := s.Create(ctx, 1, "trace", r)
		require.NoError(t, e)
		accounts = append(accounts, a.PayeeAccountNo)
	}
	errs = make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			_, e := s.Change(bounded, 1, "trace", accounts[i%len(accounts)], false)
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	var n int64
	require.NoError(t, s.DB.Model(&model.PayeeAccount{}).Where("default_slot=1").Count(&n).Error)
	require.EqualValues(t, 1, n)
	var defaults model.PayeeAccount
	require.NoError(t, s.DB.Where("default_slot=1").Take(&defaults).Error)
	// The uniqueness invariant survives bypassing application locks as well.
	other := accounts[0]
	if other == defaults.PayeeAccountNo {
		other = accounts[1]
	}
	e := s.DB.Model(&model.PayeeAccount{}).Where("payee_account_no=?", other).Update("default_slot", 1).Error
	require.Error(t, e)
	var mysqlError *driver.MySQLError
	require.True(t, errors.As(e, &mysqlError))
	require.EqualValues(t, 1062, mysqlError.Number)
}
