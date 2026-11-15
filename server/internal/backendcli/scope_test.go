package backendcli

import (
	"bytes"
	"context"
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
)

func testScopedBackupAndRestore(t *testing.T, ctx context.Context, db, control *gorm.DB, source string, cfg *driver.Config, configFile string) {
	t.Helper()
	for _, statement := range []string{
		"CREATE TABLE scope_grandparent(id BIGINT PRIMARY KEY) ENGINE=InnoDB",
		"CREATE TABLE scope_parent(id BIGINT PRIMARY KEY, parent_id BIGINT, FOREIGN KEY(parent_id) REFERENCES scope_grandparent(id)) ENGINE=InnoDB",
		"CREATE TABLE wallet_scope_fixture(id BIGINT PRIMARY KEY, parent_id BIGINT, FOREIGN KEY(parent_id) REFERENCES scope_parent(id)) ENGINE=InnoDB",
		"INSERT INTO scope_grandparent VALUES(1)", "INSERT INTO scope_parent VALUES(2,1)", "INSERT INTO wallet_scope_fixture VALUES(3,2)",
		"CREATE TABLE gva_announcements_info(id BIGINT PRIMARY KEY, body TEXT) ENGINE=InnoDB",
		"INSERT INTO gva_announcements_info VALUES(1,'unrelated content must stay outside scoped backup')",
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	for _, table := range []string{"gift_order", "gift_refund_order", "gift_revenue_ledger"} {
		require.NoError(t, db.Exec("CREATE TABLE `"+table+"` (id BIGINT PRIMARY KEY, value VARBINARY(8)) ENGINE=InnoDB").Error)
		require.NoError(t, db.Exec("INSERT INTO `"+table+"` VALUES(1,X'00ff')").Error)
	}
	before, e := engagementcli.Proof(db)
	require.NoError(t, e)
	required := make([]string, 0, len(before))
	for table := range before {
		required = append(required, table)
	}

	// Model the observed inaccessible unrelated table without touching a remote
	// server or holding global locks. A selected table failure must never skip.
	blocked := "gva_announcements_info"
	var blockedRead bool
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("scope_read_failure", func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "SHOW CREATE TABLE `"+blocked+"`") {
			blockedRead = true
			tx.AddError(errors.New("test table read unavailable"))
		}
	}))
	dir := filepath.Join(t.TempDir(), "private")
	_, _, e = engagementcli.BackupAll(db, dir)
	require.Error(t, e)
	require.True(t, blockedRead, "full scope must still attempt unrelated tables")
	blockedRead = false
	file, _, e := engagementcli.BackupFinancialAndMetadata(db, dir, required, nil)
	require.NoError(t, e)
	require.False(t, blockedRead)
	data, e := os.ReadFile(file)
	require.NoError(t, e)
	require.Contains(t, string(data), "scope: financial-and-metadata")
	require.NotContains(t, string(data), "gva_announcements_info")
	require.NotContains(t, string(data), "unrelated content")
	for _, table := range append(required, "scope_parent", "scope_grandparent", "sys_base_menus", "sys_authority_menus", "sys_apis", "event_delivery") {
		require.Contains(t, string(data), "CREATE TABLE \""+table+"\"", table)
	}
	blocked = "wallet_scope_fixture"
	failedDir := filepath.Join(t.TempDir(), "private")
	failed, e := Execute(ctx, db, Options{Migrate: true, Metadata: true, BackupDir: failedDir, BackupScope: engagementcli.BackupScopeFinancialAndMetadata})
	require.Error(t, e)
	require.Equal(t, "backup", failed.Stage)
	require.False(t, failed.Migrated || failed.MetadataSynced)
	partial, e := os.ReadDir(failedDir)
	require.NoError(t, e)
	require.Empty(t, partial)
	require.NoError(t, db.Callback().Row().Remove("scope_read_failure"))
	_, _, e = engagementcli.BackupFinancialAndMetadata(db, failedDir, append(required, "gva_announcements_info"), nil)
	require.Error(t, e, "required proof coverage cannot add arbitrary tables")
	require.NoError(t, db.Exec("CREATE TRIGGER scope_external BEFORE UPDATE ON wallet_scope_fixture FOR EACH ROW BEGIN IF EXISTS(SELECT 1 FROM gva_announcements_info WHERE id=NEW.id) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='dependency'; END IF; END").Error)
	_, _, e = engagementcli.BackupFinancialAndMetadata(db, failedDir, required, nil)
	require.Error(t, e, "unbacked trigger dependency must fail closed")
	require.NoError(t, db.Exec("DROP TRIGGER scope_external").Error)

	// The public flag selects only the fixed scope and records it in the report.
	var output, progress bytes.Buffer
	require.Equal(t, 0, RunWithProgress([]string{"-config", configFile, "-migrate", "-metadata", "-backup-dir", dir, "-backup-scope", "financial-and-metadata", "-progress"}, &output, &progress), output.String())
	var report Report
	require.NoError(t, json.Unmarshal(output.Bytes(), &report))
	require.Equal(t, engagementcli.BackupScopeFinancialAndMetadata, report.BackupScope)
	require.GreaterOrEqual(t, report.BackupTables, len(before)+3+5+2)
	require.NotContains(t, progress.String(), "gva_announcements_info")
	target := source + "_scope_restore"
	require.NoError(t, control.Exec("CREATE DATABASE `"+target+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
	defer control.Exec("DROP DATABASE `" + target + "`")
	restoreSQL(t, ctx, cfg, "payout-delivery-test-mysql-1", target, report.BackupFile)
	copy := *cfg
	copy.DBName = target
	restored, e := gorm.Open(mysql.Open(copy.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := restored.DB()
	require.NoError(t, e)
	defer pool.Close()
	after, e := engagementcli.Proof(restored)
	require.NoError(t, e)
	require.Equal(t, before, after)
	require.False(t, restored.Migrator().HasTable("gva_announcements_info"))
	var parents int64
	require.NoError(t, restored.Table("scope_parent").Count(&parents).Error)
	require.Equal(t, int64(1), parents)
	require.Error(t, restored.Exec("INSERT INTO wallet_scope_fixture VALUES(4,999)").Error, "FK restored")
	require.Error(t, restored.Exec("UPDATE wallet_restore_fixture SET binary_value=X'00' WHERE id=0").Error, "trigger restored")
	t.Logf("scoped backup restore passed: protected=%d backedUp=%d; unrelated blocked table excluded, selected read failure blocked, 2-level FK parents and gift sources restored", len(before), report.BackupTables)
}

func restoreSQL(t *testing.T, ctx context.Context, cfg *driver.Config, container, target, backup string) {
	t.Helper()
	file, e := os.Open(backup)
	require.NoError(t, e)
	defer file.Close()
	command := exec.CommandContext(ctx, "docker", "exec", "-i", "-e", "MYSQL_PWD="+cfg.Passwd, container, "mysql", "--binary-mode", "-u"+cfg.User, target)
	command.Stdin = file
	output, e := command.CombinedOutput()
	require.NoError(t, e, "restore failed: %s", output)
}

// Reads a complete local financial schema without modifying that source. The
// restore is the only DB write and uses a newly created, uniquely named DB.
func TestMySQLScopedBackupRealFinancialTriggers(t *testing.T) {
	raw := os.Getenv("BACKEND_SCOPE_SOURCE_TEST_DSN")
	if raw == "" {
		t.Skip("complete isolated local financial schema required")
	}
	cfg, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	require.Equal(t, "tcp", cfg.Net)
	require.Equal(t, "127.0.0.1:13328", cfg.Addr)
	require.Equal(t, "earning_test_delivery", cfg.DBName)
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	before, e := engagementcli.Proof(db)
	require.NoError(t, e)
	required := make([]string, 0, len(before))
	for table := range before {
		required = append(required, table)
	}
	file, digest, e := engagementcli.BackupFinancialAndMetadata(db, filepath.Join(t.TempDir(), "private"), required, nil)
	require.NoError(t, e)
	require.Len(t, digest, 64)
	afterSource, e := engagementcli.Proof(db)
	require.NoError(t, e)
	require.Equal(t, before, afterSource, "source must remain untouched")
	target := fmt.Sprintf("backend_scope_real_%d", time.Now().UnixNano())
	require.NoError(t, db.Exec("CREATE DATABASE `"+target+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
	defer db.Exec("DROP DATABASE `" + target + "`")
	restoreSQL(t, ctx, cfg, "earning-delivery-test-mysql-1", target, file)
	copy := *cfg
	copy.DBName = target
	restored, e := gorm.Open(mysql.Open(copy.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	restorePool, e := restored.DB()
	require.NoError(t, e)
	defer restorePool.Close()
	after, e := engagementcli.Proof(restored)
	require.NoError(t, e)
	require.Equal(t, before, after)
	var sourceTriggers, restoredTriggers []struct{ Name, Body, SQLMode string }
	query := "SELECT TRIGGER_NAME AS name,ACTION_STATEMENT AS body,SQL_MODE AS sql_mode FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND (LEFT(EVENT_OBJECT_TABLE,7) IN ('wallet_','anchor_','payout_') OR EVENT_OBJECT_TABLE IN ('gift_order','gift_refund_order','gift_revenue_ledger')) ORDER BY TRIGGER_NAME"
	require.NoError(t, db.Raw(query).Scan(&sourceTriggers).Error)
	require.NoError(t, restored.Raw(query).Scan(&restoredTriggers).Error)
	require.NotEmpty(t, sourceTriggers)
	require.Equal(t, sourceTriggers, restoredTriggers)
	t.Logf("real scoped restore: %d financial proofs and %d complete financial triggers identical; source unchanged", len(before), len(sourceTriggers))
}
