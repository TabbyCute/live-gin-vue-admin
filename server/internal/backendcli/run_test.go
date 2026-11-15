package backendcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"tb_live_module/initialize"
	"tb_live_module/internal/engagementcli"
	em "tb_live_module/model/earning"
	pm "tb_live_module/model/payout"
	sm "tb_live_module/model/system"
	"testing"
	"time"
)

func TestFinancialProofDeltaRules(t *testing.T) {
	old := map[string]engagementcli.TableProof{"wallet_account": {Rows: 1, SHA256: "before"}, "anchor_finance_permission": {Rows: 1, SHA256: "permission"}}
	newProof := map[string]engagementcli.TableProof{"wallet_account": old["wallet_account"], "anchor_finance_permission": old["anchor_finance_permission"], "payout_bank_line": {Rows: 0, SHA256: "empty"}, "anchor_earning_audit_job": {Rows: 0, SHA256: "empty"}}
	require.True(t, unchangedFinancial(old, newProof, false))
	newProof["payout_bank_line"] = engagementcli.TableProof{Rows: 1, SHA256: "injected"}
	require.False(t, unchangedFinancial(old, newProof, false))
	delete(newProof, "payout_bank_line")
	newProof["wallet_account"] = engagementcli.TableProof{Rows: 1, SHA256: "changed"}
	require.False(t, unchangedFinancial(old, newProof, true))
	permissions := []em.AdminPermission{{ID: 1, AuthorityID: 888, Path: "/earning/settlement/review", Method: "POST"}}
	require.True(t, safePermissionChanges(permissions, append(append([]em.AdminPermission{}, permissions...), em.AdminPermission{ID: 2, AuthorityID: 888, Path: "/earning/audit/start", Method: "POST"})))
	require.False(t, safePermissionChanges(permissions, []em.AdminPermission{{ID: 1, AuthorityID: 888, Path: "/earning/audit/start", Method: "POST"}}))
	require.False(t, safePermissionChanges(permissions, nil))
}

func TestSafeCLIOutputAndPrivateDirectory(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 1, Run([]string{"-config", "password-secret-missing.yaml"}, &out))
	require.NotContains(t, out.String(), "password-secret")
	require.True(t, json.Valid(out.Bytes()))
	public := filepath.Join(t.TempDir(), "public")
	require.NoError(t, os.Mkdir(public, 0755))
	require.False(t, privateDirectory(public))
	private := filepath.Join(t.TempDir(), "private")
	require.True(t, privateDirectory(private))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(private, link))
	require.False(t, privateDirectory(link))
}

