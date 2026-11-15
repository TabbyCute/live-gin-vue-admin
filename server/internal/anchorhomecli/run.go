// Package anchorhomecli applies only the viewer homepage album schema. It never
// migrates existing wallet, gift, earning, payout, social or other business modules.
package anchorhomecli

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
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
	"syscall"
	"tb_live_module/config"
	"tb_live_module/initialize"
	"tb_live_module/internal/engagementcli"
	album "tb_live_module/service/anchorhome"
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
	MetadataPreserved   bool     `json:"existingMetadataUnchanged"`
	AccountsPreserved   bool     `json:"existingAccountAnchorUnchanged"`
	AccountProtection   string   `json:"accountAnchorProtection"`
	NewTables           []string `json:"newTables"`
	ApplicationDeployed bool     `json:"applicationDeployed"`
}

var newTables = []string{"anchor_home_album", "anchor_home_photo", "anchor_home_command", "anchor_home_audit"}

// Run emits only a structured summary, including failures. Configuration, SQL
// errors and row contents never reach stdout/stderr.
func Run(args []string, output io.Writer) int { return RunWithProgress(args, output, os.Stderr) }

// RunWithProgress accepts a separate diagnostic stream so tests can verify that
// opt-in progress never mixes into the single stdout JSON report.
func RunWithProgress(args []string, output, diagnostics io.Writer) int {
	report := Report{Stage: "arguments", NewTables: []string{}, BackupScope: engagementcli.BackupScopeFinancialAndMetadata}
	finish := func(code int) int {
		if json.NewEncoder(output).Encode(report) != nil {
			return 1
		}
		return code
	}
	flags := flag.NewFlagSet("anchor-home-migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	file := flags.String("config", "", "MySQL YAML configuration")
	migrate := flags.Bool("migrate", false, "create only new anchor-home album schema")
	metadata := flags.Bool("metadata", false, "add only new module menu/API metadata")
	verify := flags.Bool("verify", false, "read-only verification (default)")
	backup := flags.String("backup-dir", "", "absolute private 0700 backup directory required for writes")
	backupScope := flags.String("backup-scope", engagementcli.BackupScopeFinancialAndMetadata, "financial-and-metadata only: fixed financial/module/metadata tables and recursive FK parents")
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
	// Bound metadata and row-lock waits independently of the total deadline.
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
		opts.BackupScope = engagementcli.BackupScopeFinancialAndMetadata
	}
	report = Report{Stage: "preflight", NewTables: []string{}, AccountProtection: "proof-only-no-mutation"}
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
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
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
				return safeGuardFailure("migration_lock")
			}
		}
		if mutate {
			phase("existing_schema_verification")
			if e := album.VerifyExistingAlbumSchema(conn); e != nil {
				return e
			}
		}
		phase("financial_preflight")
		before, metadataBefore, accountsBefore, e := snapshot(conn)
		if e != nil {
			return e
		}
		report.ProtectedTables = len(before)
		existed := map[string]bool{}
		for _, table := range newTables {
			_, existed[table] = before[table]
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
			required := make([]string, 0, len(before))
			for table := range before {
				required = append(required, table)
			}
			report.BackupFile, report.BackupSHA256, e = engagementcli.BackupFinancialAndMetadata(conn, opts.BackupDir, required, backupProgress)
			if e != nil {
				return e
			}
			// There is no write before the complete private snapshot is fsynced.
			if opts.Migrate {
				phase("anchor_home_schema")
				if e = album.MigrateAlbumSchema(conn); e != nil {
					return e
				}
				report.Migrated = true
			}
		}
		phase("schema_verification")
		if e = album.VerifyAlbumSchema(conn); e != nil {
			return e
		}
		report.Verified = true
		if opts.Metadata {
			phase("metadata")
			if e = initialize.SyncAnchorHomeMetadata(conn); e != nil {
				return e
			}
			report.MetadataSynced = true
		}
		phase("financial_postflight")
		after, metadataAfter, accountsAfter, e := snapshot(conn)
		if e != nil {
			return e
		}
		if !unchangedMetadata(metadataBefore, metadataAfter, opts.Metadata) {
			return safeGuardFailure("permission_delta")
		}
		if !unchangedFinancial(before, after) {
			return safeGuardFailure("financial_proof_changed")
		}
		if !unchangedAccounts(accountsBefore, accountsAfter) {
			return safeGuardFailure("account_anchor_proof_changed")
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
				return safeGuardFailure("new_table_not_empty")
			}
			report.NewTables = append(report.NewTables, table)
		}
		sort.Strings(report.NewTables)
		report.FinancialUnchanged = true
		report.MetadataPreserved = true
		report.AccountsPreserved = true
		return nil
	})
	if err != nil {
		report.FailureCode = safeFailureCode(err)
		if report.FailureCode == "unknown_error" && ctx.Err() != nil {
			report.FailureCode = safeFailureCode(ctx.Err())
		}
		return report, &stageError{stage: report.Stage, cause: err}
	}
	report.OK = true
	phase("complete")
	return report, nil
}

