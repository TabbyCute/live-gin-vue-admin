package initialize

import (
	"context"
	"fmt"
	"tb_live_module/global"
	model "tb_live_module/model/social"
	liveService "tb_live_module/service/live"
	socialService "tb_live_module/service/social"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const socialCounterBackfillBatchSize = 500
const socialSchemaProbeTimeout = 5 * time.Second

type socialCounterBackfillUser struct {
	UserID uint64 `gorm:"column:user_id"`
}

type socialCounterBackfillCount struct {
	UserID        uint64 `gorm:"column:user_id"`
	RelationCount uint64 `gorm:"column:relation_count"`
}

func socialModels() []interface{} {
	return model.Models()
}

// autoMigrateSocial 将社交事实表纳入项目统一的GORM自动迁移流程。
// MySQL新建表保持二进制排序规则，确保公开编号、状态和幂等键按字节比较。
func autoMigrateSocial(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("social module requires database")
	}
	migrationDB := db
	if db.Dialector.Name() == "mysql" {
		migrationDB = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	if err := migrationDB.AutoMigrate(socialModels()...); err != nil {
		return fmt.Errorf("auto-migrate social schema: %w", err)
	}
	if err := backfillMissingSocialCounters(db); err != nil {
		return fmt.Errorf("backfill social counters: %w", err)
	}
	return nil
}

// backfillMissingSocialCounters 只为缺失计数行的存量关注事实补齐权威计数，
// 不覆盖已经上线维护的计数。按user_id游标分批，避免启动时全量载入内存。
func backfillMissingSocialCounters(db *gorm.DB) error {
	var cursor uint64
	for {
		var users []socialCounterBackfillUser
		if err := db.Raw(`
			SELECT social_users.user_id
			FROM (
				SELECT f.user_id
				FROM social_follow AS f
				LEFT JOIN social_user_counter AS c ON c.user_id = f.user_id
				WHERE c.user_id IS NULL AND f.user_id > ?
				UNION
				SELECT f.target_user_id AS user_id
				FROM social_follow AS f
				LEFT JOIN social_user_counter AS c ON c.user_id = f.target_user_id
				WHERE c.user_id IS NULL AND f.target_user_id > ?
			) AS social_users
			ORDER BY social_users.user_id
			LIMIT ?`, cursor, cursor, socialCounterBackfillBatchSize).Scan(&users).Error; err != nil {
			return err
		}
		if len(users) == 0 {
			return nil
		}

		ids := make([]uint64, 0, len(users))
		for _, user := range users {
			ids = append(ids, user.UserID)
		}
		outgoing, err := socialRelationCounts(db, "user_id", ids)
		if err != nil {
			return err
		}
		incoming, err := socialRelationCounts(db, "target_user_id", ids)
		if err != nil {
			return err
		}

		now := model.NewUTCTime(time.Now())
		counters := make([]model.SocialUserCounter, 0, len(ids))
		for _, id := range ids {
			counters = append(counters, model.SocialUserCounter{
				UserID: id, FollowingCount: outgoing[id], FansCount: incoming[id],
				CreatedAt: now, UpdatedAt: now,
			})
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(&counters, socialCounterBackfillBatchSize).Error; err != nil {
			return err
		}
		cursor = ids[len(ids)-1]
	}
}

func socialRelationCounts(db *gorm.DB, column string, ids []uint64) (map[uint64]uint64, error) {
	if column != "user_id" && column != "target_user_id" {
		return nil, fmt.Errorf("unsupported social relation column %q", column)
	}
	var rows []socialCounterBackfillCount
	if err := db.Model(&model.SocialFollow{}).
		Select(column+" AS user_id, COUNT(*) AS relation_count").
		Where(column+" IN ?", ids).
		Group(column).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[uint64]uint64, len(rows))
	for _, row := range rows {
		counts[row.UserID] = row.RelationCount
	}
	return counts, nil
}

// InitializeSocial 在RegisterTables完成GORM迁移后校验结构并装配运行依赖。
func InitializeSocial() error {
	s := socialService.Default
	s.Config = global.GVA_CONFIG.Social
	if global.GVA_REDIS == nil {
		return fmt.Errorf("social module requires Redis for shared write rate limits")
	}
	if global.GVA_DB == nil {
		return fmt.Errorf("social module requires database")
	}
	for _, m := range model.Models() {
		if err := validateSocialModel(global.GVA_DB, m); err != nil {
			return err
		}
	}
	s.DB = global.GVA_DB
	s.Redis = global.GVA_REDIS
	s.Log = global.GVA_LOG
	s.Directory = liveService.SocialDirectory{}
	s.Resources = map[model.ResourceType]socialService.ResourceProvider{model.ResourceLiveRoom: liveService.SocialRoomResource{}}
	// 没有已配置的站内信/WebSocket/系统推送渠道；不注册假投递器。
	return nil
}

// validateSocialModel gives each table its own bounded probe. A single deadline shared by
// every table makes later tables fail only because earlier metadata queries consumed it.
func validateSocialModel(db *gorm.DB, m interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), socialSchemaProbeTimeout)
	defer cancel()

	if !db.WithContext(ctx).Migrator().HasTable(m) {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("probe social schema table %T: %w", m, err)
		}
		return fmt.Errorf("social schema missing after GORM auto-migration: %T", m)
	}
	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(m); err != nil {
		return err
	}
	if err := db.WithContext(ctx).Model(m).Select(statement.Schema.DBNames).Limit(0).Find(m).Error; err != nil {
		return fmt.Errorf("social schema incompatible: %s: %w", statement.Schema.Table, err)
	}
	return nil
}
