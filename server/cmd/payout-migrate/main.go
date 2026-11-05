// payout-migrate upgrades payout and paid accounting guards without changing historical monetary facts.
package main

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
	"sort"
	"strings"
	"tb_live_module/config"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	"tb_live_module/model/system"
	ps "tb_live_module/service/payout"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	file := flag.String("config", "", "MySQL configuration file; credentials are never printed")
	migrate := flag.Bool("migrate", false, "Upgrade payout and paid accounting storage; preserves financial facts")
	backup := flag.String("backup-dir", "", "Private 0700 directory for a schema and row snapshot before mutation")
	verify := flag.Bool("verify", false, "Read-only payout schema verification")
	metadata := flag.Bool("sync-metadata", false, "Synchronize payout menus and role 888 permissions")
	timeout := flag.Duration("timeout", 5*time.Minute, "Maintenance deadline (1m to 30m); does not change HTTP request budgets")
	flag.Parse()
	if *timeout < time.Minute || *timeout > 30*time.Minute {
		return fmt.Errorf("maintenance timeout must be between 1m and 30m")
	}
	if (*migrate || *metadata) && *verify {
		return fmt.Errorf("verify is read-only")
	}
	raw := os.Getenv("PAYOUT_MIGRATION_DSN")
	if *file != "" {
		if raw != "" {
			return fmt.Errorf("choose config or PAYOUT_MIGRATION_DSN")
		}
		v := viper.New()
		v.SetConfigFile(*file)
		var c config.Server
		if v.ReadInConfig() != nil || v.Unmarshal(&c) != nil || c.System.DbType != "mysql" {
			return fmt.Errorf("cannot read MySQL configuration")
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
		return fmt.Errorf("cannot connect to payout database")
	}
	pool, e := db.DB()
	if e != nil {
		return fmt.Errorf("cannot initialize database pool")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	db = db.WithContext(ctx)
	before, e := counts(db)
	if e != nil {
		return e
	}
	proofBefore, e := financialProof(db)
	if e != nil {
		return e
	}
	if *migrate {
		if *backup == "" {
			return fmt.Errorf("migration requires -backup-dir for a private pre-change snapshot")
		}
		if e = backupFinancialTables(db, *backup); e != nil {
			return e
		}
		if e = initialize.MigratePayout(db); e != nil {
			return e
		}
	}
	if *verify {
		if e = initialize.ValidatePayoutSchema(db); e != nil {
			return e
		}
	}
	if *metadata {
		if e = initialize.SyncPayoutMetadata(db); e != nil {
			return fmt.Errorf("payout metadata synchronization failed")
		}
	}
	after, e := counts(db)
	if e != nil {
		return e
	}
	proofAfter, e := financialProof(db)
	if e != nil {
		return e
	}
	meta, e := metadataProof(db)
	if e != nil {
		return e
	}
	if (*metadata || *verify) && !meta.Ready {
		return fmt.Errorf("payout menu/API/role 888 metadata missing or incompatible")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"database": cfg.DBName, "migrated": *migrate, "metadataSynced": *metadata, "verified": *verify, "metadata": meta, "before": before, "after": after, "financialDigestBefore": proofBefore, "financialDigestAfter": proofAfter, "checkedAt": time.Now().UTC().Format(time.RFC3339)})
}

type metadataResult struct {
	Ready              bool  `json:"ready"`
	MenuCount          int   `json:"menuCount"`
	TopLevel           bool  `json:"topLevel"`
	Role888Menus       int64 `json:"role888Menus"`
	Role888Permissions int64 `json:"role888Permissions"`
	APIs               int64 `json:"apis"`
}

func metadataProof(db *gorm.DB) (metadataResult, error) {
	var out metadataResult
	for _, m := range []any{&system.SysBaseMenu{}, &system.SysAuthorityMenu{}, &system.SysApi{}, &em.AdminPermission{}} {
		if !db.Migrator().HasTable(m) {
			return out, nil
		}
	}
	var menus []system.SysBaseMenu
	if e := db.Where("name IN ?", []string{"payoutCenter", "payoutOrders", "payoutCompanyAccounts"}).Find(&menus).Error; e != nil {
		return out, fmt.Errorf("cannot verify payout menus")
	}
	out.MenuCount = len(menus)
	var parent, child, company system.SysBaseMenu
	for _, m := range menus {
		if m.Name == "payoutCenter" {
			parent = m
		} else if m.Name == "payoutOrders" {
			child = m
		} else {
			company = m
		}
	}
	out.TopLevel = len(menus) == 3 && parent.ParentId == 0 && parent.ID != 0 && child.ParentId == parent.ID && company.ParentId == parent.ID && parent.Path == "payoutCenter" && child.Path == "payoutOrders" && parent.Meta.Title == "打款中心" && child.Meta.Title == "出款订单" && parent.Component == "view/routerHolder.vue" && child.Component == "view/payout/order/index.vue" && company.Component == "view/payout/company/index.vue"
	if e := db.Model(&system.SysAuthorityMenu{}).Where("sys_authority_authority_id=? AND sys_base_menu_id IN ?", "888", []uint{parent.ID, child.ID, company.ID}).Count(&out.Role888Menus).Error; e != nil {
		return out, fmt.Errorf("cannot verify payout role menus")
	}
	ops := append([]ps.Operation{{Path: "order/create", Method: "POST"}, {Path: "order/list", Method: "GET"}, {Path: "order/detail", Method: "GET"}}, ps.ManualOperations...)
	for _, op := range ops {
		var n int64
		if e := db.Model(&system.SysApi{}).Where("path=? AND method=?", "/payout/"+op.Path, op.Method).Count(&n).Error; e != nil {
			return out, fmt.Errorf("cannot verify payout APIs")
		}
		if n == 1 {
			out.APIs++
		}
		if op.Path == "order/create" || op.Path == "order/list" || op.Path == "order/detail" {
			if e := db.Model(&em.AdminPermission{}).Where("authority_id=888 AND path=? AND method=?", "/payout/"+op.Path, op.Method).Count(&n).Error; e != nil {
				return out, fmt.Errorf("cannot verify payout permissions")
			}
			if n == 1 {
				out.Role888Permissions++
			}
		}
	}
	out.Ready = out.TopLevel && out.Role888Menus == 3 && out.Role888Permissions == 3 && out.APIs == int64(len(ops))
	return out, nil
}

