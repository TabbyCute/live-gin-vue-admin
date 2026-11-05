package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
	"tb_live_module/model/system"
)

type giftTrigger struct{ name, table, event, body string }

func giftTriggers(db *gorm.DB) ([]giftTrigger, error) {
	var result []giftTrigger
	for _, m := range giftModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return nil, e
		}
		table := st.Schema.Table
		if table == "anchor_income_lock" || table == "gift_category" || table == "gift_item" || table == "gift_control" || table == "gift_admin_permission" {
			continue
		}
		for _, event := range []string{"DELETE", "UPDATE"} {
			allowed := map[string]bool{}
			if event == "UPDATE" {
				switch table {
				case "gift_order":
					allowed = map[string]bool{"refunded_coin_amount": true, "status": true}
				case "gift_event_outbox":
					allowed = map[string]bool{"delivery_status": true, "retry_count": true, "last_error": true, "delivered_at": true}
				case "gift_benefit_grant":
					allowed = map[string]bool{"status": true, "retry_count": true, "last_error": true, "updated_at": true}
				case "gift_benefit_compensation":
					allowed = map[string]bool{"status": true, "retry_count": true, "last_error": true, "lease_until": true, "updated_at": true, "completed_at": true}
				case "anchor_rule_termination":
					allowed = map[string]bool{"status": true, "reviewed_by": true, "reviewed_at": true}
				case "anchor_revenue_rule":
					allowed = map[string]bool{"status": true, "approved_by": true, "approved_at": true}
				}
			}
			var changes []string
			if event == "UPDATE" && len(allowed) > 0 {
				for _, f := range st.Schema.Fields {
					if f.DBName == "" || allowed[f.DBName] {
						continue
					}
					if db.Dialector.Name() == "mysql" {
						changes = append(changes, "NOT (BINARY OLD.`"+f.DBName+"` <=> BINARY NEW.`"+f.DBName+"`)")
					} else {
						changes = append(changes, "OLD.`"+f.DBName+"` IS NOT NEW.`"+f.DBName+"`")
					}
				}
				if table == "anchor_revenue_rule" {
					changes = append(changes, "OLD.status <> 'pending'", "NEW.status NOT IN ('active','rejected')", rootSelfReviewCondition("approved_by"), "NEW.approved_by = 0")
				}
				if table == "gift_order" {
					changes = append(changes, "NEW.refunded_coin_amount < OLD.refunded_coin_amount")
				}
				if table == "anchor_rule_termination" {
					changes = append(changes, "OLD.status <> 'pending'", "NEW.status NOT IN ('approved','rejected')", rootSelfReviewCondition("reviewed_by"), "NEW.reviewed_by = 0", "NEW.reviewed_at < OLD.created_at")
				}
				if table == "gift_benefit_compensation" {
					changes = append(changes,
						"NEW.updated_at < OLD.updated_at",
						"NOT ((OLD.status IN ('pending','failed','processing') AND NEW.status='processing' AND NEW.retry_count=OLD.retry_count+1 AND NEW.lease_until > NEW.updated_at AND NEW.completed_at=0 AND NEW.last_error='' AND (OLD.status <> 'processing' OR OLD.lease_until <= NEW.updated_at)) OR (OLD.status='processing' AND NEW.status IN ('succeeded','failed') AND NEW.retry_count=OLD.retry_count AND NEW.lease_until=0 AND ((NEW.status='succeeded' AND NEW.completed_at=NEW.updated_at AND NEW.completed_at>0 AND NEW.last_error='') OR (NEW.status='failed' AND NEW.completed_at=0 AND NEW.last_error<>''))))")
				}
			}
			body := ""
			if db.Dialector.Name() == "mysql" {
				body = "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='gift financial fact immutable'"
				if len(changes) > 0 {
					body = "BEGIN IF " + strings.Join(changes, " OR ") + " THEN " + body + "; END IF; END"
				}
			} else {
				body = "BEGIN SELECT RAISE(ABORT,'gift financial fact immutable'); END"
				if len(changes) > 0 {
					body = "WHEN " + strings.Join(changes, " OR ") + " " + body
				}
			}
			name := "trg_" + table + "_" + strings.ToLower(event)
			if table == "gift_benefit_compensation" && event == "UPDATE" {
				name += "_v2"
			}
			if event == "UPDATE" && (table == "anchor_revenue_rule" || table == "anchor_rule_termination") {
				name += "_root_v2"
			}
			result = append(result, giftTrigger{name, table, event, body})
		}
	}
	return result, nil
}

