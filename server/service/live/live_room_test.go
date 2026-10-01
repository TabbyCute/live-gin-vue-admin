package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"tb_live_module/config"
	"tb_live_module/global"
	srsclient "tb_live_module/internal/srs"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type fakeSRSController struct {
	mu      sync.Mutex
	calls   int
	errors  []error
	started chan struct{}
	release chan struct{}
}

func (f *fakeSRSController) StopPublisher(_ context.Context, _ srsclient.StreamRef) error {
	f.mu.Lock()
	f.calls++
	call := f.calls
	started, release := f.started, f.release
	var err error
	if call <= len(f.errors) {
		err = f.errors[call-1]
	}
	f.mu.Unlock()
	if started != nil && call == 1 {
		close(started)
	}
	if release != nil && call == 1 {
		<-release
	}
	return err
}

func (f *fakeSRSController) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newRoomTestService() RoomService {
	return RoomService{srsController: &fakeSRSController{}}
}

func setupRoomTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// SQLite没有MySQL的行锁语义；单连接让并发测试稳定地验证服务层幂等和最终状态不变量。
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&liveModel.LiveCategory{}, &liveModel.LiveAnchor{}, &liveModel.LiveRoom{}, &liveModel.LiveSession{},
	))
	previousDB, previousLive := global.GVA_DB, global.GVA_CONFIG.Live
	global.GVA_DB = db
	global.GVA_CONFIG.Live = config.Live{
		ReconnectWindowSeconds: 1,
		PrepareTimeoutSeconds:  120,
		PushTokenSeconds:       180,
		PublishTokenKey:        "publish-token-secret",
		SRS: config.SRS{
			App:         "live",
			PushBaseURL: "rtmp://127.0.0.1/live",
			PlayBaseURL: "https://127.0.0.1/live",
		},
	}
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.GVA_CONFIG.Live = previousLive
		require.NoError(t, sqlDB.Close())
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
	return liveResFixture{
		RoomNo: prepared.RoomNo, SessionNo: prepared.SessionNo, StreamName: prepared.StreamName,
		Token: prepared.PublishToken, PushURL: prepared.PushURL, PlayURL: prepared.PlayURL,
		PrepareDeadlineAt: prepared.PrepareDeadlineAt,
	}
}

type liveResFixture struct {
	RoomNo, SessionNo, StreamName, Token, PushURL, PlayURL string
	PrepareDeadlineAt                                      int64
}

func roomTestUnpublishRequest(prepared liveResFixture, clientID string) liveReq.SRSHookReq {
	return liveReq.SRSHookReq{
		ServerID: "srs-1", ClientID: clientID, StreamID: "stream-1",
		Action: "on_unpublish", Vhost: "127.0.0.1", App: "live", Stream: prepared.StreamName,
	}
}

func roomTestPublishRequest(prepared liveResFixture, param string) liveReq.SRSHookReq {
	return liveReq.SRSHookReq{
		ServerID: "srs-1", ClientID: "client-1", StreamID: "stream-1",
		Action: "on_publish", IP: "192.0.2.10", Vhost: "127.0.0.1", App: "live",
		Stream: prepared.StreamName, Param: param,
	}
}

