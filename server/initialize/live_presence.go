package initialize

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/global"
	model "tb_live_module/model/live"
	liveService "tb_live_module/service/live"
	socialService "tb_live_module/service/social"
	"time"
)

// MigratePresence only adds presence-owned tables and two nullable-free counters.
// It never runs broad application migrations or touches any financial fact.
func MigratePresence(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("presence requires database")
	}
	migration := db
	if db.Dialector.Name() == "mysql" {
		migration = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	if e := migration.AutoMigrate(model.PresenceModels()...); e != nil {
		return e
	}
	for _, field := range []string{"TotalWatchDurationMs", "StatsCheckpointAt"} {
		if !db.Migrator().HasColumn(&model.LiveSession{}, field) {
			if e := db.Migrator().AddColumn(&model.LiveSession{}, field); e != nil {
				return e
			}
		}
	}
	if !db.Migrator().HasIndex(&model.LiveSession{}, "idx_live_presence_finalize") {
		if e := db.Migrator().CreateIndex(&model.LiveSession{}, "idx_live_presence_finalize"); e != nil {
			return e
		}
	}
	return ValidatePresenceSchema(db)
}
func ValidatePresenceSchema(db *gorm.DB) error {
	for _, m := range model.PresenceModels() {
		statement := &gorm.Statement{DB: db}
		if e := statement.Parse(m); e != nil {
			return e
		}
		if e := db.Model(m).Select(statement.Schema.DBNames).Limit(0).Find(m).Error; e != nil {
			return fmt.Errorf("presence schema incompatible: %s", statement.Schema.Table)
		}
	}
	return db.Model(&model.LiveSession{}).Select("total_watch_duration_ms,stats_checkpoint_at").Limit(0).Find(&model.LiveSession{}).Error
}
func InitializePresence() error {
	if e := ValidatePresenceSchema(global.GVA_DB); e != nil {
		return e
	}
	if global.GVA_REDIS == nil || len(global.GVA_CONFIG.JWTApp.SigningKey) < 32 {
		return fmt.Errorf("presence requires Redis and APP signing configuration")
	}
	s := liveService.DefaultPresence
	s.DB = global.GVA_DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	s.Redis = global.GVA_REDIS
	s.Config = global.GVA_CONFIG.Presence.Bounds()
	s.Log = global.GVA_LOG
	source := socialService.Default
	s.Social = &socialService.Service{DB: s.DB, Redis: s.Redis, Directory: source.Directory, Resources: source.Resources, Config: source.Config, Log: source.Log}
	h := hmac.New(sha256.New, []byte(global.GVA_CONFIG.JWTApp.SigningKey))
	h.Write([]byte("live-presence/v1/credential-key"))
	s.Key = h.Sum(nil)
	return nil
}
func StartPresenceWorker() func() {
	if liveService.DefaultPresence.DB == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); liveService.DefaultPresence.Run(ctx) }()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			if global.GVA_LOG != nil {
				global.GVA_LOG.Warn("presence worker shutdown timed out")
			}
		}
	}
}
