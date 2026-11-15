package eventdelivery

import (
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"regexp"
	"strings"
	model "tb_live_module/model/eventdelivery"
)

var integerDisplayWidth = regexp.MustCompile(`(?i)\b(tinyint|smallint|mediumint|int|bigint)\(\d+\)`)

func normalizeCheck(v string) string {
	v = strings.ToLower(v)
	for _, part := range []string{"_utf8mb4", "`", "\\", "(", ")", " ", "\n", "\t"} {
		v = strings.ReplaceAll(v, part, "")
	}
	return v
}
func validateDefinitions(db *gorm.DB) error {
	var schemas []*schema.Schema
	var names []string
	for _, m := range model.Models() {
		st := &gorm.Statement{DB: db}
		if err := st.Parse(m); err != nil {
			return err
		}
		schemas = append(schemas, st.Schema)
		names = append(names, st.Schema.Table)
	}
	if db.Dialector.Name() == "sqlite" {
		for _, sc := range schemas {
			columns, err := db.Migrator().ColumnTypes(sc.Table)
			if err != nil {
				return err
			}
			for _, field := range sc.Fields {
				if field.DBName == "" {
					continue
				}
				found := false
				for _, actual := range columns {
					if actual.Name() != field.DBName {
						continue
					}
					found = true
					expectedType := strings.ToLower(db.Dialector.DataTypeOf(field))
					expectedType = strings.TrimSuffix(expectedType, " primary key autoincrement")
					if strings.ToLower(actual.DatabaseTypeName()) != expectedType {
						return wrapSchema(sc.Table + "." + field.DBName + ".type")
					}
					if field.PrimaryKey {
						pk, known := actual.PrimaryKey()
						if !known || !pk {
							return wrapSchema(sc.Table + ".primary")
						}
					}
					if field.NotNull {
						nullable, known := actual.Nullable()
						if known && nullable {
							return wrapSchema(sc.Table + "." + field.DBName + ".nullable")
						}
					}
				}
				if !found {
					return wrapSchema(sc.Table + "." + field.DBName)
				}
			}
			var ddl string
			if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", sc.Table).Scan(&ddl).Error; err != nil {
				return err
			}
			for name, check := range sc.ParseCheckConstraints() {
				if !strings.Contains(normalizeCheck(ddl), normalizeCheck("CONSTRAINT "+name+" CHECK "+check.Constraint)) {
					return wrapSchema(name + ".definition")
				}
			}
		}
		return nil
	}
	if db.Dialector.Name() != "mysql" {
		return ErrUnavailable
	}
	var columns []struct{ TableName, ColumnName, ColumnType, IsNullable string }
	var checks []struct{ TableName, ConstraintName, CheckClause string }
	var indexes []struct {
		TableName, IndexName, ColumnName, IsVisible string
		SeqInIndex, NonUnique                       int
		SubPart                                     *int
	}
	if err := db.Raw("SELECT table_name,column_name,column_type,is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ?", names).Scan(&columns).Error; err != nil {
		return err
	}
	if err := db.Raw("SELECT tc.table_name,tc.constraint_name,cc.check_clause FROM information_schema.table_constraints tc JOIN information_schema.check_constraints cc ON tc.constraint_schema=cc.constraint_schema AND tc.constraint_name=cc.constraint_name WHERE tc.constraint_schema=DATABASE() AND tc.table_name IN ? AND tc.constraint_type='CHECK'", names).Scan(&checks).Error; err != nil {
		return err
	}
	if err := db.Raw("SELECT table_name,index_name,column_name,is_visible,seq_in_index,non_unique,sub_part FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name IN ? ORDER BY table_name,index_name,seq_in_index", names).Scan(&indexes).Error; err != nil {
		return err
	}
	for _, sc := range schemas {
		for _, field := range sc.Fields {
			if field.DBName == "" {
				continue
			}
			expected := strings.ReplaceAll(strings.ToLower(db.Dialector.DataTypeOf(field)), " auto_increment", "")
			nullable := "YES"
			if field.NotNull || field.PrimaryKey {
				nullable = "NO"
			}
			found := false
			for _, c := range columns {
				if c.TableName == sc.Table && c.ColumnName == field.DBName && integerDisplayWidth.ReplaceAllString(c.ColumnType, "$1") == expected && c.IsNullable == nullable {
					found = true
					break
				}
			}
			if !found {
				return wrapSchema(sc.Table + "." + field.DBName + ".definition")
			}
		}
		expectedIndexes := map[string][]string{"PRIMARY": {}}
		expectedUnique := map[string]bool{"PRIMARY": true}
		for _, field := range sc.PrimaryFields {
			expectedIndexes["PRIMARY"] = append(expectedIndexes["PRIMARY"], field.DBName)
		}
		for _, idx := range sc.ParseIndexes() {
			for _, field := range idx.Fields {
				expectedIndexes[idx.Name] = append(expectedIndexes[idx.Name], field.Field.DBName)
			}
			expectedUnique[idx.Name] = idx.Class == "UNIQUE"
		}
		for name, want := range expectedIndexes {
			var got []string
			for _, idx := range indexes {
				if idx.TableName != sc.Table || idx.IndexName != name {
					continue
				}
				if idx.SubPart != nil || idx.IsVisible != "YES" || (expectedUnique[name] && idx.NonUnique != 0) {
					return wrapSchema(name + ".definition")
				}
				got = append(got, idx.ColumnName)
			}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				return wrapSchema(name + ".columns")
			}
		}
		for name, want := range sc.ParseCheckConstraints() {
			found := false
			for _, actual := range checks {
				if actual.TableName == sc.Table && actual.ConstraintName == name && normalizeCheck(actual.CheckClause) == normalizeCheck(want.Constraint) {
					found = true
					break
				}
			}
			if !found {
				return wrapSchema(name + ".definition")
			}
		}
	}
	return nil
}