func TestLiveRoomLifecycleReconnectAndFinalize(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	prepared := prepareRoomTestSession(t, &service, category.ID)
	require.NotEmpty(t, prepared.Token)
	require.Regexp(t, `^\d{17}-1500001-1500001$`, prepared.SessionNo)
	require.Equal(t, "rtmp://127.0.0.1/live/1500001?pt="+prepared.Token, prepared.PushURL)
	require.Equal(t, "https://127.0.0.1/live/1500001", prepared.PlayURL)
	claims := decryptLivePublishTokenForTest(t, prepared.Token, "publish-token-secret")
	require.Equal(t, prepared.SessionNo, claims.SessionNo)
	require.Equal(t, prepared.StreamName, claims.Stream)
	require.Equal(t, "live", claims.App)
	require.Equal(t, "127.0.0.1", claims.Vhost)
	require.Equal(t, uint32(1), claims.CredentialVersion)
	require.Positive(t, prepared.PrepareDeadlineAt)
	var preparedRoom liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.RoomNo).First(&preparedRoom).Error)
	require.Equal(t, uint32(1), preparedRoom.StreamKeyVersion)
	require.Equal(t, secretHash(prepared.Token), preparedRoom.PublishSecretHash)

	started, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	require.Equal(t, liveModel.LiveSessionLiving, started.Status)
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, "192.0.2.10", session.PublishIP)
	require.Equal(t, "srs-1", session.SRSServerID)
	require.Equal(t, "stream-1", session.SRSStreamID)
	require.Equal(t, "client-1", session.SRSClientID)
	require.Equal(t, "127.0.0.1", session.SRSVhost)
	require.Equal(t, "live", session.SRSApp)
	firstSessionID := session.ID

	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	firstDeadline := session.ReconnectDeadlineAt
	require.Positive(t, firstDeadline)
	require.Equal(t, uint32(1), session.DisconnectCount)
	// 同一次断流的重复回调不得累计次数，也不得延长窗口。
	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	require.Equal(t, firstDeadline, session.ReconnectDeadlineAt)
	require.Equal(t, uint32(1), session.DisconnectCount)

	reconnected, err := service.OnPublish(roomTestPublishRequest(prepared, "pt="+prepared.Token))
	require.NoError(t, err)
	require.Equal(t, prepared.SessionNo, reconnected.SessionNo)
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	require.Zero(t, session.ReconnectDeadlineAt)
	require.NoError(t, service.UpdateSessionStats(liveReq.LiveSessionStatsReq{
		SessionNo: prepared.SessionNo, ViewCount: 88, ViewerCount: 50, PeakOnlineCount: 23,
		LikeCount: 120, GiftCount: 9, GiftCoinAmount: 660, GiftUserCount: 4,
	}))

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	lastUnpublishAt, deadline := session.LastUnpublishAt, session.ReconnectDeadlineAt
	require.NoError(t, service.ProcessPendingSessions(deadline+1))
	require.NoError(t, global.GVA_DB.First(&session, firstSessionID).Error)
	require.Equal(t, liveModel.LiveSessionEnded, session.Status)
	require.Equal(t, lastUnpublishAt, session.EndedAt, "最终重连等待窗口不能计入逻辑直播时长")
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
	require.Equal(t, firstSessionID, room.LastSessionId)
	require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)
}

func TestPrepareTimeoutCancelsSessionAndInvalidatesPublish(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, prepared.PrepareDeadlineAt, session.PrepareDeadlineAt)
	require.NoError(t, service.ProcessPendingSessions(session.PrepareDeadlineAt-1))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status, "截止时间之前不能取消")

	require.NoError(t, service.ProcessPendingSessions(session.PrepareDeadlineAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	require.Equal(t, liveModel.LiveSessionEndPrepareTimeout, session.EndReason)
	require.Equal(t, "准备开播超时", session.FailureReason)
	require.Zero(t, session.PrepareDeadlineAt)
	require.Zero(t, session.StartedAt)
	require.Zero(t, controller.callCount(), "准备超时没有真正开播，不能请求SRS断流")

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.RoomNo).First(&room).Error)
	require.Zero(t, room.CurrentSessionId)
	require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)
	require.NoError(t, global.GVA_DB.First(&anchor, anchor.ID).Error)
	require.Zero(t, anchor.TotalLiveCount, "准备超时不能计入完成直播次数")

	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.Error(t, err, "已取消场次的旧推流凭证不能再次开播")
	replacement := prepareRoomTestSession(t, &service, category.ID)
	require.NotEqual(t, prepared.SessionNo, replacement.SessionNo)
	require.NotEqual(t, prepared.Token, replacement.Token)
}

func TestPrepareRejectsTokenLifetimeShorterThanPrepareWindow(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	global.GVA_CONFIG.Live.PrepareTimeoutSeconds = 300
	global.GVA_CONFIG.Live.PushTokenSeconds = 120
	visibility := liveModel.LiveRoomVisibilityPublic
	_, err := service.PrepareSession(501, liveReq.LiveSessionPrepareReq{
		CategoryId: uint64(category.ID), Title: "配置错误测试", Visibility: &visibility,
	})
	require.ErrorIs(t, err, ErrLivePublishTokenConfigInvalid)
	var count int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&count).Error)
	require.Zero(t, count, "配置不一致时不能留下半成品场次")
}

