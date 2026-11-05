package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	gm "tb_live_module/model/guard"
	rm "tb_live_module/model/ranking"
	"time"
)

func guardModels() []any {
	return []any{&gm.Plan{}, &gm.Membership{}, &gm.Grant{}, &gm.Compensation{}, &gm.Ledger{}, &gm.Task{}, &gm.AdminPermission{}, &gm.Audit{}}
}
func engagementPresence(db *gorm.DB) (int64, error) {
	names := []string{}
	for _, m := range append(guardModels(), rankingModels()...) {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return 0, e
		}
		names = append(names, st.Schema.Table)
	}
	var count int64
	var e error
	if db.Dialector.Name() == "mysql" {
		e = db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ?", names).Scan(&count).Error
	} else {
		e = db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ?", names).Scan(&count).Error
	}
	return count, e
}
func rankingModels() []any {
	return []any{&rm.AppliedEvent{}, &rm.Receipt{}, &rm.ScoreBucket{}, &rm.Checkpoint{}, &rm.RebuildJob{}, &rm.AdminPermission{}, &rm.Audit{}}
}
func immutableEngagement(table string) bool {
	switch table {
	case "guard_grant", "guard_compensation", "guard_ledger", "guard_audit_log", "ranking_applied_event", "ranking_generation_receipt", "ranking_audit_log":
		return true
	}
	return false
}
func engagementTriggers(db *gorm.DB, models []any) ([]giftTrigger, error) {
	out := []giftTrigger{}
	for _, m := range models {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return nil, e
		}
		table := st.Schema.Table
		if !immutableEngagement(table) {
			continue
		}
		for _, event := range []string{"UPDATE", "DELETE"} {
			body := "BEGIN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='immutable entitlement/projection fact'; END"
			if db.Dialector.Name() == "sqlite" {
				body = "BEGIN SELECT RAISE(ABORT, 'immutable entitlement/projection fact'); END"
			}
			if table == "guard_grant" && event == "UPDATE" {
				changes := []string{}
				for _, f := range st.Schema.Fields {
					if f.DBName == "" || f.DBName == "status" || f.DBName == "compensated_duration" || f.DBName == "expires_at" || f.DBName == "updated_at" {
						continue
					}
					expr := "NOT (NEW.`" + f.DBName + "` <=> OLD.`" + f.DBName + "`)"
					if db.Dialector.Name() == "sqlite" {
						expr = "NEW.`" + f.DBName + "` IS NOT OLD.`" + f.DBName + "`"
					}
					changes = append(changes, expr)
				}
				condition := strings.Join(changes, " OR ")
				body = "BEGIN IF " + condition + " THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='immutable guard source'; END IF; END"
				if db.Dialector.Name() == "sqlite" {
					body = "WHEN " + condition + " BEGIN SELECT RAISE(ABORT, 'immutable guard source'); END"
				}
			}
			out = append(out, giftTrigger{name: "trg_" + table + "_" + strings.ToLower(event), table: table, event: event, body: body})
		}
	}
	// Catalog numbers are never reused, including disabled plans.
	body := "BEGIN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='guard plan cannot be deleted'; END"
	if db.Dialector.Name() == "sqlite" {
		body = "BEGIN SELECT RAISE(ABORT, 'guard plan cannot be deleted'); END"
	}
	for _, m := range models {
		if _, ok := m.(*gm.Plan); ok {
			out = append(out, giftTrigger{name: "trg_guard_plan_delete", table: "guard_plan", event: "DELETE", body: body})
			condition := "NEW.id <> OLD.id OR NEW.plan_no <> OLD.plan_no OR NEW.created_at <> OLD.created_at OR NEW.created_by <> OLD.created_by"
			update := "BEGIN IF " + condition + " THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='guard plan identity immutable'; END IF; END"
			if db.Dialector.Name() == "sqlite" {
				update = "WHEN " + condition + " BEGIN SELECT RAISE(ABORT, 'guard plan identity immutable'); END"
			}
			out = append(out, giftTrigger{name: "trg_guard_plan_update", table: "guard_plan", event: "UPDATE", body: update})
		}
	}
	return out, nil
}
func migrateEngagement(db *gorm.DB, models []any) error {
	return withGiftMigrationLock(db, func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "mysql" {
			if e := validateWalletMySQLRuntime(tx); e != nil {
				return e
			}
			tx = tx.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
		}
		if e := tx.AutoMigrate(models...); e != nil {
			return e
		}
		triggers, e := engagementTriggers(tx, models)
		if e != nil {
			return e
		}
		for _, t := range triggers {
			var n int64
			if tx.Dialector.Name() == "mysql" {
				e = tx.Raw("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=?", t.name).Scan(&n).Error
			} else {
				e = tx.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?", t.name).Scan(&n).Error
			}
			if e != nil {
				return e
			}
			if n == 0 {
				if e = tx.Exec("CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + func() string {
					if tx.Dialector.Name() == "mysql" {
						return " FOR EACH ROW "
					}
					return " "
				}() + t.body).Error; e != nil {
					return e
				}
			}
		}
		return validateEngagement(tx, models)
	})
}
func validateEngagement(db *gorm.DB, models []any) error {
	if db == nil {
		return fmt.Errorf("engagement database unavailable")
	}
	if db.Dialector.Name() == "mysql" {
		return validateEngagementMySQL(db, models)
	}
	for _, m := range models {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		table := st.Schema.Table
		if !db.Migrator().HasTable(m) {
			return fmt.Errorf("missing table %s", table)
		}
		columns, e := db.Migrator().ColumnTypes(m)
		if e != nil {
			return e
		}
		have := map[string]gorm.ColumnType{}
		for _, c := range columns {
			have[c.Name()] = c
		}
		for _, f := range st.Schema.Fields {
			if f.DBName == "" {
				continue
			}
			c, ok := have[f.DBName]
			if !ok {
				return fmt.Errorf("missing column %s.%s", table, f.DBName)
			}
			nullable, known := c.Nullable()
			if f.NotNull && known && nullable {
				return fmt.Errorf("nullable safety column %s.%s", table, f.DBName)
			}
			if size, known := c.Length(); known && f.Size > 0 && size < int64(f.Size) && f.DataType == "string" {
				return fmt.Errorf("undersized column %s.%s", table, f.DBName)
			}
		}
		indexes, e := db.Migrator().GetIndexes(m)
		if e != nil {
			return e
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
					return fmt.Errorf("invalid unique index %s", expected.Name)
				}
				cols := []string{}
				for _, f := range expected.Fields {
					cols = append(cols, f.Field.DBName)
				}
				if strings.Join(cols, ",") != strings.Join(actual.Columns(), ",") {
					return fmt.Errorf("invalid index columns %s", expected.Name)
				}
			}
			if !found {
				return fmt.Errorf("missing index %s", expected.Name)
			}
		}
		for name := range st.Schema.ParseCheckConstraints() {
			if !db.Migrator().HasConstraint(m, name) {
				return fmt.Errorf("missing constraint %s", name)
			}
		}
		if db.Dialector.Name() == "mysql" {
			var n int64
			if e := db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND ENGINE='InnoDB' AND TABLE_COLLATION='utf8mb4_bin'", table).Scan(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return fmt.Errorf("invalid storage for %s", table)
			}
			if e := db.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLLATION_NAME IS NOT NULL AND COLLATION_NAME<>'utf8mb4_bin'", table).Scan(&n).Error; e != nil {
				return e
			}
			if n != 0 {
				return fmt.Errorf("invalid collation for %s", table)
			}
		}
	}
	triggers, e := engagementTriggers(db, models)
	if e != nil {
		return e
	}
	for _, t := range triggers {
		var body string
		expected := t.body
		if db.Dialector.Name() == "mysql" {
			e = db.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE=? AND EVENT_MANIPULATION=? AND ACTION_TIMING='BEFORE'", t.name, t.table, t.event).Scan(&body).Error
		} else {
			e = db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name=?", t.name, t.table).Scan(&body).Error
			expected = "CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + " " + t.body
		}
		if e != nil {
			return e
		}
		if strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(expected), " ") {
			return fmt.Errorf("invalid immutable trigger %s", t.name)
		}
	}
	return nil
}
func MigrateGuard(db *gorm.DB) error {
	if e := migrateEngagement(db, guardModels()); e != nil {
		return e
	}
	now := time.Now().UnixMilli()
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&gm.Task{TaskNo: "guard-admin-control", SourceNo: "guard-admin-control", Kind: "control", Status: "succeeded", CreatedAt: now, UpdatedAt: now, CompletedAt: now}).Error
}
func ValidateGuardSchema(db *gorm.DB) error { return validateEngagement(db, guardModels()) }
