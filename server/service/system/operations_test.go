package system

import (
	"context"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/global"
	model "tb_live_module/model/system"
	"testing"
	"time"
)

func TestOperationsAuthorizationBoundedQueuesAndNoSecrets(t *testing.T) {
	oldDB, oldCfg, oldRedis := global.GVA_DB, global.GVA_CONFIG, global.GVA_REDIS
	t.Cleanup(func() { global.GVA_DB, global.GVA_CONFIG, global.GVA_REDIS = oldDB, oldCfg, oldRedis })
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/ops.sqlite"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { _ = pool.Close() })
	global.GVA_DB = db
	global.GVA_CONFIG.System.UseRedis = false
	require.NoError(t, db.AutoMigrate(&model.SysUser{}))
	require.NoError(t, db.Create(&model.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 1}, Username: "private-name", Password: "private-secret", Enable: 1}).Error)
	require.NoError(t, db.Exec("CREATE TABLE payout_order (status TEXT, created_at BIGINT, secret TEXT)").Error)
	for i := 0; i < 1002; i++ {
		require.NoError(t, db.Exec("INSERT INTO payout_order VALUES (?,?,?)", "unknown", time.Now().Add(-time.Hour).UnixMilli(), "private-bank-account").Error)
	}
	s := &SystemConfigService{}
	_, e = s.Operations(context.Background(), 2)
	require.ErrorIs(t, e, ErrOperationsForbidden)
	out, e := s.Operations(context.Background(), 1)
	require.NoError(t, e)
	require.Equal(t, "ready", out.Database)
	require.Equal(t, 1000, out.Backlogs[0].Count)
	require.True(t, out.Backlogs[0].Truncated)
	require.NotEmpty(t, out.Alerts)
	b, e := json.Marshal(out)
	require.NoError(t, e)
	require.NotContains(t, string(b), "private-")
	require.NoError(t, db.Model(&model.SysUser{}).Where("id=1").Update("enable", 2).Error)
	_, e = s.Operations(context.Background(), 1)
	require.ErrorIs(t, e, ErrOperationsForbidden)
}