func TestExpiredPreparingSessionRejectsPublishBeforeScannerRuns(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	prepared := prepareRoomTestSession(t, &service, category.ID)
	expiredAt := time.Now().Add(-time.Second).UnixMilli()
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("session_no = ?", prepared.SessionNo).Update("prepare_deadline_at", expiredAt).Error)

	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.ErrorIs(t, err, ErrLiveSessionStateInvalid)
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status, "拒绝回调不能绕过事务提交场次状态")
	require.NoError(t, service.ProcessPendingSessions(time.Now().UnixMilli()))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
}

func TestStaleSRSCallbacksCannotReplaceOrDisconnectCurrentPublisher(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	prepared := prepareRoomTestSession(t, &service, category.ID)
	require.NoError(t, func() error {
		_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
		return err
	}())
	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))

	reconnect := roomTestPublishRequest(prepared, "?pt="+prepared.Token)
	reconnect.ClientID = "client-2"
	reconnect.StreamID = "stream-2"
	_, err := service.OnPublish(reconnect)
	require.NoError(t, err)

	// 已被client-2接管后，client-1的延迟断流不能开启新的重连窗口。
	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, "client-2", session.SRSClientID)
	require.Equal(t, "stream-2", session.SRSStreamID)
	require.Zero(t, session.ReconnectDeadlineAt)
	require.Equal(t, uint32(1), session.DisconnectCount)

	// 直播仍活跃时，另一个发布连接不能通过迟到的on_publish覆盖当前连接身份。
	competing := reconnect
	competing.ClientID = "client-3"
	competing.StreamID = "stream-3"
	_, err = service.OnPublish(competing)
	require.ErrorIs(t, err, ErrLiveSessionStateInvalid)
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, "client-2", session.SRSClientID)
	require.Equal(t, "stream-2", session.SRSStreamID)

	currentUnpublish := roomTestUnpublishRequest(prepared, "client-2")
	currentUnpublish.StreamID = "stream-2"
	require.NoError(t, service.OnUnpublish(currentUnpublish))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	deadline := session.ReconnectDeadlineAt
	require.Positive(t, deadline)
	require.Equal(t, uint32(2), session.DisconnectCount)
	require.NoError(t, service.OnUnpublish(currentUnpublish))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, deadline, session.ReconnectDeadlineAt)
	require.Equal(t, uint32(2), session.DisconnectCount)
}

func TestConcurrentDuplicateUnpublishWritesOnce(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1"))
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, uint32(1), session.DisconnectCount)
	require.Positive(t, session.ReconnectDeadlineAt)
}

func TestPrepareTimeoutAndManyPublishCallbacksRemainConsistent(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	prepared := prepareRoomTestSession(t, &service, category.ID)
	deadline := time.Now().Add(time.Minute).UnixMilli()
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("session_no = ?", prepared.SessionNo).Update("prepare_deadline_at", deadline).Error)

	const publishers = 32
	start := make(chan struct{})
	errs := make(chan error, publishers)
	var wg sync.WaitGroup
	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, publishErr := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
			errs <- publishErr
		}()
	}
	scanDone := make(chan error, 1)
	go func() {
		<-start
		scanDone <- service.ProcessPendingSessions(deadline)
	}()
	close(start)
	wg.Wait()
	close(errs)
	require.NoError(t, <-scanDone)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.First(&room, session.RoomId).Error)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		require.True(t, errors.Is(err, ErrLiveSessionStateInvalid) || errors.Is(err, ErrLiveRoomUnavailable), err)
	}
	switch session.Status {
	case liveModel.LiveSessionLiving:
		require.Positive(t, successes)
		require.Equal(t, session.ID, room.CurrentSessionId)
		require.Equal(t, liveModel.LiveRoomLiving, room.LiveStatus)
		require.Zero(t, session.PrepareDeadlineAt)
	case liveModel.LiveSessionCancelled:
		require.Zero(t, successes)
		require.Zero(t, room.CurrentSessionId)
		require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)
	default:
		t.Fatalf("unexpected session status after race: %d", session.Status)
	}
}

