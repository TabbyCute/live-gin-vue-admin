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

func TestBackfillLiveRoomLastSession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveRoom{}, &liveModel.LiveSession{}))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stream_info"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "publish_ip"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "prepare_deadline_at"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "srs_stream_id"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stop_next_retry_at"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_prepare_timeout"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_end_retry"))

	room := liveModel.LiveRoom{RoomNo: "10001", AnchorId: 1, StreamName: "10001"}
	require.NoError(t, db.Create(&room).Error)
	cancelled := liveModel.LiveSession{SessionNo: "cancelled", RoomId: room.ID, AnchorId: 1, Status: liveModel.LiveSessionCancelled}
	ended1 := liveModel.LiveSession{SessionNo: "ended-1", RoomId: room.ID, AnchorId: 1, Status: liveModel.LiveSessionEnded}
	ended2 := liveModel.LiveSession{SessionNo: "ended-2", RoomId: room.ID, AnchorId: 1, Status: liveModel.LiveSessionEnded}
	require.NoError(t, db.Create(&cancelled).Error)
	require.NoError(t, db.Create(&ended1).Error)
	require.NoError(t, db.Create(&ended2).Error)

	require.NoError(t, backfillLiveRoomLastSession(db))
	require.NoError(t, db.First(&room, room.ID).Error)
	require.Equal(t, ended2.ID, room.LastSessionId)

	// 已经写入的指针不能在重复启动时被覆盖。
	require.NoError(t, db.Model(&room).Update("last_session_id", ended1.ID).Error)
	require.NoError(t, backfillLiveRoomLastSession(db))
	require.NoError(t, db.First(&room, room.ID).Error)
	require.Equal(t, ended1.ID, room.LastSessionId)
}
