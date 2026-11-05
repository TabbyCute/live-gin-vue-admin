package main

import (
	"context"
	"encoding/json"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMySQLPrivateBackupAndFinancialProof(t *testing.T) {
	raw := os.Getenv("PAYOUT_CLI_TEST_MYSQL_DSN")
	if raw == "" {
		t.Skip("isolated migration CLI fixture only")
	}
	cfg, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	require.Equal(t, "127.0.0.1:13328", cfg.Addr)
	require.Equal(t, "payout_cli_test", cfg.DBName)
	db, e := gorm.Open(mysql.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { pool.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	for _, table := range []string{"anchor_withdrawable_ledger", "payout_order", "payout_status_history", "payout_audit_log"} {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+table+"`").Error)
		require.NoError(t, db.Exec("CREATE TABLE `"+table+"` (id bigint PRIMARY KEY,amount bigint NOT NULL,marker varchar(100) NOT NULL)").Error)
		require.NoError(t, db.Exec("INSERT INTO `"+table+"` VALUES (1,100,'isolated backup fixture')").Error)
	}
	before, e := financialProof(db)
	require.NoError(t, e)
	private := filepath.Join(t.TempDir(), "private")
	require.NoError(t, backupFinancialTables(db, private))
	files, e := os.ReadDir(private)
	require.NoError(t, e)
	require.Len(t, files, 1)
	info, e := files[0].Info()
	require.NoError(t, e)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	b, e := os.ReadFile(filepath.Join(private, files[0].Name()))
	require.NoError(t, e)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	require.GreaterOrEqual(t, len(lines), 10)
	var last map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &last))
	require.Equal(t, true, last["complete"])
	for _, table := range []string{"anchor_withdrawable_ledger", "payout_order", "payout_status_history", "payout_audit_log"} {
		require.Contains(t, string(b), `"table":"`+table+`"`)
	}
	require.Error(t, backupFinancialTables(db, "relative"))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(private, link))
	require.Error(t, backupFinancialTables(db, link))
	require.NoError(t, db.Exec("ALTER TABLE anchor_withdrawable_ledger ADD paid_before bigint NOT NULL DEFAULT 0, ADD paid_after bigint NOT NULL DEFAULT 0").Error)
	require.NoError(t, db.Exec("ALTER TABLE payout_order ADD state_version bigint NOT NULL DEFAULT 1, ADD last_transition_no varchar(40) NOT NULL DEFAULT ''").Error)
	require.NoError(t, db.Exec("ALTER TABLE payout_status_history ADD sequence bigint NOT NULL DEFAULT 1, ADD attempt_no varchar(40) NOT NULL DEFAULT '', ADD request_id varchar(80) NOT NULL DEFAULT '', ADD reason varchar(500) NOT NULL DEFAULT ''").Error)
	require.NoError(t, db.Exec("ALTER TABLE payout_audit_log ADD object_no varchar(40) NOT NULL DEFAULT '', ADD transition_no varchar(40) NOT NULL DEFAULT ''").Error)
	after, e := financialProof(db)
	require.NoError(t, e)
	require.Equal(t, before, after, "default-only upgrade preserves canonical historical facts")
}
