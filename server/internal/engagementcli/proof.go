package engagementcli

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"gorm.io/gorm"
)

type TableProof struct {
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

// Proof reads, never updates, financial facts and balance projections. Digests
// allow the operator to identify concurrent business writes without printing PII.
func Proof(db *gorm.DB) (map[string]TableProof, error) {
	out := map[string]TableProof{}
	e := db.Transaction(func(tx *gorm.DB) error {
		var names []string
		if e := tx.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND (LEFT(TABLE_NAME,7)='wallet_' OR LEFT(TABLE_NAME,7)='payout_' OR LEFT(TABLE_NAME,7)='anchor_' OR TABLE_NAME IN ('gift_order','gift_refund_order','gift_revenue_ledger')) ORDER BY TABLE_NAME").Scan(&names).Error; e != nil {
			return e
		}
		for _, name := range names {
			q, e := quoted(name)
			if e != nil {
				return e
			}
			rows, e := tx.Raw("SELECT * FROM " + q + " ORDER BY 1").Rows()
			if e != nil {
				return e
			}
			cols, e := rows.Columns()
			if e != nil {
				rows.Close()
				return e
			}
			hash := sha256.New()
			encoder := json.NewEncoder(hash)
			var count int64
			for rows.Next() {
				values := make([]any, len(cols))
				dest := make([]any, len(cols))
				for i := range values {
					dest[i] = &values[i]
				}
				if e = rows.Scan(dest...); e != nil {
					rows.Close()
					return e
				}
				if e = encoder.Encode(values); e != nil {
					rows.Close()
					return e
				}
				count++
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			out[name] = TableProof{Rows: count, SHA256: hex.EncodeToString(hash.Sum(nil))}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
