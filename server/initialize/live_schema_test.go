package initialize

import (
	"testing"

	liveModel "tb_live_module/model/live"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateLiveDurationColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE live_anchor (
		id integer PRIMARY KEY AUTOINCREMENT,
		user_id bigint unsigned NOT NULL DEFAULT 0,
		anchor_no varchar(32) NOT NULL DEFAULT '',
		total_live_duration bigint unsigned NOT NULL DEFAULT 0
	)`).Error)
	require.NoError(t, db.Exec("INSERT INTO live_anchor (total_live_duration) VALUES (?)", 42).Error)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAnchor{}))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveAnchor{}, "total_live_duration_ms"))

	require.NoError(t, migrateLiveDurationColumn(db))
	var duration uint64
	require.NoError(t, db.Table("live_anchor").Select("total_live_duration_ms").Where("id = ?", 1).Scan(&duration).Error)
	require.Equal(t, uint64(42000), duration)
	require.False(t, db.Migrator().HasColumn(&liveModel.LiveAnchor{}, "total_live_duration"))

	// 重复执行必须是幂等操作。
	require.NoError(t, migrateLiveDurationColumn(db))
}