func validBackupScope(scope string) bool {
	return scope == engagementcli.BackupScopeFinancialAndMetadata
}

func unchangedFinancial(before, after map[string]engagementcli.TableProof) bool {
	allowed := map[string]bool{}
	for _, name := range newTables {
		allowed[name] = true
	}
	for name, want := range before {
		got, ok := after[name]
		if !ok {
			return false
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

// Both the exact finance proof and metadata multiset share the same read view.
func snapshot(db *gorm.DB) (proof map[string]engagementcli.TableProof, metadata metadataProof, accounts map[string]engagementcli.TableProof, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		var e error
		proof, e = engagementcli.ProofInSnapshot(tx)
		if e != nil {
			return e
		}
		metadata, e = metadataInSnapshot(tx)
		if e != nil {
			return e
		}
		accounts, e = accountProofInSnapshot(tx)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return proof, metadata, accounts, err
}

func migrationLockName(database string) string {
	digest := sha256.Sum256([]byte(database))
	return fmt.Sprintf("anchor-home:migrate:%x", digest[:22])
}

// All Error() strings are safe even if another CLI prints these errors directly.
type stageError struct {
	stage string
	cause error
}

func (e *stageError) Error() string { return e.stage }
func (e *stageError) Unwrap() error { return e.cause }

// Only known local guards may supply a string code. External error messages are
// never inspected, formatted, or copied into a diagnostic result.
type safeGuardFailure string

func (e safeGuardFailure) Error() string { return string(e) }

func errorType[T error](err error) bool { var target T; return errors.As(err, &target) }

func safeFailureCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	// Stable local guard failures carry no configuration, SQL or row contents.
	var guard safeGuardFailure
	if errors.As(err, &guard) {
		switch guard {
		case "permission_delta", "financial_proof_changed", "account_anchor_proof_changed", "new_table_not_empty", "migration_lock":
			return string(guard)
		}
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "timeout"
	}
	var mysqlError *mysqlDriver.MySQLError
	if errors.As(err, &mysqlError) {
		return fmt.Sprintf("mysql_%d", mysqlError.Number)
	}
	for _, item := range []struct {
		cause error
		code  string
	}{
		{syscall.ECONNREFUSED, "connection_refused"},
		{syscall.ENETUNREACH, "network_unreachable"},
		{syscall.EHOSTUNREACH, "host_unreachable"},
		{syscall.ECONNRESET, "connection_reset"},
		{syscall.EACCES, "access_denied"},
		{syscall.EPERM, "operation_not_permitted"},
		{syscall.ECONNABORTED, "connection_aborted"},
		{syscall.EPIPE, "broken_pipe"},
		{net.ErrClosed, "connection_closed"},
		{io.EOF, "connection_closed"},
		{io.ErrUnexpectedEOF, "connection_closed"},
	} {
		if errors.Is(err, item.cause) {
			return item.code
		}
	}
	if errorType[*net.DNSError](err) {
		return "dns_error"
	}
	if errorType[x509.UnknownAuthorityError](err) || errorType[*x509.UnknownAuthorityError](err) {
		return "tls_unknown_authority"
	}
	if errorType[x509.HostnameError](err) || errorType[*x509.HostnameError](err) {
		return "tls_hostname_mismatch"
	}
	if errorType[x509.CertificateInvalidError](err) || errorType[*x509.CertificateInvalidError](err) {
		return "tls_certificate_invalid"
	}
	if errorType[x509.SystemRootsError](err) || errorType[*x509.SystemRootsError](err) {
		return "tls_system_roots"
	}
	if errorType[*tls.CertificateVerificationError](err) {
		return "tls_certificate_verification"
	}
	if errorType[tls.RecordHeaderError](err) || errorType[*tls.RecordHeaderError](err) {
		return "tls_record_error"
	}
	if errorType[tls.AlertError](err) || errorType[*tls.AlertError](err) {
		return "tls_alert"
	}
	if errorType[*net.OpError](err) {
		return "network_error"
	}
	return "unknown_error"
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
