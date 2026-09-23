package live

import (
	"encoding/json"
	"testing"
	"time"

	"tb_live_module/config"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRoomTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&liveModel.LiveCategory{}, &liveModel.LiveAnchor{}, &liveModel.LiveRoom{}, &liveModel.LiveSession{},
	))
	previousDB, previousLive := global.GVA_DB, global.GVA_CONFIG.Live
	global.GVA_DB = db
	global.GVA_CONFIG.Live = config.Live{
		ReconnectWindowSeconds: 1,
		SRSHookToken:           "hook-secret",
		PushBaseURL:            "rtmp://127.0.0.1/live",
		PlayBaseURL:            "https://127.0.0.1/live",
	}
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.GVA_CONFIG.Live = previousLive
	})
}

func createRoomTestAnchor(t *testing.T) (liveModel.LiveAnchor, liveModel.LiveCategory) {
	t.Helper()
	category := liveModel.LiveCategory{Code: "talk", Name: "聊天", Status: liveModel.LiveCategoryStatusEnabled}
	require.NoError(t, global.GVA_DB.Create(&category).Error)
	anchor := liveModel.LiveAnchor{
		UserId: 501, AnchorNo: "1500001", Nickname: "测试主播", CategoryId: uint64(category.ID),
		ApplyStatus: liveModel.AnchorApplyStatusApproved, Status: liveModel.AnchorStatusNormal,
		LivePermission: liveModel.AnchorPermissionEnabled,
	}
	require.NoError(t, global.GVA_DB.Create(&anchor).Error)
	return anchor, category
}

func prepareRoomTestSession(t *testing.T, service *RoomService, categoryID uint) liveResFixture {
	t.Helper()
	visibility := liveModel.LiveRoomVisibilityPublic
	prepared, err := service.PrepareSession(501, liveReq.LiveSessionPrepareReq{
		CategoryId: uint64(categoryID), Title: "第一场直播", Visibility: &visibility,
	})
	require.NoError(t, err)
	return liveResFixture{RoomNo: prepared.RoomNo, SessionNo: prepared.SessionNo, StreamName: prepared.StreamName, Token: prepared.PublishToken}
}

type liveResFixture struct {
	RoomNo, SessionNo, StreamName, Token string
}

func TestLiveRoomLifecycleReconnectAndFinalize(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	service := RoomService{}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	require.NotEmpty(t, prepared.Token)

	started, err := service.OnPublish(prepared.StreamName, "?token="+prepared.Token)
	require.NoError(t, err)
	require.Equal(t, liveModel.LiveSessionLiving, started.Status)
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	firstSessionID := session.ID

	require.NoError(t, service.OnUnpublish(prepared.StreamName))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	firstDeadline := session.ReconnectDeadlineAt
	require.Positive(t, firstDeadline)
	require.Equal(t, uint32(1), session.DisconnectCount)
	// 同一次断流的重复回调不得累计次数，也不得延长窗口。
	require.NoError(t, service.OnUnpublish(prepared.StreamName))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	require.Equal(t, firstDeadline, session.ReconnectDeadlineAt)
	require.Equal(t, uint32(1), session.DisconnectCount)

	reconnected, err := service.OnPublish(prepared.StreamName, "token="+prepared.Token)
	require.NoError(t, err)
	require.Equal(t, prepared.SessionNo, reconnected.SessionNo)
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	require.Zero(t, session.ReconnectDeadlineAt)
	require.NoError(t, service.UpdateSessionStats(liveReq.LiveSessionStatsReq{
		SessionNo: prepared.SessionNo, ViewCount: 88, ViewerCount: 50, PeakOnlineCount: 23,
		LikeCount: 120, GiftCount: 9, GiftCoinAmount: 660, GiftUserCount: 4,
	}))

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, service.OnUnpublish(prepared.StreamName))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	lastUnpublishAt, deadline := session.LastUnpublishAt, session.ReconnectDeadlineAt
	require.NoError(t, service.ProcessPendingSessions(deadline+1))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	require.Equal(t, liveModel.LiveSessionEnding, session.Status)
	require.Equal(t, lastUnpublishAt, session.EndedAt, "最终重连等待窗口不能计入逻辑直播时长")

	require.NoError(t, service.ProcessPendingSessions(deadline+2))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	require.Equal(t, liveModel.LiveSessionEnded, session.Status)
	require.Equal(t, uint64(session.EndedAt-session.StartedAt), session.DurationMs)
	require.Equal(t, uint64(660), session.GiftCoinAmount)
	require.NoError(t, global.GVA_DB.First(&anchor, anchor.ID).Error)
	require.Equal(t, uint64(1), anchor.TotalLiveCount)
	require.Equal(t, session.DurationMs, anchor.TotalLiveDurationMs)
	require.Equal(t, uint64(88), anchor.TotalViewCount)
	require.Equal(t, uint32(23), anchor.MaxOnlineCount)

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.RoomNo).First(&room).Error)
	require.Zero(t, room.CurrentSessionId)
	require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)
}

