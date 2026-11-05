package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
)

func containsLegacyConstraint(ddl, name string) bool {
	return strings.Contains(ddl, "CONSTRAINT `"+name+"`") || strings.Contains(ddl, "CONSTRAINT \""+name+"\"")
}
func validateNoLegacyManualGuards(db *gorm.DB) error {
	checks := []string{"ck_payout_status", "ck_payout_time", "ck_payout_history", "ck_payout_audit", "ck_withdrawable_balances", "ck_withdrawable_entry", "ck_withdrawal_status"}
	guards := []string{"trg_payout_order_update", "trg_payout_order_insert", "trg_anchor_withdrawable_account_update", "trg_anchor_withdrawal_order_update"}
	var n int64
	if db.Dialector.Name() == "mysql" {
		if e := db.Raw("SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND CONSTRAINT_TYPE='CHECK' AND CONSTRAINT_NAME IN ?", checks).Scan(&n).Error; e != nil {
			return e
		}
		if n != 0 {
			return fmt.Errorf("legacy financial CHECK requires manual payout migration")
		}
		if e := db.Raw("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME IN ?", guards).Scan(&n).Error; e != nil {
			return e
		}
	} else {
		var tables []struct{ SQL string }
		if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND (name LIKE 'payout_%' OR name LIKE 'anchor_withdraw%')").Scan(&tables).Error; e != nil {
			return e
		}
		for _, table := range tables {
			for _, check := range checks {
				if containsLegacyConstraint(table.SQL, check) {
					return fmt.Errorf("legacy financial CHECK requires manual payout migration")
				}
			}
		}
		if e := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name IN ?", guards).Scan(&n).Error; e != nil {
			return e
		}
	}
	if n != 0 {
		return fmt.Errorf("legacy financial trigger requires manual payout migration")
	}
	if db.Migrator().HasTable("payout_status_history") && db.Migrator().HasIndex("payout_status_history", "idx_payout_status_history_payout_no") {
		return fmt.Errorf("legacy single-history index requires manual payout migration")
	}
	return nil
}
