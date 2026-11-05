// gift-migrate 使用专用迁移凭证执行礼物DDL，或只读验证已有结构。
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
	earning "tb_live_module/model/earning"
	model "tb_live_module/model/gift"
	"time"
)

func main() {
	metadata := flag.Bool("metadata", false, "同步菜单和超级管理员操作权限")
	verify := flag.Bool("verify", false, "只校验结构，不执行DDL")
	seed := flag.Bool("seed-catalog", false, "显式补齐3个基础分类和6个草稿礼物，不覆盖已有配置")
	catalogOnly := flag.Bool("catalog-only", false, "仅导入草稿目录数据，不执行DDL或权限同步；须与-seed-catalog一起使用")
	inspect := flag.Bool("inspect-catalog", false, "只读显示目标库、礼物表是否存在及基础草稿数据")
	configFile := flag.String("config", "", "从指定服务配置读取MySQL连接，避免在命令行传递凭证")
	flag.Parse()
	if (*verify && (*seed || *metadata || *inspect || *catalogOnly)) || (*inspect && (*seed || *metadata || *catalogOnly)) || (*catalogOnly && (!*seed || *metadata)) {
		fmt.Fprintln(os.Stderr, "incompatible operation flags; inspect/verify are read-only; catalog-only requires seed-catalog")
		os.Exit(2)
	}
	raw := os.Getenv("GIFT_MIGRATION_DSN")
	if *configFile != "" {
		if raw != "" {
			fmt.Fprintln(os.Stderr, "select either -config or GIFT_MIGRATION_DSN, never both")
			os.Exit(2)
		}
		v := viper.New()
		v.SetConfigFile(*configFile)
		var c config.Server
		if v.ReadInConfig() != nil || v.Unmarshal(&c) != nil || c.System.DbType != "mysql" {
			fmt.Fprintln(os.Stderr, "cannot read MySQL configuration")
			os.Exit(2)
		}
		raw = c.Mysql.Dsn()
	}
	if raw == "" {
		fmt.Fprintln(os.Stderr, "GIFT_MIGRATION_DSN is required")
		os.Exit(2)
	}
	cfg, err := driver.ParseDSN(raw)
	if err != nil || cfg.DBName == "" {
		fmt.Fprintln(os.Stderr, "invalid GIFT_MIGRATION_DSN or missing database")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	if *inspect {
		err = inspectCatalog(db, cfg.Addr)
	} else if *catalogOnly {
		err = initialize.SeedGiftCatalog(db)
	} else if *verify {
		err = initialize.ValidateGiftSchema(db)
	} else {
		err = initialize.MigrateGift(db)
	}
	if err == nil && *metadata {
		err = initialize.SyncGiftMetadata(db)
	}
	if err == nil && *seed && !*catalogOnly {
		err = initialize.SeedGiftCatalog(db)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gift schema operation failed:", err)
		os.Exit(1)
	}
	if *catalogOnly {
		fmt.Println("gift catalog drafts imported; existing records preserved; no schema migration executed")
	} else if !*inspect {
		fmt.Println("gift schema verified")
	}
}

func inspectCatalog(db *gorm.DB, address string) error {
	var database string
	if e := db.Raw("SELECT DATABASE()").Scan(&database).Error; e != nil {
		return e
	}
	tables := map[string]bool{}
	var tableNames []string
	for _, m := range []any{&model.Category{}, &model.Item{}, &model.Control{}, &model.Audit{}, &model.AdminPermission{}, &model.Order{}, &model.Refund{}, &model.Revenue{}, &model.BenefitGrant{}, &model.BenefitCompensation{}, &model.Event{}, &earning.RevenueRule{}, &earning.Ledger{}, &earning.RuleTermination{}} {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		tables[st.Schema.Table] = false
		tableNames = append(tableNames, st.Schema.Table)
	}
	var existing []string
	if e := db.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", tableNames).Scan(&existing).Error; e != nil {
		return e
	}
	for _, name := range existing {
		tables[name] = true
	}
	var categories []model.Category
	var items []model.Item
	if tables["gift_category"] {
		if e := db.Where("category_no IN ?", []string{"basic_affection", "basic_support", "basic_premium"}).Order("category_no").Find(&categories).Error; e != nil {
			return e
		}
	}
	if tables["gift_item"] {
		if e := db.Where("gift_no IN ?", []string{"basic_rose", "basic_heart", "basic_cheer", "basic_cake", "basic_rocket", "basic_crown"}).Order("gift_no").Find(&items).Error; e != nil {
			return e
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"address": address, "database": database, "tables": tables, "categories": categories, "items": items})
}
