package engagementcli

import "gorm.io/gorm"

// Preflight uses only counts and execution plans, never outputs business rows.
// Source tables are never altered by these migrations.
func Preflight(db *gorm.DB) (map[string]any, error) {
	out := map[string]any{}
	var tables []struct {
		TableName     string `json:"table"`
		EstimatedRows int64  `json:"estimatedRows"`
		Bytes         int64  `json:"bytes"`
	}
	if e := db.Raw("SELECT TABLE_NAME, COALESCE(TABLE_ROWS,0) AS estimated_rows, COALESCE(DATA_LENGTH,0)+COALESCE(INDEX_LENGTH,0) AS bytes FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND (LEFT(TABLE_NAME,6)='guard_' OR LEFT(TABLE_NAME,8)='ranking_' OR TABLE_NAME IN ('gift_order','gift_revenue_ledger','gift_item','sys_base_menus')) ORDER BY TABLE_NAME").Scan(&tables).Error; e != nil {
		return nil, e
	}
	out["tables"] = tables
	checks := map[string]string{
		"guard_plan":                 "SELECT COUNT(*) FROM guard_plan",
		"guard_membership":           "SELECT COUNT(*) FROM guard_membership",
		"guard_grant":                "SELECT COUNT(*) FROM guard_grant",
		"guard_compensation":         "SELECT COUNT(*) FROM guard_compensation",
		"guard_admin_permission":     "SELECT COUNT(*) FROM guard_admin_permission WHERE authority_id=888",
		"ranking_admin_permission":   "SELECT COUNT(*) FROM ranking_admin_permission WHERE authority_id=888",
		"ranking_applied_event":      "SELECT COUNT(*) FROM ranking_applied_event",
		"ranking_generation_receipt": "SELECT COUNT(*) FROM ranking_generation_receipt WHERE generation=(SELECT generation FROM ranking_checkpoint WHERE consumer_name='gift-revenue')",
		"ranking_score_bucket":       "SELECT COUNT(*) FROM ranking_score_bucket WHERE generation=(SELECT generation FROM ranking_checkpoint WHERE consumer_name='gift-revenue')",
		"gift_item":                  "SELECT COUNT(*) FROM gift_item WHERE benefit_type='anchor_guard' AND benefit_status='enabled'",
		"sys_base_menus":             "SELECT COUNT(*) FROM sys_base_menus WHERE name IN ('guard-plan','guard-member','guard-ledger','guard-task','live-ranking') AND deleted_at IS NULL",
	}
	counts := map[string]int64{}
	for _, t := range tables {
		if query, ok := checks[t.TableName]; ok {
			var count int64
			if e := db.Raw(query).Scan(&count).Error; e != nil {
				return nil, e
			}
			counts[t.TableName] = count
		}
		if t.TableName != "gift_revenue_ledger" {
			continue
		}
		var count int64
		if e := db.Table("gift_revenue_ledger").Count(&count).Error; e != nil {
			return nil, e
		}
		out["sourceCount"] = count
		var plan []map[string]any
		if e := db.Raw("EXPLAIN SELECT id FROM gift_revenue_ledger ORDER BY id LIMIT 20").Scan(&plan).Error; e != nil {
			return nil, e
		}
		out["sourcePlan"] = plan
		var receiptExists int64
		if e := db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ranking_generation_receipt'").Scan(&receiptExists).Error; e != nil {
			return nil, e
		}
		if receiptExists == 1 {
			var antiJoin []map[string]any
			if e := db.Raw("EXPLAIN SELECT l.id FROM gift_revenue_ledger l WHERE NOT EXISTS (SELECT 1 FROM ranking_generation_receipt r WHERE r.generation=1 AND r.source_id=l.id) ORDER BY l.id LIMIT 20").Scan(&antiJoin).Error; e != nil {
				return nil, e
			}
			out["consumerPlan"] = antiJoin
		}
	}
	out["verificationCounts"] = counts
	return out, nil
}