func TestMySQLUnifiedMigrationAndFullRestore(t *testing.T) {
	raw := os.Getenv("BACKEND_MIGRATION_TEST_DSN")
	if raw == "" {
		t.Skip("dedicated local MySQL required")
	}
	cfg, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	require.Equal(t, "tcp", cfg.Net)
	require.Equal(t, "127.0.0.1:13329", cfg.Addr)
	require.Equal(t, "earning_test_scale", cfg.DBName)
	cfg.ParseTime = true
	cfg.Loc, e = time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, e)
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["lock_wait_timeout"] = "5"
	cfg.Params["innodb_lock_wait_timeout"] = "5"
	cfg.Params["time_zone"] = "'+00:00'"
	cfg.Params["sql_mode"] = "'STRICT_TRANS_TABLES,NO_AUTO_VALUE_ON_ZERO,ANSI_QUOTES,NO_BACKSLASH_ESCAPES'"
	open := func(name string) *gorm.DB {
		copy := *cfg
		copy.DBName = name
		db, e := gorm.Open(mysql.Open(copy.FormatDSN()), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
		pool, e := db.DB()
		require.NoError(t, e)
		pool.SetMaxOpenConns(4)
		t.Cleanup(func() { pool.Close() })
		return db
	}
	control := open(cfg.DBName)
	source := fmt.Sprintf("backend_test_%d", time.Now().UnixNano())
	target := source + "_restore"
	for _, name := range []string{source, target} {
		require.NoError(t, control.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
		defer control.Exec("DROP DATABASE `" + name + "`")
	}
	db := open(source)
	require.NoError(t, db.AutoMigrate(&sm.SysBaseMenu{}, &sm.SysAuthorityMenu{}, &sm.SysApi{}, &em.AdminPermission{}, &pm.Attempt{}))
	require.NoError(t, db.Create(&sm.SysBaseMenu{Name: "liveManagement", Path: "liveManagement"}).Error)
	require.NoError(t, db.Create(&sm.SysBaseMenu{Name: "payoutCenter", Path: "payoutCenter"}).Error)
	require.NoError(t, db.Create(&em.AdminPermission{AuthorityID: 888, Path: "/earning/settlement/review", Method: "POST"}).Error)
	// First column deliberately is not the PK; include arbitrary bytes, Unicode,
	// high-precision decimal, timestamp, explicit zero ID and generated columns.
	require.NoError(t, db.Exec("CREATE TABLE wallet_restore_fixture (same_sort INT NOT NULL, id BIGINT PRIMARY KEY AUTO_INCREMENT, binary_value VARBINARY(16), payload BLOB, text_value VARCHAR(64), amount DECIMAL(30,8), happened TIMESTAMP(6), json_value JSON, latin_text VARCHAR(64) CHARACTER SET latin1, default_stamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP, zero_date DATETIME, copied BIGINT GENERATED ALWAYS AS (same_sort+1) STORED) ENGINE=InnoDB").Error)
	require.NoError(t, db.Exec("SET SESSION sql_mode='STRICT_TRANS_TABLES,NO_AUTO_VALUE_ON_ZERO,ANSI_QUOTES,NO_BACKSLASH_ESCAPES'").Error)
	for _, id := range []int{2, 0, 1} {
		require.NoError(t, db.Exec("INSERT INTO wallet_restore_fixture(same_sort,id,binary_value,payload,text_value,amount,happened,json_value,latin_text,zero_date) VALUES(1,?,?,?,?,?,?,?,?,?)", id, []byte{0, 255, 128, 39, 92}, []byte{0, 1, 254, 255}, "引号'\\💛", "1234567890123456789012.12345678", "2026-11-12 01:02:03.123456", `{"name":"值"}`, "café", "0000-00-00 00:00:00").Error)
	}
	require.NoError(t, db.Exec("CREATE TRIGGER restore_immutable BEFORE UPDATE ON wallet_restore_fixture FOR EACH ROW BEGIN IF NOT (OLD.binary_value <=> NEW.binary_value) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='restore immutable proof'; END IF; END").Error)
	before, e := engagementcli.Proof(db)
	require.NoError(t, e)
	// Force an update precisely between proof collection and permission reads.
	var injected atomic.Bool
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("permission_snapshot_race", func(tx *gorm.DB) {
		if tx.Statement.Table == "anchor_finance_permission" && injected.CompareAndSwap(false, true) {
			tx.AddError(db.Model(&em.AdminPermission{}).Where("id=1").Update("path", "/earning/unexpected").Error)
		}
	}))
	oldProof, oldPermissions, e := financialSnapshot(db, true)
	require.NoError(t, e)
	require.True(t, injected.Load())
	require.Equal(t, before["anchor_finance_permission"], oldProof["anchor_finance_permission"])
	require.Equal(t, "/earning/settlement/review", oldPermissions[0].Path)
	require.NoError(t, db.Callback().Query().Remove("permission_snapshot_race"))
	changedProof, changedPermissions, e := financialSnapshot(db, true)
	require.NoError(t, e)
	require.NotEqual(t, oldProof["anchor_finance_permission"], changedProof["anchor_finance_permission"])
	require.False(t, safePermissionChanges(oldPermissions, changedPermissions))
	require.NoError(t, db.Model(&em.AdminPermission{}).Where("id=1").Update("path", "/earning/settlement/review").Error)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	readOnly, e := Execute(ctx, db, Options{})
	require.Error(t, e)
	require.Equal(t, "schema_verification", readOnly.Stage)
	for _, table := range newTables {
		require.False(t, db.Migrator().HasTable(table), "read-only default created %s", table)
	}
	dir := filepath.Join(t.TempDir(), "private")
	require.NoError(t, db.Exec("CREATE VIEW unbacked_view AS SELECT 1 AS value").Error)
	failed, e := Execute(ctx, db, Options{Migrate: true, BackupDir: dir})
	require.Error(t, e)
	require.Equal(t, "backup", failed.Stage)
	require.Empty(t, failed.BackupFile)
	for _, table := range newTables {
		require.False(t, db.Migrator().HasTable(table), "failed backup permitted mutation")
	}
	require.NoError(t, db.Exec("DROP VIEW unbacked_view").Error)
	// Kill only connections belonging to this freshly created local test DB,
	// after the backup starts. Both backup and advisory-lock cleanup must fail;
	// the report must retain the primary backup failure and remove its partial file.
	var interrupted bool
	failed, e = Execute(ctx, db, Options{Migrate: true, BackupDir: dir, Progress: func(p Progress) {
		if interrupted || p.BackupStep != "table_rows" {
			return
		}
		interrupted = true
		var connectionIDs []uint64
		require.NoError(t, control.Raw("SELECT ID FROM information_schema.PROCESSLIST WHERE DB=?", source).Scan(&connectionIDs).Error)
		require.NotEmpty(t, connectionIDs)
		for _, id := range connectionIDs {
			require.NoError(t, control.Exec(fmt.Sprintf("KILL CONNECTION %d", id)).Error)
		}
	}})
	require.True(t, interrupted)
	require.Error(t, e)
	require.Equal(t, "backup", e.Error())
	require.Equal(t, "backup", failed.Stage)
	require.True(t, failed.CleanupFailed)
	require.Equal(t, "table_rows", failed.LastBackupStep)
	require.NotEmpty(t, failed.LastBackupTableName)
	var backupFailure *engagementcli.BackupError
	require.True(t, errors.As(e, &backupFailure))
	require.Empty(t, failed.BackupFile)
	partial, e := os.ReadDir(dir)
	require.NoError(t, e)
	require.Empty(t, partial)
	for _, table := range newTables {
		require.False(t, db.Migrator().HasTable(table), "interrupted backup permitted mutation")
	}
	require.NoError(t, control.Connection(func(locker *gorm.DB) error {
		var got int
		require.NoError(t, locker.Raw("SELECT GET_LOCK(?,0)", migrationLockName(source)).Scan(&got).Error)
		require.Equal(t, 1, got)
		defer locker.Exec("SELECT RELEASE_LOCK(?)", migrationLockName(source))
		start := time.Now()
		blocked, e := Execute(ctx, db, Options{Migrate: true, BackupDir: dir})
		require.Error(t, e)
		require.Equal(t, "migration_lock", blocked.Stage)
		require.Empty(t, blocked.BackupFile)
		require.Less(t, time.Since(start), 8*time.Second)
		return nil
	}))
	first, e := Execute(ctx, db, Options{Migrate: true, Metadata: true, BackupDir: dir})
	require.NoError(t, e, "stage=%s", first.Stage)
	require.True(t, first.OK)
	require.Len(t, first.NewTables, len(newTables))
	require.Len(t, first.BackupSHA256, 64)
	second, e := Execute(ctx, db, Options{Migrate: true, Metadata: true, BackupDir: dir})
	require.NoError(t, e, "stage=%s", second.Stage)
	require.Empty(t, second.NewTables)
	verified, e := Execute(ctx, db, Options{})
	require.NoError(t, e)
	require.True(t, verified.Verified)
	require.Empty(t, verified.BackupFile)
	configFile := filepath.Join(t.TempDir(), "local.yaml")
	configYAML := fmt.Sprintf("system:\n  db-type: mysql\nmysql:\n  path: 127.0.0.1\n  port: \"13329\"\n  db-name: %q\n  username: %q\n  password: %q\n  config: \"charset=utf8mb4&parseTime=true&loc=UTC\"\n", source, cfg.User, cfg.Passwd)
	require.NoError(t, os.WriteFile(configFile, []byte(configYAML), 0600))
	var cliOutput bytes.Buffer
	require.Equal(t, 0, Run([]string{"-config", configFile}, &cliOutput), cliOutput.String())
	require.True(t, json.Valid(cliOutput.Bytes()))
	require.NotContains(t, cliOutput.String(), cfg.Passwd)
	require.NotContains(t, cliOutput.String(), cfg.Addr)
	require.NotContains(t, cliOutput.String(), "引号")
	var cliReport Report
	require.NoError(t, json.Unmarshal(cliOutput.Bytes(), &cliReport))
	require.True(t, cliReport.Verified)
	require.False(t, cliReport.Migrated)
	cliOutput.Reset()
	var diagnosticOutput bytes.Buffer
	require.Equal(t, 0, RunWithProgress([]string{"-config", configFile, "-migrate", "-backup-dir", dir, "-progress", "-read-timeout", "2m"}, &cliOutput, &diagnosticOutput), cliOutput.String())
	require.True(t, json.Valid(cliOutput.Bytes()), "stdout must contain exactly one JSON report")
	var sawFixture, sawSchema, sawRows bool
	for _, line := range strings.Split(strings.TrimSpace(diagnosticOutput.String()), "\n") {
		var progress Progress
		require.NoError(t, json.Unmarshal([]byte(line), &progress))
		if progress.Table != "" {
			require.Regexp(t, regexp.MustCompile(`^[a-zA-Z0-9_]+$`), progress.Table)
			require.Greater(t, progress.TableIndex, 0)
			require.LessOrEqual(t, progress.TableIndex, progress.TableTotal)
		}
		sawFixture = sawFixture || progress.Table == "wallet_restore_fixture"
		sawSchema = sawSchema || progress.BackupStep == "table_schema"
		sawRows = sawRows || progress.BackupStep == "table_rows"
	}
	require.True(t, sawFixture && sawSchema && sawRows)
	for _, secret := range []string{cfg.Passwd, cfg.Addr, "引号", "SELECT ", "CREATE TABLE ", "INSERT INTO "} {
		require.NotContains(t, diagnosticOutput.String(), secret)
		require.NotContains(t, cliOutput.String(), secret)
	}
	// Restore the pre-write whole-database SQL into a fresh isolated database.
	file, e := os.Open(first.BackupFile)
	require.NoError(t, e)
	defer file.Close()
	st, e := file.Stat()
	require.NoError(t, e)
	require.Equal(t, os.FileMode(0600), st.Mode().Perm())
	command := exec.CommandContext(ctx, "docker", "exec", "-i", "-e", "MYSQL_PWD="+cfg.Passwd, "payout-delivery-test-mysql-1", "mysql", "--binary-mode", "-u"+cfg.User, target)
	command.Stdin = file
	restoredOutput, e := command.CombinedOutput()
	require.NoError(t, e, "restore failed: %s", string(restoredOutput))
	restored := open(target)
	after, e := engagementcli.Proof(restored)
	require.NoError(t, e)
	require.Equal(t, before, after, "restored financial table proof differs")
	var bytesRestored []byte
	// Scan a binary scalar via database/sql so GORM does not infer a row slice.
	row := restored.Raw("SELECT binary_value FROM wallet_restore_fixture WHERE id=0").Row()
	require.NoError(t, row.Scan(&bytesRestored))
	require.Equal(t, []byte{0, 255, 128, 39, 92}, bytesRestored)
	require.Error(t, restored.Exec("UPDATE wallet_restore_fixture SET binary_value=X'00' WHERE id=0").Error)
	var originalTriggers, restoredTriggers []struct{ Name, Body, SQLMode string }
	query := "SELECT TRIGGER_NAME AS name,ACTION_STATEMENT AS body,SQL_MODE AS sql_mode FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() ORDER BY TRIGGER_NAME"
	require.NoError(t, db.Raw(query).Scan(&originalTriggers).Error)
	require.NoError(t, restored.Raw(query).Scan(&restoredTriggers).Error)
	var originalTestTrigger any
	for _, trigger := range originalTriggers {
		if trigger.Name == "restore_immutable" {
			originalTestTrigger = trigger
		}
	}
	require.Len(t, restoredTriggers, 1)
	require.Equal(t, originalTestTrigger, restoredTriggers[0])
	backupBytes, e := os.ReadFile(first.BackupFile)
	require.NoError(t, e)
	require.NotContains(t, string(backupBytes), "DROP TABLE")
	require.NotContains(t, string(backupBytes), "USE `")
	require.NotContains(t, string(backupBytes), cfg.Passwd)
	require.True(t, strings.HasSuffix(string(backupBytes), "-- COMPLETE\n"))
	require.NoError(t, initialize.ValidateEarningAuditSchema(db))
	require.NoError(t, initialize.ValidateEventDeliverySchema(db))
	require.NoError(t, initialize.ValidateBankReconcileSchema(db))
	testScopedBackupAndRestore(t, ctx, db, control, source, cfg, configFile)
	t.Logf("two additive migrations and read-only verify passed; newTables=%d; restored financial tables=%d; binary/zero-ID/generated/timestamp/trigger proof identical", len(first.NewTables), len(before))
}