func upgradeCompensationTrigger(db *gorm.DB) error {
	const name = "trg_gift_benefit_compensation_update"
	var actual string
	expected := "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='gift financial fact immutable'"
	if db.Dialector.Name() == "mysql" {
		if e := db.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE='gift_benefit_compensation' AND EVENT_MANIPULATION='UPDATE' AND ACTION_TIMING='BEFORE'", name).Scan(&actual).Error; e != nil {
			return e
		}
	} else {
		if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name='gift_benefit_compensation'", name).Scan(&actual).Error; e != nil {
			return e
		}
		expected = "CREATE TRIGGER " + name + " BEFORE UPDATE ON gift_benefit_compensation BEGIN SELECT RAISE(ABORT,'gift financial fact immutable'); END"
	}
	if actual == "" {
		return nil
	}
	if strings.Join(strings.Fields(actual), " ") != strings.Join(strings.Fields(expected), " ") {
		return fmt.Errorf("unknown legacy compensation trigger; manual investigation required")
	}
	return db.Exec("DROP TRIGGER " + name).Error
}

// Keep the database guard aligned with the backend root policy.
func rootSelfReviewCondition(column string) string {
	return fmt.Sprintf("(NEW.%s = OLD.created_by AND NEW.%s <> %d)", column, column, system.SuperAdminUserID)
}

// Install the new guard first, then remove only the exact known old guard.
// This is restartable and never leaves financial facts without a trigger.
func upgradeRootReviewTriggers(db *gorm.DB, triggers []giftTrigger) error {
	for _, t := range triggers {
		if !strings.HasSuffix(t.name, "_root_v2") {
			continue
		}
		legacyName := strings.TrimSuffix(t.name, "_root_v2")
		column := "approved_by"
		if t.table == "anchor_rule_termination" {
			column = "reviewed_by"
		}
		oldCondition := "NEW." + column + " = OLD.created_by"
		if t.table == "anchor_settlement_rule" || t.table == "anchor_settlement" {
			oldCondition = "NEW." + column + "=OLD.created_by"
		}
		expected := strings.ReplaceAll(t.body, rootSelfReviewCondition(column), oldCondition)
		var actual string
		if db.Dialector.Name() == "mysql" {
			if e := db.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE=? AND EVENT_MANIPULATION='UPDATE' AND ACTION_TIMING='BEFORE'", legacyName, t.table).Scan(&actual).Error; e != nil {
				return e
			}
		} else {
			if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name=?", legacyName, t.table).Scan(&actual).Error; e != nil {
				return e
			}
			expected = "CREATE TRIGGER " + legacyName + " BEFORE UPDATE ON " + t.table + " " + expected
		}
		if actual == "" {
			continue
		}
		if strings.Join(strings.Fields(actual), " ") != strings.Join(strings.Fields(expected), " ") {
			return fmt.Errorf("unknown legacy review trigger: %s", legacyName)
		}
		if e := db.Exec("DROP TRIGGER " + legacyName).Error; e != nil {
			return e
		}
	}
	return nil
}

func validateRootReviewUpgrade(db *gorm.DB, triggers []giftTrigger) error {
	var names []string
	for _, t := range triggers {
		if strings.HasSuffix(t.name, "_root_v2") {
			names = append(names, strings.TrimSuffix(t.name, "_root_v2"))
		}
	}
	if len(names) == 0 {
		return nil
	}
	var n int64
	if e := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name IN ?", names).Scan(&n).Error; e != nil {
		return e
	}
	if n != 0 {
		return fmt.Errorf("legacy review trigger requires root migration")
	}
	return nil
}
