package anchorhome

import (
	"database/sql"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"regexp"
	"strings"
	model "tb_live_module/model/anchorhome"
)

func AlbumModels() []any {
	return []any{&model.Album{}, &model.Photo{}, &model.Command{}, &model.Audit{}}
}

type albumGuard struct{ table, operation string }
type albumMigration struct {
	missing []any
	guards  []albumGuard
}

// MigrateAlbumSchema is a first-release, create-only migration. Every existing
// table/guard is verified before any DDL. It never repairs or alters a deployed
// table, including missing indexes/checks or incompatible columns.
func MigrateAlbumSchema(db *gorm.DB) error {
	plan, err := prepareAlbumMigration(db)
	if err != nil {
		return err
	}
	migration := db
	if db.Dialector.Name() == "mysql" {
		migration = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	for _, model := range plan.missing {
		if err := migration.Migrator().CreateTable(model); err != nil {
			return err
		}
	}
	for _, guard := range plan.guards {
		name := "trg_" + guard.table + "_" + guard.operation
		body := "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='anchor_home_append_only'"
		if db.Dialector.Name() == "sqlite" {
			body = "BEGIN SELECT RAISE(ABORT, 'anchor_home_append_only'); END"
		}
		if err := db.Exec("CREATE TRIGGER " + name + " BEFORE " + strings.ToUpper(guard.operation) + " ON " + guard.table + " FOR EACH ROW " + body).Error; err != nil {
			return err
		}
	}
	return VerifyAlbumSchema(db)
}

// VerifyExistingAlbumSchema is a read-only migration preflight. It allows absent
// tables but rejects any incompatible existing object before backup or DDL.
func VerifyExistingAlbumSchema(db *gorm.DB) error { _, err := prepareAlbumMigration(db); return err }

func prepareAlbumMigration(db *gorm.DB) (albumMigration, error) {
	plan := albumMigration{}
	if db == nil || (db.Dialector.Name() != "mysql" && db.Dialector.Name() != "sqlite") {
		return plan, ErrUnavailable
	}
	existing := map[string]bool{}
	for _, model := range AlbumModels() {
		statement := &gorm.Statement{DB: db}
		if err := statement.Parse(model); err != nil {
			return plan, err
		}
		table := statement.Schema.Table
		var kinds []string
		if db.Dialector.Name() == "mysql" {
			if err := db.Raw("SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", table).Scan(&kinds).Error; err != nil {
				return plan, err
			}
			if len(kinds) > 1 || len(kinds) == 1 && kinds[0] != "BASE TABLE" {
				return plan, fmt.Errorf("anchor-home incompatible existing object %s", table)
			}
		} else {
			if err := db.Raw("SELECT type FROM sqlite_master WHERE name=?", table).Scan(&kinds).Error; err != nil {
				return plan, err
			}
			if len(kinds) > 1 || len(kinds) == 1 && kinds[0] != "table" {
				return plan, fmt.Errorf("anchor-home incompatible existing object %s", table)
			}
		}
		if len(kinds) == 0 {
			plan.missing = append(plan.missing, model)
			continue
		}
		if err := verifyAlbumTable(db, model); err != nil {
			return plan, err
		}
		existing[table] = true
	}
	for _, table := range []string{"anchor_home_audit", "anchor_home_command"} {
		for _, operation := range []string{"update", "delete"} {
			found, err := verifyAlbumGuard(db, table, operation)
			if err != nil {
				return plan, err
			}
			if found {
				continue
			}
			if existing[table] {
				// MySQL DDL is not transactional: an interrupted first migration
				// may have created this complete empty table but not its guards.
				// Never bless pre-existing audit/command facts without a guard.
				var occupied bool
				if err := db.Raw("SELECT EXISTS(SELECT 1 FROM " + table + " LIMIT 1)").Scan(&occupied).Error; err != nil {
					return plan, err
				}
				if occupied {
					return plan, fmt.Errorf("anchor-home nonempty table missing immutable guard %s", table)
				}
			}
			plan.guards = append(plan.guards, albumGuard{table, operation})
		}
	}
	return plan, nil
}

// VerifyAlbumSchema performs only reads. Missing/tampered indexes, checks, or audit
// guards fail startup rather than silently weakening the module's guarantees.
func VerifyAlbumSchema(db *gorm.DB) error {
	if db == nil {
		return ErrUnavailable
	}
	for _, model := range AlbumModels() {
		if err := verifyAlbumTable(db, model); err != nil {
			return err
		}
	}
	for _, table := range []string{"anchor_home_audit", "anchor_home_command"} {
		for _, operation := range []string{"update", "delete"} {
			found, err := verifyAlbumGuard(db, table, operation)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("anchor-home missing immutable guard %s_%s", table, operation)
			}
		}
	}
	return nil
}

