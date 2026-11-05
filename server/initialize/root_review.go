package initialize

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type RootReviewTrigger struct {
	Name, TableName, Timing, Event, Body, Definer, SQLMode, CharacterSetClient, CollationConnection, DatabaseCollation string
}

type RootReviewPlan struct {
	Database   string
	Previous   []RootReviewTrigger
	Statements []string
}

// PlanRootReviewMigration reads metadata only. No AutoMigrate, data writes or other triggers.
func PlanRootReviewMigration(db *gorm.DB) (plan RootReviewPlan, err error) {
	if db == nil || db.Dialector.Name() != "mysql" {
		return plan, fmt.Errorf("root review migration requires MySQL")
	}
	if err = db.Raw("SELECT DATABASE()").Scan(&plan.Database).Error; err != nil {
		return
	}
	gift, err := giftTriggers(db)
	if err != nil {
		return plan, err
	}
	earning, err := earningTriggers(db)
	if err != nil {
		return plan, err
	}
	var expected []giftTrigger
	var names []string
	for _, trigger := range append(gift, earning...) {
		if strings.HasSuffix(trigger.name, "_root_v2") {
			expected = append(expected, trigger)
			names = append(names, trigger.name, strings.TrimSuffix(trigger.name, "_root_v2"))
		}
	}
	err = db.Raw("SELECT TRIGGER_NAME AS name, EVENT_OBJECT_TABLE AS table_name, ACTION_TIMING AS timing, EVENT_MANIPULATION AS event, ACTION_STATEMENT AS body, DEFINER AS definer, SQL_MODE AS sql_mode, CHARACTER_SET_CLIENT AS character_set_client, COLLATION_CONNECTION AS collation_connection, DATABASE_COLLATION AS database_collation FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME IN ? ORDER BY TRIGGER_NAME", names).Scan(&plan.Previous).Error
	if err != nil {
		return
	}
	actual := make(map[string]RootReviewTrigger)
	for _, trigger := range plan.Previous {
		actual[trigger.Name] = trigger
	}
	normalize := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	var drops []string
	for _, trigger := range expected {
		legacyName := strings.TrimSuffix(trigger.name, "_root_v2")
		old, hasOld := actual[legacyName]
		current, hasCurrent := actual[trigger.name]
		if !hasOld && !hasCurrent {
			return plan, fmt.Errorf("review guard missing: %s", trigger.table)
		}
		column, oldCondition := "approved_by", "NEW.approved_by = OLD.created_by"
		if trigger.table == "anchor_rule_termination" {
			column, oldCondition = "reviewed_by", "NEW.reviewed_by = OLD.created_by"
		}
		if strings.HasPrefix(trigger.table, "anchor_settlement") {
			oldCondition = "NEW.approved_by=OLD.created_by"
		}
		legacyBody := strings.ReplaceAll(trigger.body, rootSelfReviewCondition(column), oldCondition)
		for _, check := range []struct {
			row    RootReviewTrigger
			exists bool
			body   string
		}{{old, hasOld, legacyBody}, {current, hasCurrent, trigger.body}} {
			if check.exists && (check.row.TableName != trigger.table || check.row.Event != "UPDATE" || check.row.Timing != "BEFORE" || normalize(check.row.Body) != normalize(check.body)) {
				return plan, fmt.Errorf("unknown review trigger: %s", check.row.Name)
			}
		}
		if !hasCurrent {
			plan.Statements = append(plan.Statements, "CREATE TRIGGER "+trigger.name+" BEFORE UPDATE ON "+trigger.table+" FOR EACH ROW "+trigger.body)
		}
		if hasOld {
			drops = append(drops, "DROP TRIGGER "+legacyName)
		}
	}
	// All new guards exist before any old guard is removed.
	plan.Statements = append(plan.Statements, drops...)
	return
}

// MigrateRootReviewTriggers upgrades only four known guards, after a successful backup.
// The same connection and named lock serialize this with existing financial migrations.
func MigrateRootReviewTriggers(db *gorm.DB, backup func(RootReviewPlan) error) error {
	return withGiftMigrationLock(db, func(conn *gorm.DB) error {
		plan, err := PlanRootReviewMigration(conn)
		if err != nil {
			return err
		}
		if len(plan.Statements) == 0 {
			return nil
		}
		if backup == nil {
			return fmt.Errorf("root review migration requires a backup callback")
		}
		if err := backup(plan); err != nil {
			return err
		}
		for _, statement := range plan.Statements {
			if err := conn.Exec(statement).Error; err != nil {
				return err
			}
		}
		remaining, err := PlanRootReviewMigration(conn)
		if err != nil {
			return err
		}
		if len(remaining.Statements) != 0 {
			return fmt.Errorf("root review migration incomplete")
		}
		return nil
	})
}
