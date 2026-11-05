package initialize

import (
	"database/sql"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"strings"
)

// Match MySQL's canonical CHECK expressions, including grouping and literals.
// A same-named CHECK(1=1) must not qualify for the no-DDL migration path.
var payoutMySQLChecks = map[string]string{
	"ck_payee_owner":     "(`owner_type` = 'anchor')",
	"ck_payee_method":    "(`method` in ('bank_card','alipay','wechat'))",
	"ck_payee_payload":   "((`payload_version` = 1) and (`version` = 1))",
	"ck_payee_status":    "(`status` in ('active','disabled'))",
	"ck_payee_default":   "((`default_slot` is null) or ((`default_slot` = 1) and (`status` = 'active')))",
	"ck_payee_time":      "((`created_at` > 0) and (`updated_at` >= `created_at`) and (((`status` = 'active') and (`disabled_at` = 0)) or ((`status` = 'disabled') and (`disabled_at` >= `created_at`))))",
	"ck_payout_business": "(`business_type` = 'anchor_withdrawal')",
	"ck_payout_amount":   "(`amount` > 0)",
	"ck_payout_method":   "(`payee_method` in ('bank_card','alipay','wechat'))",
	"ck_payout_payload":  "(`payload_version` = 1)",
	"ck_payout_status":   "(`status` in ('ready','processing','succeeded','failed','unknown','cancelled'))",
	"ck_payout_actor":    "(`created_by` > 0)",
	"ck_payout_time":     "((`created_at` > 0) and (`updated_at` = `created_at`))",
	"ck_payout_history":  "((`from_status` = '') and (`to_status` = 'ready'))",
	"ck_payout_audit":    "(`action` = 'order.create')",
}

func validatePayoutMySQLDefinitions(db *gorm.DB) error {
	return validatePayoutModelDefinitions(db, payoutModels())
}
func validatePayoutModelDefinitions(db *gorm.DB, models []any) error {
	var names []string
	var schemas []*schema.Schema
	for _, m := range models {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		names = append(names, st.Schema.Table)
		schemas = append(schemas, st.Schema)
	}
	var columns []walletMySQLColumn
	var checks []walletMySQLCheck
	var primary []walletMySQLIndex
	for _, q := range []struct {
		sql  string
		dest any
	}{
		{"SELECT TABLE_NAME AS table_name,COLUMN_NAME AS column_name,COLUMN_TYPE AS column_type,IS_NULLABLE AS is_nullable,COLUMN_DEFAULT AS column_default,EXTRA AS extra FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", &columns},
		{"SELECT tc.TABLE_NAME AS table_name,tc.CONSTRAINT_NAME AS constraint_name,cc.CHECK_CLAUSE AS check_clause FROM information_schema.TABLE_CONSTRAINTS tc JOIN information_schema.CHECK_CONSTRAINTS cc ON cc.CONSTRAINT_SCHEMA=tc.CONSTRAINT_SCHEMA AND cc.CONSTRAINT_NAME=tc.CONSTRAINT_NAME WHERE tc.CONSTRAINT_SCHEMA=DATABASE() AND tc.TABLE_NAME IN ? AND tc.CONSTRAINT_TYPE='CHECK'", &checks},
		{"SELECT TABLE_NAME AS table_name,COLUMN_NAME AS column_name,SEQ_IN_INDEX AS seq_in_index FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ? AND INDEX_NAME='PRIMARY' ORDER BY TABLE_NAME,SEQ_IN_INDEX", &primary},
	} {
		if e := db.Raw(q.sql, names).Scan(q.dest).Error; e != nil {
			return e
		}
	}
	for _, sc := range schemas {
		cols := map[string]walletMySQLColumn{}
		for _, c := range columns {
			if c.TableName == sc.Table {
				cols[c.ColumnName] = c
			}
		}
		if len(cols) != len(sc.DBNames) {
			return fmt.Errorf("payout column set incompatible: %s", sc.Table)
		}
		for _, f := range sc.Fields {
			if f.DBName == "" {
				continue
			}
			expectedType := strings.ToLower(f.TagSettings["TYPE"])
			if expectedType == "" {
				switch f.DataType {
				case schema.String:
					expectedType = fmt.Sprintf("varchar(%d)", f.Size)
				case schema.Int:
					expectedType = "bigint"
				case schema.Uint:
					expectedType = "bigint unsigned"
				}
			}
			nullable, extra := "YES", ""
			if f.NotNull || f.PrimaryKey {
				nullable = "NO"
			}
			if f.AutoIncrement {
				extra = "auto_increment"
			}
			def := sql.NullString{}
			if f.HasDefaultValue && !f.AutoIncrement {
				def = sql.NullString{String: f.DefaultValue, Valid: true}
			}
			c, found := cols[f.DBName]
			if !found || expectedType == "" || walletBigIntDisplayWidth.ReplaceAllString(c.ColumnType, "bigint$1") != expectedType || c.IsNullable != nullable || c.Extra != extra || c.ColumnDefault != def {
				return fmt.Errorf("payout column definition incompatible: %s.%s", sc.Table, f.DBName)
			}
		}
		var keys []walletMySQLIndex
		for _, p := range primary {
			if p.TableName == sc.Table {
				keys = append(keys, p)
			}
		}
		if len(keys) != len(sc.PrimaryFields) {
			return fmt.Errorf("payout primary key incompatible: %s", sc.Table)
		}
		for i, f := range sc.PrimaryFields {
			if keys[i].ColumnName != f.DBName || keys[i].SeqInIndex != i+1 {
				return fmt.Errorf("payout primary key incompatible: %s", sc.Table)
			}
		}
		for name := range sc.ParseCheckConstraints() {
			expected := walletSQLLiteral.ReplaceAllStringFunc(payoutMySQLChecks[name], func(v string) string { return "_utf8mb4\\'" + v[1:len(v)-1] + "\\'" })
			found := false
			for _, c := range checks {
				if c.TableName == sc.Table && c.ConstraintName == name && expected != "" && c.CheckClause == expected {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("payout check definition incompatible: %s", name)
			}
		}
	}
	return nil
}
