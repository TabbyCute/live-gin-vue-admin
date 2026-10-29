// wallet-migrate 使用专用迁移凭证执行钱包DDL，或只读验证已有结构。
package main

import (
	"flag"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"tb_live_module/initialize"
	"time"
)

func main() {
	verify := flag.Bool("verify", false, "只校验结构，不执行DDL")
	flag.Parse()
	raw := os.Getenv("WALLET_MIGRATION_DSN")
	if raw == "" {
		fmt.Fprintln(os.Stderr, "WALLET_MIGRATION_DSN is required")
		os.Exit(2)
	}
	cfg, err := driver.ParseDSN(raw)
	if err != nil || cfg.DBName == "" {
		fmt.Fprintln(os.Stderr, "invalid WALLET_MIGRATION_DSN or missing database")
		os.Exit(2)
	}
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 60 * time.Second
	cfg.WriteTimeout = 60 * time.Second
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot connect to migration database; check credentials and connectivity")
		os.Exit(1)
	}
	pool, err := db.DB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot initialize migration pool")
		os.Exit(1)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	if *verify {
		err = initialize.ValidateWalletSchema(db)
	} else {
		err = initialize.MigrateWallet(db)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wallet schema operation failed:", err)
		os.Exit(1)
	}
	fmt.Println("wallet schema verified")
}
