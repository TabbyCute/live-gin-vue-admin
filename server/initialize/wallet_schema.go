package initialize

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	model "tb_live_module/model/wallet"
)

var walletMySQLVersion = regexp.MustCompile(`^8\.([0-9]+)\.([0-9]+)(?:[-+.]|$)`)
var walletBigIntDisplayWidth = regexp.MustCompile(`^bigint\([0-9]+\)( unsigned)?$`)

// MySQL把CHECK保存为规范表达式。保留括号/操作符/字面量逐字核验，避免同名
// CHECK(1=1)或改变优先级仍通过启动检查。增加模型约束时须同步并跑真实MySQL测试。
var walletMySQLCheckClauses = map[string]string{
	"ck_wallet_account_balance": "((`available_balance` >= 0) and (`frozen_balance` >= 0) and (`version` >= 0))",
	"ck_wallet_account_status":  "(`status` in ('normal','disabled'))",
	"ck_wallet_ledger_amount":   "((`change_amount` <> 0) and (`balance_before` >= 0) and (`balance_after` >= 0) and ((`balance_after` - `balance_before`) = `change_amount`) and (`wallet_version` > 0))",
	"ck_wallet_order_amount":    "(`amount` > 0)",
	"ck_wallet_order_status":    "(`status` in ('pending','credited','rejected','reversed'))",
	"ck_wallet_order_source":    "((`external_order_no` <> '') and (((`kind` = 'grant') and (`source_type` in ('manual','legacy'))) or ((`kind` = 'reverse') and (`source_type` = 'reversal'))))",
	"ck_wallet_order_kind":      "(`kind` in ('grant','reverse'))",
	"ck_wallet_quota_amount":    "(`amount` >= 0)",
}
var walletSQLLiteral = regexp.MustCompile(`'[^']*'`)

func walletMySQLCheckClause(name string) string {
	return walletSQLLiteral.ReplaceAllStringFunc(walletMySQLCheckClauses[name], func(literal string) string {
		return "_utf8mb4\\'" + literal[1:len(literal)-1] + "\\'"
	})
}

func checkWalletMySQLVersion(version, mode string) error {
	m := walletMySQLVersion.FindStringSubmatch(version)
	if m == nil || strings.Contains(strings.ToLower(version), "mariadb") {
		return fmt.Errorf("wallet requires MySQL 8.0.16+ or 8.4, got %q", version)
	}
	minor, _ := strconv.Atoi(m[1])
	patch, _ := strconv.Atoi(m[2])
	if minor == 0 && patch < 16 {
		return fmt.Errorf("wallet requires enforced CHECK constraints (MySQL >=8.0.16), got %q", version)
	}
	strict := false
	for _, v := range strings.Split(mode, ",") {
		if v == "STRICT_TRANS_TABLES" || v == "STRICT_ALL_TABLES" {
			strict = true
		}
	}
	if !strict {
		return fmt.Errorf("wallet requires strict MySQL session sql_mode")
	}
	return nil
}

func validateWalletMySQLRuntime(db *gorm.DB) error {
	var runtime struct{ Version, Mode string }
	if err := db.Raw("SELECT VERSION() AS version, @@SESSION.sql_mode AS mode").Scan(&runtime).Error; err != nil {
		return err
	}
	return checkWalletMySQLVersion(runtime.Version, runtime.Mode)
}

type walletMySQLColumn struct {
	TableName, ColumnName, ColumnType, IsNullable, Extra string
	ColumnDefault, CollationName                         sql.NullString
}
type walletMySQLIndex struct {
	TableName, IndexName, ColumnName, IndexType, IsVisible string
	SubPart                                                sql.NullInt64
	NonUnique, SeqInIndex                                  int
}
type walletMySQLCheck struct {
	TableName, ConstraintName, Enforced, CheckClause string
}

