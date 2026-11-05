package engagementcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"reflect"
	"strings"
	"tb_live_module/config"
	"tb_live_module/initialize"
	gm "tb_live_module/model/guard"
	rm "tb_live_module/model/ranking"
	guardservice "tb_live_module/service/guard"
	"tb_live_module/service/ranking"
	"time"
)

func Main(module string) {
	if e := Run(module); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func Run(module string) error {
	file := flag.String("config", "", "MySQL config file; never prints credentials")
	migrate := flag.Bool("migrate", false, "Create/validate module schema")
	metadata := flag.Bool("metadata", false, "Synchronize five menus and exact permissions after both schemas exist")
	verify := flag.Bool("verify", false, "Read-only structural verification")
	backup := flag.String("backup-dir", "", "Private absolute 0700 directory; required for any mutation")
	backfill := flag.Bool("backfill", false, "Bounded resumable ranking historical backfill")
	reconcile := flag.Bool("reconcile", false, "Read-only ranking source/projection comparison")
	batch := flag.Int("batch", 20, "Backfill batch 1..200")
	maxBatches := flag.Int("max-batches", 1000, "Maximum batches in this invocation")
	timeout := flag.Duration("timeout", 5*time.Minute, "Operation deadline, 1..30 minutes")
	flag.Parse()
	if *timeout < time.Minute || *timeout > 30*time.Minute || *batch < 1 || *batch > 200 || *maxBatches < 1 || *maxBatches > 10000 {
		return fmt.Errorf("invalid operation bounds")
	}
	if *verify && (*migrate || *metadata || *backfill) {
		return fmt.Errorf("verify is read-only")
	}
	if module != "ranking" && (*backfill || *reconcile) {
		return fmt.Errorf("backfill/reconcile require ranking-migrate")
	}
	raw := os.Getenv(strings.ToUpper(module) + "_MIGRATION_DSN")
	zone := "Asia/Kuala_Lumpur"
	var configuredGuardEnabled *bool
	if *file != "" {
		if raw != "" {
			return fmt.Errorf("select config or DSN environment, not both")
		}
		v := viper.New()
		v.SetConfigFile(*file)
		var c config.Server
		if v.ReadInConfig() != nil || v.Unmarshal(&c) != nil || c.System.DbType != "mysql" {
			return fmt.Errorf("cannot read MySQL configuration")
		}
		raw = c.Mysql.Dsn()
		zone = c.Ranking.Zone()
		configuredGuardEnabled = &c.Gift.GuardEnabled
	}
	cfg, e := driver.ParseDSN(raw)
	if e != nil || cfg.DBName == "" {
		return fmt.Errorf("valid database configuration required")
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 60 * time.Second
	cfg.WriteTimeout = 60 * time.Second
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return fmt.Errorf("cannot connect to configured database")
	}
	pool, e := db.DB()
	if e != nil {
		return fmt.Errorf("cannot initialize database pool")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db = db.WithContext(ctx)
	var version, database string
	if e = db.Raw("SELECT VERSION()").Scan(&version).Error; e != nil {
		return fmt.Errorf("target preflight failed")
	}
	if e = db.Raw("SELECT DATABASE()").Scan(&database).Error; e != nil {
		return fmt.Errorf("target preflight failed")
	}
	hostHash := sha256.Sum256([]byte(cfg.Addr))
	output := map[string]any{"database": database, "hostFingerprint": hex.EncodeToString(hostHash[:6]), "version": version, "module": module, "migrated": false, "metadataSynced": false, "backfillComplete": false, "applicationDeployed": false, "configuredGuardEnabled": configuredGuardEnabled, "runtimeReadinessVerified": false}
	preflight, e := Preflight(db)
	if e != nil {
		return fmt.Errorf("source preflight failed")
	}
	output["sourcePreflight"] = preflight
	before, e := Proof(db)
	if e != nil {
		return fmt.Errorf("financial preflight failed")
	}
	output["financialBefore"] = before
	// Snapshot includes existing gift/guard/ranking and affected admin metadata.
	if *migrate || *metadata || *backfill {
		if *backup == "" {
			return fmt.Errorf("mutation requires -backup-dir")
		}
		path, digest, e := Backup(db, *backup)
		if e != nil {
			return e
		}
		output["backupFile"] = path
		output["backupSHA256"] = digest
	}
	if *migrate {
		if module == "guard" {
			e = initialize.MigrateGuard(db)
		} else {
			e = initialize.MigrateRanking(db, zone)
		}
		if e != nil {
			return fmt.Errorf("module migration failed; guard remains disabled: %w", e)
		}
		output["migrated"] = true
	}
	if *verify || *migrate || *backfill || *reconcile {
		if module == "guard" {
			e = initialize.ValidateGuardSchema(db)
		} else {
			e = initialize.ValidateRankingSchema(db)
		}
		if e != nil {
			return fmt.Errorf("module structure verification failed: %w", e)
		}
		output["verified"] = true
		if module == "guard" {
			if e = (&guardservice.Service{DB: db}).VerifyAll(ctx); e != nil {
				return fmt.Errorf("guard facts/projection verification failed")
			}
			output["guardFactsConsistent"] = true
		}
	}
	if *metadata {
		if e = initialize.SyncEngagementMetadata(db); e != nil {
			return fmt.Errorf("menu synchronization failed")
		}
		output["metadataSynced"] = true
	}
	rs := &ranking.Service{DB: db, Timezone: zone}
	if *backfill {
		processed := 0
		for i := 0; i < *maxBatches; i++ {
			n, e := rs.Consume(ctx, *batch)
			if e != nil {
				return fmt.Errorf("ranking backfill stopped; progress retained")
			}
			processed += n
			if n == 0 {
				output["backfillComplete"] = true
				break
			}
		}
		output["processed"] = processed
	}
	if *reconcile {
		out, e := rs.ReconcileSnapshot(ctx)
		if e != nil {
			return fmt.Errorf("ranking reconcile failed")
		}
		output["reconciliation"] = out
	}
	if db.Migrator().HasTable(&rm.Checkpoint{}) {
		var cp rm.Checkpoint
		if e = db.Where("consumer_name='gift-revenue'").Take(&cp).Error; e != nil {
			return fmt.Errorf("cannot read checkpoint")
		}
		output["checkpoint"] = cp
	}
	if db.Migrator().HasTable(&gm.Plan{}) {
		var plans int64
		if e = db.Model(&gm.Plan{}).Where("status='enabled'").Count(&plans).Error; e != nil {
			return fmt.Errorf("cannot inspect plans")
		}
		output["enabledPlanCount"] = plans
	}
	var tables []string
	if e = db.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND (LEFT(TABLE_NAME,6)='guard_' OR LEFT(TABLE_NAME,8)='ranking_') ORDER BY TABLE_NAME").Scan(&tables).Error; e != nil {
		return fmt.Errorf("cannot inspect module tables")
	}
	output["tables"] = tables
	after, e := Proof(db)
	if e != nil {
		return fmt.Errorf("financial postflight failed")
	}
	output["financialAfter"] = after
	output["financialUnchanged"] = reflect.DeepEqual(before, after)
	return json.NewEncoder(os.Stdout).Encode(output)
}
