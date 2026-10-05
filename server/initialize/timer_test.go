package initialize

import (
	"testing"
	"time"

	"tb_live_module/config"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	timerutil "tb_live_module/utils/timer"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTimerImmediatelyProcessesPendingLiveSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveRoom{}, &liveModel.LiveSession{}, &liveModel.LiveSRSRuntime{}))

	room := liveModel.LiveRoom{
		RoomNo: "startup-room", AnchorId: 1001, StreamName: "startup-stream",
		Status: liveModel.LiveRoomStatusNormal, LiveStatus: liveModel.LiveRoomPreparing,
		Visibility: liveModel.LiveRoomVisibilityPublic,
	}
	require.NoError(t, db.Create(&room).Error)
	session := liveModel.LiveSession{
		SessionNo: "startup-expired", RoomId: room.ID, AnchorId: room.AnchorId,
		Status: liveModel.LiveSessionPreparing, PrepareDeadlineAt: time.Now().Add(-time.Minute).UnixMilli(),
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Model(&room).Update("current_session_id", session.ID).Error)

	previousDB, previousLive, previousTimer := global.GVA_DB, global.GVA_CONFIG.Live, global.GVA_Timer
	testTimer := timerutil.NewTimerTask()
	global.GVA_DB = db
	global.GVA_CONFIG.Live = config.Live{EndScanIntervalSeconds: 3600, EndScanBatchSize: 100}
	global.GVA_Timer = testTimer
	t.Cleanup(func() {
		testTimer.Close()
		global.GVA_DB, global.GVA_CONFIG.Live, global.GVA_Timer = previousDB, previousLive, previousTimer
		require.NoError(t, sqlDB.Close())
	})

	startedAt := time.Now()
	Timer()
	for _, name := range []string{
		"LiveSessionLifecycle", "LiveSessionStop", "LiveStreamReadiness", "LiveStreamReconciliation",
	} {
		_, exists := testTimer.FindCron(name)
		require.True(t, exists, "定时任务未注册: %s", name)
	}
	require.Eventually(t, func() bool {
		if err := db.First(&session, session.ID).Error; err != nil {
			return false
		}
		return session.Status == liveModel.LiveSessionCancelled
	}, time.Second, 10*time.Millisecond)
	require.Less(t, time.Since(startedAt), time.Second, "启动补偿不能等待首个一小时扫描周期")
	require.Equal(t, liveModel.LiveSessionEndPrepareTimeout, session.EndReason)
	require.NoError(t, db.First(&room, room.ID).Error)
	require.Zero(t, room.CurrentSessionId)
	require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)
}
