package initialize

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"strings"
	"tb_live_module/global"
	model "tb_live_module/model/bankreconcile"
	"tb_live_module/model/system"
	svc "tb_live_module/service/bankreconcile"
	payout "tb_live_module/service/payout"
	"time"
)

func bankModels() []any {
	return []any{&model.Batch{}, &model.Line{}, &model.BatchLine{}, &model.Run{}, &model.Check{}, &model.Review{}, &model.Receipt{}, &model.Audit{}}
}
func bankTriggers(db *gorm.DB) ([]giftTrigger, error) {
	var result []giftTrigger
	for _, m := range bankModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return nil, e
		}
		for _, event := range []string{"UPDATE", "DELETE"} {
			body := "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='bank reconciliation facts immutable'"
			if db.Dialector.Name() == "sqlite" {
				body = "BEGIN SELECT RAISE(ABORT,'bank reconciliation facts immutable'); END"
			}
			result = append(result, giftTrigger{"trg_" + st.Schema.Table + "_" + strings.ToLower(event), st.Schema.Table, event, body})
		}
	}
	return result, nil
}

// MigrateBankReconcile only creates additive tables and immutable guards. Existing
// financial tables, statuses, permissions and money are never rewritten.
func MigrateBankReconcile(db *gorm.DB) error {
	if db == nil || (db.Dialector.Name() != "mysql" && db.Dialector.Name() != "sqlite") {
		return fmt.Errorf("bank reconciliation requires MySQL8 or test SQLite")
	}
	return withFinancialMigrationLock(db, 5*time.Minute, func(conn *gorm.DB) error {
		target := conn
		if conn.Dialector.Name() == "mysql" {
			if e := validateWalletMySQLRuntime(conn); e != nil {
				return e
			}
			target = conn.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
		}
		if e := ensureBankAttemptIndex(conn); e != nil {
			return e
		}
		for _, m := range bankModels() {
			if !conn.Migrator().HasTable(m) {
				if e := target.Migrator().CreateTable(m); e != nil {
					return e
				}
			}
		}
		triggers, e := bankTriggers(conn)
		if e != nil {
			return e
		}
		if e = installManualTriggers(conn, triggers, nil); e != nil {
			return e
		}
		return ValidateBankReconcileSchema(conn)
	})
}
func ValidateBankReconcileSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("bank reconciliation database missing")
	}
	if e := validateBankAttemptIndex(db); e != nil {
		return e
	}
	if db.Dialector.Name() == "mysql" {
		triggers, e := bankTriggers(db)
		if e != nil {
			return e
		}
		if e = validateFinancialMySQLSchema(db, bankModels(), triggers); e != nil {
			return e
		}
		return validateBankDefinitions(db)
	}

	for _, m := range bankModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		if !db.Migrator().HasTable(m) {
			return fmt.Errorf("bank reconciliation schema missing: %s", st.Schema.Table)
		}
		for _, f := range st.Schema.Fields {
			if f.DBName != "" && !db.Migrator().HasColumn(m, f.DBName) {
				return fmt.Errorf("bank reconciliation column missing: %s", f.DBName)
			}
		}
		for _, idx := range st.Schema.ParseIndexes() {
			if !db.Migrator().HasIndex(m, idx.Name) {
				return fmt.Errorf("bank reconciliation index missing: %s", idx.Name)
			}
		}
		for _, check := range st.Schema.ParseCheckConstraints() {
			if !db.Migrator().HasConstraint(m, check.Name) {
				return fmt.Errorf("bank reconciliation constraint missing: %s", check.Name)
			}
		}
	}
	triggers, e := bankTriggers(db)
	if e != nil {
		return e
	}
	if db.Dialector.Name() == "mysql" {
		return validateFinancialMySQLSchema(db, bankModels(), triggers)
	}
	for _, t := range triggers {
		var body string
		if db.Dialector.Name() == "mysql" {
			e = db.Raw("SELECT action_statement FROM information_schema.triggers WHERE trigger_schema=DATABASE() AND trigger_name=? AND event_object_table=? AND event_manipulation=? AND action_timing='BEFORE'", t.name, t.table, t.event).Scan(&body).Error
		} else {
			e = db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=?", t.name).Scan(&body).Error
		}
		if e != nil {
			return e
		}
		expected := "CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + " " + t.body
		if strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(expected), " ") {
			return fmt.Errorf("bank reconciliation immutable guard missing: %s", t.name)
		}
	}
	return nil
}
func InitializeBankReconcile() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := ValidateBankReconcileSchema(global.GVA_DB.WithContext(ctx)); e != nil {
		return e
	}
	svc.Default.DB = global.GVA_DB
	svc.Default.Payout = payout.Default
	return nil
}

// API registration is not financial authorization. No ordinary role is granted.
func SyncBankReconcileMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var parent system.SysBaseMenu
		if e := tx.Where("name='payoutCenter'").First(&parent).Error; e != nil {
			return e
		}
		menu := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: "payoutBankReconciliation", Name: "payoutBankReconciliation", Component: "view/payout/reconciliation/index.vue", Sort: 3, Meta: system.Meta{Title: "银行账单核对", Icon: "tickets"}}
		if e := tx.Where("name=?", menu.Name).FirstOrCreate(&menu).Error; e != nil {
			return e
		}
		for _, op := range []struct{ action, method, description string }{{"import", "POST", "导入外部银行账单声明"}, {"run", "POST", "双向核对银行账单"}, {"review", "POST", "独立复核核对记录"}, {"list", "GET", "查看银行导入批次"}, {"detail", "GET", "查看脱敏银行核对结果"}} {
			api := system.SysApi{Path: "/payout/bank-reconciliation/" + op.action, Method: op.method, ApiGroup: "银行账单核对", Description: op.description}
			if e := tx.Where("path=? AND method=?", api.Path, api.Method).FirstOrCreate(&api).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
