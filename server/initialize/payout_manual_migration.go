package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
	em "tb_live_module/model/earning"
	pm "tb_live_module/model/payout"
	"time"
)

// Run under the existing financial migration lock and a maintenance window.
// MySQL DDL commits independently; all steps are restartable and validated.
func prepareManualChecks(db *gorm.DB, earning bool) error {
	changes := []struct {
		model            any
		name, expression string
	}{}
	if earning {
		changes = append(changes, struct {
			model            any
			name, expression string
		}{&em.Account{}, "ck_withdrawable_balances", "available>=0 AND frozen>=0 AND paid=0 AND debt>=0 AND (debt=0 OR available=0)"}, struct {
			model            any
			name, expression string
		}{&em.AccountLedger{}, "ck_withdrawable_entry", "entry_type IN ('settlement_credit','negative_adjustment','withdrawal_freeze','withdrawal_release')"}, struct {
			model            any
			name, expression string
		}{&em.Withdrawal{}, "ck_withdrawal_status", "status IN ('pending_review','rejected','approved')"})
	} else {
		changes = append(changes, struct {
			model            any
			name, expression string
		}{&pm.Order{}, "ck_payout_status", "status IN ('ready','processing','succeeded','failed','unknown','cancelled')"}, struct {
			model            any
			name, expression string
		}{&pm.Order{}, "ck_payout_time", "created_at>0 AND updated_at=created_at"}, struct {
			model            any
			name, expression string
		}{&pm.StatusHistory{}, "ck_payout_history", "from_status='' AND to_status='ready'"}, struct {
			model            any
			name, expression string
		}{&pm.OrderAudit{}, "ck_payout_audit", "action='order.create'"})
	}
	for _, c := range changes {
		if !db.Migrator().HasTable(c.model) || !db.Migrator().HasConstraint(c.model, c.name) {
			continue
		}
		st := &gorm.Statement{DB: db}
		if e := st.Parse(c.model); e != nil {
			return e
		}
		var actual string
		if db.Dialector.Name() == "mysql" {
			if e := db.Raw("SELECT CHECK_CLAUSE FROM information_schema.CHECK_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND CONSTRAINT_NAME=?", c.name).Scan(&actual).Error; e != nil {
				return e
			}
		} else {
			var ddl string
			if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", st.Schema.Table).Scan(&ddl).Error; e != nil {
				return e
			}
			start := strings.Index(ddl, "CONSTRAINT `"+c.name+"` CHECK (")
			if start < 0 {
				return fmt.Errorf("unrecognized legacy constraint %s", c.name)
			}
			raw := ddl[start+len("CONSTRAINT `"+c.name+"` CHECK ("):]
			depth := 1
			for i, r := range raw {
				if r == '(' {
					depth++
				}
				if r == ')' {
					depth--
					if depth == 0 {
						actual = raw[:i]
						break
					}
				}
			}
		}
		expected := c.expression
		if db.Dialector.Name() == "mysql" {
			expected = payoutMySQLChecks[c.name]
			if expected == "" {
				return fmt.Errorf("legacy check specification missing: %s", c.name)
			}
		}
		if normalizeLegacyCheck(actual) != normalizeLegacyCheck(expected) {
			return fmt.Errorf("refuse unknown legacy constraint %s", c.name)
		}
		if e := db.Migrator().DropConstraint(c.model, c.name); e != nil {
			return e
		}
	}
	return nil
}
func normalizeLegacyCheck(v string) string {
	v = strings.ReplaceAll(v, "\\'", "'")
	v = strings.ReplaceAll(v, "_utf8mb4", "")
	v = strings.ReplaceAll(v, "`", "")
	// Keep boolean grouping and literal case: a same-name weaker CHECK must
	// never be mistaken for the known prior schema.
	return strings.Join(strings.Fields(v), "")
}
func installManualTriggers(db *gorm.DB, desired, legacy []giftTrigger) error {
	norm := func(v string) string { return strings.Join(strings.Fields(v), " ") }
	read := func(t giftTrigger) (string, error) {
		var body string
		var e error
		if db.Dialector.Name() == "mysql" {
			e = db.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=? AND EVENT_OBJECT_TABLE=? AND EVENT_MANIPULATION=? AND ACTION_TIMING='BEFORE'", t.name, t.table, t.event).Scan(&body).Error
		} else {
			e = db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name=?", t.name, t.table).Scan(&body).Error
		}
		return body, e
	}
	sqlFor := func(t giftTrigger) string {
		row := " "
		if db.Dialector.Name() == "mysql" {
			row = " FOR EACH ROW "
		}
		return "CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + row + t.body
	}
	expected := func(t giftTrigger) string {
		if db.Dialector.Name() == "mysql" {
			return t.body
		}
		return sqlFor(t)
	}
	keep := map[string]bool{}
	for _, t := range desired {
		keep[t.name] = true
		actual, e := read(t)
		if e != nil {
			return e
		}
		if actual != "" && norm(actual) != norm(expected(t)) {
			return fmt.Errorf("refuse unknown trigger %s", t.name)
		}
	}
	for _, t := range legacy {
		if keep[t.name] {
			continue
		}
		actual, e := read(t)
		if e != nil {
			return e
		}
		if actual != "" && norm(actual) != norm(expected(t)) {
			return fmt.Errorf("refuse unknown legacy trigger %s", t.name)
		}
	}
	for _, t := range desired {
		actual, e := read(t)
		if e != nil {
			return e
		}
		if actual == "" {
			if e = db.Exec(sqlFor(t)).Error; e != nil {
				return e
			}
		}
	}
	for _, t := range legacy {
		if !keep[t.name] {
			if e := db.Exec("DROP TRIGGER IF EXISTS " + t.name).Error; e != nil {
				return e
			}
		}
	}
	return nil
}
func upgradeManualEarningTriggers(db *gorm.DB) error {
	current, e := earningTriggers(db)
	if e != nil {
		return e
	}
	old, e := earningTriggersVersion(db, false)
	if e != nil {
		return e
	}
	filter := func(in []giftTrigger) []giftTrigger {
		var out []giftTrigger
		for _, t := range in {
			if (t.table == "anchor_withdrawable_account" || t.table == "anchor_withdrawal_order" || t.table == "anchor_withdrawable_ledger") && db.Migrator().HasTable(t.table) {
				out = append(out, t)
			}
		}
		return out
	}
	return installManualTriggers(db, filter(current), filter(old))
}
func upgradePayoutEarningTables(db *gorm.DB) error {
	if e := prepareManualChecks(db, true); e != nil {
		return e
	}
	for _, m := range []any{&em.Account{}, &em.AccountLedger{}, &em.Withdrawal{}} {
		if db.Migrator().HasTable(m) {
			if e := db.AutoMigrate(m); e != nil {
				return e
			}
		}
	}
	return upgradeManualEarningTriggers(db)
}
func seedManualChannel(db *gorm.DB) error {
	return db.Where("channel_code='manual_bank'").FirstOrCreate(&pm.Channel{ChannelCode: "manual_bank", ChannelType: "manual", CreatedAt: time.Now().UnixMilli()}).Error
}
