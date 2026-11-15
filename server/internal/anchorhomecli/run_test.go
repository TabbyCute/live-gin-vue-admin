package anchorhomecli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/internal/engagementcli"
	am "tb_live_module/model/anchorhome"
	sm "tb_live_module/model/system"
)

func TestUnchangedFactsAndPermissions(t *testing.T) {
	before := map[string]engagementcli.TableProof{"wallet_account": {Rows: 1, SHA256: "wallet"}, "anchor_finance_permission": {Rows: 1, SHA256: "permission"}}
	after := map[string]engagementcli.TableProof{"wallet_account": before["wallet_account"], "anchor_finance_permission": before["anchor_finance_permission"], "anchor_home_album": {Rows: 0, SHA256: "empty"}}
	require.True(t, unchangedFinancial(before, after))
	after["anchor_finance_permission"] = engagementcli.TableProof{Rows: 2, SHA256: "broadened"}
	require.False(t, unchangedFinancial(before, after))
	after["anchor_finance_permission"] = before["anchor_finance_permission"]
	after["anchor_home_album"] = engagementcli.TableProof{Rows: 1, SHA256: "injected"}
	require.False(t, unchangedFinancial(before, after))
	old := metadataProof{Rows: map[string]map[string]int{"sys_apis": {"old": 2}}}
	next := metadataProof{Rows: map[string]map[string]int{"sys_apis": {"old": 2, "module": 1}}, Allowed: map[string]map[string]bool{"sys_apis": {"module": true}}}
	require.True(t, unchangedMetadata(old, next, true))
	require.False(t, unchangedMetadata(old, next, false))
	next.Rows["sys_apis"]["old"] = 1
	require.False(t, unchangedMetadata(old, next, true), "deleted duplicate is still a permission change")
	next.Rows["sys_apis"]["old"] = 2
	next.Rows["sys_apis"]["unrelated"] = 1
	require.False(t, unchangedMetadata(old, next, true))
	require.False(t, unchangedAccounts(nil, nil))
}

func TestCLIFailuresRemainPrivate(t *testing.T) {
	for _, args := range [][]string{
		{"-config", "secret-password-missing.yaml"},
		{"-config", "secret-password-missing.yaml", "-verify", "-migrate"},
		{"-config", "secret-password-missing.yaml", "-backup-scope", "full"},
		{"-config", "secret-password-missing.yaml", "-read-timeout", "5m"},
	} {
		var output, progress bytes.Buffer
		require.Equal(t, 1, RunWithProgress(args, &output, &progress))
		require.True(t, json.Valid(output.Bytes()))
		require.NotContains(t, output.String(), "secret-password")
		require.Empty(t, progress.String())
	}
	public := filepath.Join(t.TempDir(), "public")
	require.NoError(t, os.Mkdir(public, 0755))
	require.False(t, privateDirectory(public))
	private := filepath.Join(t.TempDir(), "private")
	require.True(t, privateDirectory(private))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(private, link))
	require.False(t, privateDirectory(link))
	for _, err := range []error{errors.New("sql-with-private-data"), &driver.MySQLError{Number: 1045, Message: "secret-password"}} {
		require.NotContains(t, safeFailureCode(err), "secret")
		require.NotContains(t, safeFailureCode(err), "private")
	}
	require.Equal(t, "timeout", safeFailureCode(context.DeadlineExceeded))
	require.Equal(t, "canceled", safeFailureCode(context.Canceled))
}

