package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"time"
)

// A private pre-DDL snapshot of affected facts and metadata, never stdout.
// Operators must pause financial writes before migration. This backup is not
// a substitute for the deployment's independent full database/secret backups.
func backupFinancialTables(db *gorm.DB, dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("backup directory must be absolute")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return fmt.Errorf("cannot create private backup directory")
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("backup directory must be a non-symlink 0700 directory")
	}
	file, e := os.CreateTemp(dir, "payout-before-*.jsonl")
	if e != nil {
		return fmt.Errorf("cannot open private backup")
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			os.Remove(file.Name())
		}
	}()
	encoder := json.NewEncoder(file)
	e = db.Transaction(func(tx *gorm.DB) error {
		if e := encoder.Encode(map[string]any{"format": "payout-migration-backup-v1", "createdAt": time.Now().UTC().Format(time.RFC3339)}); e != nil {
			return e
		}
		var tables []string
		if e := tx.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND (TABLE_NAME LIKE 'payout\\_%' OR TABLE_NAME IN ('anchor_withdrawable_account','anchor_withdrawable_ledger','anchor_withdrawal_order','anchor_finance_audit','anchor_reconciliation','anchor_finance_permission','sys_base_menus','sys_authority_menus','sys_apis')) ORDER BY TABLE_NAME").Scan(&tables).Error; e != nil {
			return e
		}
		for _, table := range tables {
			// Names come only from the selected database catalog, and are still quoted.
			for _, r := range table {
				if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
					return fmt.Errorf("unexpected backup table name")
				}
			}
			var ddl []map[string]any
			if e := tx.Raw("SHOW CREATE TABLE `" + table + "`").Scan(&ddl).Error; e != nil {
				return e
			}
			if e := encoder.Encode(map[string]any{"table": table, "schema": ddl}); e != nil {
				return e
			}
			rows, e := tx.Table(table).Rows()
			if e != nil {
				return e
			}
			cols, e := rows.Columns()
			if e != nil {
				rows.Close()
				return e
			}
			for rows.Next() {
				values := make([]any, len(cols))
				dest := make([]any, len(cols))
				for i := range dest {
					dest[i] = &values[i]
				}
				if e = rows.Scan(dest...); e == nil {
					e = encoder.Encode(map[string]any{"table": table, "columns": cols, "values": values})
				}
				if e != nil {
					rows.Close()
					return e
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		var triggers []map[string]any
		if e := tx.Raw("SELECT TRIGGER_NAME,EVENT_OBJECT_TABLE,ACTION_TIMING,EVENT_MANIPULATION,ACTION_STATEMENT,DEFINER,SQL_MODE FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE IN ? ORDER BY TRIGGER_NAME", tables).Scan(&triggers).Error; e != nil {
			return e
		}
		return encoder.Encode(map[string]any{"triggers": triggers, "complete": true})
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return fmt.Errorf("private backup failed before migration")
	}
	if e = file.Sync(); e != nil {
		return fmt.Errorf("cannot sync private backup")
	}
	if e = file.Close(); e != nil {
		return fmt.Errorf("cannot finish private backup")
	}
	complete = true
	return nil
}
