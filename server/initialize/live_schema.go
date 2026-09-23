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
