// event-delivery-migrate is an offline additive migration, read-only by default.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"tb_live_module/config"
	"tb_live_module/initialize"
	"tb_live_module/internal/engagementcli"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	file := flag.String("config", "", "MySQL configuration; never printed")
	migrate := flag.Bool("migrate", false, "Add only delivery tables/protections")
	metadata := flag.Bool("metadata", false, "Add root-only admin menu/API metadata")
	verify := flag.Bool("verify", false, "Read-only schema verification")
	backupDir := flag.String("backup-dir", "", "Absolute private backup directory, required before writes")
	flag.Parse()
	if *file == "" || (*verify && (*migrate || *metadata)) {
		return fmt.Errorf("provide -config and inspect, -verify, or -migrate/-metadata")
	}
	v := viper.New()
	v.SetConfigFile(*file)
	var cfg config.Server
	if v.ReadInConfig() != nil || v.Unmarshal(&cfg) != nil || cfg.System.DbType != "mysql" {
		return fmt.Errorf("cannot read MySQL configuration")
	}
	mc, err := driver.ParseDSN(cfg.Mysql.Dsn())
	if err != nil {
		return fmt.Errorf("invalid MySQL configuration")
	}
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Timeout = 5 * time.Second
	mc.ReadTimeout = 30 * time.Second
	mc.WriteTimeout = 30 * time.Second
	db, err := gorm.Open(mysql.Open(mc.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("cannot connect to configured MySQL")
	}
	pool, err := db.DB()
	if err != nil {
		return fmt.Errorf("database pool unavailable")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	var database, version string
	if db.Raw("SELECT DATABASE()").Scan(&database).Error != nil || db.Raw("SELECT VERSION()").Scan(&version).Error != nil {
		return fmt.Errorf("preflight failed")
	}
	fingerprint := sha256.Sum256([]byte(mc.Addr))
	out := map[string]any{"database": database, "version": version, "hostFingerprint": hex.EncodeToString(fingerprint[:6]), "applicationDeployed": false, "migrated": false}
	if *migrate || *metadata {
		if !filepath.IsAbs(*backupDir) {
			return fmt.Errorf("writes require absolute -backup-dir")
		}
		err = db.Connection(func(conn *gorm.DB) error {
			var acquired int
			if e := conn.Raw("SELECT GET_LOCK('event_delivery_migration',10)").Scan(&acquired).Error; e != nil || acquired != 1 {
				return fmt.Errorf("migration lock unavailable")
			}
			defer conn.Exec("SELECT RELEASE_LOCK('event_delivery_migration')")
			before, e := engagementcli.Proof(conn)
			if e != nil {
				return fmt.Errorf("financial preflight failed")
			}
			path, digest, e := backup(conn, *backupDir)
			if e != nil {
				return e
			}
			out["backupFile"] = path
			out["backupSHA256"] = digest
			if *migrate {
				if e = initialize.MigrateEventDelivery(conn); e != nil {
					return fmt.Errorf("delivery migration failed; inspect schema and private backup")
				}
			}
			if *metadata {
				if e = initialize.SyncEventDeliveryMetadata(conn); e != nil {
					return fmt.Errorf("delivery metadata migration failed")
				}
			}
			after, e := engagementcli.Proof(conn)
			if e != nil || !reflect.DeepEqual(before, after) {
				return fmt.Errorf("financial proof changed or unavailable")
			}
			out["financialTablesUnchanged"] = true
			out["migrated"] = *migrate
			out["metadataSynced"] = *metadata
			return nil
		})
		if err != nil {
			return err
		}
	}
	schemaErr := initialize.ValidateEventDeliverySchema(db)
	out["schemaReady"] = schemaErr == nil
	if *verify && schemaErr != nil {
		return fmt.Errorf("event delivery schema verification failed")
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// Private consistent JSONL snapshot contains exact DDL and typed row values.
// Restore only into a separate database during recovery; never execute against live finance tables.
func backup(db *gorm.DB, dir string) (string, string, error) {
	if !filepath.IsAbs(dir) {
		return "", "", fmt.Errorf("absolute backup directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", fmt.Errorf("cannot create backup directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", "", fmt.Errorf("backup directory must be private 0700, no symlink")
	}
	f, err := os.CreateTemp(dir, "event-delivery-before-*.jsonl")
	if err != nil {
		return "", "", err
	}
	complete := false
	defer func() {
		f.Close()
		if !complete {
			os.Remove(f.Name())
		}
	}()
	encoder := json.NewEncoder(f)
	err = db.Transaction(func(tx *gorm.DB) error {
		var tables []string
		if e := tx.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND (TABLE_NAME LIKE 'event\\_delivery%' OR TABLE_NAME IN ('sys_base_menus','sys_authority_menus','sys_apis')) ORDER BY TABLE_NAME").Scan(&tables).Error; e != nil {
			return e
		}
		if e := encoder.Encode(map[string]any{"format": "event-delivery-backup/v1", "createdAt": time.Now().UTC(), "tables": tables}); e != nil {
			return e
		}

		var triggers []struct {
			Name      string `gorm:"column:TRIGGER_NAME" json:"name"`
			Table     string `gorm:"column:EVENT_OBJECT_TABLE" json:"table"`
			Timing    string `gorm:"column:ACTION_TIMING" json:"timing"`
			Event     string `gorm:"column:EVENT_MANIPULATION" json:"event"`
			Statement string `gorm:"column:ACTION_STATEMENT" json:"statement"`
		}
		if e := tx.Raw("SELECT TRIGGER_NAME,EVENT_OBJECT_TABLE,ACTION_TIMING,EVENT_MANIPULATION,ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE IN ? ORDER BY TRIGGER_NAME", tables).Scan(&triggers).Error; e != nil {
			return e
		}
		if e := encoder.Encode(map[string]any{"triggers": triggers}); e != nil {
			return e
		}
		for _, table := range tables {
			var ddl struct {
				Create string `gorm:"column:Create Table"`
			}
			if e := tx.Raw("SHOW CREATE TABLE `" + table + "`").Scan(&ddl).Error; e != nil {
				return e
			}
			if e := encoder.Encode(map[string]any{"table": table, "ddl": ddl.Create}); e != nil {
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
				for i := range values {
					dest[i] = &values[i]
				}
				if e = rows.Scan(dest...); e != nil {
					rows.Close()
					return e
				}
				if e = encoder.Encode(map[string]any{"table": table, "columns": cols, "values": values}); e != nil {
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
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("private backup failed")
	}
	if err = f.Sync(); err != nil {
		return "", "", err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return "", "", err
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", "", err
	}
	complete = true
	return f.Name(), hex.EncodeToString(h.Sum(nil)), nil
}
