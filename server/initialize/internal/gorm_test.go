package internal

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"tb_live_module/config"
	"tb_live_module/global"
)

func TestSQLLogsDoNotExpandBoundSessionTokens(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	previous := global.GVA_LOG
	global.GVA_LOG = zap.New(core)
	t.Cleanup(func() { global.GVA_LOG = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), Gorm.Config(config.GeneralDB{LogMode: "info", LogZap: true}))
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	const secret = "private-watch-session-token"
	type session struct {
		ID           uint
		SessionToken string
	}
	require.NoError(t, db.AutoMigrate(&session{}))
	require.NoError(t, db.Create(&session{ID: 1, SessionToken: secret}).Error)
	var value session
	require.NoError(t, db.WithContext(context.Background()).Where("session_token = ?", secret).Find(&value).Error)
	require.Equal(t, secret, value.SessionToken, "只隐藏日志参数，不能改变实际查询")
	require.Error(t, db.Exec("SELECT session_token FROM missing_social_table WHERE session_token = ?", secret).Error)
	require.NotEmpty(t, logs.All())
	for _, entry := range logs.All() {
		require.NotContains(t, entry.Message, secret)
	}
}
