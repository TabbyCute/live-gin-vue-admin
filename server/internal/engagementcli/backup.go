package engagementcli

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"gorm.io/gorm"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func quoted(v string) (string, error) {
	if !identifier.MatchString(v) {
		return "", fmt.Errorf("unexpected database identifier")
	}
	return "`" + v + "`", nil
}

// Backup is a consistent read-only snapshot in a private file. SQL contains no
// DROP/USE/credentials and can be restored into a fresh isolated database.
func Backup(db *gorm.DB, dir string) (string, string, error) {
	return backupTables(db, dir, backupScopeEngagement, nil, nil)
}

// BackupAll includes every base table and trigger, without credentials, grants,
// DROP or USE statements. It is for a private restore into a NEW database.
func BackupAll(db *gorm.DB, dir string) (string, string, error) {
	return BackupAllWithProgress(db, dir, nil)
}

// BackupProgress contains fixed steps and validated ASCII table identifiers,
// never SQL text, customer values, or credentials.
type BackupProgress struct {
	Step       string `json:"step"`
	TableIndex int    `json:"tableIndex,omitempty"`
	TableTotal int    `json:"tableTotal,omitempty"`
	Table      string `json:"table,omitempty"`
}

type BackupError struct{ cause error }

func (e *BackupError) Error() string { return "backup failed before mutation" }
func (e *BackupError) Unwrap() error { return e.cause }

func BackupAllWithProgress(db *gorm.DB, dir string, progress func(BackupProgress)) (string, string, error) {
	return backupTables(db, dir, BackupScopeFull, nil, progress)
}

// BackupFinancialAndMetadata backs up the fixed migration-related scope and its
// same-database FK parents. requiredFinancial only asserts coverage; it never
// adds arbitrary caller-selected tables or permits excluding any selected table.
func BackupFinancialAndMetadata(db *gorm.DB, dir string, requiredFinancial []string, progress func(BackupProgress)) (string, string, error) {
	return backupTables(db, dir, BackupScopeFinancialAndMetadata, requiredFinancial, progress)
}

