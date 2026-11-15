package engagementcli

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

const (
	BackupScopeFull                 = "full"
	BackupScopeFinancialAndMetadata = "financial-and-metadata"
	backupScopeEngagement           = "engagement"
)

// This fixed predicate covers Proof's financial tables, every existing table
// modified by the additive backend migrations, and all three metadata tables.
// No user-supplied allowlist/denylist or skip-on-error behavior is supported.
func migrationBackupTable(name string) bool {
	for _, prefix := range []string{"wallet_", "anchor_", "payout_", "event_delivery_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	switch name {
	case "event_delivery", "gift_order", "gift_refund_order", "gift_revenue_ledger", "sys_base_menus", "sys_authority_menus", "sys_apis":
		return true
	}
	return false
}

func financialAndMetadataTables(tx *gorm.DB, requiredFinancial []string) ([]string, error) {
	var database string
	if e := tx.Raw("SELECT DATABASE()").Scan(&database).Error; e != nil {
		return nil, e
	}
	type tableInfo struct{ Name, Kind, Engine string }
	var catalog []tableInfo
	// Catalog names/engine only: do not read unrelated table definitions,
	// columns, rows, or triggers. A selected view must fail instead of vanish.
	if e := tx.Raw("SELECT TABLE_NAME AS name,TABLE_TYPE AS kind,COALESCE(ENGINE,'') AS engine FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE()").Scan(&catalog).Error; e != nil {
		return nil, e
	}
	available := make(map[string]tableInfo, len(catalog))
	selected := map[string]bool{}
	var frontier []string
	for _, table := range catalog {
		available[table.Name] = table
		if migrationBackupTable(table.Name) {
			selected[table.Name] = true
			frontier = append(frontier, table.Name)
		}
	}
	for _, required := range requiredFinancial {
		if !selected[required] {
			return nil, fmt.Errorf("financial backup coverage incomplete")
		}
	}
	for len(frontier) > 0 {
		for _, name := range frontier {
			table, ok := available[name]
			if !ok || table.Kind != "BASE TABLE" || table.Engine != "InnoDB" {
				return nil, fmt.Errorf("selected backup tables must be InnoDB base tables")
			}
			if _, e := quoted(name); e != nil {
				return nil, e
			}
		}
		var references []struct{ Schema, Name string }
		if e := tx.Raw("SELECT DISTINCT REFERENCED_TABLE_SCHEMA AS `schema`,REFERENCED_TABLE_NAME AS name FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ? AND REFERENCED_TABLE_NAME IS NOT NULL", frontier).Scan(&references).Error; e != nil {
			return nil, e
		}
		frontier = nil
		for _, ref := range references {
			if ref.Schema != database {
				return nil, fmt.Errorf("selected backup has cross-database foreign keys")
			}
			if !selected[ref.Name] {
				selected[ref.Name] = true
				frontier = append(frontier, ref.Name)
			}
		}
	}
	tables := make([]string, 0, len(selected))
	for name := range selected {
		tables = append(tables, name)
	}
	sort.Strings(tables)
	if len(tables) == 0 {
		return nil, fmt.Errorf("migration backup scope is empty")
	}
	var unsupported int64
	if e := tx.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ? AND DATA_TYPE IN ('geometry','point','linestring','polygon','multipoint','multilinestring','multipolygon','geometrycollection')", tables).Scan(&unsupported).Error; e != nil {
		return nil, e
	}
	if unsupported != 0 {
		return nil, fmt.Errorf("selected backup contains unsupported spatial data")
	}
	return tables, nil
}
