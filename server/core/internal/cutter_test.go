package internal

import (
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tb_live_module/global"
	"tb_live_module/model/system"
	"testing"
	"time"
)

func TestCutterConcurrentReuseRotationAndRetention(t *testing.T) {
	dir := t.TempDir()
	c := NewCutter(dir, "info", 7, CutterWithLayout(time.DateOnly))
	c.maxBytes = 128
	t.Cleanup(func() { _ = c.Close() })
	_, e := c.Write([]byte("initial\n"))
	require.NoError(t, e)
	first := c.file
	_, e = c.Write([]byte("reuse\n"))
	require.NoError(t, e)
	require.Same(t, first, c.file)
	old := time.Now().AddDate(0, 0, -10).Format(time.DateOnly)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, old), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "unrelated"), 0700))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "unrelated"), time.Time{}, time.Time{}))
	require.NoError(t, removeNDaysFolders(dir, 7))
	_, e = os.Stat(filepath.Join(dir, old))
	require.True(t, os.IsNotExist(e))
	_, e = os.Stat(filepath.Join(dir, "unrelated"))
	require.NoError(t, e)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, err := c.Write([]byte("bounded-line\n"))
				if err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	require.NoError(t, c.Sync())
	files, e := os.ReadDir(filepath.Join(dir, time.Now().Format(time.DateOnly)))
	require.NoError(t, e)
	require.Len(t, files, 4)
	for _, f := range files {
		s, e := f.Info()
		require.NoError(t, e)
		require.LessOrEqual(t, s.Size(), int64(128))
		require.Equal(t, os.FileMode(0600), s.Mode().Perm())
	}
	_, e = c.Write([]byte(strings.Repeat("x", 129)))
	require.Error(t, e)
}

func TestCutterRejectsSymlinkAndPathLabels(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(outside, []byte("original"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "info.log")))
	c := NewCutter(dir, "info", 1, CutterWithFormats("../escape", "/absolute"))
	_, e := c.Write([]byte("changed"))
	require.Error(t, e)
	b, e := os.ReadFile(outside)
	require.NoError(t, e)
	require.Equal(t, "original", string(b))
}

func TestCutterTightensLegacyPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "legacy")
	day := filepath.Join(dir, time.Now().Format(time.DateOnly))
	require.NoError(t, os.MkdirAll(day, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(day, "info.log"), []byte("old\n"), 0644))
	c := NewCutter(dir, "info", 7, CutterWithLayout(time.DateOnly))
	t.Cleanup(func() { c.Close() })
	_, e := c.Write([]byte("new\n"))
	require.NoError(t, e)
	for _, p := range []string{dir, day} {
		st, e := os.Stat(p)
		require.NoError(t, e)
		require.Equal(t, os.FileMode(0700), st.Mode().Perm())
	}
	st, e := os.Stat(filepath.Join(day, "info.log"))
	require.NoError(t, e)
	require.Equal(t, os.FileMode(0600), st.Mode().Perm())
}

func TestZapCoreConcurrentBusinessLabelsDoNotMutateCore(t *testing.T) {
	previous := global.GVA_DB
	global.GVA_DB = nil
	t.Cleanup(func() { global.GVA_DB = previous })
	base, logs := observer.New(zapcore.InfoLevel)
	core := &ZapCore{level: zapcore.InfoLevel, Core: base}
	l := zap.New(core)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.With(zap.String("folder", fmt.Sprint(n))).Info("event", zap.String("business", "../../unsafe"))
			}
		}(i)
	}
	wg.Wait()
	require.Same(t, base, core.Core)
	require.Equal(t, 1600, logs.Len())
}

func BenchmarkCutterWrite(b *testing.B) {
	c := NewCutter(b.TempDir(), "info", 7, CutterWithLayout(time.DateOnly))
	defer c.Close()
	line := []byte("{\"route\":\"/gift/send\",\"status\":200,\"duration\":0.01}\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := c.Write(line); e != nil {
			b.Fatal(e)
		}
	}
}

func TestZapCorePersistsBoundTraceWithoutSensitiveCopies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/logs.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&system.SysError{}))
	previous := global.GVA_DB
	global.GVA_DB = db
	t.Cleanup(func() { global.GVA_DB = previous })
	base, _ := observer.New(zapcore.ErrorLevel)
	core := &ZapCore{level: zapcore.ErrorLevel, Core: base}
	trace := "rt_93388ef6-4541-4ea4-91de-3ee7d925861d"
	zap.New(core).With(zap.String("traceId", trace)).Error("secret bank payload", zap.String("password", "secret-password"))
	var record system.SysError
	require.NoError(t, db.First(&record).Error)
	require.NotNil(t, record.Info)
	require.Contains(t, *record.Info, trace)
	require.NotContains(t, *record.Info, "secret")
	require.Equal(t, trace, traceFromFields(trace, []zapcore.Field{zap.String("traceId", "forged\nsecret")}))
}
