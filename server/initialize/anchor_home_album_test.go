package initialize

import (
	"context"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"tb_live_module/config"
	"tb_live_module/global"
	svc "tb_live_module/service/anchorhome"
	"testing"
)

func TestAnchorHomeAlbumStartupPrivateStorageAndNoDDL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "init.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	previousConfig, previousAlbum := global.GVA_CONFIG, svc.DefaultAlbum
	t.Cleanup(func() {
		if svc.DefaultAlbum != previousAlbum {
			svc.DefaultAlbum.Close()
		}
		global.GVA_CONFIG = previousConfig
		svc.DefaultAlbum = previousAlbum
	})
	storage := filepath.Join(t.TempDir(), "private-media")
	global.GVA_CONFIG.AnchorHome = config.AnchorHome{StorageDir: storage}
	global.GVA_CONFIG.Local = config.Local{StorePath: filepath.Join(t.TempDir(), "uploads")}
	require.Error(t, InitializeAnchorHomeAlbum(db))
	require.False(t, db.Migrator().HasTable("anchor_home_photo"))
	_, err = os.Stat(storage)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, MigrateAnchorHomeAlbum(db))
	global.GVA_CONFIG.System.RouterPrefix = "/api"
	require.NoError(t, InitializeAnchorHomeAlbum(db))
	require.Equal(t, "/api/v1/app/anchor-home", svc.DefaultAlbum.Options.PublicBase)
	info, err := os.Stat(storage)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	require.NoError(t, svc.DefaultAlbum.Close())
	public := filepath.Join(t.TempDir(), "public")
	require.NoError(t, os.MkdirAll(public, 0755))
	global.GVA_CONFIG.Local.StorePath = public
	global.GVA_CONFIG.AnchorHome.StorageDir = filepath.Join(public, "photos")
	require.Error(t, InitializeAnchorHomeAlbum(db))
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.MkdirAll(outside, 0700))
	link := filepath.Join(public, "link")
	require.NoError(t, os.Symlink(outside, link))
	global.GVA_CONFIG.AnchorHome.StorageDir = link
	require.Error(t, InitializeAnchorHomeAlbum(db))
	global.GVA_CONFIG.AnchorHome.StorageDir = filepath.Join("assets", "unsafe-album-test")
	require.Error(t, InitializeAnchorHomeAlbum(db))
	_ = os.Remove(filepath.Join("assets", "unsafe-album-test"))
	global.GVA_CONFIG.AnchorHome.Disabled = true
	require.NoError(t, InitializeAnchorHomeAlbum(db))
	require.Nil(t, svc.DefaultAlbum.DB)
	select {
	case <-StartAnchorHomeMediaCleanup(context.Background()):
	default:
		t.Fatal("disabled cleanup must not start a worker")
	}
}