func TestLiveSessionActiveUniqueAndClientIDs(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := RoomService{}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.RoomNo).First(&room).Error)

	duplicate := liveModel.LiveSession{SessionNo: "Sduplicate", RoomId: room.ID, AnchorId: room.AnchorId, Status: liveModel.LiveSessionPreparing}
	require.Error(t, global.GVA_DB.Create(&duplicate).Error, "生成列唯一索引必须兜底阻止同一房间出现两个活动场次")

	owner, err := service.GetOwnerRoom(501)
	require.NoError(t, err)
	current, err := service.GetOwnerCurrentSession(501)
	require.NoError(t, err)
	for name, value := range map[string]interface{}{"owner": owner, "current": current} {
		raw, marshalErr := json.Marshal(value)
		require.NoError(t, marshalErr)
		require.NotContains(t, string(raw), `"id"`, "%s 响应不得暴露数据库主键", name)
		require.NotContains(t, string(raw), `"roomId"`, "%s 响应不得暴露直播间主键", name)
		require.NotContains(t, string(raw), `"anchorId"`, "%s 响应不得暴露主播主键", name)
	}
	require.ErrorIs(t, ValidateLiveHookToken("wrong"), ErrLiveHookUnauthorized)
	require.NoError(t, ValidateLiveHookToken("hook-secret"))

	disabled := liveModel.LiveRoomStatusDisabled
	require.NoError(t, service.UpdateAdminRoomStatus(liveReq.LiveRoomAdminStatusReq{
		RoomID: room.ID, Status: &disabled, StatusReason: "风控禁用",
	}))
	require.ErrorIs(t, service.UpdateOwnerRoomStatus(501, liveModel.LiveRoomStatusNormal), ErrLiveRoomUnavailable,
		"主播不能自行恢复后台禁用的直播间")
}

func TestAdminCanChangeRoomNoWithoutChangingStreamName(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	service := RoomService{}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	require.Equal(t, anchor.AnchorNo, prepared.RoomNo)
	require.Equal(t, anchor.AnchorNo, prepared.StreamName)

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("anchor_id = ?", anchor.ID).First(&room).Error)
	visibility := liveModel.LiveRoomVisibilityPublic
	require.NoError(t, service.UpdateAdminRoom(liveReq.LiveRoomAdminUpdateReq{
		RoomID: room.ID, RoomNo: "custom-room-no", CategoryId: uint64(category.ID),
		Title: "修改后的直播间", Visibility: &visibility,
	}))
	require.NoError(t, global.GVA_DB.First(&room, room.ID).Error)
	require.Equal(t, "custom-room-no", room.RoomNo)
	require.Equal(t, anchor.AnchorNo, room.StreamName, "修改对外房间号不能改变稳定推流名称")

	otherAnchor := liveModel.LiveAnchor{
		UserId: 502, AnchorNo: "1500002", Nickname: "另一主播",
		ApplyStatus: liveModel.AnchorApplyStatusApproved, Status: liveModel.AnchorStatusNormal,
	}
	require.NoError(t, global.GVA_DB.Create(&otherAnchor).Error)
	otherRoom, err := ensureRoomForAnchor(global.GVA_DB, otherAnchor)
	require.NoError(t, err)
	require.ErrorIs(t, service.UpdateAdminRoom(liveReq.LiveRoomAdminUpdateReq{
		RoomID: otherRoom.ID, RoomNo: room.RoomNo, CategoryId: uint64(category.ID),
		Title: "冲突编号", Visibility: &visibility,
	}), ErrLiveRoomNoExists)
}

func TestBackfillApprovedAnchorRoomsIsIdempotent(t *testing.T) {
	setupRoomTestDB(t)
	approved := liveModel.LiveAnchor{
		UserId: 601, AnchorNo: "1600001", Nickname: "存量主播",
		ApplyStatus: liveModel.AnchorApplyStatusApproved, Status: liveModel.AnchorStatusNormal,
	}
	pending := liveModel.LiveAnchor{
		UserId: 602, AnchorNo: "1600002", Nickname: "待审核主播",
		ApplyStatus: liveModel.AnchorApplyStatusPending, Status: liveModel.AnchorStatusNormal,
	}
	require.NoError(t, global.GVA_DB.Create(&approved).Error)
	require.NoError(t, global.GVA_DB.Create(&pending).Error)
	service := RoomService{}
	require.NoError(t, service.BackfillApprovedAnchorRooms(global.GVA_DB))
	require.NoError(t, service.BackfillApprovedAnchorRooms(global.GVA_DB))

	var rooms []liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Order("id ASC").Find(&rooms).Error)
	require.Len(t, rooms, 1)
	require.Equal(t, approved.ID, rooms[0].AnchorId)
	require.Equal(t, approved.AnchorNo, rooms[0].RoomNo)
	require.Equal(t, approved.AnchorNo, rooms[0].StreamName)
}

func TestAnchorBanCancelsPreparedSession(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	roomService := RoomService{}
	prepared := prepareRoomTestSession(t, &roomService, category.ID)
	reason := "违规封禁"
	banned := liveModel.AnchorStatusBanned
	require.NoError(t, (&AnchorService{}).UpdateAnchorStatus(liveReq.AnchorStatusUpdateReq{
		AnchorId: anchor.ID, Status: &banned, StatusReason: reason,
	}))
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	require.Equal(t, liveModel.LiveSessionEndAnchorBlocked, session.EndReason)
	require.Equal(t, reason, session.FailureReason)
	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.RoomNo).First(&room).Error)
	require.Zero(t, room.CurrentSessionId)
}