func verifyAlbumTable(db *gorm.DB, m any) error {
	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(m); err != nil {
		return err
	}
	sc := statement.Schema
	if !db.Migrator().HasTable(m) {
		return fmt.Errorf("anchor-home schema missing table %s", sc.Table)
	}
	if db.Dialector.Name() == "mysql" {
		if err := verifyAlbumColumns(db, sc); err != nil {
			return err
		}
		primary := &schema.Index{Name: "PRIMARY", Class: "UNIQUE"}
		for _, field := range sc.PrimaryFields {
			primary.Fields = append(primary.Fields, schema.IndexOption{Field: field})
		}
		if err := verifyAlbumIndex(db, sc.Table, primary); err != nil {
			return err
		}
	}
	for _, field := range sc.Fields {
		if field.DBName != "" && !db.Migrator().HasColumn(m, field.DBName) {
			return fmt.Errorf("anchor-home schema missing %s.%s", sc.Table, field.DBName)
		}
	}
	for _, index := range sc.ParseIndexes() {
		if err := verifyAlbumIndex(db, sc.Table, index); err != nil {
			return err
		}
	}
	for name, check := range sc.ParseCheckConstraints() {
		if db.Dialector.Name() == "mysql" {
			var rows []struct {
				CheckClause string
				Enforced    string
			}
			err := db.Raw("SELECT c.CHECK_CLAUSE, t.ENFORCED FROM information_schema.CHECK_CONSTRAINTS c JOIN information_schema.TABLE_CONSTRAINTS t ON t.CONSTRAINT_SCHEMA=c.CONSTRAINT_SCHEMA AND t.CONSTRAINT_NAME=c.CONSTRAINT_NAME WHERE c.CONSTRAINT_SCHEMA=DATABASE() AND t.TABLE_NAME=? AND c.CONSTRAINT_NAME=?", sc.Table, name).Scan(&rows).Error
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].Enforced != "YES" || normalizeSchema(rows[0].CheckClause) != normalizeSchema(check.Constraint) {
				return fmt.Errorf("anchor-home invalid check %s", name)
			}
		} else if db.Dialector.Name() == "sqlite" {
			var ddl string
			if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", sc.Table).Scan(&ddl).Error; err != nil {
				return err
			}
			if !strings.Contains(normalizeSchema(ddl), normalizeSchema(check.Constraint)) {
				return fmt.Errorf("anchor-home invalid check %s", name)
			}
		} else {
			return ErrUnavailable
		}
	}
	return nil
}

func verifyAlbumGuard(db *gorm.DB, table, operation string) (bool, error) {
	name := "trg_" + table + "_" + operation
	if db.Dialector.Name() == "mysql" {
		var rows []struct {
			EventManipulation string `gorm:"column:EVENT_MANIPULATION"`
			EventObjectTable  string `gorm:"column:EVENT_OBJECT_TABLE"`
			ActionTiming      string `gorm:"column:ACTION_TIMING"`
			ActionStatement   string `gorm:"column:ACTION_STATEMENT"`
		}
		if err := db.Raw("SELECT EVENT_MANIPULATION,EVENT_OBJECT_TABLE,ACTION_TIMING,ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=?", name).Scan(&rows).Error; err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		if len(rows) != 1 || rows[0].EventObjectTable != table || rows[0].ActionTiming != "BEFORE" || rows[0].EventManipulation != strings.ToUpper(operation) || normalizeSchema(rows[0].ActionStatement) != normalizeSchema("SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='anchor_home_append_only'") {
			return false, fmt.Errorf("anchor-home invalid immutable guard %s", name)
		}
		return true, nil
	}
	var rows []struct{ SQL string }
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=?", name).Scan(&rows).Error; err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	want := "CREATE TRIGGER " + name + " BEFORE " + strings.ToUpper(operation) + " ON " + table + " FOR EACH ROW BEGIN SELECT RAISE(ABORT, 'anchor_home_append_only'); END"
	if len(rows) != 1 || normalizeSchema(rows[0].SQL) != normalizeSchema(want) {
		return false, fmt.Errorf("anchor-home invalid immutable guard %s", name)
	}
	return true, nil
}