func TestPrepareTimeoutScanHonorsBatchLimit(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndScanBatchSize = 25
	service := newRoomTestService()
	now := time.Now().UnixMilli()
	for i := 0; i < 31; i++ {
		room := liveModel.LiveRoom{
			RoomNo: fmt.Sprintf("batch-room-%03d", i), AnchorId: uint(1000 + i),
			StreamName: fmt.Sprintf("batch-stream-%03d", i), Status: liveModel.LiveRoomStatusNormal,
			LiveStatus: liveModel.LiveRoomPreparing, Visibility: liveModel.LiveRoomVisibilityPublic,
		}
		require.NoError(t, global.GVA_DB.Create(&room).Error)
		session := liveModel.LiveSession{
			SessionNo: fmt.Sprintf("batch-session-%03d", i), RoomId: room.ID, AnchorId: room.AnchorId,
			Status: liveModel.LiveSessionPreparing, PrepareDeadlineAt: now - 1,
		}
		require.NoError(t, global.GVA_DB.Create(&session).Error)
		require.NoError(t, global.GVA_DB.Model(&room).Update("current_session_id", session.ID).Error)
	}

	require.NoError(t, service.ProcessPendingSessions(now))
	var cancelled, preparing int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Where("status = ?", liveModel.LiveSessionCancelled).Count(&cancelled).Error)
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Where("status = ?", liveModel.LiveSessionPreparing).Count(&preparing).Error)
	require.Equal(t, int64(25), cancelled)
	require.Equal(t, int64(6), preparing)
	require.NoError(t, service.ProcessPendingSessions(now))
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Where("status = ?", liveModel.LiveSessionCancelled).Count(&cancelled).Error)
	require.Equal(t, int64(31), cancelled)
}

func TestOwnerEndRetriesSRSFailureAndIgnoresUnpublishForEndingSession(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{errors: []error{errors.New("SRS timeout"), nil}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	require.NoError(t, service.RequestOwnerEnd(anchor.UserId))
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, liveModel.LiveSessionEnding, session.Status)
	require.Equal(t, uint32(1), session.StopAttempts)
	require.Contains(t, session.StopLastError, "SRS timeout")
	require.Greater(t, session.StopNextRetryAt, session.StopRequestedAt)
	require.Equal(t, 1, controller.callCount())

	// 用户明确要求on_unpublish不负责主动结束：结束中回调只幂等返回，不结算也不安排重连。
	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnding, session.Status)
	require.Zero(t, session.ReconnectDeadlineAt)

	require.NoError(t, service.ProcessPendingSessions(session.StopNextRetryAt-1))
	require.Equal(t, 1, controller.callCount(), "未到重试时间不能调用SRS")
	require.NoError(t, service.ProcessPendingSessions(session.StopNextRetryAt))
	require.Equal(t, 2, controller.callCount())
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnded, session.Status)
	require.Empty(t, session.StopLastError)
	require.Zero(t, session.StopNextRetryAt)

	require.NoError(t, global.GVA_DB.First(&anchor, anchor.ID).Error)
	require.Equal(t, uint64(1), anchor.TotalLiveCount, "重试成功只能结算一次")
}

func TestOwnerEndAndTimerUseLeaseToAvoidDuplicateSRSCalls(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{started: make(chan struct{}), release: make(chan struct{})}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	requestDone := make(chan error, 1)
	go func() {
		requestDone <- service.RequestOwnerEnd(anchor.UserId)
	}()
	select {
	case <-controller.started:
	case <-time.After(2 * time.Second):
		t.Fatal("主动结束没有调用SRS")
	}

	// 人工请求已经认领处理权且正在等待SRS；同一时刻扫描只能看到未到期租约并跳过。
	require.NoError(t, service.ProcessPendingSessions(time.Now().UnixMilli()))
	require.Equal(t, 1, controller.callCount())
	close(controller.release)
	select {
	case requestErr := <-requestDone:
		require.NoError(t, requestErr)
	case <-time.After(2 * time.Second):
		t.Fatal("主动结束未返回")
	}

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, liveModel.LiveSessionEnded, session.Status)
	require.NoError(t, service.ProcessPendingSessions(time.Now().Add(time.Minute).UnixMilli()))
	require.Equal(t, 1, controller.callCount())
	require.NoError(t, global.GVA_DB.First(&anchor, anchor.ID).Error)
	require.Equal(t, uint64(1), anchor.TotalLiveCount)
}

