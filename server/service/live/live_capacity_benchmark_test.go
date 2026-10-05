package live

import (
	"fmt"
	"testing"
	"time"

	"tb_live_module/config"
	"tb_live_module/global"
	srsclient "tb_live_module/internal/srs"
	liveModel "tb_live_module/model/live"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// BenchmarkLiveStreamReconciliationCapacity 是可重复运行的应用层容量基准，覆盖
// 100、500、1000、5000、10000路直播。它使用SQLite验证批量算法和SQL次数趋势，不能替代
// 生产环境中的MySQL 8、真实SRS和网络压测。
func BenchmarkLiveStreamReconciliationCapacity(b *testing.B) {
	for _, sessionCount := range []int{100, 500, 1000, 5000, 10000} {
		b.Run(fmt.Sprintf("streams_%d", sessionCount), func(b *testing.B) {
			benchmarkLiveStreamReconciliation(b, sessionCount)
		})
	}
}

func benchmarkLiveStreamReconciliation(b *testing.B, sessionCount int) {
	db, err := gorm.Open(sqlite.Open("file:"+b.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}
	if err = db.AutoMigrate(&liveModel.LiveRoom{}, &liveModel.LiveSession{}, &liveModel.LiveSRSRuntime{}); err != nil {
		b.Fatal(err)
	}
	previousDB, previousLive := global.GVA_DB, global.GVA_CONFIG.Live
	global.GVA_DB = db
	global.GVA_CONFIG.Live = config.Live{
		MaxConcurrentStreams:            10000,
		ReconnectWindowSeconds:          20,
		StreamReconcileIntervalSeconds:  5,
		StreamReconcileMissingThreshold: 3,
		StreamReconcileBatchSize:        1000,
		StreamReconcileMaxBatchesPerRun: 10,
	}
	b.Cleanup(func() {
		global.GVA_DB, global.GVA_CONFIG.Live = previousDB, previousLive
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	now := time.Now().UnixMilli()
	snapshots := make([]srsclient.PublisherSnapshot, 0, sessionCount)
	if err = db.Transaction(func(tx *gorm.DB) error {
		for index := 0; index < sessionCount; index++ {
			room := liveModel.LiveRoom{
				RoomNo: fmt.Sprintf("capacity-room-%06d", index), AnchorId: uint(index + 1),
				StreamName: fmt.Sprintf("capacity-stream-%06d", index),
				Status:     liveModel.LiveRoomStatusNormal, LiveStatus: liveModel.LiveRoomLiving,
				Visibility: liveModel.LiveRoomVisibilityPublic,
			}
			if createErr := tx.Create(&room).Error; createErr != nil {
				return createErr
			}
			streamID := fmt.Sprintf("capacity-srs-%06d", index)
			session := liveModel.LiveSession{
				SessionNo: fmt.Sprintf("capacity-session-%06d", index), RoomId: room.ID,
				AnchorId: room.AnchorId, Status: liveModel.LiveSessionLiving, StartedAt: now - 60_000,
				SRSStreamID: streamID, SRSVhost: "127.0.0.1", SRSApp: "live",
				StreamReconcileCheckedAt: now - 10_000,
			}
			if createErr := tx.Create(&session).Error; createErr != nil {
				return createErr
			}
			if updateErr := tx.Model(&room).Update("current_session_id", session.ID).Error; updateErr != nil {
				return updateErr
			}
			snapshots = append(snapshots, srsclient.PublisherSnapshot{
				StreamID: streamID, Vhost: "127.0.0.1", App: "live", Stream: room.StreamName, Active: true,
			})
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	controller := &fakeSRSController{snapshots: snapshots}
	service := RoomService{srsController: controller}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		checkedAt := now + int64(iteration+1)*10_000
		if err = db.Model(&liveModel.LiveSession{}).Where("1 = 1").
			Update("stream_reconcile_checked_at", checkedAt-10_000).Error; err != nil {
			b.Fatal(err)
		}
		if err = service.ProcessLiveStreamReconciliation(checkedAt); err != nil {
			b.Fatal(err)
		}
	}
}