func normalizeSchema(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "\\'", "'")
	for _, value := range []string{" ", "\n", "\r", "\t", "`", "(", ")", "_utf8mb4"} {
		s = strings.ReplaceAll(s, value, "")
	}
	return s
}
func verifyAlbumIndex(db *gorm.DB, table string, index *schema.Index) error {
	var cols []string
	unique := false
	if db.Dialector.Name() == "mysql" {
		var rows []struct {
			ColumnName string
			NonUnique  int
			IsVisible  string
			SubPart    sql.NullInt64
		}
		if err := db.Raw("SELECT COLUMN_NAME,NON_UNIQUE,IS_VISIBLE,SUB_PART FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME=? ORDER BY SEQ_IN_INDEX", table, index.Name).Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if r.IsVisible != "YES" || r.SubPart.Valid {
				return fmt.Errorf("anchor-home invisible index %s", index.Name)
			}
			cols = append(cols, r.ColumnName)
			unique = r.NonUnique == 0
		}
	} else {
		var rows []struct {
			Name   string
			Unique int
		}
		if err := db.Raw("PRAGMA index_list(" + table + ")").Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if r.Name == index.Name {
				unique = r.Unique == 1
			}
		}
		var details []struct{ Name string }
		if err := db.Raw("PRAGMA index_info(" + index.Name + ")").Scan(&details).Error; err != nil {
			return err
		}
		for _, r := range details {
			cols = append(cols, r.Name)
		}
	}
	if len(cols) != len(index.Fields) || unique != (index.Class == "UNIQUE") {
		return fmt.Errorf("anchor-home invalid index %s", index.Name)
	}
	for i, f := range index.Fields {
		if cols[i] != f.DBName {
			return fmt.Errorf("anchor-home invalid index columns %s", index.Name)
		}
	}
	return nil
}

var albumIntegerWidth = regexp.MustCompile(`\b(tinyint|smallint|mediumint|int|bigint)\([0-9]+\)`)

func verifyAlbumColumns(db *gorm.DB, sc *schema.Schema) error {
	var table struct{ Engine, TableCollation string }
	if err := db.Raw("SELECT ENGINE,TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", sc.Table).Scan(&table).Error; err != nil {
		return err
	}
	if table.Engine != "InnoDB" || table.TableCollation != "utf8mb4_bin" {
		return fmt.Errorf("anchor-home invalid engine/collation %s", sc.Table)
	}
	var columns []struct {
		ColumnName, ColumnType, IsNullable string
		CollationName                      *string
		ColumnDefault                      sql.NullString
		Extra                              string
	}
	if err := db.Raw("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLLATION_NAME,COLUMN_DEFAULT,EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", sc.Table).Scan(&columns).Error; err != nil {
		return err
	}
	expectedColumns := 0
	for _, field := range sc.Fields {
		if field.DBName != "" {
			expectedColumns++
		}
	}
	if len(columns) != expectedColumns {
		return fmt.Errorf("anchor-home unexpected columns %s", sc.Table)
	}
	for _, field := range sc.Fields {
		if field.DBName == "" {
			continue
		}
		found := false
		want := strings.ToLower(db.Migrator().FullDataTypeOf(field).SQL)
		want = strings.TrimSpace(strings.Split(strings.Split(strings.Split(want, " auto_increment")[0], " not null")[0], " default")[0])
		want = albumIntegerWidth.ReplaceAllString(want, "$1")
		for _, column := range columns {
			if column.ColumnName != field.DBName {
				continue
			}
			found = true
			actual := albumIntegerWidth.ReplaceAllString(strings.ToLower(column.ColumnType), "$1")
			nullable := "YES"
			if field.NotNull || field.PrimaryKey {
				nullable = "NO"
			}
			if actual != want || column.IsNullable != nullable || (column.CollationName != nil && *column.CollationName != "utf8mb4_bin") {
				return fmt.Errorf("anchor-home invalid column %s.%s", sc.Table, field.DBName)
			}
			wantExtra := ""
			if field.AutoIncrement {
				wantExtra = "auto_increment"
			}
			if strings.ToLower(column.Extra) != wantExtra {
				return fmt.Errorf("anchor-home invalid generated column %s.%s", sc.Table, field.DBName)
			}
			wantDefault := field.HasDefaultValue && !field.AutoIncrement
			if column.ColumnDefault.Valid != wantDefault || wantDefault && column.ColumnDefault.String != field.DefaultValue {
				return fmt.Errorf("anchor-home invalid default %s.%s", sc.Table, field.DBName)
			}
		}
		if !found {
			return fmt.Errorf("anchor-home missing column %s.%s", sc.Table, field.DBName)
		}
	}
	return nil
}
