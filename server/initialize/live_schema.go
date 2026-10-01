package initialize

import (
	"tb_live_module/model/live"

	"gorm.io/gorm"
)

// migrateLiveDurationColumn 将旧版以秒保存的累计直播时长一次性迁移为毫秒。
// 先由 AutoMigrate 创建新列，再回填并删除旧列；重复执行不会重复换算。
func migrateLiveDurationColumn(db *gorm.DB) error {
	if !db.Migrator().HasTable(&live.LiveAnchor{}) || !db.Migrator().HasColumn("live_anchor", "total_live_duration") {
		return nil
	}
	if err := db.Exec(`UPDATE live_anchor
		SET total_live_duration_ms = total_live_duration * 1000
		WHERE total_live_duration_ms = 0 AND total_live_duration > 0`).Error; err != nil {
		return err
	}
	// 字段名是固定常量；使用标准 DDL 可避免部分 SQLite Migrator 在“模型已删除旧字段”时
	// 无法从 Schema 定位列而触发空指针。
	return db.Exec("ALTER TABLE live_anchor DROP COLUMN total_live_duration").Error
}

// backfillLiveRoomLastSession 为存量直播间补齐最近一次已完成直播场次指针。
// 只处理 status=3 的正常已结算场次，准备后取消或失败的场次不属于“最后一次直播”。
func backfillLiveRoomLastSession(db *gorm.DB) error {
	if !db.Migrator().HasTable(&live.LiveRoom{}) || !db.Migrator().HasTable(&live.LiveSession{}) ||
		!db.Migrator().HasColumn(&live.LiveRoom{}, "last_session_id") {
		return nil
	}
	return db.Exec(`UPDATE live_room
		SET last_session_id = COALESCE((
			SELECT MAX(live_session.id)
			FROM live_session
			WHERE live_session.room_id = live_room.id
				AND live_session.status = ?
				AND live_session.deleted_at IS NULL
		), 0)
		WHERE last_session_id = 0`, live.LiveSessionEnded).Error
}
