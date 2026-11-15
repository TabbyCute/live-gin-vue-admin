// Package backendcli runs this release's additive migrations. It never runs old
// wallet/gift/earning/payout migrations or uses historical reports to move money.
package backendcli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"tb_live_module/config"
	"tb_live_module/initialize"
	"tb_live_module/internal/engagementcli"
	em "tb_live_module/model/earning"
	"time"
)

type Options struct {
	Migrate, Metadata bool
	BackupDir         string
	BackupScope       string
	Progress          func(Progress)
}
type Progress struct {
	Stage      string `json:"stage"`
	BackupStep string `json:"backupStep,omitempty"`
	TableIndex int    `json:"tableIndex,omitempty"`
	TableTotal int    `json:"tableTotal,omitempty"`
	Table      string `json:"table,omitempty"`
}

type Report struct {
	FailureCode         string   `json:"failureCode,omitempty"`
	CleanupFailed       bool     `json:"cleanupFailed"`
	CleanupFailureCode  string   `json:"cleanupFailureCode,omitempty"`
	LastBackupStep      string   `json:"lastBackupStep,omitempty"`
	LastBackupTable     int      `json:"lastBackupTable,omitempty"`
	LastBackupTableName string   `json:"lastBackupTableName,omitempty"`
	OK                  bool     `json:"ok"`
	Stage               string   `json:"stage"`
	Verified            bool     `json:"verified"`
	Migrated            bool     `json:"migrated"`
	MetadataSynced      bool     `json:"metadataSynced"`
	BackupFile          string   `json:"backupFile,omitempty"`
	BackupSHA256        string   `json:"backupSHA256,omitempty"`
	BackupScope         string   `json:"backupScope"`
	BackupTables        int      `json:"backupTables,omitempty"`
	ProtectedTables     int      `json:"protectedFinancialTables"`
	FinancialUnchanged  bool     `json:"financialUnchanged"`
	NewTables           []string `json:"newTables"`
	ApplicationDeployed bool     `json:"applicationDeployed"`
}

var newTables = []string{"anchor_earning_audit_job", "event_delivery", "event_delivery_attempt", "event_delivery_cursor", "event_delivery_event", "event_delivery_retry_audit", "payout_bank_audit", "payout_bank_batch", "payout_bank_batch_line", "payout_bank_check", "payout_bank_line", "payout_bank_request", "payout_bank_review", "payout_bank_run"}

// Run emits only a structured summary, including failures. Configuration, SQL
// errors and row contents never reach stdout/stderr.
func Run(args []string, output io.Writer) int { return RunWithProgress(args, output, os.Stderr) }

