package initialize

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/ranking"
	"time"
)

func MigrateRanking(db *gorm.DB, timezone string) error {
	if _, e := time.LoadLocation(timezone); e != nil {
		return e
	}
	if e := migrateEngagement(db, rankingModels()); e != nil {
		return e
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Checkpoint{ConsumerName: "gift-revenue", Generation: 1, Timezone: timezone, UpdatedAt: time.Now().UnixMilli()}).Error
}
func ValidateRankingSchema(db *gorm.DB) error { return validateEngagement(db, rankingModels()) }
