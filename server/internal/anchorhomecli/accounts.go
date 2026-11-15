package anchorhomecli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"tb_live_module/internal/engagementcli"
)

// These old tables are never modified by this migration. Their content is also
// proven unchanged, without expanding the existing backup tool's fixed scope.
func accountProofInSnapshot(tx *gorm.DB) (map[string]engagementcli.TableProof, error) {
	out := map[string]engagementcli.TableProof{}
	identifier := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	for _, table := range []string{"live_account", "live_anchor"} {
		var supported int64
		if err := tx.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND TABLE_TYPE='BASE TABLE' AND ENGINE='InnoDB'", table).Scan(&supported).Error; err != nil {
			return nil, err
		}
		if supported != 1 {
			return nil, fmt.Errorf("account_proof_requires_innodb")
		}
		var columns []string
		if err := tx.Raw("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION", table).Scan(&columns).Error; err != nil {
			return nil, err
		}
		if len(columns) == 0 {
			return nil, fmt.Errorf("account_proof_primary_key_required")
		}
		for i, column := range columns {
			if !identifier.MatchString(column) {
				return nil, fmt.Errorf("account_proof_invalid_column")
			}
			columns[i] = "`" + column + "`"
		}
		rows, err := tx.Raw("SELECT * FROM `" + table + "` ORDER BY " + strings.Join(columns, ",")).Rows()
		if err != nil {
			return nil, err
		}
		names, err := rows.Columns()
		if err != nil {
			rows.Close()
			return nil, err
		}
		hash := sha256.New()
		encoder := json.NewEncoder(hash)
		var count int64
		for rows.Next() {
			values, destinations := make([]any, len(names)), make([]any, len(names))
			for i := range values {
				destinations[i] = &values[i]
			}
			if err = rows.Scan(destinations...); err != nil {
				rows.Close()
				return nil, err
			}
			if err = encoder.Encode(values); err != nil {
				rows.Close()
				return nil, err
			}
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		out[table] = engagementcli.TableProof{Rows: count, SHA256: hex.EncodeToString(hash.Sum(nil))}
	}
	return out, nil
}

func unchangedAccounts(before, after map[string]engagementcli.TableProof) bool {
	if len(before) != 2 || len(after) != 2 {
		return false
	}
	for _, table := range []string{"live_account", "live_anchor"} {
		if before[table] != after[table] {
			return false
		}
	}
	return true
}
