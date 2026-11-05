// earning-migrate operates independently of the HTTP server; it never invokes wallet migration.
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
	model "tb_live_module/model/earning"
	"tb_live_module/model/system"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	configFile := flag.String("config", "", "服务配置文件，凭证不放入命令行")
	migrate := flag.Bool("migrate", false, "创建收益结算/提现审核表及保护约束，不修改钱包内核")
	backfill := flag.Bool("backfill", false, "按已有礼物事实幂等回填统一收益事实")
	metadata := flag.Bool("metadata-only", false, "仅补齐直播管理菜单和角色888操作权限")
	verify := flag.Bool("verify", false, "只读核验结构及历史收益覆盖")
	flag.Parse()
	if *verify && (*migrate || *backfill || *metadata) {
		return fmt.Errorf("verify is read-only")
	}
	if *metadata && (*migrate || *backfill) {
		return fmt.Errorf("metadata-only cannot be combined with migration/backfill")
	}
	raw := os.Getenv("EARNING_MIGRATION_DSN")
	if *configFile != "" {
		if raw != "" {
			return fmt.Errorf("select config or EARNING_MIGRATION_DSN")
		}
		v := viper.New()
		v.SetConfigFile(*configFile)
		var c config.Server
		if v.ReadInConfig() != nil || v.Unmarshal(&c) != nil || c.System.DbType != "mysql" {
			return fmt.Errorf("cannot read MySQL config")
		}
		raw = c.Mysql.Dsn()
	}
	cfg, e := driver.ParseDSN(raw)
	if e != nil || cfg.DBName == "" {
		return fmt.Errorf("database configuration required")
	}
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 60 * time.Second
	cfg.WriteTimeout = 60 * time.Second
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return fmt.Errorf("cannot connect to earning database")
	}
	pool, e := db.DB()
	if e != nil {
		return fmt.Errorf("cannot initialize earning pool")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	if *migrate {
		if e = initialize.MigrateEarning(db); e != nil {
			return e
		}
	}
	if *backfill {
		if e = initialize.BackfillGiftIncome(db); e != nil {
			return e
		}
	}
	if *metadata {
		if e = initialize.SyncEarningMetadata(db); e != nil {
			return e
		}
	}
	if *verify {
		if e = initialize.ValidateEarningSchema(db); e != nil {
			return e
		}
		if e = initialize.ValidateIncomeCoverage(db); e != nil {
			return e
		}
	}
	var menus []system.SysBaseMenu
	if e = db.Where("name='liveManagement' OR name LIKE 'earning-%'").Order("parent_id,sort,id").Find(&menus).Error; e != nil {
		return e
	}
	var grants []struct {
		MenuID      uint
		AuthorityID uint
	}
	if e = db.Raw("SELECT m.sys_base_menu_id AS menu_id,m.sys_authority_authority_id AS authority_id FROM sys_authority_menus m JOIN sys_base_menus b ON b.id=m.sys_base_menu_id WHERE b.deleted_at IS NULL AND (b.name='liveManagement' OR b.name LIKE 'earning-%') ORDER BY menu_id,authority_id").Scan(&grants).Error; e != nil {
		return e
	}
	var apis []system.SysApi
	if e = db.Where("path LIKE '/earning/%'").Order("path,method").Find(&apis).Error; e != nil {
		return e
	}
	var permissions []model.AdminPermission
	if db.Migrator().HasTable(&model.AdminPermission{}) {
		if e = db.Order("authority_id,path,method").Find(&permissions).Error; e != nil {
			return e
		}
	}
	counts := map[string]int64{}
	for _, table := range []string{"anchor_earning_ledger", "anchor_income_fact", "anchor_settlement_rule", "anchor_settlement", "anchor_withdrawable_account", "anchor_withdrawable_ledger", "anchor_withdrawal_order"} {
		if db.Migrator().HasTable(table) {
			var count int64
			if e = db.Table(table).Count(&count).Error; e != nil {
				return e
			}
			counts[table] = count
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"factCounts": counts, "database": cfg.DBName, "address": cfg.Addr, "menus": menus, "grants": grants, "apis": apis, "permissions": permissions, "migrated": *migrate, "backfilled": *backfill, "metadataSynced": *metadata, "verified": *verify})
}
