package eventdelivery

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
	model "tb_live_module/model/eventdelivery"
)

// Migrate adds only module-owned tables. Run offline, never as startup DDL.
func Migrate(db *gorm.DB) error {
	if db == nil || (db.Dialector.Name() != "mysql" && db.Dialector.Name() != "sqlite") {
		return ErrUnavailable
	}
	migration := db
	if db.Dialector.Name() == "mysql" {
		migration = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	if err := migration.AutoMigrate(model.Models()...); err != nil {
		return err
	}
	for _, t := range immutableTriggers(db) {
		var n int64
		if db.Dialector.Name() == "mysql" {
			if err := db.Raw("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=?", t.name).Scan(&n).Error; err != nil {
				return err
			}
		} else {
			if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?", t.name).Scan(&n).Error; err != nil {
				return err
			}
		}
		if n == 0 {
			if err := db.Exec(t.ddl).Error; err != nil {
				return err
			}
		}
	}
	return ValidateSchema(db)
}

type trigger struct{ name, body, ddl, table, action string }

func immutableTriggers(db *gorm.DB) []trigger {
	var out []trigger
	for _, table := range []string{"event_delivery_event", "event_delivery_retry_audit"} {
		for _, action := range []string{"UPDATE", "DELETE"} {
			name := "trg_" + table + "_" + strings.ToLower(action)
			body := "BEGIN SELECT RAISE(ABORT, 'immutable event delivery fact'); END"
			if db.Dialector.Name() == "mysql" {
				body = "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='immutable event delivery fact'"
				if action == "UPDATE" {
					var m any = &model.Event{}
					if table == "event_delivery_retry_audit" {
						m = &model.RetryAudit{}
					}
					st := &gorm.Statement{DB: db}
					_ = st.Parse(m)
					var changed []string
					for _, f := range st.Schema.Fields {
						if f.DBName != "" {
							changed = append(changed, "NOT (BINARY OLD.`"+f.DBName+"` <=> BINARY NEW.`"+f.DBName+"`)")
						}
					}
					body = "BEGIN IF " + strings.Join(changed, " OR ") + " THEN " + body + "; END IF; END"
				}
			}
			out = append(out, trigger{name, body, "CREATE TRIGGER " + name + " BEFORE " + action + " ON " + table + " FOR EACH ROW " + body, table, action})
		}
	}
	return out
}

// ValidateSchema performs reads only. It rejects missing columns, uniqueness,
// queue indexes and immutable fact protections before recorder installation.
func ValidateSchema(db *gorm.DB) error {
	if db == nil {
		return ErrUnavailable
	}
	for _, m := range model.Models() {
		st := &gorm.Statement{DB: db}
		if err := st.Parse(m); err != nil {
			return err
		}
		if !db.Migrator().HasTable(m) {
			return wrapSchema(st.Schema.Table)
		}
		for _, f := range st.Schema.Fields {
			if f.DBName != "" && !db.Migrator().HasColumn(m, f.DBName) {
				return wrapSchema(st.Schema.Table + "." + f.DBName)
			}
		}
		indexes, err := db.Migrator().GetIndexes(m)
		if err != nil {
			return err
		}
		for _, expected := range st.Schema.ParseIndexes() {
			found := false
			for _, actual := range indexes {
				if actual.Name() != expected.Name {
					continue
				}
				found = true
				unique, known := actual.Unique()
				if expected.Class == "UNIQUE" && (!known || !unique) {
					return wrapSchema(expected.Name)
				}
				var columns []string
				for _, f := range expected.Fields {
					columns = append(columns, f.Field.DBName)
				}
				if strings.Join(columns, ",") != strings.Join(actual.Columns(), ",") {
					return wrapSchema(expected.Name)
				}
			}
			if !found {
				return wrapSchema(expected.Name)
			}
		}
		for name := range st.Schema.ParseCheckConstraints() {
			if !db.Migrator().HasConstraint(m, name) {
				return wrapSchema(name)
			}
		}
		if db.Dialector.Name() == "mysql" {
			var unsafe int64
			if err := db.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLLATION_NAME IS NOT NULL AND COLLATION_NAME<>'utf8mb4_bin'", st.Schema.Table).Scan(&unsafe).Error; err != nil {
				return err
			}
			if unsafe > 0 {
				return wrapSchema(st.Schema.Table + ".collation")
			}
			if err := db.Raw("SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME=? AND CONSTRAINT_TYPE='CHECK' AND ENFORCED<>'YES'", st.Schema.Table).Scan(&unsafe).Error; err != nil {
				return err
			}
			if unsafe > 0 {
				return wrapSchema(st.Schema.Table + ".check")
			}

			var engine string
			if err := db.Raw("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", st.Schema.Table).Scan(&engine).Error; err != nil {
				return err
			}
			if engine != "InnoDB" {
				return wrapSchema(st.Schema.Table + ".engine")
			}
		}
	}
	if err := validateDefinitions(db); err != nil {
		return err
	}
	for _, t := range immutableTriggers(db) {
		var body string
		if db.Dialector.Name() == "mysql" {
			if err := db.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE=? AND EVENT_MANIPULATION=? AND ACTION_TIMING='BEFORE'", t.name, t.table, t.action).Scan(&body).Error; err != nil {
				return err
			}
		} else {
			if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name=?", t.name, t.table).Scan(&body).Error; err != nil {
				return err
			}
			body = strings.TrimPrefix(body, strings.TrimSuffix(t.ddl, t.body))
		}
		if normalizeDDL(body) != normalizeDDL(t.body) {
			return fmt.Errorf("event delivery immutable protection invalid: %s", t.name)
		}
	}
	return nil
}
func normalizeDDL(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
