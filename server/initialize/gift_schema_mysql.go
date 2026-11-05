package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"strings"
)

// Six batch reads, independent of table/column count: startup must tolerate remote DB latency.
// Metadata types are shared with the existing read-only wallet validator; no wallet DDL or writes.
func validateGiftMySQLSchema(db *gorm.DB) error {
	expected, e := giftTriggers(db)
	if e != nil {
		return e
	}
	return validateFinancialMySQLSchema(db, giftModels(), expected)
}
func validateFinancialMySQLSchema(db *gorm.DB, financialModels []any, expected []giftTrigger) error {
	if e := validateWalletMySQLRuntime(db); e != nil {
		return e
	}
	var names []string
	var models []*schema.Schema
	for _, m := range financialModels {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		names = append(names, st.Schema.Table)
		models = append(models, st.Schema)
	}
	var tables []struct{ TableName, Engine, TableCollation string }
	var columns []walletMySQLColumn
	var indexes []walletMySQLIndex
	var checks []walletMySQLCheck
	var triggers []struct{ TriggerName, ActionStatement, EventManipulation, EventObjectTable, ActionTiming string }
	for _, q := range []struct {
		sql  string
		dest any
	}{
		{"SELECT TABLE_NAME AS table_name,ENGINE AS engine,TABLE_COLLATION AS table_collation FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", &tables},
		{"SELECT TABLE_NAME AS table_name,COLUMN_NAME AS column_name,COLLATION_NAME AS collation_name FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", &columns},
		{"SELECT TABLE_NAME AS table_name,INDEX_NAME AS index_name,COLUMN_NAME AS column_name,NON_UNIQUE AS non_unique,SEQ_IN_INDEX AS seq_in_index,SUB_PART AS sub_part,IS_VISIBLE AS is_visible FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ? ORDER BY TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX", &indexes},
		{"SELECT TABLE_NAME AS table_name,CONSTRAINT_NAME AS constraint_name,ENFORCED AS enforced FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME IN ? AND CONSTRAINT_TYPE='CHECK'", &checks},
		{"SELECT TRIGGER_NAME AS trigger_name,ACTION_STATEMENT AS action_statement,EVENT_MANIPULATION AS event_manipulation,EVENT_OBJECT_TABLE AS event_object_table,ACTION_TIMING AS action_timing FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE IN ?", &triggers},
	} {
		if e := db.Raw(q.sql, names).Scan(q.dest).Error; e != nil {
			return e
		}
	}
	for _, sc := range models {
		found := false
		for _, t := range tables {
			if t.TableName == sc.Table {
				found = true
				if t.Engine != "InnoDB" || t.TableCollation != "utf8mb4_bin" {
					return fmt.Errorf("gift requires InnoDB/utf8mb4_bin: %s", sc.Table)
				}
			}
		}
		if !found {
			return fmt.Errorf("gift table missing: %s", sc.Table)
		}
		cols := map[string]bool{}
		for _, c := range columns {
			if c.TableName == sc.Table {
				cols[c.ColumnName] = true
				if c.CollationName.Valid && c.CollationName.String != "utf8mb4_bin" {
					return fmt.Errorf("gift column collation incompatible: %s.%s", sc.Table, c.ColumnName)
				}
			}
		}
		for _, f := range sc.Fields {
			if f.DBName != "" && !cols[f.DBName] {
				return fmt.Errorf("gift column missing: %s.%s", sc.Table, f.DBName)
			}
		}
		byIndex := map[string][]walletMySQLIndex{}
		for _, i := range indexes {
			if i.TableName == sc.Table {
				byIndex[i.IndexName] = append(byIndex[i.IndexName], i)
			}
		}
		for _, expected := range sc.ParseIndexes() {
			actual := byIndex[expected.Name]
			if len(actual) != len(expected.Fields) {
				return fmt.Errorf("gift index missing/incompatible: %s", expected.Name)
			}
			for i, f := range expected.Fields {
				a := actual[i]
				if a.ColumnName != f.Field.DBName || a.SeqInIndex != i+1 || a.SubPart.Valid || a.IsVisible != "YES" || (expected.Class == "UNIQUE" && a.NonUnique != 0) {
					return fmt.Errorf("gift index incompatible: %s", expected.Name)
				}
			}
		}
		byCheck := map[string]bool{}
		for _, c := range checks {
			if c.TableName == sc.Table {
				byCheck[c.ConstraintName] = true
				if c.Enforced != "YES" {
					return fmt.Errorf("gift check not enforced: %s", c.ConstraintName)
				}
			}
		}
		for name := range sc.ParseCheckConstraints() {
			if !byCheck[name] {
				return fmt.Errorf("gift check missing: %s", name)
			}
		}
	}
	for _, t := range triggers {
		if t.TriggerName == "trg_gift_benefit_compensation_update" {
			return fmt.Errorf("legacy compensation trigger requires gift migration")
		}
	}
	normalize := func(v string) string { return strings.Join(strings.Fields(v), " ") }
	for _, t := range expected {
		found := false
		for _, actual := range triggers {
			if strings.HasSuffix(t.name, "_root_v2") && actual.TriggerName == strings.TrimSuffix(t.name, "_root_v2") {
				return fmt.Errorf("legacy review trigger requires root migration: %s", actual.TriggerName)
			}
			if actual.TriggerName == t.name && actual.EventObjectTable == t.table && actual.EventManipulation == t.event && actual.ActionTiming == "BEFORE" && normalize(actual.ActionStatement) == normalize(t.body) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("gift immutable trigger incompatible: %s", t.name)
		}
	}
	return nil
}
