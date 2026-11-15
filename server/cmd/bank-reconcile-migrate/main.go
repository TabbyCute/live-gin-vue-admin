package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"tb_live_module/config"
	"tb_live_module/initialize"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	configFile := flag.String("config", "", "MySQL config; do not use for tests")
	migrate := flag.Bool("migrate", false, "create new bank reconciliation tables and immutable guards only")
	metadata := flag.Bool("sync-metadata", false, "register menu and APIs without granting financial permissions")
	flag.Parse()
	raw := os.Getenv("BANK_RECONCILE_MIGRATION_DSN")
	if *configFile != "" {
		if raw != "" {
			return fmt.Errorf("choose config or migration DSN")
		}
		v := viper.New()
		v.SetConfigFile(*configFile)
		var cfg config.Server
		if v.ReadInConfig() != nil || v.Unmarshal(&cfg) != nil || cfg.System.DbType != "mysql" {
			return fmt.Errorf("invalid MySQL configuration")
		}
		raw = cfg.Mysql.Dsn()
	}
	cfg, e := driver.ParseDSN(raw)
	if e != nil || cfg.DBName == "" {
		return fmt.Errorf("MySQL configuration required")
	}
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 60 * time.Second
	cfg.WriteTimeout = 60 * time.Second
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return fmt.Errorf("cannot connect to bank reconciliation database")
	}
	pool, e := db.DB()
	if e != nil {
		return e
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	if *migrate {
		if e = initialize.MigrateBankReconcile(db); e != nil {
			return e
		}
	}
	if *metadata {
		if e = initialize.SyncBankReconcileMetadata(db); e != nil {
			return e
		}
	}
	if e = initialize.ValidateBankReconcileSchema(db); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"verified": true, "migrated": *migrate, "metadataSynced": *metadata, "financialTablesModified": false})
}
