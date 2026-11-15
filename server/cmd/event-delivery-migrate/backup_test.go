package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"tb_live_module/internal/testlocal"
	"testing"
)

// Read-only against the explicitly isolated local test schema; never reset/drop tables.
func TestBackupMySQL(t *testing.T) {
	dsn := os.Getenv("EVENT_DELIVERY_BACKUP_MYSQL_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL backup DSN not set")
	}
	validated, err := testlocal.EventDeliveryMySQL(dsn)
	require.NoError(t, err, "backup test must remain local")
	db, err := gorm.Open(mysql.Open(validated), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	dir := filepath.Join(t.TempDir(), "private")
	path, digest, err := backup(db, dir)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(contents)
	require.Equal(t, hex.EncodeToString(sum[:]), digest)
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	counts := map[string]int64{}
	schema := map[string]bool{}
	header, triggers := false, false
	for scanner.Scan() {
		var row map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &row))
		if raw, ok := row["format"]; ok {
			require.JSONEq(t, `"event-delivery-backup/v1"`, string(raw))
			header = true
		}
		if _, ok := row["triggers"]; ok {
			triggers = true
		}
		var table string
		if raw, ok := row["table"]; ok {
			require.NoError(t, json.Unmarshal(raw, &table))
		}
		if _, ok := row["ddl"]; ok {
			schema[table] = true
		}
		if _, ok := row["values"]; ok {
			counts[table]++
		}
	}
	require.NoError(t, scanner.Err())
	require.True(t, header)
	require.True(t, triggers)
	for table := range schema {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		require.Equal(t, count, counts[table])
	}
	insecure := filepath.Join(t.TempDir(), "public")
	require.NoError(t, os.Mkdir(insecure, 0755))
	_, _, err = backup(db, insecure)
	require.Error(t, err)
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(dir, link))
	_, _, err = backup(db, link)
	require.Error(t, err)
}