func TestMySQLScopedMigrationAndActualRestore(t *testing.T) {
	raw := os.Getenv("ANCHOR_HOME_MIGRATION_TEST_DSN")
	if raw == "" {
		t.Skip("dedicated local MySQL required")
	}
	cfg, err := driver.ParseDSN(raw)
	require.NoError(t, err)
	// Guard the actual endpoint and control database BEFORE opening a socket.
	require.Equal(t, "tcp", cfg.Net)
	require.Equal(t, "127.0.0.1:13339", cfg.Addr)
	require.Equal(t, "anchor_home_test_migration", cfg.DBName)
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	cfg.Logger = silentDriverLogger{}
	open := func(name string) *gorm.DB {
		require.True(t, strings.HasPrefix(name, "anchor_home_test_"))
		copy := *cfg
		copy.DBName = name
		connector, err := driver.NewConnector(&copy)
		require.NoError(t, err)
		pool := sql.OpenDB(connector)
		db, err := gorm.Open(mysql.New(mysql.Config{Conn: pool}), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		pool.SetMaxOpenConns(4)
		t.Cleanup(func() { pool.Close() })
		return db
	}
	control := open(cfg.DBName)
	source := fmt.Sprintf("anchor_home_test_migrate_%d", time.Now().UnixNano())
	target := source + "_restore"
	for _, name := range []string{source, target} {
		require.NoError(t, control.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
		defer control.Exec("DROP DATABASE `" + name + "`")
	}
	db := open(source)
	require.NoError(t, db.AutoMigrate(&sm.SysBaseMenu{}, &sm.SysAuthorityMenu{}, &sm.SysApi{}))
	require.NoError(t, db.Create(&sm.SysBaseMenu{Name: "liveManagement", Path: "liveManagement"}).Error)
	require.NoError(t, db.Create(&sm.SysApi{Path: "/old/private", Method: "POST", Description: "old-private-metadata"}).Error)
	for _, sql := range []string{
		"CREATE TABLE anchor_finance_permission(id BIGINT PRIMARY KEY, authority_id BIGINT, path VARCHAR(128)) ENGINE=InnoDB",
		"INSERT INTO anchor_finance_permission VALUES(1,888,'/existing/review')",
		"CREATE TABLE scope_grandparent(id BIGINT PRIMARY KEY) ENGINE=InnoDB",
		"CREATE TABLE scope_parent(id BIGINT PRIMARY KEY, parent_id BIGINT, FOREIGN KEY(parent_id) REFERENCES scope_grandparent(id)) ENGINE=InnoDB",
		"INSERT INTO scope_grandparent VALUES(1)", "INSERT INTO scope_parent VALUES(2,1)",
		"CREATE TABLE wallet_fixture(id BIGINT PRIMARY KEY, parent_id BIGINT, amount DECIMAL(30,8), binary_value VARBINARY(8), FOREIGN KEY(parent_id) REFERENCES scope_parent(id)) ENGINE=InnoDB",
		"INSERT INTO wallet_fixture VALUES(1,2,1234567890123456789012.12345678,X'00ff01')",
		"CREATE TRIGGER wallet_immutable BEFORE UPDATE ON wallet_fixture FOR EACH ROW BEGIN IF NOT (OLD.binary_value <=> NEW.binary_value) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='immutable'; END IF; END",
		"CREATE TABLE live_account(id BIGINT PRIMARY KEY, private_value VARCHAR(128)) ENGINE=InnoDB",
		"INSERT INTO live_account VALUES(1,'account-private-fact')",
		"CREATE TABLE live_anchor(id BIGINT PRIMARY KEY, private_value VARCHAR(128)) ENGINE=InnoDB",
		"INSERT INTO live_anchor VALUES(1,'anchor-private-fact')",
		"CREATE VIEW gva_announcements_info AS SELECT 1 AS unrelated",
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	for _, name := range []string{"gift_order", "gift_refund_order", "gift_revenue_ledger"} {
		require.NoError(t, db.Exec("CREATE TABLE `"+name+"` (id BIGINT PRIMARY KEY, value VARBINARY(8)) ENGINE=InnoDB").Error)
		require.NoError(t, db.Exec("INSERT INTO `"+name+"` VALUES(1,X'00ff')").Error)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	readonly, err := Execute(ctx, db, Options{})
	require.Error(t, err)
	require.Equal(t, "schema_verification", readonly.Stage)
	for _, name := range newTables {
		require.False(t, db.Migrator().HasTable(name))
	}
	testExistingSchemaSafety(t, ctx, db)
	// A selected backup read failure must leave no partial file and permit no DDL.
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("album_backup_fail", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "SHOW CREATE TABLE `wallet_fixture`") {
			tx.AddError(errors.New("private-row-must-not-log"))
		}
	}))
	dir := filepath.Join(t.TempDir(), "private")
	failed, err := Execute(ctx, db, Options{Migrate: true, Metadata: true, BackupDir: dir})
	require.Error(t, err)
	require.Equal(t, "backup", failed.Stage)
	require.Empty(t, failed.BackupFile)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)
	for _, name := range newTables {
		require.False(t, db.Migrator().HasTable(name))
	}
	require.NoError(t, db.Callback().Row().Remove("album_backup_fail"))
	// Explicitly fail any attempt to inspect the unrelated inaccessible object.
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("album_unrelated_fail", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "SHOW CREATE TABLE `gva_announcements_info`") {
			tx.AddError(errors.New("unrelated must not be read"))
		}
	}))
	require.NoError(t, control.Connection(func(locker *gorm.DB) error {
		var got int
		require.NoError(t, locker.Raw("SELECT GET_LOCK(?,0)", migrationLockName(source)).Scan(&got).Error)
		require.Equal(t, 1, got)
		defer locker.Exec("SELECT RELEASE_LOCK(?)", migrationLockName(source))
		bounded, stop := context.WithTimeout(ctx, 300*time.Millisecond)
		defer stop()
		blocked, err := Execute(bounded, db, Options{Migrate: true, BackupDir: dir})
		require.Error(t, err)
		require.Equal(t, "migration_lock", blocked.Stage)
		require.Empty(t, blocked.BackupFile)
		return nil
	}))
	before, beforeMeta, beforeAccounts, err := snapshot(db)
	require.NoError(t, err)
	first, err := Execute(ctx, db, Options{Migrate: true, Metadata: true, BackupDir: dir})
	require.NoError(t, err, "local fixture stage=%s cause=%v", first.Stage, errors.Unwrap(err))
	require.True(t, first.OK && first.Verified && first.FinancialUnchanged && first.MetadataPreserved && first.AccountsPreserved)
	require.Len(t, first.NewTables, 4)
	require.GreaterOrEqual(t, first.BackupTables, len(before)+5)
	require.NoError(t, db.Create(&am.Album{OwnerID: 7, Version: 1}).Error)
	require.NoError(t, db.Create(&am.Photo{PhotoNo: "photo_local_fixture", OwnerID: 7, AnchorNo: "anchor_local", StorageKey: strings.Repeat("a", 32), Status: "pending", Version: 1, Sort: 1, Width: 1, Height: 1, Bytes: 32, CreatedAt: 1, UpdatedAt: 1}).Error)
	require.NoError(t, db.Create(&am.Audit{PhotoNo: "photo_local_fixture", OwnerID: 7, ActorKind: "owner", ActorID: 7, RequestID: "local_request", Action: "upload", ToStatus: "pending", AlbumVersion: 1, CreatedAt: 1}).Error)
	require.NoError(t, db.Create(&am.Command{ActorKind: "owner", ActorID: 7, RequestID: "local_request", PayloadHash: strings.Repeat("b", 64), Result: `{"version":1}`, CreatedAt: 1}).Error)
	beforeSecond, metadataSecond, accountsSecond, err := snapshot(db)
	require.NoError(t, err)
	second, err := Execute(ctx, db, Options{Migrate: true, Metadata: true, BackupDir: dir})
	require.NoError(t, err, "local fixture stage=%s cause=%v", second.Stage, errors.Unwrap(err))
	require.Empty(t, second.NewTables)
	afterSecond, metadataAfter, accountsAfter, err := snapshot(db)
	require.NoError(t, err)
	require.Equal(t, beforeSecond, afterSecond)
	require.True(t, unchangedMetadata(metadataSecond, metadataAfter, false))
	require.Equal(t, accountsSecond, accountsAfter)
	data, err := os.ReadFile(second.BackupFile)
	require.NoError(t, err)
	digest := sha256.Sum256(data)
	require.Equal(t, second.BackupSHA256, hex.EncodeToString(digest[:]))
	require.NotContains(t, string(data), "gva_announcements_info")
	require.NotContains(t, string(data), "account-private-fact")
	for _, name := range []string{"scope_parent", "scope_grandparent", "anchor_home_audit", "sys_apis"} {
		require.Contains(t, string(data), name)
	}
	st, err := os.Stat(second.BackupFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), st.Mode().Perm())
	file, err := os.Open(second.BackupFile)
	require.NoError(t, err)
	defer file.Close()
	command := exec.CommandContext(ctx, "docker", "exec", "-i", "-e", "MYSQL_PWD="+cfg.Passwd, "anchor-home-tests-mysql-1", "mysql", "--binary-mode", "-u"+cfg.User, target)
	command.Stdin = file
	restoreOutput, err := command.CombinedOutput()
	require.NoError(t, err, "restore failed: %s", restoreOutput)
	restored := open(target)
	restoredProof, err := engagementcli.Proof(restored)
	require.NoError(t, err)
	require.Equal(t, beforeSecond, restoredProof)
	require.Error(t, restored.Exec("UPDATE wallet_fixture SET binary_value=X'00' WHERE id=1").Error)
	require.Error(t, restored.Exec("INSERT INTO wallet_fixture VALUES(2,999,1,X'00')").Error)
	require.Error(t, restored.Exec("UPDATE anchor_home_audit SET action='changed' WHERE id=1").Error)
	require.Error(t, restored.Exec("DELETE FROM anchor_home_audit WHERE id=1").Error)
	require.Error(t, restored.Exec("UPDATE anchor_home_command SET result='{}' WHERE id=1").Error)
	require.Error(t, restored.Exec("DELETE FROM anchor_home_command WHERE id=1").Error)
	// Public CLI uses a private local YAML, never shell-visible production secrets.
	configPath := filepath.Join(t.TempDir(), "local.yaml")
	yaml := fmt.Sprintf("system:\n  db-type: mysql\nmysql:\n  path: 127.0.0.1\n  port: \"13339\"\n  db-name: %q\n  username: %q\n  password: %q\n  config: \"charset=utf8mb4&parseTime=true&loc=UTC\"\n", source, cfg.User, cfg.Passwd)
	require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0600))
	var output, progress bytes.Buffer
	require.Equal(t, 0, RunWithProgress([]string{"-config", configPath, "-verify", "-progress"}, &output, &progress), output.String())
	require.True(t, json.Valid(output.Bytes()))
	for _, secret := range []string{cfg.Passwd, cfg.Addr, "account-private-fact", "anchor-private-fact", "SELECT ", "CREATE TABLE ", "private-row"} {
		require.NotContains(t, output.String(), secret)
		require.NotContains(t, progress.String(), secret)
	}
	// Detect concurrent account/finance changes even though this CLI never writes them.
	failed, err = Execute(ctx, db, Options{Progress: func(p Progress) {
		if p.Stage == "financial_postflight" {
			require.NoError(t, db.Exec("UPDATE live_anchor SET private_value='concurrent-change' WHERE id=1").Error)
		}
	}})
	require.Error(t, err)
	require.False(t, failed.OK)
	require.Equal(t, "financial_postflight", failed.Stage)
	require.NoError(t, db.Exec("UPDATE live_anchor SET private_value='anchor-private-fact' WHERE id=1").Error)
	failed, err = Execute(ctx, db, Options{Progress: func(p Progress) {
		if p.Stage == "financial_postflight" {
			require.NoError(t, db.Exec("UPDATE anchor_finance_permission SET authority_id=777 WHERE id=1").Error)
		}
	}})
	require.Error(t, err)
	require.False(t, failed.OK)
	require.NoError(t, db.Exec("UPDATE anchor_finance_permission SET authority_id=888 WHERE id=1").Error)
	// Interrupted backup cannot leak a pooled advisory lock or lose primary stage.
	interrupted := false
	failed, err = Execute(ctx, db, Options{Migrate: true, BackupDir: filepath.Join(t.TempDir(), "private"), Progress: func(p Progress) {
		if interrupted || p.BackupStep != "table_rows" {
			return
		}
		interrupted = true
		var connections []uint64
		require.NoError(t, control.Raw("SELECT ID FROM information_schema.PROCESSLIST WHERE DB=?", source).Scan(&connections).Error)
		for _, id := range connections {
			require.NoError(t, control.Exec(fmt.Sprintf("KILL CONNECTION %d", id)).Error)
		}
	}})
	require.True(t, interrupted)
	require.Error(t, err)
	require.Equal(t, "backup", failed.Stage)
	require.Equal(t, "backup", err.Error())
	require.True(t, failed.CleanupFailed)
	require.Empty(t, failed.BackupFile)
	finalProof, finalMeta, finalAccounts, err := snapshot(db)
	require.NoError(t, err)
	require.True(t, unchangedFinancial(before, finalProof) == false, "seeded album rows are protected, not treated as new empty tables")
	require.True(t, unchangedMetadata(beforeMeta, finalMeta, true))
	require.Equal(t, beforeAccounts, finalAccounts)
	t.Logf("isolated migration+restore passed: %d existing finance/module proofs, immutable audit and wallet triggers, recursive FK parents; old metadata and account facts preserved", len(beforeSecond))
}
