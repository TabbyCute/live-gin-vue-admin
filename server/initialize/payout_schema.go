package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
)

func ValidatePayoutSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("payout database unavailable")
	}
	if e := validateNoLegacyManualGuards(db); e != nil {
		return e
	}
	if db.Migrator().HasTable("anchor_withdrawal_order") {
		indexes, e := db.Migrator().GetIndexes("anchor_withdrawal_order")
		if e != nil {
			return e
		}
		ok := false
		for _, index := range indexes {
			if index.Name() == "idx_withdrawal_payee_ref" && strings.Join(index.Columns(), ",") == "payee_ref" {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("payout withdrawal reference index missing or incompatible")
		}
	}
	triggers, e := payoutTriggers(db)
	if e != nil {
		return e
	}
	if db.Dialector.Name() == "mysql" {
		if e := validateFinancialMySQLSchema(db, payoutModels(), triggers); e != nil {
			return e
		}
		if e := validatePayoutMySQLDefinitions(db); e != nil {
			return e
		}
		return validateManualFinanceSchema(db)
	}
	for _, m := range payoutModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		if !db.Migrator().HasTable(m) {
			return fmt.Errorf("payout table missing: %s", st.Schema.Table)
		}
		for _, f := range st.Schema.Fields {
			if f.DBName != "" && !db.Migrator().HasColumn(m, f.DBName) {
				return fmt.Errorf("payout column missing: %s.%s", st.Schema.Table, f.DBName)
			}
		}
		indexes, e := db.Migrator().GetIndexes(m)
		if e != nil {
			return e
		}
		for _, index := range st.Schema.ParseIndexes() {
			ok := false
			for _, actual := range indexes {
				if actual.Name() != index.Name {
					continue
				}
				cols := []string{}
				for _, f := range index.Fields {
					cols = append(cols, f.Field.DBName)
				}
				unique, _ := actual.Unique()
				ok = strings.Join(cols, ",") == strings.Join(actual.Columns(), ",") && (index.Class != "UNIQUE" || unique)
			}
			if !ok {
				return fmt.Errorf("payout index incompatible: %s", index.Name)
			}
		}
		for name := range st.Schema.ParseCheckConstraints() {
			if !db.Migrator().HasConstraint(m, name) {
				return fmt.Errorf("payout check missing: %s", name)
			}
		}
	}
	for _, t := range triggers {
		var body string
		if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name=?", t.name, t.table).Scan(&body).Error; e != nil {
			return e
		}
		expected := "CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + " " + t.body
		if strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(expected), " ") {
			return fmt.Errorf("payout trigger incompatible: %s", t.name)
		}
	}
	return validateManualFinanceSchema(db)
}
