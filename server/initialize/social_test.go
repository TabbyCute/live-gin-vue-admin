package initialize

import (
	"testing"
	"time"

	socialModel "tb_live_module/model/social"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAutoMigrateSocialCreatesSchemaConstraintsAndBackfillsCounters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, autoMigrateSocial(db))

	expectedChecks := map[string][]string{
		"social_follow":            {"ck_social_follow_1"},
		"social_user_counter":      {"ck_social_user_counter_1"},
		"social_block":             {"ck_social_block_1"},
		"social_mute":              {"ck_social_mute_1", "ck_social_mute_2"},
		"social_favorite":          {"ck_social_favorite_1"},
		"social_watch_history":     {"ck_social_watch_history_1", "ck_social_watch_history_2"},
		"social_live_subscription": {"ck_social_live_subscription_1"},
		"social_outbox_event":      {"ck_social_outbox_event_1"},
		"social_event_consumer":    {"ck_social_event_consumer_1"},
	}
	for _, table := range socialModels() {
		require.True(t, db.Migrator().HasTable(table))
		statement := &gorm.Statement{DB: db}
		require.NoError(t, statement.Parse(table))
		checks := statement.Schema.ParseCheckConstraints()
		for _, name := range expectedChecks[statement.Schema.Table] {
			require.Contains(t, checks, name)
			require.True(t, db.Migrator().HasConstraint(table, name))
		}
	}
	require.True(t, db.Migrator().HasIndex(&socialModel.SocialFollow{}, "uk_social_follow_pair"))
	require.True(t, db.Migrator().HasIndex(&socialModel.SocialOutboxEvent{}, "idx_social_event_claim"))

	now := socialModel.NewUTCTime(time.Now())
	err = db.Create(&socialModel.SocialFollow{
		Base: socialModel.Base{CreatedAt: now, UpdatedAt: now}, UserID: 1, TargetUserID: 1,
	}).Error
	require.Error(t, err, "GORM模型必须保留禁止自己关注自己的数据库约束")

	require.NoError(t, db.Create(&socialModel.SocialUserCounter{
		UserID: 1, FollowingCount: 99, FansCount: 88, CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create([]socialModel.SocialFollow{
		{Base: socialModel.Base{CreatedAt: now, UpdatedAt: now}, UserID: 1, TargetUserID: 2},
		{Base: socialModel.Base{CreatedAt: now, UpdatedAt: now}, UserID: 1, TargetUserID: 3},
		{Base: socialModel.Base{CreatedAt: now, UpdatedAt: now}, UserID: 2, TargetUserID: 1},
	}).Error)

	require.NoError(t, autoMigrateSocial(db))
	require.NoError(t, autoMigrateSocial(db), "自动迁移与数据补齐必须可重复执行")

	var counters []socialModel.SocialUserCounter
	require.NoError(t, db.Order("user_id").Find(&counters).Error)
	require.Len(t, counters, 3)
	require.Equal(t, uint64(99), counters[0].FollowingCount, "不得覆盖已经上线维护的计数")
	require.Equal(t, uint64(88), counters[0].FansCount)
	require.Equal(t, uint64(1), counters[1].FollowingCount)
	require.Equal(t, uint64(1), counters[1].FansCount)
	require.Zero(t, counters[2].FollowingCount)
	require.Equal(t, uint64(1), counters[2].FansCount)
}
