package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
)

// Batch information_schema reads keep startup verification independent of the
// number of columns. Runtime requests do not execute migration/introspection.
func validateEngagementMySQL(db *gorm.DB, models []any) error {
	triggers, e := engagementTriggers(db, models)
	if e != nil {
		return e
	}
	if e = validateFinancialMySQLSchema(db, models, triggers); e != nil {
		return e
	}
	names := []string{}
	for _, m := range models {
		st := &gorm.Statement{DB: db}
		if e = st.Parse(m); e != nil {
			return e
		}
		names = append(names, st.Schema.Table)
	}
	var columns []struct{ TableName, ColumnName, ColumnType, IsNullable string }
	if e = db.Raw("SELECT TABLE_NAME,COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", names).Scan(&columns).Error; e != nil {
		return e
	}
	var checks []struct{ TableName, ConstraintName, CheckClause string }
	if e = db.Raw("SELECT t.TABLE_NAME,c.CONSTRAINT_NAME,c.CHECK_CLAUSE FROM information_schema.CHECK_CONSTRAINTS c JOIN information_schema.TABLE_CONSTRAINTS t ON t.CONSTRAINT_SCHEMA=c.CONSTRAINT_SCHEMA AND t.CONSTRAINT_NAME=c.CONSTRAINT_NAME WHERE t.CONSTRAINT_SCHEMA=DATABASE() AND t.TABLE_NAME IN ?", names).Scan(&checks).Error; e != nil {
		return e
	}
	// Our check expressions use conjunctions only; MySQL adds redundant grouping,
	// identifier quotes and charset prefixes when serializing these expressions.
	canonical := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "_utf8mb4", "")
		s = strings.ReplaceAll(s, "\\'", "'")
		return strings.NewReplacer(" ", "", "\n", "", "\t", "", "`", "", "(", "", ")", "").Replace(s)
	}
	for _, m := range models {
		st := &gorm.Statement{DB: db}
		if e = st.Parse(m); e != nil {
			return e
		}
		for _, f := range st.Schema.Fields {
			if f.DBName == "" {
				continue
			}
			expected := strings.ToLower(db.Dialector.DataTypeOf(f))
			expected = strings.TrimSpace(strings.ReplaceAll(expected, "auto_increment", ""))
			found := false
			for _, c := range columns {
				if c.TableName == st.Schema.Table && c.ColumnName == f.DBName {
					found = true
					if strings.ToLower(c.ColumnType) != expected || (f.NotNull || f.PrimaryKey) && c.IsNullable != "NO" {
						return fmt.Errorf("engagement column type/nullability incompatible: %s.%s", st.Schema.Table, f.DBName)
					}
				}
			}
			if !found {
				return fmt.Errorf("engagement column missing")
			}
		}
		for name, check := range st.Schema.ParseCheckConstraints() {
			found := false
			for _, actual := range checks {
				if actual.TableName == st.Schema.Table && actual.ConstraintName == name && canonical(actual.CheckClause) == canonical(check.Constraint) {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("engagement check definition incompatible: %s", name)
			}
		}
	}
	return nil
}
