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
	if !filepath.IsAbs(dir) {
		return "", "", fmt.Errorf("absolute private backup directory required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", "", fmt.Errorf("cannot create backup directory")
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
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
		if _, e := fmt.Fprintln(w, "-- Engagement migration snapshot; restore only into a new isolated database.\nSET NAMES utf8mb4;\nSET time_zone = '+00:00';\nSET FOREIGN_KEY_CHECKS=0;"); e != nil {
			return e
		}
		var tables []string
		if e := tx.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND (LEFT(TABLE_NAME,6)='guard_' OR LEFT(TABLE_NAME,8)='ranking_' OR LEFT(TABLE_NAME,5)='gift_' OR TABLE_NAME IN ('sys_base_menus','sys_authority_menus','sys_apis')) ORDER BY TABLE_NAME").Scan(&tables).Error; e != nil {
			return e
		}
		for _, table := range tables {
			name, e := quoted(table)
			if e != nil {
				return e
			}
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
			rows, e := tx.Table(table).Rows()
			if e != nil {
				return e
			}
			cols, e := rows.Columns()
			if e != nil {
				rows.Close()
				return e
			}
			columnSQL := []string{}
			for _, c := range cols {
				q, e := quoted(c)
				if e != nil {
					rows.Close()
					return e
				}
				columnSQL = append(columnSQL, q)
			}
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
				literals := []string{}
				for _, v := range values {
					literals = append(literals, literal(v))
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
		}
		var triggers []struct {
			Name   string
			Table  string
			Timing string
			Event  string
			Body   string
		}
		if len(tables) > 0 {
			if e := tx.Raw("SELECT TRIGGER_NAME AS name, EVENT_OBJECT_TABLE AS `table`, ACTION_TIMING AS timing, EVENT_MANIPULATION AS event, ACTION_STATEMENT AS body FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE IN ? ORDER BY TRIGGER_NAME", tables).Scan(&triggers).Error; e != nil {
				return e
			}
		}
		if _, e := fmt.Fprintln(w, "DELIMITER $$"); e != nil {
			return e
		}
		for _, t := range triggers {
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
			if _, e = fmt.Fprintln(w, "CREATE TRIGGER "+n+" "+t.Timing+" "+t.Event+" ON "+table+" FOR EACH ROW "+t.Body+"$$"); e != nil {
				return e
			}
		}
		_, e := fmt.Fprintln(w, "DELIMITER ;\nSET FOREIGN_KEY_CHECKS=1;\n-- COMPLETE")
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return "", "", fmt.Errorf("backup failed before mutation")
	}
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
	complete = true
	return f.Name(), hex.EncodeToString(h.Sum(nil)), nil
}
func literal(v any) string {
	if v == nil {
		return "NULL"
	}
	var b []byte
	switch x := v.(type) {
	case []byte:
		b = x
	case time.Time:
		b = []byte(x.UTC().Format("2006-01-02 15:04:05.999999"))
	default:
		b = []byte(fmt.Sprint(v))
	}
	return "CONVERT(X'" + hex.EncodeToString(b) + "' USING utf8mb4)"
}
