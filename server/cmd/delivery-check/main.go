// delivery-check is a read-only release preflight. It never migrates or repairs
// business data and never emits credentials, account identifiers or balances.
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
	"strings"
	"tb_live_module/config"
	"tb_live_module/initialize"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	file := flag.String("config", "", "private server configuration file")
	flag.Parse()
	if *file == "" {
		return fmt.Errorf("-config is required")
	}
	v := viper.New()
	v.SetConfigFile(*file)
	var cfg config.Server
	if v.ReadInConfig() != nil || v.Unmarshal(&cfg) != nil || cfg.System.DbType != "mysql" {
		return fmt.Errorf("cannot read MySQL configuration")
	}
	dsn, e := driver.ParseDSN(cfg.Mysql.Dsn())
	if e != nil {
		return fmt.Errorf("invalid database configuration")
	}
	dsn.Timeout = 5 * time.Second
	dsn.ReadTimeout = 10 * time.Second
	dsn.WriteTimeout = 10 * time.Second
	dsn.ParseTime = true
	db, e := gorm.Open(mysql.Open(dsn.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return fmt.Errorf("cannot connect to configured database")
	}
	pool, e := db.DB()
	if e != nil {
		return fmt.Errorf("cannot obtain database pool")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	checks := []struct {
		name     string
		validate func(*gorm.DB) error
	}{
		{"wallet", initialize.ValidateWalletSchema}, {"gift", initialize.ValidateGiftSchema}, {"earning", initialize.ValidateEarningSchema},
		{"payout", initialize.ValidatePayoutSchema}, {"guard", initialize.ValidateGuardSchema}, {"ranking", initialize.ValidateRankingSchema}, {"presence", initialize.ValidatePresenceSchema},
	}
	results := map[string]bool{}
	passed := true
	for _, check := range checks {
		ok := check.validate(db) == nil
		results[check.name] = ok
		passed = passed && ok
	}
	var durability struct {
		Flush  int `gorm:"column:flush"`
		Sync   int `gorm:"column:sync"`
		Binlog int `gorm:"column:binlog"`
	}
	if e = db.Raw("SELECT @@innodb_flush_log_at_trx_commit AS `flush`, @@sync_binlog AS `sync`, @@log_bin AS binlog").Scan(&durability).Error; e != nil {
		return fmt.Errorf("cannot read durability configuration")
	}
	var grants []string
	if e = db.Raw("SHOW GRANTS FOR CURRENT_USER()").Scan(&grants).Error; e != nil {
		return fmt.Errorf("cannot inspect runtime grants")
	}
	// Report risk only, never the grant text (it includes usernames/hostnames).
	ddl := hasDDL(grants)
	report := map[string]any{"checkedAt": time.Now().UTC().Format(time.RFC3339), "readOnly": true, "schema": results, "schemaPassed": passed,
		"durability":          map[string]any{"flushAtCommit": durability.Flush, "syncBinlog": durability.Sync, "binlogEnabled": durability.Binlog == 1},
		"configuration":       map[string]any{"automaticMigrationDisabled": cfg.System.DisableAutoMigrate, "registrationUsesSharedCode": cfg.LiveAccountAuth.SuperCode != "", "corsMode": cfg.Cors.Mode, "trustedProxyCount": len(cfg.LiveAccountAuth.TrustedProxies), "configuredDBAccountHasDirectDDL": ddl},
		"applicationDeployed": false, "bankTransferVerified": false}
	if e = json.NewEncoder(os.Stdout).Encode(report); e != nil {
		return e
	}
	if !passed {
		return fmt.Errorf("schema verification failed; inspect schema results")
	}
	return nil
}

func hasDDL(grants []string) bool {
	for _, grant := range grants {
		privileges, _, ok := strings.Cut(strings.ToUpper(grant), " ON ")
		if !ok {
			continue
		}
		for _, privilege := range strings.Split(strings.TrimPrefix(privileges, "GRANT "), ",") {
			switch strings.TrimSpace(privilege) {
			case "ALL PRIVILEGES", "CREATE", "ALTER", "DROP", "TRIGGER":
				return true
			}
		}
	}
	return false
}
