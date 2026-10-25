package initialize

import (
	"regexp"
	"testing"

	liveModel "tb_live_module/model/live"
	"tb_live_module/model/system"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPrepareLiveAccountAutoMigrationBackfillsAccountNumber(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE live_account (
		id integer PRIMARY KEY AUTOINCREMENT,
		created_at datetime, updated_at datetime, deleted_at datetime,
		username varchar(32) NOT NULL, password varchar(255) NOT NULL,
		nickname varchar(64) NOT NULL DEFAULT '', avatar varchar(500) NOT NULL DEFAULT '',
		status tinyint NOT NULL DEFAULT 1
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO live_account (username, password, nickname) VALUES
		('legacy_one', 'hash', 'One'), ('legacy_two', 'hash', 'Two')`).Error)

	require.NoError(t, prepareLiveAccountAutoMigration(db))
	var accountNos []string
	require.NoError(t, db.Table("live_account").Order("id").Pluck("account_no", &accountNos).Error)
	require.Len(t, accountNos, 2)
	require.NotEqual(t, accountNos[0], accountNos[1])
	for _, accountNo := range accountNos {
		require.Regexp(t, regexp.MustCompile(`^U[A-F0-9]{20}$`), accountNo)
	}
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}, &liveModel.LiveAccountNumberSequence{}, &liveModel.LiveAccountRealName{}))
	require.NoError(t, ensureLiveAccountPerformanceIndexes(db))
	require.NoError(t, ensureLiveAccountNumberSequence(db))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveAccount{}, "uk_live_account_account_no"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveAccount{}, "idx_live_account_created_at"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveAccountRealName{}, "uk_live_account_real_name_account"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveAccountRealName{}, "uk_live_account_real_name_document"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveAccountRealName{}, "uk_live_account_real_name_provider_ref"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveAccountRealName{}, "idx_live_account_real_name_status_expire"))
	var sequence liveModel.LiveAccountNumberSequence
	require.NoError(t, db.First(&sequence, 1).Error)
	require.Equal(t, liveModel.LiveAccountNumberStart, sequence.NextValue)
}

func TestSyncLiveAccountAdminMetadataIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&system.SysApi{}, &system.SysBaseMenu{}, &system.SysAuthority{}, &system.SysAuthorityMenu{},
	))
	require.NoError(t, db.Create(&system.SysAuthority{AuthorityId: 888, AuthorityName: "超级管理员"}).Error)
	require.NoError(t, syncLiveAccountAdminMetadata(db))
	require.NoError(t, syncLiveAccountAdminMetadata(db))

	var menuCount, apiCount, relationCount int64
	require.NoError(t, db.Model(&system.SysBaseMenu{}).Where("name IN ?", []string{"appUserManagement", "liveAccount"}).Count(&menuCount).Error)
	require.NoError(t, db.Model(&system.SysApi{}).Where("api_group = ?", "客户端用户").Count(&apiCount).Error)
	require.NoError(t, db.Model(&system.SysAuthorityMenu{}).Where("sys_authority_authority_id = ?", "888").Count(&relationCount).Error)
	require.EqualValues(t, 2, menuCount)
	require.EqualValues(t, len(liveAccountAdminAPIs()), apiCount)
	require.EqualValues(t, 2, relationCount)
}

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
	require.NoError(t, db.AutoMigrate(
		&liveModel.LiveRoom{}, &liveModel.LiveSession{},
		&liveModel.LiveSRSRuntime{}, &liveModel.LiveSessionMediaAudit{},
	))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stream_info"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "publish_ip"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "prepare_deadline_at"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "publish_credential_issue_count"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "publish_idempotency_key_hash"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "publish_result_ciphertext"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "srs_stream_id"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stream_ready_deadline_at"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stream_probe_attempts"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stream_probe_next_at"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stream_probe_last_error"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stop_next_retry_at"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stop_lease_token"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stop_lease_until"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "srs_generation"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "publisher_epoch"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "media_state"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "stop_recovery_state"))
	require.True(t, db.Migrator().HasColumn(&liveModel.LiveSession{}, "media_stop_source"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_prepare_timeout"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_stream_ready"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_stream_probe"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_end_retry"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_stop_lease"))
	require.True(t, db.Migrator().HasIndex(&liveModel.LiveSession{}, "idx_live_session_end_stall"))
	require.True(t, db.Migrator().HasTable(&liveModel.LiveSRSRuntime{}))
	require.True(t, db.Migrator().HasTable(&liveModel.LiveSessionMediaAudit{}))

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
