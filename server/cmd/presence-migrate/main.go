// presence-migrate is deliberately separate from application startup: inspect by
// default, back up affected tables before additive DDL, never mutate money tables.
package main

import (
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
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
	liveService "tb_live_module/service/live"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	file := flag.String("config", "", "Configuration file, secrets never printed")
	migrate := flag.Bool("migrate", false, "Add presence tables and session counters")
	backupDir := flag.String("backup-dir", "", "Absolute private backup directory, required for writes")
	recoverSession := flag.String("recover-session", "", "Recover an entirely lost Redis session; never deletes existing keys")
	reason := flag.String("reason", "", "Required operational reason for recovery")
	verify := flag.Bool("verify", false, "Verify schema read-only")
	flag.Parse()
	if *file == "" || (*verify && (*migrate || *recoverSession != "")) || (*migrate && *recoverSession != "") {
		return fmt.Errorf("provide -config and choose inspect, -verify, or -migrate")
	}
	v := viper.New()
	v.SetConfigFile(*file)
	var cfg config.Server
	if v.ReadInConfig() != nil || v.Unmarshal(&cfg) != nil || cfg.System.DbType != "mysql" {
		return fmt.Errorf("cannot read MySQL configuration")
	}
	mc, e := driver.ParseDSN(cfg.Mysql.Dsn())
	if e != nil {
		return fmt.Errorf("invalid MySQL configuration")
	}
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Timeout = 5 * time.Second
	mc.ReadTimeout = 30 * time.Second
	mc.WriteTimeout = 30 * time.Second
	db, e := gorm.Open(mysql.Open(mc.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return fmt.Errorf("cannot connect to configured MySQL")
	}
	pool, e := db.DB()
	if e != nil {
		return fmt.Errorf("database pool unavailable")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	var version, database string
	if db.Raw("SELECT VERSION()").Scan(&version).Error != nil || db.Raw("SELECT DATABASE()").Scan(&database).Error != nil {
		return fmt.Errorf("database preflight failed")
	}
	fingerprint := sha256.Sum256([]byte(mc.Addr))
	out := map[string]any{"database": database, "hostFingerprint": hex.EncodeToString(fingerprint[:6]), "version": version, "migrated": false, "applicationDeployed": false}
	tables := []string{"live_session", "live_viewer_visit", "live_viewer_session", "live_presence_checkpoint", "live_room_event"}
	var existing []string
	if db.Raw("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ? ORDER BY TABLE_NAME", tables).Scan(&existing).Error != nil {
		return fmt.Errorf("schema preflight failed")
	}
	out["existingTables"] = existing
	var sessions, living int64
	if db.Table("live_session").Count(&sessions).Error != nil || db.Table("live_session").Where("status IN ?", []int{0, 1, 2}).Count(&living).Error != nil {
		return fmt.Errorf("session preflight failed")
	}
	out["sessions"] = sessions
	out["activeSessions"] = living
	if *migrate || *recoverSession != "" {
		if *backupDir == "" || !filepath.IsAbs(*backupDir) {
			return fmt.Errorf("migration requires absolute -backup-dir")
		}
		before, e := engagementcli.Proof(db)
		if e != nil {
			return fmt.Errorf("financial preflight failed")
		}
		path, digest, e := backup(db, *backupDir, existing)
		if e != nil {
			return e
		}
		out["backupFile"] = path
		out["backupSHA256"] = digest
		if *migrate {
			if e = initialize.MigratePresence(db); e != nil {
				return fmt.Errorf("presence additive migration failed; inspect private backup and schema")
			}
		} else {
			if len(*reason) < 8 || len(*reason) > 200 {
				return fmt.Errorf("recovery requires a reason of 8..200 bytes")
			}
			if initialize.ValidatePresenceSchema(db) != nil {
				return fmt.Errorf("recovery requires current presence schema")
			}
			var client redis.UniversalClient
			if cfg.Redis.UseCluster {
				client = redis.NewClusterClient(&redis.ClusterOptions{Addrs: cfg.Redis.ClusterAddrs, Password: cfg.Redis.Password, ContextTimeoutEnabled: true, MaxRetries: 0})
			} else {
				client = redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB, ContextTimeoutEnabled: true, MaxRetries: -1})
			}
			defer client.Close()
			h := hmac.New(sha256.New, []byte(cfg.JWTApp.SigningKey))
			h.Write([]byte("live-presence/v1/credential-key"))
			service := &liveService.PresenceService{DB: db, Redis: client, Key: h.Sum(nil), Config: cfg.Presence}
			if e = service.RecoverLostSession(ctx, *recoverSession, *reason); e != nil {
				return fmt.Errorf("recovery refused or failed; existing Redis keys must be entirely absent")
			}
			out["recoveredSession"] = *recoverSession
			out["recoveryReason"] = *reason
		}
		after, e := engagementcli.Proof(db)
		if e != nil {
			return fmt.Errorf("financial verification failed")
		}
		out["financialTablesUnchanged"] = reflect.DeepEqual(before, after)
		out["financialBefore"] = before
		out["financialAfter"] = after
		out["migrated"] = *migrate
	}
	if *migrate || *verify {
		if initialize.ValidatePresenceSchema(db) != nil {
			return fmt.Errorf("presence schema verification failed")
		}
		out["verified"] = true
	}
	if e := json.NewEncoder(os.Stdout).Encode(out); e != nil {
		return e
	}
	if unchanged, ok := out["financialTablesUnchanged"].(bool); ok && !unchanged {
		return fmt.Errorf("financial snapshot changed; review the report and concurrent business writes before accepting this migration")
	}
	return nil
}
func backup(db *gorm.DB, dir string, tables []string) (string, string, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", "", fmt.Errorf("cannot create private backup directory")
	}
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return "", "", fmt.Errorf("backup directory must be private 0700 and not a symlink")
	}
	path := filepath.Join(dir, "presence-before-"+time.Now().UTC().Format("20060102T150405.000000000")+".jsonl.gz")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", "", fmt.Errorf("cannot create backup file")
	}
	defer f.Close()
	h := sha256.New()
	z := gzip.NewWriter(io.MultiWriter(f, h))
	enc := json.NewEncoder(z)
	e = db.Transaction(func(tx *gorm.DB) error {
		for _, table := range tables {
			// Names come exclusively from the fixed allowlist in run().
			var schema struct {
				Table  string
				Create string `gorm:"column:Create Table"`
			}
			if e := tx.Raw("SHOW CREATE TABLE `" + table + "`").Scan(&schema).Error; e != nil {
				return e
			}
			if e := enc.Encode(map[string]any{"table": table, "ddl": schema.Create}); e != nil {
				return e
			}
			rows, e := tx.Raw("SELECT * FROM `" + table + "` ORDER BY 1").Rows()
			if e != nil {
				return e
			}
			cols, e := rows.Columns()
			if e != nil {
				rows.Close()
				return e
			}
			if e = enc.Encode(map[string]any{"columns": cols}); e != nil {
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
				for i, x := range values {
					if b, ok := x.([]byte); ok {
						values[i] = string(b)
					}
				}
				if e = enc.Encode(map[string]any{"row": values}); e != nil {
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
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		z.Close()
		return path, "", fmt.Errorf("backup incomplete; no migration performed")
	}
	if z.Close() != nil || f.Sync() != nil {
		return path, "", fmt.Errorf("backup could not be synced")
	}
	return path, hex.EncodeToString(h.Sum(nil)), nil
}
