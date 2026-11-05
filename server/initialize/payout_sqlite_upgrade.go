package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
)

// SQLite rebuilds tables to change CHECK constraints. Its global schema parser
// rejects dangling cross-table trigger references during the rename. Remove
// only recognized guards inside the surrounding DDL transaction, then restore
// the full current set before commit. Any failure restores the old schema.
func suspendSQLiteManualTriggers(db *gorm.DB) error {
	if db.Dialector.Name() != "sqlite" {
		return nil
	}
	var known []giftTrigger
	for _, generate := range []func(*gorm.DB) ([]giftTrigger, error){payoutTriggers, legacyPayoutTriggers, earningTriggers, func(d *gorm.DB) ([]giftTrigger, error) { return earningTriggersVersion(d, false) }} {
		rows, e := generate(db)
		if e != nil {
			return e
		}
		known = append(known, rows...)
	}
	accepted := map[string][]string{}
	var names []string
	norm := func(v string) string { return strings.Join(strings.Fields(v), " ") }
	for _, t := range known {
		names = append(names, t.name)
		accepted[t.name] = append(accepted[t.name], norm("CREATE TRIGGER "+t.name+" BEFORE "+t.event+" ON "+t.table+" "+t.body))
	}
	var actual []struct{ Name, SQL string }
	if e := db.Raw("SELECT name,sql FROM sqlite_master WHERE type='trigger' AND name IN ?", names).Scan(&actual).Error; e != nil {
		return e
	}
	for _, row := range actual {
		match := false
		for _, expected := range accepted[row.Name] {
			if norm(row.SQL) == expected {
				match = true
			}
		}
		if !match {
			return fmt.Errorf("refuse unknown SQLite guard %s", row.Name)
		}
	}
	for _, row := range actual {
		if e := db.Exec("DROP TRIGGER `" + row.Name + "`").Error; e != nil {
			return e
		}
	}
	return nil
}