// 钱包全部结构只需6次批量SELECT（包括运行环境），查询失败保留原始错误。
// 不使用会吞查询错误的HasConstraint/CurrentDatabase，也不逐列/逐索引往返远端。
func validateWalletMySQLSchema(db *gorm.DB) error {
	if err := validateWalletMySQLRuntime(db); err != nil {
		return fmt.Errorf("wallet runtime: %w", err)
	}
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
	var tables []struct{ TableName, Engine, TableCollation string }
	var columns []walletMySQLColumn
	var indexes []walletMySQLIndex
	var checks []walletMySQLCheck
	var triggers []struct{ TriggerName, ActionStatement, EventManipulation, EventObjectTable, ActionTiming string }
	for _, query := range []struct {
		name, sql string
		dest      any
	}{
		{"tables", "SELECT TABLE_NAME AS table_name, ENGINE AS engine, TABLE_COLLATION AS table_collation FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", &tables},
		{"columns", "SELECT TABLE_NAME AS table_name, COLUMN_NAME AS column_name, COLUMN_TYPE AS column_type, IS_NULLABLE AS is_nullable, COLUMN_DEFAULT AS column_default, EXTRA AS extra, COLLATION_NAME AS collation_name FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", &columns},
		{"indexes", "SELECT TABLE_NAME AS table_name, INDEX_NAME AS index_name, COLUMN_NAME AS column_name, INDEX_TYPE AS index_type, IS_VISIBLE AS is_visible, SUB_PART AS sub_part, NON_UNIQUE AS non_unique, SEQ_IN_INDEX AS seq_in_index FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ? ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX", &indexes},
		{"checks", `SELECT tc.TABLE_NAME AS table_name, tc.CONSTRAINT_NAME AS constraint_name, tc.ENFORCED AS enforced, cc.CHECK_CLAUSE AS check_clause FROM information_schema.TABLE_CONSTRAINTS tc JOIN information_schema.CHECK_CONSTRAINTS cc ON cc.CONSTRAINT_SCHEMA=tc.CONSTRAINT_SCHEMA AND cc.CONSTRAINT_NAME=tc.CONSTRAINT_NAME WHERE tc.CONSTRAINT_SCHEMA=DATABASE() AND tc.TABLE_NAME IN ? AND tc.CONSTRAINT_TYPE='CHECK'`, &checks},
		{"triggers", "SELECT TRIGGER_NAME AS trigger_name, ACTION_STATEMENT AS action_statement, EVENT_MANIPULATION AS event_manipulation, EVENT_OBJECT_TABLE AS event_object_table, ACTION_TIMING AS action_timing FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE IN ?", &triggers},
	} {
		if err := db.Raw(query.sql, names).Scan(query.dest).Error; err != nil {
			return fmt.Errorf("query wallet %s: %w", query.name, err)
		}
	}
	for _, sc := range schemas {
		found := false
		for _, table := range tables {
			if table.TableName == sc.Table {
				found = true
				if table.Engine != "InnoDB" || table.TableCollation != "utf8mb4_bin" {
					return fmt.Errorf("wallet table %s requires InnoDB/utf8mb4_bin", sc.Table)
				}
			}
		}
		if !found {
			return fmt.Errorf("wallet table missing: %s", sc.Table)
		}
		var tableColumns []walletMySQLColumn
		var tableIndexes []walletMySQLIndex
		var tableChecks []walletMySQLCheck
		for _, col := range columns {
			if col.TableName == sc.Table {
				tableColumns = append(tableColumns, col)
			}
		}
		for _, idx := range indexes {
			if idx.TableName == sc.Table {
				tableIndexes = append(tableIndexes, idx)
			}
		}
		for _, check := range checks {
			if check.TableName == sc.Table {
				tableChecks = append(tableChecks, check)
			}
		}
		if err := validateWalletMySQLColumns(sc, tableColumns, tableIndexes, tableChecks); err != nil {
			return err
		}
	}
	for _, expected := range walletTriggers() {
		found := false
		for _, actual := range triggers {
			if actual.TriggerName == expected.name && actual.ActionStatement == expected.body("mysql") && actual.EventManipulation == expected.event && actual.EventObjectTable == expected.table && actual.ActionTiming == "BEFORE" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("wallet immutable trigger missing/incompatible: %s", expected.name)
		}
	}
	return nil
}

