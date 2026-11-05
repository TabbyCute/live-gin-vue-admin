package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
	"tb_live_module/global"
)

// ValidateGiftSchema never changes data; fail closed on missing safety constraints.
func ValidateGiftSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("gift database unavailable")
	}
	if db.Dialector.Name() == "mysql" {
		return validateGiftMySQLSchema(db)
	}
	for _, m := range giftModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		if !db.Migrator().HasTable(m) {
			return fmt.Errorf("gift table missing: %s", st.Schema.Table)
		}
		columns, e := db.Migrator().ColumnTypes(m)
		if e != nil {
			return e
		}
		have := map[string]bool{}
		for _, c := range columns {
			have[c.Name()] = true
		}
		for _, f := range st.Schema.Fields {
			if f.DBName != "" && !have[f.DBName] {
				return fmt.Errorf("gift column missing: %s.%s", st.Schema.Table, f.DBName)
			}
		}
		indexes, e := db.Migrator().GetIndexes(m)
		if e != nil {
			return e
		}
		for _, expected := range st.Schema.ParseIndexes() {
			found := false
			for _, actual := range indexes {
				if expected.Name == actual.Name() {
					found = true
					unique, known := actual.Unique()
					if expected.Class == "UNIQUE" && (!known || !unique) {
						return fmt.Errorf("gift unique index invalid: %s", expected.Name)
					}
					var cols []string
					for _, f := range expected.Fields {
						cols = append(cols, f.Field.DBName)
					}
					if strings.Join(cols, ",") != strings.Join(actual.Columns(), ",") {
						return fmt.Errorf("gift index columns invalid: %s", expected.Name)
					}
				}
			}
			if !found {
				return fmt.Errorf("gift index missing: %s", expected.Name)
			}
		}
		for name := range st.Schema.ParseCheckConstraints() {
			if !db.Migrator().HasConstraint(m, name) {
				return fmt.Errorf("gift check constraint missing: %s", name)
			}
		}
		table := st.Schema.Table
		if db.Dialector.Name() == "mysql" {
			var unsafe int64
			if e := db.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLLATION_NAME IS NOT NULL AND COLLATION_NAME <> 'utf8mb4_bin'", table).Scan(&unsafe).Error; e != nil {
				return e
			}
			if unsafe > 0 {
				return fmt.Errorf("gift requires binary string collation: %s", table)
			}
			if e := db.Raw("SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME=? AND CONSTRAINT_TYPE='CHECK' AND ENFORCED <> 'YES'", table).Scan(&unsafe).Error; e != nil {
				return e
			}
			if unsafe > 0 {
				return fmt.Errorf("gift requires enforced CHECK constraints: %s", table)
			}
			var engine string
			if e := db.Raw("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", table).Scan(&engine).Error; e != nil {
				return e
			}
			if engine != "InnoDB" {
				return fmt.Errorf("gift requires InnoDB: %s", table)
			}
		}
		if table == "gift_category" || table == "gift_item" || table == "gift_control" || table == "gift_admin_permission" {
			continue
		}

	}
	triggers, e := giftTriggers(db)
	if e != nil {
		return e
	}
	if e := validateRootReviewUpgrade(db, triggers); e != nil {
		return e
	}
	var legacy int64
	if db.Dialector.Name() == "mysql" {
		if e := db.Raw("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME='trg_gift_benefit_compensation_update'").Scan(&legacy).Error; e != nil {
			return e
		}
	} else {
		if e := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='trg_gift_benefit_compensation_update'").Scan(&legacy).Error; e != nil {
			return e
		}
	}
	if legacy != 0 {
		return fmt.Errorf("legacy compensation trigger requires gift migration")
	}
	for _, t := range triggers {
		var body string
		expected := t.body
		if db.Dialector.Name() == "mysql" {
			if e := db.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE=? AND EVENT_MANIPULATION=? AND ACTION_TIMING='BEFORE'", t.name, t.table, t.event).Scan(&body).Error; e != nil {
				return e
			}
		} else {
			if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name=?", t.name, t.table).Scan(&body).Error; e != nil {
				return e
			}
			expected = "CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + " " + t.body
		}
		if strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(expected), " ") {
			return fmt.Errorf("gift immutable trigger incompatible: %s", t.name)
		}
	}
	return nil
}
func InitializeGift() error {
	if e := global.GVA_CONFIG.Gift.Validate(global.GuardReady.Load()); e != nil {
		return e
	}
	if e := ValidateGiftSchema(global.GVA_DB); e != nil {
		return e
	}
	global.GiftFactsReady.Store(true)
	return nil
}
