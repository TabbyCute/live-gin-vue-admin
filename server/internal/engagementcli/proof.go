package engagementcli

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strings"
)

type TableProof struct {
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

// Proof reads, never updates, financial facts and balance projections. Digests
// allow the operator to identify concurrent business writes without printing PII.
func Proof(db *gorm.DB) (out map[string]TableProof, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		out, err = ProofInSnapshot(tx)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}

// ProofInSnapshot requires an existing repeatable-read, read-only transaction.
// Callers can collect associated permission details in the very same snapshot;
// a separate transaction for those details would leave a comparison race.
func ProofInSnapshot(tx *gorm.DB) (map[string]TableProof, error) {
	out := map[string]TableProof{}
	if tx == nil {
		return out, fmt.Errorf("financial snapshot transaction required")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return out, fmt.Errorf("financial snapshot transaction required")
	}
	e := func() error {
		var oldZone string
		if e := tx.Raw("SELECT @@SESSION.time_zone").Scan(&oldZone).Error; e != nil {
			return e
		}
		if e := tx.Exec("SET SESSION time_zone='+00:00'").Error; e != nil {
			return e
		}
		defer tx.Exec("SET SESSION time_zone=?", oldZone)
		var names []string
		if e := tx.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE' AND (LEFT(TABLE_NAME,7)='wallet_' OR LEFT(TABLE_NAME,7)='payout_' OR LEFT(TABLE_NAME,7)='anchor_' OR TABLE_NAME IN ('gift_order','gift_refund_order','gift_revenue_ledger')) ORDER BY TABLE_NAME").Scan(&names).Error; e != nil {
			return e
		}
		for _, name := range names {
			q, e := quoted(name)
			if e != nil {
				return e
			}
			// Column 1 need not be a primary key (e.g. payout state_version).
			// A schema/index change must not reorder equal-key rows and produce a
			// false financial difference in an otherwise identical snapshot.
			var keys []string
			if e := tx.Raw("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION", name).Scan(&keys).Error; e != nil {
				return e
			}
			if len(keys) == 0 {
				return fmt.Errorf("financial proof requires a primary key: %s", name)
			}
			order := make([]string, 0, len(keys))
			for _, key := range keys {
				value, e := quoted(key)
				if e != nil {
					return e
				}
				order = append(order, value)
			}
			rows, e := tx.Raw("SELECT * FROM " + q + " ORDER BY " + strings.Join(order, ",")).Rows()
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
	}()
	return out, e
}
