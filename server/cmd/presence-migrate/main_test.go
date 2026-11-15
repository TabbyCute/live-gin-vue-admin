package main

import (
	"compress/gzip"
	"encoding/json"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"os"
	"path/filepath"
	"strings"
	"tb_live_module/initialize"
	model "tb_live_module/model/live"
	"testing"
	"time"
)

func TestPresenceMySQLMigrationAndBackup(t *testing.T) {
	raw := os.Getenv("PRESENCE_TEST_MYSQL_DSN")
	if raw == "" {
		t.Skip("isolated MySQL required")
	}
	cfg, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Addr, "127.0.0.1:"))
	require.Equal(t, "presence_test", cfg.DBName)
	root, e := gorm.Open(mysql.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := root.DB()
	defer pool.Close()
	name := "presence_test_migrate_" + time.Now().Format("150405000000")
	require.NoError(t, root.Exec("CREATE DATABASE "+name).Error)
	defer root.Exec("DROP DATABASE " + name)
	cfg.DBName = name
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	p, _ := db.DB()
	defer p.Close()
	require.NoError(t, db.AutoMigrate(&model.LiveSession{}))
	row := model.LiveSession{SessionNo: "backup_session", RoomId: 1, AnchorId: 1, Status: model.LiveSessionEnded, ViewCount: 100, GiftCoinAmount: 987654321}
	require.NoError(t, db.Create(&row).Error)
	dir := filepath.Join(t.TempDir(), "private")
	require.NoError(t, os.Mkdir(dir, 0700))
	file, digest, e := backup(db, dir, []string{"live_session"})
	require.NoError(t, e)
	require.Len(t, digest, 64)
	st, e := os.Stat(file)
	require.NoError(t, e)
	require.Equal(t, os.FileMode(0600), st.Mode().Perm())
	f, e := os.Open(file)
	require.NoError(t, e)
	defer f.Close()
	z, e := gzip.NewReader(f)
	require.NoError(t, e)
	decoder := json.NewDecoder(z)
	var header map[string]any
	require.NoError(t, decoder.Decode(&header))
	require.Contains(t, header["ddl"], "CREATE TABLE")
	records := 0
	for {
		var v map[string]any
		e = decoder.Decode(&v)
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		if _, ok := v["row"]; ok {
			records++
		}
	}
	require.Equal(t, 1, records)
	// Simulate a previous version without the newly introduced counters.
	require.NoError(t, db.Migrator().DropColumn(&model.LiveSession{}, "TotalWatchDurationMs"))
	require.NoError(t, db.Migrator().DropColumn(&model.LiveSession{}, "StatsCheckpointAt"))
	require.NoError(t, initialize.MigratePresence(db))
	require.NoError(t, initialize.MigratePresence(db))
	require.NoError(t, initialize.ValidatePresenceSchema(db))
	var after model.LiveSession
	require.NoError(t, db.First(&after, row.ID).Error)
	require.Equal(t, row.GiftCoinAmount, after.GiftCoinAmount)
	require.Equal(t, row.ViewCount, after.ViewCount)
}
