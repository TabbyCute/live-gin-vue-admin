package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
	em "tb_live_module/model/earning"
)

// The no-DDL fast path is valid only when both payout and its monetary storage
// upgrade finished. A previous interrupted MySQL DDL run must resume safely.
func validateManualFinanceSchema(db *gorm.DB) error {
	var models []any
	tables := map[string]bool{}
	for _, m := range []any{&em.Account{}, &em.AccountLedger{}, &em.Withdrawal{}} {
		if db.Migrator().HasTable(m) {
			models = append(models, m)
			st := &gorm.Statement{DB: db}
			if e := st.Parse(m); e != nil {
				return e
			}
			tables[st.Schema.Table] = true
		}
	}
	if len(models) == 0 {
		return nil
	}
	all, e := earningTriggers(db)
	if e != nil {
		return e
	}
	var triggers []giftTrigger
	for _, t := range all {
		if tables[t.table] {
			triggers = append(triggers, t)
		}
	}
	if db.Dialector.Name() == "mysql" {
		if e := validateFinancialMySQLSchema(db, models, triggers); e != nil {
			return e
		}
		return validatePayoutModelDefinitions(db, models)
	}
	for _, m := range models {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		for _, f := range st.Schema.Fields {
			if f.DBName != "" && !db.Migrator().HasColumn(m, f.DBName) {
				return fmt.Errorf("manual financial column missing: %s.%s", st.Schema.Table, f.DBName)
			}
		}
		var ddl string
		if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", st.Schema.Table).Scan(&ddl).Error; e != nil {
			return e
		}
		compact := func(s string) string { return strings.Join(strings.Fields(s), "") }
		for name, c := range st.Schema.ParseCheckConstraints() {
			if !strings.Contains(compact(ddl), compact("CONSTRAINT `"+name+"` CHECK ("+c.Constraint+")")) {
				return fmt.Errorf("manual financial check incompatible: %s", name)
			}
		}
	}
	for _, t := range triggers {
		var actual string
		if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=?", t.name).Scan(&actual).Error; e != nil {
			return e
		}
		expected := "CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + " " + t.body
		if strings.Join(strings.Fields(actual), " ") != strings.Join(strings.Fields(expected), " ") {
			return fmt.Errorf("manual financial guard incompatible: %s", t.name)
		}
	}
	return nil
}