// RunWithProgress accepts a separate diagnostic stream so tests can verify that
// opt-in progress never mixes into the single stdout JSON report.
func RunWithProgress(args []string, output, diagnostics io.Writer) int {
	report := Report{Stage: "arguments", NewTables: []string{}, BackupScope: engagementcli.BackupScopeFull}
	finish := func(code int) int {
		if json.NewEncoder(output).Encode(report) != nil {
			return 1
		}
		return code
	}
	flags := flag.NewFlagSet("backend-migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	file := flags.String("config", "", "MySQL YAML configuration")
	migrate := flags.Bool("migrate", false, "create only new backend module schema")
	metadata := flags.Bool("metadata", false, "add only new module menu/API metadata")
	verify := flags.Bool("verify", false, "read-only verification (default)")
	backup := flags.String("backup-dir", "", "absolute private 0700 backup directory required for writes")
	backupScope := flags.String("backup-scope", engagementcli.BackupScopeFull, "full or financial-and-metadata (fixed migration-related tables and FK parents)")
	timeout := flags.Duration("timeout", 15*time.Minute, "deadline between 1 and 15 minutes")
	readTimeout := flags.Duration("read-timeout", 2*time.Minute, "per-read network deadline between 1 second and 2 minutes")
	progress := flags.Bool("progress", false, "emit stages and validated backup table identifiers as JSON to stderr")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *file == "" || *timeout < time.Minute || *timeout > 15*time.Minute || *readTimeout < time.Second || *readTimeout > 2*time.Minute || !validBackupScope(*backupScope) || (*verify && (*migrate || *metadata)) {
		return finish(1)
	}
	report.BackupScope = *backupScope
	if (*migrate || *metadata) && !privateDirectory(*backup) {
		report.Stage = "backup_directory"
		return finish(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report.Stage = "configuration"
	v := viper.New()
	v.SetConfigFile(*file)
	var cfg config.Server
	if v.ReadInConfig() != nil || v.Unmarshal(&cfg) != nil || cfg.System.DbType != "mysql" {
		return finish(1)
	}
	mc, e := mysqlDriver.ParseDSN(cfg.Mysql.Dsn())
	if e != nil || mc.DBName == "" {
		return finish(1)
	}
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Timeout = 5 * time.Second
	mc.ReadTimeout = *readTimeout
	mc.WriteTimeout = 30 * time.Second
	if mc.Params == nil {
		mc.Params = map[string]string{}
	}
	// Applies to both the pinned orchestration connection and the bank migration's
	// own financial migration connection. The latter has a 10s index-DDL budget.
	mc.Params["lock_wait_timeout"] = "5"
	mc.Params["innodb_lock_wait_timeout"] = "5"
	mc.Params["time_zone"] = "'+00:00'"
	report.Stage = "connection"
	mc.Logger = silentDriverLogger{}
	connector, e := mysqlDriver.NewConnector(mc)
	if e != nil {
		report.FailureCode = safeFailureCode(e)
		return finish(1)
	}
	pool := sql.OpenDB(connector)
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	// GORM's initial version probe uses a background context. Bind that probe
	// to the command deadline while preserving the driver's version detection.
	initialization := &initializationPool{DB: pool, ctx: ctx}
	db, e := gorm.Open(mysql.New(mysql.Config{Conn: initialization}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		report.FailureCode = safeFailureCode(e)
		return finish(1)
	}
	db.ConnPool = pool
	db.Statement.ConnPool = pool
	opts := Options{Migrate: *migrate, Metadata: *metadata, BackupDir: *backup, BackupScope: *backupScope}
	if *progress {
		encoder := json.NewEncoder(diagnostics)
		opts.Progress = func(p Progress) { _ = encoder.Encode(p) }
	}
	report, e = Execute(ctx, db, opts)
	if e != nil {
		return finish(1)
	}
	return finish(0)
}

func privateDirectory(dir string) bool {
	if !filepath.IsAbs(dir) {
		return false
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return false
	}
	st, e := os.Lstat(dir)
	return e == nil && st.IsDir() && st.Mode().Perm() == 0700
}

// Execute is exposed for guarded local integration tests; production uses Run's
// YAML loader and finite database/session budgets.
func Execute(ctx context.Context, db *gorm.DB, opts Options) (report Report, err error) {
	if opts.BackupScope == "" {
		opts.BackupScope = engagementcli.BackupScopeFull
	}
	report = Report{Stage: "preflight", NewTables: []string{}}
	if !validBackupScope(opts.BackupScope) {
		report.Stage = "backup_scope"
		return report, fmt.Errorf("backup_scope")
	}
	report.BackupScope = opts.BackupScope
	phase := func(stage string) {
		report.Stage = stage
		if opts.Progress != nil {
			opts.Progress(Progress{Stage: stage})
		}
	}
	phase("preflight")
	if db == nil || db.Dialector.Name() != "mysql" {
		return report, fmt.Errorf("preflight")
	}
	mutate := opts.Migrate || opts.Metadata
	if mutate && !privateDirectory(opts.BackupDir) {
		phase("backup_directory")
		return report, fmt.Errorf("backup_directory")
	}
	db = db.WithContext(ctx)
	err = db.Connection(func(conn *gorm.DB) (runErr error) {
		conn = conn.Session(&gorm.Session{NewDB: true})
		if e := conn.Exec("SET SESSION lock_wait_timeout=5").Error; e != nil {
			return e
		}
		if e := conn.Exec("SET SESSION innodb_lock_wait_timeout=5").Error; e != nil {
			return e
		}
		var database string
		if e := conn.Raw("SELECT DATABASE()").Scan(&database).Error; e != nil {
			return e
		}
		if database == "" {
			return fmt.Errorf("preflight")
		}
		if mutate {
			phase("migration_lock")
			lock := migrationLockName(database)
			// Even a lost GET_LOCK reply must not leak a named lock into the pool.
			defer func() {
				cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				if e := conn.WithContext(cleanup).Exec("SELECT RELEASE_LOCK(?)", lock).Error; e != nil {
					if pinned, ok := conn.Statement.ConnPool.(*sql.Conn); ok {
						_ = pinned.Raw(func(any) error { return driver.ErrBadConn })
					}
					recordCleanupFailure(&report, &runErr, e)
				}
			}()
			var got sql.NullInt64
			if e := conn.Raw("SELECT GET_LOCK(?,5)", lock).Scan(&got).Error; e != nil {
				return e
			}
			if !got.Valid || got.Int64 != 1 {
				return fmt.Errorf("migration_lock")
			}
		}
		phase("financial_preflight")
		before, permissions, e := financialSnapshot(conn, opts.Metadata)
		if e != nil {
			return e
		}
		report.ProtectedTables = len(before)
		existed := map[string]bool{}
		for _, table := range newTables {
			existed[table] = conn.Migrator().HasTable(table)
		}
		if mutate {
			phase("backup")
			backupProgress := func(p engagementcli.BackupProgress) {
				report.LastBackupStep, report.LastBackupTable, report.LastBackupTableName = p.Step, p.TableIndex, p.Table
				if p.TableTotal > 0 {
					report.BackupTables = p.TableTotal
				}
				if opts.Progress != nil {
					opts.Progress(Progress{Stage: "backup", BackupStep: p.Step, TableIndex: p.TableIndex, TableTotal: p.TableTotal, Table: p.Table})
				}
			}
			if opts.BackupScope == engagementcli.BackupScopeFinancialAndMetadata {
				required := make([]string, 0, len(before))
				for table := range before {
					required = append(required, table)
				}
				report.BackupFile, report.BackupSHA256, e = engagementcli.BackupFinancialAndMetadata(conn, opts.BackupDir, required, backupProgress)
			} else {
				report.BackupFile, report.BackupSHA256, e = engagementcli.BackupAllWithProgress(conn, opts.BackupDir, backupProgress)
			}
			if e != nil {
				return e
			}
			// There is no write before the complete private snapshot is fsynced.
			if opts.Migrate {
				phase("earning_audit_schema")
				if e = initialize.MigrateEarningAudit(conn); e != nil {
					return e
				}
				phase("event_delivery_schema")
				if e = initialize.MigrateEventDelivery(conn); e != nil {
					return e
				}
				phase("bank_reconciliation_schema")
				if e = initialize.MigrateBankReconcile(db); e != nil {
					return e
				}
				report.Migrated = true
			}
		}
		phase("schema_verification")
		if e = initialize.ValidateEarningAuditSchema(conn); e != nil {
			return e
		}
		if e = initialize.ValidateEventDeliverySchema(conn); e != nil {
			return e
		}
		if e = initialize.ValidateBankReconcileSchema(conn); e != nil {
			return e
		}
		report.Verified = true
		if opts.Metadata {
			phase("metadata")
			if e = initialize.SyncEarningAuditMetadata(conn); e != nil {
				return e
			}
			if e = initialize.SyncEventDeliveryMetadata(conn); e != nil {
				return e
			}
			if e = initialize.SyncBankReconcileMetadata(conn); e != nil {
				return e
			}
			report.MetadataSynced = true
		}
		phase("financial_postflight")
		after, afterPermissions, e := financialSnapshot(conn, opts.Metadata)
		if e != nil {
			return e
		}
		if opts.Metadata && !safePermissionChanges(permissions, afterPermissions) {
			return fmt.Errorf("permission_delta")
		}
		if !unchangedFinancial(before, after, opts.Metadata) {
			return fmt.Errorf("financial_proof_changed")
		}
		for _, table := range newTables {
			if existed[table] {
				continue
			}
			var count int64
			if e = conn.Table(table).Count(&count).Error; e != nil {
				return e
			}
			if count != 0 {
				return fmt.Errorf("new_table_not_empty")
			}
			report.NewTables = append(report.NewTables, table)
		}
		sort.Strings(report.NewTables)
		report.FinancialUnchanged = true
		return nil
	})
	if err != nil {
		report.FailureCode = safeFailureCode(err)
		if report.FailureCode == "" && ctx.Err() != nil {
			report.FailureCode = safeFailureCode(ctx.Err())
		}
		return report, &stageError{stage: report.Stage, cause: err}
	}
	report.OK = true
	phase("complete")
	return report, nil
}

func validBackupScope(scope string) bool {
	return scope == engagementcli.BackupScopeFull || scope == engagementcli.BackupScopeFinancialAndMetadata
}

func unchangedFinancial(before, after map[string]engagementcli.TableProof, metadata bool) bool {
	allowed := map[string]bool{}
	for _, name := range newTables {
		allowed[name] = true
	}
	for name, want := range before {
		got, ok := after[name]
		if !ok {
			return false
		}
		if metadata && name == "anchor_finance_permission" {
			continue
		}
		if got != want {
			return false
		}
	}
	for name, got := range after {
		if _, ok := before[name]; !ok && (!allowed[name] || got.Rows != 0) {
			return false
		}
	}
	return true
}

func safePermissionChanges(before, after []em.AdminPermission) bool {
	old := map[uint64]em.AdminPermission{}
	for _, p := range before {
		old[p.ID] = p
	}
	for _, p := range after {
		if want, ok := old[p.ID]; ok {
			if p != want {
				return false
			}
			delete(old, p.ID)
			continue
		}
		if p.AuthorityID != 888 {
			return false
		}
		if !((p.Path == "/earning/audit/start" || p.Path == "/earning/audit/cancel") && p.Method == "POST") && !(p.Path == "/earning/audit/list" && p.Method == "GET") {
			return false
		}
	}
	return len(old) == 0
}

func financialSnapshot(db *gorm.DB, metadata bool) (proof map[string]engagementcli.TableProof, permissions []em.AdminPermission, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		var e error
		proof, e = engagementcli.ProofInSnapshot(tx)
		if e != nil {
			return e
		}
		if metadata {
			return tx.Order("id").Find(&permissions).Error
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return proof, permissions, err
}

func migrationLockName(database string) string {
	digest := sha256.Sum256([]byte(database))
	return fmt.Sprintf("backend:migrate:%x", digest[:22])
}

// All Error() strings are safe even if another CLI prints these errors directly.
type stageError struct {
	stage string
	cause error
}

func (e *stageError) Error() string { return e.stage }
func (e *stageError) Unwrap() error { return e.cause }

func safeFailureCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "timeout"
	}
	var mysqlError *mysqlDriver.MySQLError
	if errors.As(err, &mysqlError) {
		return fmt.Sprintf("mysql_%d", mysqlError.Number)
	}
	return ""
}

func recordCleanupFailure(report *Report, primary *error, cleanup error) {
	report.CleanupFailed = true
	report.CleanupFailureCode = safeFailureCode(cleanup)
	if *primary == nil {
		report.Stage = "migration_lock_release"
		*primary = &stageError{stage: report.Stage, cause: cleanup}
	}
}

type silentDriverLogger struct{}

func (silentDriverLogger) Print(...any) {}

// Used only during gorm.Open; normal operations use the original *sql.DB.
type initializationPool struct {
	*sql.DB
	ctx context.Context
}

func (p *initializationPool) QueryRowContext(_ context.Context, query string, args ...any) *sql.Row {
	return p.DB.QueryRowContext(p.ctx, query, args...)
}