func TestLiveSessionNoUsesTimeAnchorRoomAndAvoidsCollision(t *testing.T) {
	setupRoomTestDB(t)
	anchor, _ := createRoomTestAnchor(t)
	room, err := ensureRoomForAnchor(global.GVA_DB, anchor)
	require.NoError(t, err)

	fixedTime := time.Date(2029, time.August, 6, 13, 56, 12, 345_000_000, time.FixedZone("MYT", 8*60*60))
	first, err := nextLiveSessionNo(global.GVA_DB, fixedTime, anchor.AnchorNo, room.RoomNo)
	require.NoError(t, err)
	require.Equal(t, "20290806135612345-1500001-1500001", first)

	require.NoError(t, global.GVA_DB.Create(&liveModel.LiveSession{
		SessionNo: first,
		RoomId:    room.ID,
		AnchorId:  anchor.ID,
		Status:    liveModel.LiveSessionEnded,
	}).Error)
	second, err := nextLiveSessionNo(global.GVA_DB, fixedTime, anchor.AnchorNo, room.RoomNo)
	require.NoError(t, err)
	require.Equal(t, "20290806135612346-1500001-1500001", second,
		"相同毫秒、主播号和房间号碰撞时应顺延逻辑毫秒")
}

func TestPublicRoomDetailIncludesOfflineRoomButDiscoveryListDoesNot(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	room, err := ensureRoomForAnchor(global.GVA_DB, anchor)
	require.NoError(t, err)
	service := newRoomTestService()

	queryCount := 0
	const queryCounterCallback = "test:count_public_room_detail_queries"
	require.NoError(t, global.GVA_DB.Callback().Row().Before("gorm:row").Register(queryCounterCallback, func(*gorm.DB) {
		queryCount++
	}))
	detail, err := service.GetPublicRoom(room.RoomNo)
	require.NoError(t, global.GVA_DB.Callback().Row().Remove(queryCounterCallback))
	require.NoError(t, err)
	require.Equal(t, 1, queryCount, "公开直播间详情应只执行一次查询")
	require.Equal(t, room.RoomNo, detail.RoomNo)
	require.Equal(t, liveModel.LiveRoomOffline, detail.LiveStatus)
	require.Equal(t, anchor.AnchorNo, detail.AnchorInfo.AnchorNo)
	require.Equal(t, anchor.Nickname, detail.AnchorInfo.Nickname)
	require.Empty(t, detail.LatestSession.SessionNo)
	detailJSON, err := json.Marshal(detail)
	require.NoError(t, err)
	var detailObject map[string]interface{}
	require.NoError(t, json.Unmarshal(detailJSON, &detailObject))
	require.Contains(t, detailObject, "anchorInfo")
	require.Contains(t, detailObject, "latestSession")
	require.Equal(t, map[string]interface{}{}, detailObject["latestSession"], "从未完成直播时应返回空对象")
	for _, internalOrDuplicate := range []string{
		"currentSession", "sessionNo", "statusReason", "statusChangedAt", "visibility", "extra", "liveStartedAt",
	} {
		require.NotContains(t, detailObject, internalOrDuplicate)
	}

	listReq := liveReq.LiveRoomPublicListReq{}
	listReq.Page, listReq.PageSize = 1, 20
	list, total, err := service.GetPublicRoomList(listReq)
	require.NoError(t, err)
	require.Empty(t, list)
	require.Zero(t, total, "公开直播列表仍然只展示直播中的房间")

	prepared := prepareRoomTestSession(t, &service, category.ID)
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("session_no = ?", prepared.SessionNo).
		Updates(map[string]interface{}{
			"publish_ip":  "2001:db8::1",
			"stream_info": datatypes.JSON([]byte(`{"videoCodec":"h264","width":1920,"height":1080}`)),
		}).Error)
	detail, err = service.GetPublicRoom(room.RoomNo)
	require.NoError(t, err)
	require.Equal(t, liveModel.LiveRoomPreparing, detail.LiveStatus)
	require.Equal(t, prepared.SessionNo, detail.LatestSession.SessionNo)
	require.Equal(t, liveModel.LiveSessionPreparing, detail.LatestSession.Status)
	require.JSONEq(t, `{"videoCodec":"h264","width":1920,"height":1080}`, string(detail.LatestSession.StreamInfo))
	detailJSON, err = json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(detailJSON), "publishIp")
	require.Contains(t, string(detailJSON), `"streamInfo":{"videoCodec":"h264"`)

	current, err := service.GetOwnerCurrentSession(anchor.UserId)
	require.NoError(t, err)
	require.NotNil(t, current)
	currentJSON, err := json.Marshal(current)
	require.NoError(t, err)
	require.NotContains(t, string(currentJSON), "publishIp")
	require.Contains(t, string(currentJSON), `"streamInfo":{"videoCodec":"h264"`)

	var storedSession liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&storedSession).Error)
	adminDetail, err := service.GetAdminSessionDetail(storedSession.ID)
	require.NoError(t, err)
	require.Equal(t, "2001:db8::1", adminDetail.PublishIP)
	require.JSONEq(t, string(detail.LatestSession.StreamInfo), string(adminDetail.StreamInfo))
	adminListReq := liveReq.LiveSessionAdminListReq{}
	adminListReq.Page, adminListReq.PageSize = 1, 20
	adminList, adminTotal, err := service.GetAdminSessionList(adminListReq)
	require.NoError(t, err)
	require.Equal(t, int64(1), adminTotal)
	require.Len(t, adminList, 1)
	require.Equal(t, "2001:db8::1", adminList[0].PublishIP)
	require.JSONEq(t, string(detail.LatestSession.StreamInfo), string(adminList[0].StreamInfo))
	list, total, err = service.GetPublicRoomList(listReq)
	require.NoError(t, err)
	require.Empty(t, list, "准备中尚未开播的房间不应进入公开直播列表")
	require.Zero(t, total)

	_, err = service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	detail, err = service.GetPublicRoom(room.RoomNo)
	require.NoError(t, err)
	require.Equal(t, liveModel.LiveRoomLiving, detail.LiveStatus)
	require.Equal(t, prepared.SessionNo, detail.LatestSession.SessionNo)
	require.Equal(t, liveModel.LiveSessionLiving, detail.LatestSession.Status)
	list, total, err = service.GetPublicRoomList(listReq)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, int64(1), total)
	listJSON, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(listJSON), "streamInfo", "公开发现列表保持轻量，不携带流信息JSON")
	require.NotContains(t, string(listJSON), "publishIp")

	require.NoError(t, service.UpdateSessionStats(liveReq.LiveSessionStatsReq{
		SessionNo: prepared.SessionNo, ViewCount: 88, ViewerCount: 50, PeakOnlineCount: 23,
		LikeCount: 120, GiftCount: 9, GiftCoinAmount: 660, GiftUserCount: 4,
	}))
	require.NoError(t, service.RequestOwnerEnd(anchor.UserId))
	require.NoError(t, service.ProcessPendingSessions(time.Now().UnixMilli()+1))
	queryCount = 0
	require.NoError(t, global.GVA_DB.Callback().Row().Before("gorm:row").Register(queryCounterCallback, func(*gorm.DB) {
		queryCount++
	}))
	detail, err = service.GetPublicRoom(room.RoomNo)
	require.NoError(t, global.GVA_DB.Callback().Row().Remove(queryCounterCallback))
	require.NoError(t, err)
	require.Equal(t, 1, queryCount, "读取最近一次已结束直播也应只执行一次查询")
	require.Equal(t, liveModel.LiveRoomOffline, detail.LiveStatus)
	require.Equal(t, prepared.SessionNo, detail.LatestSession.SessionNo)
	require.Equal(t, liveModel.LiveSessionEnded, detail.LatestSession.Status)
	require.Equal(t, uint64(88), detail.LatestSession.ViewCount)
	require.Equal(t, uint64(120), detail.LatestSession.LikeCount)
	detailJSON, err = json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(detailJSON), "giftCoinAmount")
	require.NotContains(t, string(detailJSON), "reconnectDeadlineAt")
	require.NotContains(t, string(detailJSON), "failureReason")

	prepareRoomTestSession(t, &service, category.ID)
	require.NoError(t, service.RequestOwnerEnd(anchor.UserId))
	require.NoError(t, service.RequestOwnerEnd(anchor.UserId))
	detail, err = service.GetPublicRoom(room.RoomNo)
	require.NoError(t, err)
	require.Equal(t, prepared.SessionNo, detail.LatestSession.SessionNo,
		"准备后取消的场次不能覆盖最近一次真正完成的直播")
}

