package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"strings"
)

const bankAttemptIndex = "idx_bank_attempt_account_status_time"

func ensureBankAttemptIndex(db *gorm.DB) error {
	if !db.Migrator().HasTable("payout_attempt") {
		return fmt.Errorf("migrate payout before bank reconciliation")
	}
	if db.Migrator().HasIndex("payout_attempt", bankAttemptIndex) {
		return nil
	}
	if db.Dialector.Name() == "mysql" {
		// Dedicated migration connection; restore session budget even after failure.
		var previous int64
		if e := db.Raw("SELECT @@SESSION.lock_wait_timeout").Scan(&previous).Error; e != nil {
			return e
		}
		if e := db.Exec("SET SESSION lock_wait_timeout=10").Error; e != nil {
			return e
		}
		defer db.Exec(fmt.Sprintf("SET SESSION lock_wait_timeout=%d", previous))
		return db.Exec("ALTER TABLE payout_attempt ADD INDEX " + bankAttemptIndex + " (channel_account_no,status,payment_time), ALGORITHM=INPLACE, LOCK=NONE").Error
	}
	return db.Exec("CREATE INDEX " + bankAttemptIndex + " ON payout_attempt(channel_account_no,status,payment_time)").Error
}
func validateBankAttemptIndex(db *gorm.DB) error {
	if db.Dialector.Name() == "mysql" {
		var columns []struct {
			ColumnName, IsVisible string
			SeqInIndex, NonUnique int
			SubPart               *int
		}
		if e := db.Raw("SELECT column_name,is_visible,seq_in_index,non_unique,sub_part FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='payout_attempt' AND index_name=? ORDER BY seq_in_index", bankAttemptIndex).Scan(&columns).Error; e != nil {
			return e
		}
		expected := []string{"channel_account_no", "status", "payment_time"}
		if len(columns) != len(expected) {
			return fmt.Errorf("bank reconciliation payout range index missing")
		}
		for i, c := range columns {
			if c.ColumnName != expected[i] || c.SeqInIndex != i+1 || c.NonUnique != 1 || c.SubPart != nil || c.IsVisible != "YES" {
				return fmt.Errorf("bank reconciliation payout range index incompatible")
			}
		}
		return nil
	}
	indexes, e := db.Migrator().GetIndexes("payout_attempt")
	if e != nil {
		return e
	}
	for _, idx := range indexes {
		if idx.Name() == bankAttemptIndex && strings.Join(idx.Columns(), ",") == "channel_account_no,status,payment_time" {
			return nil
		}
	}
	return fmt.Errorf("bank reconciliation payout range index missing or incompatible")
}
func validateBankDefinitions(db *gorm.DB) error {
	var names []string
	var schemas []*schema.Schema
	for _, m := range bankModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		schemas = append(schemas, st.Schema)
		names = append(names, st.Schema.Table)
	}
	var columns []struct{ TableName, ColumnName, ColumnType, IsNullable string }
	var checks []struct{ TableName, ConstraintName, CheckClause string }
	var primary []struct {
		TableName, ColumnName string
		SeqInIndex            int
	}
	if e := db.Raw("SELECT table_name,column_name,column_type,is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ?", names).Scan(&columns).Error; e != nil {
		return e
	}
	if e := db.Raw("SELECT tc.table_name,tc.constraint_name,cc.check_clause FROM information_schema.table_constraints tc JOIN information_schema.check_constraints cc ON tc.constraint_schema=cc.constraint_schema AND tc.constraint_name=cc.constraint_name WHERE tc.constraint_schema=DATABASE() AND tc.table_name IN ? AND tc.constraint_type='CHECK'", names).Scan(&checks).Error; e != nil {
		return e
	}
	if e := db.Raw("SELECT table_name,column_name,seq_in_index FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name IN ? AND index_name='PRIMARY' ORDER BY table_name,seq_in_index", names).Scan(&primary).Error; e != nil {
		return e
	}
	normalize := func(v string) string {
		v = strings.ToLower(v)
		for _, part := range []string{"_utf8mb4", "`", "\\", "(", ")", " ", "\n", "\t"} {
			v = strings.ReplaceAll(v, part, "")
		}
		return v
	}
	for _, sc := range schemas {
		for _, field := range sc.Fields {
			if field.DBName == "" {
				continue
			}
			expected := strings.ToLower(db.Dialector.DataTypeOf(field))
			if expected == "boolean" {
				expected = "tinyint(1)"
			}
			nullable := "YES"
			if field.NotNull || field.PrimaryKey {
				nullable = "NO"
			}
			found := false
			for _, actual := range columns {
				if actual.TableName == sc.Table && actual.ColumnName == field.DBName && walletBigIntDisplayWidth.ReplaceAllString(actual.ColumnType, "bigint$1") == expected && actual.IsNullable == nullable {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("bank reconciliation column definition incompatible: %s.%s", sc.Table, field.DBName)
			}
		}
		var keys []string
		for _, key := range primary {
			if key.TableName == sc.Table {
				keys = append(keys, key.ColumnName)
			}
		}
		var expectedKeys []string
		for _, field := range sc.PrimaryFields {
			expectedKeys = append(expectedKeys, field.DBName)
		}
		if strings.Join(keys, ",") != strings.Join(expectedKeys, ",") {
			return fmt.Errorf("bank reconciliation primary key incompatible: %s", sc.Table)
		}
		for name, expected := range sc.ParseCheckConstraints() {
			found := false
			for _, actual := range checks {
				if actual.TableName == sc.Table && actual.ConstraintName == name && normalize(actual.CheckClause) == normalize(expected.Constraint) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("bank reconciliation check definition incompatible: %s", name)
			}
		}
	}
	return nil
}
