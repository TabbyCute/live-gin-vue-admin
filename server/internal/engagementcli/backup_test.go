package engagementcli

import (
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"tb_live_module/initialize"
	model "tb_live_module/model/guard"
	"testing"
	"time"
)

func TestMySQLBackupRestoresSchemaDataAndTriggers(t *testing.T) {
	raw := os.Getenv("GIFT_TEST_MYSQL_DSN")
	if raw == "" {
		t.Skip("isolated local MySQL required")
	}
	cfg, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	require.Equal(t, "127.0.0.1:13327", cfg.Addr)
	require.True(t, strings.HasPrefix(cfg.DBName, "gift_test_"))
	control, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := control.DB()
	require.NoError(t, e)
	defer pool.Close()
	suffix := time.Now().UnixNano()
	source := fmt.Sprintf("gift_test_backup_%d", suffix)
	target := source + "_restore"
	for _, name := range []string{source, target} {
		require.NoError(t, control.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
		defer control.Exec("DROP DATABASE `" + name + "`")
	}
	cfg.DBName = source
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	p, e := db.DB()
	require.NoError(t, e)
	defer p.Close()
	require.NoError(t, initialize.MigrateGuard(db))
	preflight, e := Preflight(db)
	require.NoError(t, e)
	require.NotEmpty(t, preflight["tables"])
	plan := model.Plan{PlanNo: "restore_plan", Name: "守护 ' \" 💛", Level: 1, Priority: 1, DefaultDurationDays: 30, BenefitConfig: `{"badge":"守护"}`, Status: "disabled", Version: 1}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.Ledger{LedgerNo: "restore_ledger", MembershipNo: "member01", EntryType: "grant", SourceNo: "source01", BeforeSnapshot: "[]", AfterSnapshot: `[{"name":"恢复"}]`}).Error)
	dir := filepath.Join(t.TempDir(), "private")
	file, digest, e := Backup(db, dir)
	require.NoError(t, e)
	require.Len(t, digest, 64)
	info, e := os.Stat(file)
	require.NoError(t, e)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	f, e := os.Open(file)
	require.NoError(t, e)
	defer f.Close()
	compose, e := filepath.Abs("../../tests/gift/compose.yaml")
	require.NoError(t, e)
	command := exec.Command("docker", "compose", "-f", compose, "exec", "-T", "mysql", "mysql", "-uroot", "-pgift-local-test-only", target)
	command.Stdin = f
	out, e := command.CombinedOutput()
	require.NoError(t, e, string(out))
	cfg.DBName = target
	restored, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	rp, e := restored.DB()
	require.NoError(t, e)
	defer rp.Close()
	require.NoError(t, initialize.ValidateGuardSchema(restored))
	var got model.Plan
	require.NoError(t, restored.Where("plan_no=?", plan.PlanNo).Take(&got).Error)
	require.Equal(t, plan.Name, got.Name)
	require.Equal(t, plan.BenefitConfig, got.BenefitConfig)
	require.Error(t, restored.Model(&model.Ledger{}).Where("ledger_no='restore_ledger'").Update("after_snapshot", "bad").Error)
	require.Error(t, restored.Model(&model.Plan{}).Where("plan_no='restore_plan'").Update("plan_no", "renamed").Error)
}