func TestAdminRoomDetailIncludesCurrentSessionSnapshot(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	prepared := prepareRoomTestSession(t, &service, category.ID)

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.RoomNo).First(&room).Error)
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("session_no = ?", prepared.SessionNo).
		Updates(map[string]interface{}{
			"stream_info":       datatypes.JSON([]byte(`{"videoCodec":"h264","width":1280,"height":720}`)),
			"view_count":        uint64(18),
			"viewer_count":      uint64(12),
			"peak_online_count": uint32(7),
			"like_count":        uint64(31),
		}).Error)

	detail, err := service.GetAdminRoomDetail(room.ID)
	require.NoError(t, err)
	require.Equal(t, room.CurrentSessionId, detail.CurrentSessionID)
	require.NotNil(t, detail.CurrentSession)
	require.Equal(t, room.CurrentSessionId, detail.CurrentSession.ID)
	require.Equal(t, prepared.SessionNo, detail.CurrentSession.SessionNo)
	require.Equal(t, anchor.ID, detail.CurrentSession.AnchorID)
	require.Equal(t, room.ID, detail.CurrentSession.RoomID)
	require.Equal(t, uint64(18), detail.CurrentSession.ViewCount)
	require.Equal(t, uint64(12), detail.CurrentSession.ViewerCount)
	require.Equal(t, uint32(7), detail.CurrentSession.PeakOnlineCount)
	require.Equal(t, uint64(31), detail.CurrentSession.LikeCount)
	require.JSONEq(t, `{"videoCodec":"h264","width":1280,"height":720}`, string(detail.CurrentSession.StreamInfo))
	detailJSON, err := json.Marshal(detail)
	require.NoError(t, err)
	var detailObject map[string]interface{}
	require.NoError(t, json.Unmarshal(detailJSON, &detailObject))
	require.Contains(t, detailObject, "currentSession")

	listReq := liveReq.LiveRoomAdminListReq{}
	listReq.Page, listReq.PageSize = 1, 20
	list, total, err := service.GetAdminRoomList(listReq)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, list, 1)
	listJSON, err := json.Marshal(list[0])
	require.NoError(t, err)
	var listObject map[string]interface{}
	require.NoError(t, json.Unmarshal(listJSON, &listObject))
	require.NotContains(t, listObject, "currentSession", "列表元素不应包含场次快照字段")
}

func TestPublicStreamInfoDefaultsToObject(t *testing.T) {
	require.JSONEq(t, `{}`, string(publicStreamInfo(nil)))
	require.JSONEq(t, `{}`, string(publicStreamInfo([]byte(`null`))))
	require.JSONEq(t, `{}`, string(publicStreamInfo([]byte(`[]`))))
	require.JSONEq(t, `{}`, string(publicStreamInfo([]byte(`"raw-srs-value"`))))
	require.JSONEq(t, `{"width":1920}`, string(publicStreamInfo([]byte(`{"width":1920}`))))
}

func TestLiveSessionActiveUniqueAndClientIDs(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
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
	service := newRoomTestService()
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
	service := newRoomTestService()
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
	roomService := newRoomTestService()
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