func validateWalletMySQLColumns(sc *schema.Schema, columns []walletMySQLColumn, indexes []walletMySQLIndex, actualChecks []walletMySQLCheck) error {
	if len(columns) != len(sc.DBNames) {
		return fmt.Errorf("wallet column set incompatible: %s", sc.Table)
	}
	byName := make(map[string]walletMySQLColumn, len(columns))
	for _, c := range columns {
		byName[c.ColumnName] = c
	}
	for _, field := range sc.Fields {
		if field.DBName == "" {
			continue
		}
		actual, found := byName[field.DBName]
		// 8.0早期的整数显示宽度不影响存储范围；不能把INT或UNSIGNED归一化掉。
		actual.ColumnType = walletBigIntDisplayWidth.ReplaceAllString(actual.ColumnType, "bigint$1")
		expectedType := strings.ToLower(field.TagSettings["TYPE"])
		if expectedType == "" {
			switch field.DataType {
			case schema.Int:
				expectedType = "bigint"
			case schema.Uint:
				expectedType = "bigint unsigned"
			}
		}
		nullable := "YES"
		if field.NotNull || field.PrimaryKey {
			nullable = "NO"
		}
		extra := ""
		if field.AutoIncrement {
			extra = "auto_increment"
		}
		if !found || expectedType == "" || actual.ColumnType != expectedType || actual.IsNullable != nullable || actual.Extra != extra {
			return fmt.Errorf("wallet column type/null/extra incompatible: %s.%s (expected %s %s %s)", sc.Table, field.DBName, expectedType, nullable, extra)
		}
		expectedDefault := sql.NullString{}
		if field.HasDefaultValue && !field.AutoIncrement {
			expectedDefault = sql.NullString{String: field.DefaultValue, Valid: true}
		}
		if actual.ColumnDefault != expectedDefault {
			return fmt.Errorf("wallet column default incompatible: %s.%s", sc.Table, field.DBName)
		}
		if field.GORMDataType == schema.String && (!actual.CollationName.Valid || actual.CollationName.String != "utf8mb4_bin") {
			return fmt.Errorf("wallet column collation incompatible: %s.%s", sc.Table, field.DBName)
		}
	}
	// 禁止前缀/函数/不可见索引以及错误主键；否则同名索引检查仍可能误放行。
	primary := 0
	byIndexName := map[string][]walletMySQLIndex{}
	for _, idx := range indexes {
		byIndexName[idx.IndexName] = append(byIndexName[idx.IndexName], idx)
		if idx.SubPart.Valid || idx.ColumnName == "" || idx.IndexType != "BTREE" || idx.IsVisible != "YES" {
			return fmt.Errorf("wallet index incompatible: %s.%s", sc.Table, idx.IndexName)
		}
		if idx.IndexName == "PRIMARY" {
			primary++
			if idx.ColumnName != "id" || idx.NonUnique != 0 || idx.SeqInIndex != 1 {
				return fmt.Errorf("wallet primary key incompatible: %s", sc.Table)
			}
		}
	}
	if primary != 1 {
		return fmt.Errorf("wallet primary key missing: %s", sc.Table)
	}
	for _, expected := range sc.ParseIndexes() {
		actual, found := byIndexName[expected.Name]
		if !found {
			return fmt.Errorf("wallet index missing: %s", expected.Name)
		}
		if len(actual) != len(expected.Fields) {
			return fmt.Errorf("wallet index columns incompatible: %s", expected.Name)
		}
		for i, field := range expected.Fields {
			if actual[i].ColumnName != field.Field.DBName || actual[i].SeqInIndex != i+1 {
				return fmt.Errorf("wallet index columns incompatible: %s", expected.Name)
			}
			if expected.Class == "UNIQUE" && actual[i].NonUnique != 0 {
				return fmt.Errorf("wallet index must be unique: %s", expected.Name)
			}
		}
	}
	byCheckName := make(map[string]walletMySQLCheck, len(actualChecks))
	for _, check := range actualChecks {
		byCheckName[check.ConstraintName] = check
	}
	expectedChecks := sc.ParseCheckConstraints()
	checkNames := make([]string, 0, len(expectedChecks))
	for name := range expectedChecks {
		checkNames = append(checkNames, name)
	}
	sort.Strings(checkNames)
	for _, name := range checkNames {
		actual, found := byCheckName[name]
		if !found {
			return fmt.Errorf("wallet check missing: %s", name)
		}
		if actual.Enforced != "YES" {
			return fmt.Errorf("wallet check not enforced: %s", name)
		}
		if expected := walletMySQLCheckClause(name); expected == "" || actual.CheckClause != expected {
			return fmt.Errorf("wallet check definition incompatible: %s", name)
		}
	}
	return nil
}