// Hash existing financial records without printing or writing their contents.
// The CLI records concurrent legitimate changes rather than overwriting them.
func financialProof(db *gorm.DB) (map[string]string, error) {
	out := map[string]string{}
	tables := []string{"anchor_withdrawal_order", "anchor_withdrawable_account", "anchor_withdrawable_ledger", "anchor_settlement", "anchor_income_fact", "payout_order", "payout_status_history", "payout_audit_log", "payout_payee_account"}
	var catalog []struct {
		TableName  string
		ColumnName string
	}
	if e := db.Raw("SELECT TABLE_NAME AS table_name,COLUMN_NAME AS column_name FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", tables).Scan(&catalog).Error; e != nil {
		return nil, fmt.Errorf("cannot read financial proof schema")
	}
	byTable := map[string][]string{}
	for _, c := range catalog {
		byTable[c.TableName] = append(byTable[c.TableName], c.ColumnName)
	}
	for _, table := range tables {
		columns := byTable[table]
		if len(columns) == 0 {
			continue
		}
		var fields []string
		found := map[string]bool{}
		for _, c := range columns {
			found[c] = true
			fields = append(fields, c)
		}
		defaults := map[string]string{}
		switch table {
		case "anchor_withdrawable_ledger":
			defaults = map[string]string{"paid_before": "0", "paid_after": "0"}
		case "payout_order":
			defaults = map[string]string{"state_version": "1", "last_transition_no": ""}
		case "payout_status_history":
			defaults = map[string]string{"sequence": "1", "attempt_no": "", "request_id": "", "reason": ""}
		case "payout_audit_log":
			defaults = map[string]string{"object_no": "", "transition_no": ""}
		}
		{
			for col := range defaults {
				if !found[col] {
					fields = append(fields, col)
				}
			}
		}
		sort.Strings(fields)
		selects := make([]string, len(fields))
		for i, col := range fields {
			selects[i] = "`" + strings.ReplaceAll(col, "`", "``") + "`"
			if !found[col] {
				if defaults[col] == "" {
					selects[i] = "CAST('' AS CHAR) AS `" + col + "`"
				} else {
					selects[i] = "CAST(" + defaults[col] + " AS SIGNED) AS `" + col + "`"
				}
			}
		}
		rows, e := db.Table(table).Select(strings.Join(selects, ",")).Order("id ASC").Rows()
		if e != nil {
			return nil, fmt.Errorf("cannot read financial proof")
		}
		cols, e := rows.Columns()
		if e != nil {
			rows.Close()
			return nil, fmt.Errorf("cannot read financial proof columns")
		}
		h := sha256.New()
		encoder := json.NewEncoder(h)
		for rows.Next() {
			values := make([]any, len(cols))
			dest := make([]any, len(cols))
			for i := range values {
				dest[i] = &values[i]
			}
			if rows.Scan(dest...) != nil {
				rows.Close()
				return nil, fmt.Errorf("cannot scan financial proof")
			}
			if encoder.Encode(values) != nil {
				rows.Close()
				return nil, fmt.Errorf("cannot encode financial proof")
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, fmt.Errorf("incomplete financial proof")
		}
		out[table] = hex.EncodeToString(h.Sum(nil))
	}
	return out, nil
}
func counts(db *gorm.DB) (map[string]int64, error) {
	out := map[string]int64{}
	tables := []string{"payout_channel", "payout_channel_account", "payout_attempt", "payout_attempt_event", "payout_manual_request", "payout_channel_daily_usage", "payout_evidence", "payout_reveal_grant", "payout_terminal_event", "payout_reconciliation", "payout_investigation", "payout_order", "payout_order_request", "payout_status_history", "payout_audit_log", "payout_payee_account", "payout_owner_lock", "payout_payee_audit", "anchor_withdrawal_order", "anchor_withdrawable_account", "anchor_withdrawable_ledger", "anchor_settlement", "anchor_income_fact", "gift_order", "wallet_account"}
	var existing []string
	if e := db.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", tables).Scan(&existing).Error; e != nil {
		return nil, fmt.Errorf("cannot read verification tables")
	}
	found := map[string]bool{}
	for _, name := range existing {
		found[name] = true
	}
	for _, table := range tables {
		if !found[table] {
			continue
		}
		var n int64
		if e := db.Table(table).Count(&n).Error; e != nil {
			return nil, fmt.Errorf("cannot read verification counts")
		}
		out[table] = n
	}
	return out, nil
}
