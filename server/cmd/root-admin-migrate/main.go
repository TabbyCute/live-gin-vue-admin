// root-admin-migrate upgrades only the four root self-review triggers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/config"
	"tb_live_module/initialize"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configFile := flag.String("config", "", "数据库配置文件")
	apply := flag.Bool("apply", false, "执行触发器升级，默认只读预检")
	backup := flag.String("backup", "", "升级前触发器备份路径，禁止覆盖")
	flag.Parse()
	if *configFile == "" || (*apply && *backup == "") {
		return fmt.Errorf("-config is required; -apply also requires -backup")
	}
	v := viper.New()
	v.SetConfigFile(*configFile)
	var config config.Server
	if v.ReadInConfig() != nil || v.Unmarshal(&config) != nil || config.System.DbType != "mysql" {
		return fmt.Errorf("cannot read MySQL configuration")
	}
	cfg, err := driver.ParseDSN(config.Mysql.Dsn())
	if err != nil || cfg.DBName == "" {
		return fmt.Errorf("invalid database configuration")
	}
	cfg.ParseTime = true
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 30*time.Second, 30*time.Second
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("cannot connect to configured database")
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	plan, err := initialize.PlanRootReviewMigration(db)
	if err != nil {
		return err
	}
	fmt.Printf("target=%s database=%s pendingStatements=%d\n", cfg.Addr, plan.Database, len(plan.Statements))
	if !*apply {
		return nil
	}
	err = initialize.MigrateRootReviewTriggers(db, func(plan initialize.RootReviewPlan) error {
		file, err := os.OpenFile(*backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := json.NewEncoder(file).Encode(plan); err != nil {
			return err
		}
		return file.Sync()
	})
	if err != nil {
		return err
	}
	fmt.Println("root review triggers upgraded; tables and business records unchanged")
	if err := initialize.ValidateGiftSchema(db); err != nil {
		return fmt.Errorf("post-migration gift validation: %w", err)
	}
	if err := initialize.ValidateEarningSchema(db); err != nil {
		return fmt.Errorf("post-migration earning validation: %w", err)
	}
	if err := initialize.ValidateIncomeCoverage(db); err != nil {
		return fmt.Errorf("post-migration income validation: %w", err)
	}
	fmt.Println("gift, earning and income coverage startup checks passed")
	return nil
}