func backupTables(db *gorm.DB, dir string, scope string, requiredFinancial []string, progress func(BackupProgress)) (file string, digest string, err error) {
	defer func() {
		if err != nil {
			err = &BackupError{cause: err}
		}
	}()
	emit := func(step string, index, total int, tables ...string) {
		if progress != nil {
			p := BackupProgress{Step: step, TableIndex: index, TableTotal: total}
			if len(tables) > 0 && identifier.MatchString(tables[0]) {
				p.Table = tables[0]
			}
			progress(p)
		}
	}
	emit("preflight", 0, 0)
	if !filepath.IsAbs(dir) {
		return "", "", fmt.Errorf("absolute private backup directory required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", "", fmt.Errorf("cannot create backup directory")
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", "", fmt.Errorf("backup must be a non-symlink 0700 directory")
	}
	f, e := os.CreateTemp(dir, "engagement-before-*.sql")
	if e != nil {
		return "", "", e
	}
	complete := false
	defer func() {
		f.Close()
		if !complete {
			os.Remove(f.Name())
		}
	}()
	w := bufio.NewWriter(f)
	e = db.Transaction(func(tx *gorm.DB) error {
		var oldZone string
		if e := tx.Raw("SELECT @@SESSION.time_zone").Scan(&oldZone).Error; e != nil {
			return e
		}
		if e := tx.Exec("SET SESSION time_zone='+00:00'").Error; e != nil {
			return e
		}
		defer tx.Exec("SET SESSION time_zone=?", oldZone)
		var unsupported int64
		if scope == BackupScopeFull {
			if e := tx.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE' AND ENGINE<>'InnoDB'").Scan(&unsupported).Error; e != nil {
				return e
			}
			if unsupported != 0 {
				return fmt.Errorf("consistent backup requires InnoDB base tables")
			}
			for _, query := range []string{
				"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE<>'BASE TABLE'",
				"SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=DATABASE()",
				"SELECT COUNT(*) FROM information_schema.EVENTS WHERE EVENT_SCHEMA=DATABASE()",
				"SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=DATABASE() AND REFERENCED_TABLE_SCHEMA IS NOT NULL AND REFERENCED_TABLE_SCHEMA<>DATABASE()",
				"SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND DATA_TYPE IN ('geometry','point','linestring','polygon','multipoint','multilinestring','multipolygon','geometrycollection')",
			} {
				if e := tx.Raw(query).Scan(&unsupported).Error; e != nil {
					return e
				}
				if unsupported != 0 {
					return fmt.Errorf("full backup contains unsupported database objects")
				}
			}
		}
		var sourceMode string
		if e := tx.Raw("SELECT @@SESSION.sql_mode").Scan(&sourceMode).Error; e != nil {
			return e
		}
		if _, e := fmt.Fprintf(w, "-- Database migration snapshot; scope: %s; restore only into a new isolated database.\nSET NAMES utf8mb4;\nSET @backup_old_sql_mode=@@SESSION.sql_mode;\nSET SESSION sql_mode=%s;\nSET time_zone = '+00:00';\nSET FOREIGN_KEY_CHECKS=0;\n", scope, literal(restoreDataMode(sourceMode))); e != nil {
			return e
		}
		var tables []string
		if scope == BackupScopeFinancialAndMetadata {
			var e error
			tables, e = financialAndMetadataTables(tx, requiredFinancial)
			if e != nil {
				return e
			}
		} else {
			query := "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE'"
			if scope == backupScopeEngagement {
				query += " AND (LEFT(TABLE_NAME,6)='guard_' OR LEFT(TABLE_NAME,8)='ranking_' OR LEFT(TABLE_NAME,5)='gift_' OR TABLE_NAME IN ('sys_base_menus','sys_authority_menus','sys_apis'))"
			}
			if e := tx.Raw(query + " ORDER BY TABLE_NAME").Scan(&tables).Error; e != nil {
				return e
			}
		}
		for tableIndex, table := range tables {
			name, e := quoted(table)
			if e != nil {
				return e
			}
			emit("table_schema", tableIndex+1, len(tables), table)
			var ddl struct {
				Table  string
				Create string `gorm:"column:Create Table"`
			}
			if e = tx.Raw("SHOW CREATE TABLE " + name).Scan(&ddl).Error; e != nil {
				return e
			}
			if ddl.Create == "" {
				return fmt.Errorf("missing schema in backup")
			}
			if _, e = fmt.Fprintln(w, ddl.Create+";"); e != nil {
				return e
			}
			var columns []struct{ Name, DataType string }
			if e := tx.Raw("SELECT COLUMN_NAME AS name,DATA_TYPE AS data_type FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND EXTRA NOT LIKE '%VIRTUAL GENERATED%' AND EXTRA NOT LIKE '%STORED GENERATED%' ORDER BY ORDINAL_POSITION", table).Scan(&columns).Error; e != nil {
				return e
			}
			if len(columns) == 0 {
				return fmt.Errorf("backup requires writable columns")
			}
			columnSQL := []string{}
			for _, c := range columns {
				q, e := quoted(c.Name)
				if e != nil {
					return e
				}
				columnSQL = append(columnSQL, q)
			}
			emit("table_rows", tableIndex+1, len(tables), table)
			rows, e := tx.Raw("SELECT " + strings.Join(columnSQL, ",") + " FROM " + name).Rows()
			if e != nil {
				return e
			}
			for rows.Next() {
				values := make([]any, len(columns))
				dest := make([]any, len(columns))
				for i := range values {
					dest[i] = &values[i]
				}
				if e = rows.Scan(dest...); e != nil {
					rows.Close()
					return e
				}
				literals := []string{}
				for i, v := range values {
					dataType := columns[i].DataType
					binary := dataType == "binary" || dataType == "varbinary" || dataType == "bit" || strings.HasSuffix(dataType, "blob")
					if b, ok := v.([]byte); ok && !binary {
						// Text/JSON/DECIMAL need character conversion. A naked hex literal in
						// DECIMAL context is interpreted as a binary number, not decimal text.
						literals = append(literals, "CONVERT(X'"+hex.EncodeToString(b)+"' USING utf8mb4)")
					} else {
						literals = append(literals, literal(v))
					}
				}
				if _, e = fmt.Fprintln(w, "INSERT INTO "+name+" ("+strings.Join(columnSQL, ",")+") VALUES ("+strings.Join(literals, ",")+");"); e != nil {
					rows.Close()
					return e
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			emit("table_complete", tableIndex+1, len(tables), table)
		}
		emit("triggers", 0, len(tables))
		var triggers []struct {
			Name    string
			Table   string
			Timing  string
			Event   string
			Body    string
			SQLMode string
		}
		if len(tables) > 0 {
			if e := tx.Raw("SELECT TRIGGER_NAME AS name, EVENT_OBJECT_TABLE AS `table`, ACTION_TIMING AS timing, EVENT_MANIPULATION AS event, ACTION_STATEMENT AS body, SQL_MODE AS sql_mode FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE IN ? ORDER BY TRIGGER_NAME", tables).Scan(&triggers).Error; e != nil {
				return e
			}
		}
		if _, e := fmt.Fprintln(w, "DELIMITER $$"); e != nil {
			return e
		}
		for _, t := range triggers {
			if scope == BackupScopeFinancialAndMetadata {
				if e := scopedTriggerDependencies(t.Body, t.SQLMode, tables); e != nil {
					return e
				}
			}
			n, e := quoted(t.Name)
			if e != nil {
				return e
			}
			table, e := quoted(t.Table)
			if e != nil {
				return e
			}
			if (t.Timing != "BEFORE" && t.Timing != "AFTER") || (t.Event != "INSERT" && t.Event != "UPDATE" && t.Event != "DELETE") {
				return fmt.Errorf("unexpected trigger")
			}
			if _, e = fmt.Fprintln(w, "SET SESSION sql_mode="+literal(t.SQLMode)+"$$"); e != nil {
				return e
			}
			if _, e = fmt.Fprintln(w, "CREATE TRIGGER "+n+" "+t.Timing+" "+t.Event+" ON "+table+" FOR EACH ROW "+t.Body+"$$"); e != nil {
				return e
			}
		}
		_, e := fmt.Fprintln(w, "DELIMITER ;\nSET FOREIGN_KEY_CHECKS=1;\nSET SESSION sql_mode=@backup_old_sql_mode;\n-- COMPLETE")
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return "", "", e
	}
	emit("file_flush", 0, 0)
	if e = w.Flush(); e != nil {
		return "", "", e
	}
	if e = f.Sync(); e != nil {
		return "", "", e
	}
	if _, e = f.Seek(0, 0); e != nil {
		return "", "", e
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", "", e
	}
	if e = f.Close(); e != nil {
		return "", "", e
	}
	directory, e := os.Open(dir)
	if e != nil {
		return "", "", e
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return "", "", syncErr
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	complete = true
	emit("complete", 0, 0)
	return f.Name(), hex.EncodeToString(h.Sum(nil)), nil
}
func literal(v any) string {
	if v == nil {
		return "NULL"
	}
	var b []byte
	switch x := v.(type) {
	case []byte:
		// Preserve binary columns exactly; implicit column conversion also works
		// for utf8 text without coercing arbitrary bytes through UTF-8 first.
		return "X'" + hex.EncodeToString(x) + "'"
	case time.Time:
		if x.IsZero() {
			return "'0000-00-00 00:00:00'"
		}
		// The SELECT session is UTC. Preserve the returned wall-clock fields even
		// when a caller configured the driver to decode DATE/DATETIME in another Loc.
		b = []byte(x.Format("2006-01-02 15:04:05.999999"))
	default:
		b = []byte(fmt.Sprint(v))
	}
	return "CONVERT(X'" + hex.EncodeToString(b) + "' USING utf8mb4)"
}

// SHOW CREATE output is sensitive to ANSI_QUOTES/NO_BACKSLASH_ESCAPES. Keep
// those source modes while allowing exact old zero dates and explicit ID zero.
func restoreDataMode(source string) string {
	modes := []string{"NO_AUTO_VALUE_ON_ZERO"}
	for _, mode := range strings.Split(source, ",") {
		switch mode {
		case "", "NO_AUTO_VALUE_ON_ZERO", "STRICT_TRANS_TABLES", "STRICT_ALL_TABLES", "NO_ZERO_DATE", "NO_ZERO_IN_DATE":
			continue
		}
		modes = append(modes, mode)
	}
	return strings.Join(modes, ",")
}
