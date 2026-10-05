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
	liveRes "tb_live_module/model/live/response"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type fakeSRSController struct {
	mu          sync.Mutex
	calls       int
	listCalls   int
	errors      []error
	listError   error
	snapshots   []srsclient.PublisherSnapshot
	listDelay   time.Duration
	listStarted chan struct{}
	listRelease chan struct{}
	started     chan struct{}
	release     chan struct{}
	stopBlock   chan struct{}
	stopDelay   time.Duration
	activeStops int
	maxStops    int
}

type fakeBatchSRSController struct {
	*fakeSRSController
	muBatch    sync.Mutex
	batchCalls int
	batchRefs  int
	batchErrs  []error
}

type runtimeFakeSRSController struct {
	*fakeSRSController
	runtimeMu sync.Mutex
	runtime   srsclient.RuntimeIdentity
}

func (f *runtimeFakeSRSController) CurrentRuntime(context.Context) (srsclient.RuntimeIdentity, error) {
	f.runtimeMu.Lock()
	defer f.runtimeMu.Unlock()
	return f.runtime, nil
}

func (f *runtimeFakeSRSController) ListPublishersWithRuntime(
	ctx context.Context,
) (srsclient.PublisherBatch, error) {
	publishers, err := f.ListPublishers(ctx)
	if err != nil {
		return srsclient.PublisherBatch{}, err
	}
	identity, err := f.CurrentRuntime(ctx)
	return srsclient.PublisherBatch{Runtime: identity, Publishers: publishers}, err
}

func (f *runtimeFakeSRSController) setRuntime(serverID string) {
	f.runtimeMu.Lock()
	defer f.runtimeMu.Unlock()
	f.runtime = srsclient.RuntimeIdentity{ServerID: serverID, ServiceID: "service-1"}
}

func (f *fakeBatchSRSController) StopPublishers(
	_ context.Context,
	refs []srsclient.StreamRef,
	_ int,
) []error {
	f.muBatch.Lock()
	defer f.muBatch.Unlock()
	f.batchCalls++
	f.batchRefs += len(refs)
	results := make([]error, len(refs))
	copy(results, f.batchErrs)
	return results
}

func (f *fakeBatchSRSController) batchCounts() (int, int) {
	f.muBatch.Lock()
	defer f.muBatch.Unlock()
	return f.batchCalls, f.batchRefs
}

type memoryLivePrepareResultCache struct {
	mu     sync.Mutex
	values map[string]memoryLivePrepareResult
	getErr error
	setErr error
}

type memoryLivePrepareResult struct {
	value     string
	expiresAt time.Time
}

func newMemoryLivePrepareResultCache() *memoryLivePrepareResultCache {
	return &memoryLivePrepareResultCache{values: make(map[string]memoryLivePrepareResult)}
}

func (c *memoryLivePrepareResultCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return "", c.getErr
	}
	value, ok := c.values[key]
	if !ok || time.Now().After(value.expiresAt) {
		return "", errLivePrepareResultCacheMiss
	}
	return value.value, nil
}

func (c *memoryLivePrepareResultCache) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	c.values[key] = memoryLivePrepareResult{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (c *memoryLivePrepareResultCache) value(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[key].value
}

func (f *fakeSRSController) ListPublishers(ctx context.Context) ([]srsclient.PublisherSnapshot, error) {
	f.mu.Lock()
	f.listCalls++
	call := f.listCalls
	snapshots := append([]srsclient.PublisherSnapshot(nil), f.snapshots...)
	err := f.listError
	delay := f.listDelay
	started, release := f.listStarted, f.listRelease
	f.mu.Unlock()
	if started != nil && call == 1 {
		close(started)
	}
	if release != nil && call == 1 {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return snapshots, err
}

func (f *fakeSRSController) StopPublisher(ctx context.Context, _ srsclient.StreamRef) error {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.activeStops++
	if f.activeStops > f.maxStops {
		f.maxStops = f.activeStops
	}
	started, release, stopBlock, stopDelay := f.started, f.release, f.stopBlock, f.stopDelay
	var err error
	if call <= len(f.errors) {
		err = f.errors[call-1]
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.activeStops--
		f.mu.Unlock()
	}()
	if started != nil && call == 1 {
		close(started)
	}
	if stopBlock != nil {
		select {
		case <-stopBlock:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if release != nil && call == 1 {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if stopDelay > 0 {
		timer := time.NewTimer(stopDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (f *fakeSRSController) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeSRSController) maxStopConcurrency() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxStops
}

func (f *fakeSRSController) listCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func (f *fakeSRSController) setSnapshots(snapshots []srsclient.PublisherSnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots = append([]srsclient.PublisherSnapshot(nil), snapshots...)
}

func (f *fakeSRSController) setListError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listError = err
}

func newRoomTestService() RoomService {
	return RoomService{srsController: &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()}}}
}

func roomTestPublisherSnapshot() srsclient.PublisherSnapshot {
	return srsclient.PublisherSnapshot{
		StreamID: "stream-1", Vhost: "127.0.0.1", App: "live", Stream: "1500001", Active: true,
		VideoCodec: "H264", VideoProfile: "High", VideoLevel: "4.1", Width: 1920, Height: 1080,
		AudioCodec: "AAC", AudioProfile: "LC", AudioSampleRate: 48000, AudioChannels: 2,
	}
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
		&liveModel.LiveSRSRuntime{}, &liveModel.LiveSessionMediaAudit{},
	))
	require.NoError(t, db.Create(&liveModel.LiveSRSRuntime{ID: liveSRSRuntimeSingletonID}).Error)
	previousDB, previousLive := global.GVA_DB, global.GVA_CONFIG.Live
	global.GVA_DB = db
	global.GVA_CONFIG.Live = config.Live{
		MaxConcurrentStreams:            100,
		ReconnectWindowSeconds:          1,
		PrepareTimeoutSeconds:           120,
		StreamReadyTimeoutSeconds:       15,
		StreamReadyScanIntervalSeconds:  1,
		StreamReadyRetryBaseSeconds:     1,
		StreamReadyRetryMaxSeconds:      4,
		StreamReconcileIntervalSeconds:  5,
		StreamReconcileMissingThreshold: 3,
		StateTransitionConcurrency:      1,
		EndStopConcurrency:              8,
		PushTokenSeconds:                180,
		PublishTokenKey:                 "publish-token-secret",
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
	prepared, err := service.PrepareSession(501, roomTestPrepareRequest(categoryID))
	require.NoError(t, err)
	return liveResFixture{
		RoomNo: prepared.RoomNo, SessionNo: prepared.SessionNo, StreamName: prepared.StreamName,
		Token: prepared.PublishToken, PushURL: prepared.PushURL, PlayURL: prepared.PlayURL,
		PrepareDeadlineAt: prepared.PrepareDeadlineAt,
	}
}

func roomTestPrepareRequest(categoryID uint) liveReq.LiveSessionPrepareReq {
	visibility := liveModel.LiveRoomVisibilityPublic
	return liveReq.LiveSessionPrepareReq{
		CategoryId: uint64(categoryID), Title: "第一场直播", Visibility: &visibility,
	}
}

// blockNextQueryForTable 暂停指定表的下一次查询。SQLite 测试库使用单连接，
// 因而可稳定复现“事务已持有业务锁、另一个请求正在等待”的并发提交顺序。
func blockNextQueryForTable(t *testing.T, table string) (<-chan struct{}, func()) {
	t.Helper()
	started := make(chan struct{})
	release := make(chan struct{})
	var blockOnce sync.Once
	var releaseOnce sync.Once
	callbackName := fmt.Sprintf("test:block_next_%s_query:%s", table, t.Name())
	require.NoError(t, global.GVA_DB.Callback().Query().After("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Table != table {
			return
		}
		blockOnce.Do(func() {
			close(started)
			<-release
		})
	}))
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() {
		unblock()
		require.NoError(t, global.GVA_DB.Callback().Query().Remove(callbackName))
	})
	return started, unblock
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

func promoteReadyPublisher(t *testing.T, service *RoomService, sessionNo string) liveModel.LiveSession {
	t.Helper()
	require.NoError(t, service.ProcessStreamReadiness(time.Now().UnixMilli()))
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", sessionNo).First(&session).Error)
	require.Equal(t, liveModel.LiveSessionLiving, session.Status)
	require.Positive(t, session.StartedAt)
	var streamInfo map[string]interface{}
	require.NoError(t, json.Unmarshal(session.StreamInfo, &streamInfo))
	require.Equal(t, float64(1920), streamInfo["width"])
	require.Equal(t, float64(1080), streamInfo["height"])
	return session
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
	require.Equal(t, liveModel.LiveSessionPreparing, started.Status)
	require.JSONEq(t, `{}`, string(started.StreamInfo))
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, "192.0.2.10", session.PublishIP)
	require.Equal(t, "srs-1", session.SRSServerID)
	require.Equal(t, "stream-1", session.SRSStreamID)
	require.Equal(t, "client-1", session.SRSClientID)
	require.Equal(t, "127.0.0.1", session.SRSVhost)
	require.Equal(t, "live", session.SRSApp)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
	require.Zero(t, session.StartedAt)
	session = promoteReadyPublisher(t, &service, prepared.SessionNo)
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
	require.NoError(t, service.ProcessEndingSessions(deadline+1))
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

func TestPrepareIdempotencyReplaysSamePushURLAfterResponseLoss(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	cache := newMemoryLivePrepareResultCache()
	service := newRoomTestService()
	service.prepareResultCache = cache
	req := roomTestPrepareRequest(category.ID)
	req.IdempotencyKey = "prepare-response-loss-0001"

	first, err := service.PrepareSession(501, req)
	require.NoError(t, err)
	// 模拟数据库已经提交、但第一次HTTP响应在客户端收到前丢失。
	second, err := service.PrepareSession(501, req)
	require.NoError(t, err)
	require.Equal(t, first, second)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", first.SessionNo).First(&session).Error)
	require.Equal(t, uint32(1), session.PublishCredentialIssueCount)
	var sessionCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)

	cacheKey := livePrepareCacheKey(global.GVA_CONFIG.Live.PublishTokenKey, 501, req.IdempotencyKey)
	ciphertext := cache.value(cacheKey)
	require.NotEmpty(t, ciphertext)
	require.NotContains(t, ciphertext, first.PublishToken, "Redis只能保存加密后的prepare结果")
}

func TestPrepareIdempotencyReplaySurvivesLaterCategoryDisable(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	cache := newMemoryLivePrepareResultCache()
	service := newRoomTestService()
	service.prepareResultCache = cache
	req := roomTestPrepareRequest(category.ID)
	req.IdempotencyKey = "prepare-category-replay-0001"

	first, err := service.PrepareSession(501, req)
	require.NoError(t, err)
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveCategory{}).Where("id = ?", category.ID).
		Update("status", liveModel.LiveCategoryStatusDisabled).Error)
	replayed, err := service.PrepareSession(501, req)
	require.NoError(t, err)
	require.Equal(t, first, replayed, "分类后续停用不能改变已经提交成功的同一幂等操作结果")
}

func TestConcurrentPrepareWithSameIdempotencyKeyReturnsOneResult(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	cache := newMemoryLivePrepareResultCache()
	service := newRoomTestService()
	service.prepareResultCache = cache
	req := roomTestPrepareRequest(category.ID)
	req.IdempotencyKey = "concurrent-prepare-0001"

	start := make(chan struct{})
	results := make(chan liveRes.LiveSessionPrepareResp, 2)
	errorsCh := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.PrepareSession(501, req)
			results <- result
			errorsCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}
	var first *liveRes.LiveSessionPrepareResp
	for result := range results {
		if first == nil {
			copy := result
			first = &copy
			continue
		}
		require.Equal(t, *first, result)
	}
	var count int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestRefreshPushURLRotatesOldCredentialAndIsIdempotent(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	cache := newMemoryLivePrepareResultCache()
	service := newRoomTestService()
	service.prepareResultCache = cache

	prepared := prepareRoomTestSession(t, &service, category.ID)
	refreshed, err := service.RefreshOwnerPushURL(501, "refresh-push-url-0001")
	require.NoError(t, err)
	require.Equal(t, prepared.SessionNo, refreshed.SessionNo)
	require.Equal(t, prepared.PrepareDeadlineAt, refreshed.PrepareDeadlineAt)
	require.NotEqual(t, prepared.Token, refreshed.PublishToken)
	require.NotEqual(t, prepared.PushURL, refreshed.PushURL)

	replayed, err := service.RefreshOwnerPushURL(501, "refresh-push-url-0001")
	require.NoError(t, err)
	require.Equal(t, refreshed, replayed)
	newer, err := service.RefreshOwnerPushURL(501, "refresh-push-url-0002")
	require.NoError(t, err)
	require.NotEqual(t, refreshed.PublishToken, newer.PublishToken)
	_, err = service.RefreshOwnerPushURL(501, "refresh-push-url-0001")
	require.ErrorIs(t, err, ErrLiveIdempotencyConflict, "旧幂等键不能在后续轮换后产生第三个不同结果")
	refreshed = newer

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.RoomNo).First(&room).Error)
	require.Equal(t, uint32(3), room.StreamKeyVersion)
	require.Equal(t, secretHash(refreshed.PublishToken), room.PublishSecretHash)
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, uint32(3), session.PublishCredentialIssueCount)

	_, err = service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.ErrorIs(t, err, ErrLivePublishTokenInvalid)
	refreshedFixture := prepared
	refreshedFixture.Token = refreshed.PublishToken
	refreshedFixture.PushURL = refreshed.PushURL
	_, err = service.OnPublish(roomTestPublishRequest(refreshedFixture, "?pt="+refreshed.PublishToken))
	require.NoError(t, err)
	_, err = service.RefreshOwnerPushURL(501, "refresh-after-publish-0002")
	require.ErrorIs(t, err, ErrLiveSessionStateInvalid)
}

func TestRefreshPushURLLimitAndCacheReplay(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.PushURLRefreshLimit = 1
	_, category := createRoomTestAnchor(t)
	cache := newMemoryLivePrepareResultCache()
	service := newRoomTestService()
	service.prepareResultCache = cache
	prepareRoomTestSession(t, &service, category.ID)

	firstRefresh, err := service.RefreshOwnerPushURL(501, "refresh-limit-first-0001")
	require.NoError(t, err)
	replayed, err := service.RefreshOwnerPushURL(501, "refresh-limit-first-0001")
	require.NoError(t, err)
	require.Equal(t, firstRefresh, replayed, "缓存重放不能消耗签发次数")
	_, err = service.RefreshOwnerPushURL(501, "refresh-limit-second-0002")
	require.ErrorIs(t, err, ErrLivePushURLRefreshLimit)
}

func TestPrepareIdempotencyCacheFailureRollsBackSession(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	cache := newMemoryLivePrepareResultCache()
	cache.setErr = errors.New("redis unavailable")
	service := newRoomTestService()
	service.prepareResultCache = cache
	req := roomTestPrepareRequest(category.ID)
	req.IdempotencyKey = "prepare-cache-failure-0001"

	_, err := service.PrepareSession(501, req)
	require.ErrorIs(t, err, ErrLiveIdempotencyUnavailable)
	var sessionCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&sessionCount).Error)
	require.Zero(t, sessionCount)
	var roomCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveRoom{}).Count(&roomCount).Error)
	require.Zero(t, roomCount)
}

func TestPrepareIdempotencyKeyValidationAndCacheAAD(t *testing.T) {
	_, err := normalizeLiveIdempotencyKey("short", true)
	require.ErrorIs(t, err, ErrLiveIdempotencyKeyInvalid)
	_, err = normalizeLiveIdempotencyKey("contains whitespace", true)
	require.ErrorIs(t, err, ErrLiveIdempotencyKeyInvalid)
	key, err := normalizeLiveIdempotencyKey("valid-key_0001", true)
	require.NoError(t, err)
	require.Equal(t, "valid-key_0001", key)

	result := liveRes.LiveSessionPrepareResp{
		RoomNo: "room-1", SessionNo: "session-1", StreamName: "stream-1",
		PrepareDeadlineAt: time.Now().Add(time.Minute).UnixMilli(),
		PublishToken:      "v1_secret", PushURL: "rtmp://example/live/stream-1?pt=v1_secret",
	}
	encrypted, err := encryptLivePrepareResult("server-secret", "cache-key-1", result)
	require.NoError(t, err)
	require.NotContains(t, encrypted, result.PublishToken)
	decrypted, err := decryptLivePrepareResult("server-secret", "cache-key-1", encrypted)
	require.NoError(t, err)
	require.Equal(t, result, decrypted)
	_, err = decryptLivePrepareResult("server-secret", "cache-key-2", encrypted)
	require.Error(t, err, "Redis密文必须和缓存键通过GCM AAD绑定")
}

func TestStreamReadinessWaitsForBothDimensionsBeforeGoingLive(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{{
		StreamID: "stream-1", Vhost: "127.0.0.1", App: "live", Stream: "1500001", Active: true,
		VideoCodec: "H264", Width: 1920, Height: 0,
	}}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)

	response, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	require.Equal(t, liveModel.LiveSessionPreparing, response.Status)
	require.JSONEq(t, `{}`, string(response.StreamInfo))

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	firstProbeAt := session.StreamProbeNextAt + 100
	require.NoError(t, service.ProcessStreamReadiness(firstProbeAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
	require.Zero(t, session.StartedAt)
	require.Equal(t, uint32(1), session.StreamProbeAttempts)
	require.Equal(t, firstProbeAt+1000, session.StreamProbeNextAt)
	require.Contains(t, session.StreamProbeLastError, "width=1920 height=0")
	require.Equal(t, 1, controller.listCallCount())

	// 退避时间未到时只能查数据库，不能再次请求SRS。
	require.NoError(t, service.ProcessStreamReadiness(session.StreamProbeNextAt-1))
	require.Equal(t, 1, controller.listCallCount())

	controller.setSnapshots([]srsclient.PublisherSnapshot{roomTestPublisherSnapshot()})
	require.NoError(t, service.ProcessStreamReadiness(session.StreamProbeNextAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionLiving, session.Status)
	require.Positive(t, session.StartedAt)
	require.Zero(t, session.StreamReadyDeadlineAt)
	require.Zero(t, session.StreamProbeAttempts)
	require.Zero(t, session.StreamProbeNextAt)
	require.Empty(t, session.StreamProbeLastError)
	require.JSONEq(t, `{"videoCodec":"H264","videoProfile":"High","videoLevel":"4.1","width":1920,"height":1080,"audioCodec":"AAC","audioProfile":"LC","audioSampleRate":48000,"audioChannels":2,"collectedAt":`+
		fmt.Sprint(session.StartedAt)+`}`, string(session.StreamInfo))

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.First(&room, session.RoomId).Error)
	require.Equal(t, liveModel.LiveRoomLiving, room.LiveStatus)
	require.Equal(t, session.StartedAt, room.LiveStartedAt)
}

func TestPublishWithoutSRSStreamIDIsRejectedSafely(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	prepared := prepareRoomTestSession(t, &service, category.ID)
	req := roomTestPublishRequest(prepared, "?pt="+prepared.Token)
	req.StreamID = ""

	_, err := service.OnPublish(req)
	require.ErrorIs(t, err, ErrLiveSessionStateInvalid)
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
	require.Empty(t, session.SRSStreamID)
	require.Zero(t, session.StreamReadyDeadlineAt)
	require.Zero(t, session.StartedAt)
}

func TestStreamReadinessExponentialBackoffAndHardTimeout(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{{
		StreamID: "stream-1", Vhost: "127.0.0.1", App: "live", Stream: "1500001", Active: true,
		VideoCodec: "H264", Width: 0, Height: 0,
	}}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	firstProbeAt, deadline := session.StreamProbeNextAt, session.StreamReadyDeadlineAt
	require.Equal(t, int64(15_000), deadline-firstProbeAt)

	firstProbeAt += 100
	expectedProbeTimes := []int64{firstProbeAt, firstProbeAt + 1000, firstProbeAt + 3000, firstProbeAt + 7000, firstProbeAt + 11000}
	expectedNextTimes := []int64{firstProbeAt + 1000, firstProbeAt + 3000, firstProbeAt + 7000, firstProbeAt + 11000, deadline}
	for index, probeAt := range expectedProbeTimes {
		require.NoError(t, service.ProcessStreamReadiness(probeAt))
		require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
		require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
		require.Equal(t, uint32(index+1), session.StreamProbeAttempts)
		require.Equal(t, expectedNextTimes[index], session.StreamProbeNextAt)
	}
	require.Equal(t, 5, controller.listCallCount(), "退避周期内每个到期点最多请求一次SRS")

	// 到达硬截止时间后不再请求列表，先可靠进入结束中；独立SRS收尾任务随后断流并取消。
	require.NoError(t, service.ProcessStreamReadiness(deadline))
	require.Equal(t, 5, controller.listCallCount())
	require.Equal(t, 0, controller.callCount(), "尺寸扫描不能同步调用SRS停止接口")
	require.NoError(t, service.ProcessEndingSessions(deadline))
	require.Equal(t, 1, controller.callCount())
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	require.Equal(t, liveModel.LiveSessionEndPrepareTimeout, session.EndReason)
	require.Contains(t, session.FailureReason, "15秒")
	require.Zero(t, session.StartedAt)
	require.Zero(t, session.DurationMs)

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.First(&room, session.RoomId).Error)
	require.Zero(t, room.CurrentSessionId)
	require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)
	require.NoError(t, global.GVA_DB.First(&anchor, anchor.ID).Error)
	require.Zero(t, anchor.TotalLiveCount, "没有取得有效尺寸的推流不能计入完成直播")
}

func TestStreamReadinessAPIFailureBacksOffWithoutFalseLive(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{listError: errors.New("SRS API timeout")}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	probeAt := session.StreamProbeNextAt + 100
	err = service.ProcessStreamReadiness(probeAt)
	require.ErrorContains(t, err, "SRS API timeout")
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
	require.Zero(t, session.StartedAt)
	require.Equal(t, uint32(1), session.StreamProbeAttempts)
	require.Contains(t, session.StreamProbeLastError, "SRS API timeout")
	require.Equal(t, probeAt+1000, session.StreamProbeNextAt)
}

func TestStreamReadinessRejectsSnapshotWithMismatchedIdentity(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	snapshot := roomTestPublisherSnapshot()
	snapshot.Stream = "another-stream"
	controller := &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{snapshot}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	probeAt := session.StreamProbeNextAt + 100
	require.NoError(t, service.ProcessStreamReadiness(probeAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
	require.Zero(t, session.StartedAt)
	require.Equal(t, uint32(1), session.StreamProbeAttempts)
	require.Contains(t, session.StreamProbeLastError, "身份与当前场次不匹配")
}

func TestStreamReadinessTimeoutUsesConfiguration(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.StreamReadyTimeoutSeconds = 2
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.Equal(t, int64(2000), session.StreamReadyDeadlineAt-session.StreamProbeNextAt)
	require.NoError(t, service.ProcessStreamReadiness(session.StreamReadyDeadlineAt))
	require.NoError(t, service.ProcessEndingSessions(session.StreamReadyDeadlineAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	require.Contains(t, session.FailureReason, "2秒")
}

func TestStreamReadinessRetryUsesConfiguredCap(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.StreamReadyRetryBaseSeconds = 2
	global.GVA_CONFIG.Live.StreamReadyRetryMaxSeconds = 3
	require.Equal(t, int64(2000), liveStreamProbeRetryMillis(1))
	require.Equal(t, int64(3000), liveStreamProbeRetryMillis(2))
	require.Equal(t, int64(3000), liveStreamProbeRetryMillis(20))
}

func TestStreamReadinessRequestCrossingDeadlineCannotPromote(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{
		snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
		listDelay: 250 * time.Millisecond,
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	probeAt := time.Now().UnixMilli()
	deadline := probeAt + 100
	require.NoError(t, global.GVA_DB.Model(&session).Updates(map[string]interface{}{
		"stream_ready_deadline_at": deadline,
		"stream_probe_next_at":     probeAt,
	}).Error)

	err = service.ProcessStreamReadiness(probeAt)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, controller.listCallCount())
	require.Equal(t, 0, controller.callCount(), "尺寸扫描只推进状态，不能被SRS停止请求继续阻塞")
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.NoError(t, service.ProcessEndingSessions(session.StopNextRetryAt))
	require.Equal(t, 1, controller.callCount(), "跨过硬截止时间后应停止发布端，不能再提升为直播中")
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	require.Zero(t, session.StartedAt)
}

func TestStreamReadinessTimeoutStopFailureRemainsEndingForRetry(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{
		errors: []error{errors.New("SRS stop timeout")},
		snapshots: []srsclient.PublisherSnapshot{{
			StreamID: "stream-1", Vhost: "127.0.0.1", App: "live", Stream: "1500001", Active: true,
		}},
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	require.NoError(t, service.ProcessStreamReadiness(session.StreamReadyDeadlineAt))
	err = service.ProcessEndingSessions(session.StreamReadyDeadlineAt)
	require.ErrorContains(t, err, "SRS stop timeout")
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnding, session.Status)
	require.Equal(t, liveModel.LiveSessionEndPrepareTimeout, session.EndReason)
	require.Zero(t, session.StartedAt)
	require.Equal(t, uint32(1), session.StopAttempts)
	require.Greater(t, session.StopNextRetryAt, session.StreamReadyDeadlineAt)

	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.First(&room, session.RoomId).Error)
	require.Equal(t, liveModel.LiveRoomEnding, room.LiveStatus)
	require.Equal(t, session.ID, room.CurrentSessionId, "停止结果未知时必须保留活动场次，不能允许下一场开播")
	require.NoError(t, global.GVA_DB.First(&anchor, anchor.ID).Error)
	require.Zero(t, anchor.TotalLiveCount)

	require.NoError(t, service.ProcessEndingSessions(session.StopNextRetryAt))
	require.Equal(t, 2, controller.callCount())
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	require.Zero(t, session.DurationMs)
}

func TestPreReadyUnpublishClearsProbeAndCannotBePromotedByStaleSnapshot(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)
	oldProbeAt := session.StreamProbeNextAt
	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
	require.Empty(t, session.SRSStreamID)
	require.Zero(t, session.StreamReadyDeadlineAt)
	require.Zero(t, session.StreamProbeNextAt)

	require.NoError(t, service.ProcessStreamReadiness(oldProbeAt+1000))
	require.Equal(t, 0, controller.listCallCount())
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionPreparing, session.Status)
	require.Zero(t, session.StartedAt)
}

func TestConcurrentStreamReadinessScansUseDatabaseLease(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{
		snapshots:   []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
		listStarted: make(chan struct{}), listRelease: make(chan struct{}),
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.SessionNo).First(&session).Error)

	firstDone := make(chan error, 1)
	go func() { firstDone <- service.ProcessStreamReadiness(session.StreamProbeNextAt) }()
	select {
	case <-controller.listStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("首次流信息查询未启动")
	}
	require.NoError(t, service.ProcessStreamReadiness(session.StreamProbeNextAt))
	require.Equal(t, 1, controller.listCallCount(), "同一场次的租约有效时不能重复查询SRS")
	close(controller.listRelease)
	require.NoError(t, <-firstDone)
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionLiving, session.Status)
}

func TestStreamReadinessBatchesManySessionsIntoOneSRSRequest(t *testing.T) {
	setupRoomTestDB(t)
	now := time.Now().UnixMilli()
	controller := &fakeSRSController{}
	service := RoomService{srsController: controller}
	const sessionCount = 12
	for i := 0; i < sessionCount; i++ {
		anchor := liveModel.LiveAnchor{
			UserId: uint64(10_000 + i), AnchorNo: fmt.Sprintf("ready-anchor-%02d", i),
			Nickname: "批量测试", ApplyStatus: liveModel.AnchorApplyStatusApproved,
			Status: liveModel.AnchorStatusNormal, LivePermission: liveModel.AnchorPermissionEnabled,
		}
		require.NoError(t, global.GVA_DB.Create(&anchor).Error)
		room := liveModel.LiveRoom{
			RoomNo: fmt.Sprintf("ready-room-%02d", i), AnchorId: anchor.ID,
			StreamName: fmt.Sprintf("ready-stream-%02d", i), Status: liveModel.LiveRoomStatusNormal,
			LiveStatus: liveModel.LiveRoomPreparing, Visibility: liveModel.LiveRoomVisibilityPublic,
		}
		require.NoError(t, global.GVA_DB.Create(&room).Error)
		streamID := fmt.Sprintf("ready-srs-%02d", i)
		session := liveModel.LiveSession{
			SessionNo: fmt.Sprintf("ready-session-%02d", i), RoomId: room.ID, AnchorId: anchor.ID,
			Status: liveModel.LiveSessionPreparing, PrepareDeadlineAt: now + 60_000,
			SRSServerID: "srs-1", SRSStreamID: streamID, SRSClientID: fmt.Sprintf("client-%02d", i),
			SRSVhost: "127.0.0.1", SRSApp: "live",
			StreamReadyDeadlineAt: now + 15_000, StreamProbeNextAt: now,
		}
		require.NoError(t, global.GVA_DB.Create(&session).Error)
		require.NoError(t, global.GVA_DB.Model(&room).Update("current_session_id", session.ID).Error)
		controller.snapshots = append(controller.snapshots, srsclient.PublisherSnapshot{
			StreamID: streamID, Vhost: "127.0.0.1", App: "live", Stream: room.StreamName, Active: true,
			VideoCodec: "H264", Width: 1280, Height: 720,
		})
	}

	require.NoError(t, service.ProcessStreamReadiness(now))
	require.Equal(t, 1, controller.listCallCount(), "同一轮必须复用一份SRS流列表，不能按直播间逐个请求")
	var living int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ?", liveModel.LiveSessionLiving).Count(&living).Error)
	require.Equal(t, int64(sessionCount), living)
}

func TestLiveStreamReconciliationRequiresConfiguredConsecutiveMisses(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.StreamReconcileIntervalSeconds = 2
	global.GVA_CONFIG.Live.StreamReconcileMissingThreshold = 3
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	session := promoteReadyPublisher(t, &service, prepared.SessionNo)
	controller.setSnapshots(nil)

	firstCheckAt := session.StreamReconcileCheckedAt + 2000
	require.NoError(t, service.ProcessLiveStreamReconciliation(firstCheckAt-1))
	require.Equal(t, 1, controller.listCallCount(), "未到配置间隔时不能请求SRS；首次调用来自开播宽高探测")

	require.NoError(t, service.ProcessLiveStreamReconciliation(firstCheckAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, uint32(1), session.StreamReconcileMissingCount)
	require.Zero(t, session.ReconnectDeadlineAt)

	secondCheckAt := firstCheckAt + 2000
	require.NoError(t, service.ProcessLiveStreamReconciliation(secondCheckAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, uint32(2), session.StreamReconcileMissingCount)
	require.Zero(t, session.ReconnectDeadlineAt)

	thirdCheckAt := secondCheckAt + 2000
	require.NoError(t, service.ProcessLiveStreamReconciliation(thirdCheckAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, uint32(3), session.StreamReconcileMissingCount)
	require.Equal(t, thirdCheckAt, session.LastUnpublishAt)
	require.Equal(t, thirdCheckAt+1000, session.ReconnectDeadlineAt)
	require.Equal(t, uint32(1), session.DisconnectCount)
	require.Equal(t, 4, controller.listCallCount(), "每个到期对账周期只能批量请求一次SRS")

	// 已进入重连窗口后由现有生命周期负责等待，不得继续累计缺失或延长截止时间。
	require.NoError(t, service.ProcessLiveStreamReconciliation(thirdCheckAt+2000))
	require.Equal(t, 4, controller.listCallCount())
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, thirdCheckAt+1000, session.ReconnectDeadlineAt)
	require.Equal(t, uint32(1), session.DisconnectCount)

	// SRS重启后publisher ID会变化；连续缺失已经打开重连窗口，因此新连接可以接管并重新验证视频尺寸。
	reconnectSnapshot := roomTestPublisherSnapshot()
	reconnectSnapshot.StreamID = "stream-2"
	controller.setSnapshots([]srsclient.PublisherSnapshot{reconnectSnapshot})
	reconnectReq := roomTestPublishRequest(prepared, "?pt="+prepared.Token)
	reconnectReq.ServerID = "srs-2"
	reconnectReq.StreamID = "stream-2"
	reconnectReq.ClientID = "client-2"
	reconnected, err := service.OnPublish(reconnectReq)
	require.NoError(t, err)
	require.Equal(t, liveModel.LiveSessionLiving, reconnected.Status)
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Zero(t, session.ReconnectDeadlineAt)
	require.Equal(t, "stream-2", session.SRSStreamID)
	require.Positive(t, session.StreamReadyDeadlineAt)
	require.NoError(t, service.ProcessStreamReadiness(session.StreamProbeNextAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionLiving, session.Status)
	require.Zero(t, session.StreamReadyDeadlineAt)
	require.Zero(t, session.StreamReconcileMissingCount)
	require.Equal(t, "stream-2", session.SRSStreamID)
}

func TestLiveStreamReconciliationAPIFailureDoesNotCountAsMissing(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.StreamReconcileIntervalSeconds = 1
	global.GVA_CONFIG.Live.StreamReconcileMissingThreshold = 2
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	session := promoteReadyPublisher(t, &service, prepared.SessionNo)

	controller.setSnapshots(nil)
	controller.setListError(errors.New("SRS API timeout"))
	failedAt := session.StreamReconcileCheckedAt + 1000
	err = service.ProcessLiveStreamReconciliation(failedAt)
	require.ErrorContains(t, err, "SRS API timeout")
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Zero(t, session.StreamReconcileMissingCount, "SRS查询失败不能被当成流不存在")
	require.Zero(t, session.ReconnectDeadlineAt)
	require.Empty(t, session.StreamReconcileLastError, "单实例故障只写全局状态，不能放大为逐场更新")
	var runtime liveModel.LiveSRSRuntime
	require.NoError(t, global.GVA_DB.First(&runtime, liveSRSRuntimeSingletonID).Error)
	require.Equal(t, liveModel.LiveSRSHealthDegraded, runtime.HealthState)
	require.Contains(t, runtime.LastError, "SRS API timeout")

	controller.setListError(nil)
	firstSuccessfulMissingAt := failedAt + 1000
	require.NoError(t, service.ProcessLiveStreamReconciliation(firstSuccessfulMissingAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, uint32(1), session.StreamReconcileMissingCount)
	require.Zero(t, session.ReconnectDeadlineAt)
	require.Empty(t, session.StreamReconcileLastError)

	secondSuccessfulMissingAt := firstSuccessfulMissingAt + 1000
	require.NoError(t, service.ProcessLiveStreamReconciliation(secondSuccessfulMissingAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, secondSuccessfulMissingAt+1000, session.ReconnectDeadlineAt)
}

func TestLiveStreamReconciliationPresenceResetsConsecutiveMisses(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.StreamReconcileIntervalSeconds = 1
	global.GVA_CONFIG.Live.StreamReconcileMissingThreshold = 2
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()}}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	session := promoteReadyPublisher(t, &service, prepared.SessionNo)

	controller.setSnapshots(nil)
	firstMissingAt := session.StreamReconcileCheckedAt + 1000
	require.NoError(t, service.ProcessLiveStreamReconciliation(firstMissingAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, uint32(1), session.StreamReconcileMissingCount)

	controller.setSnapshots([]srsclient.PublisherSnapshot{roomTestPublisherSnapshot()})
	seenAt := firstMissingAt + 1000
	require.NoError(t, service.ProcessLiveStreamReconciliation(seenAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Zero(t, session.StreamReconcileMissingCount)
	require.Equal(t, seenAt, session.StreamReconcileLastSeenAt)
	require.Zero(t, session.ReconnectDeadlineAt)

	controller.setSnapshots(nil)
	nextMissingAt := seenAt + 1000
	require.NoError(t, service.ProcessLiveStreamReconciliation(nextMissingAt))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, uint32(1), session.StreamReconcileMissingCount, "中间重新发现流后必须从第一次缺失重新累计")
	require.Zero(t, session.ReconnectDeadlineAt)
}

func TestLiveStreamReconciliationBatchesLivingSessionsIntoOneSRSRequest(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.StreamReconcileIntervalSeconds = 5
	global.GVA_CONFIG.Live.StreamReconcileBatchSize = 5
	global.GVA_CONFIG.Live.StreamReconcileMaxBatchesPerRun = 3
	now := time.Now().UnixMilli()
	controller := &fakeSRSController{}
	service := RoomService{srsController: controller}
	const sessionCount = 12
	for i := 0; i < sessionCount; i++ {
		anchor := liveModel.LiveAnchor{
			UserId: uint64(20_000 + i), AnchorNo: fmt.Sprintf("reconcile-anchor-%02d", i),
			Nickname: "对账测试", ApplyStatus: liveModel.AnchorApplyStatusApproved,
			Status: liveModel.AnchorStatusNormal, LivePermission: liveModel.AnchorPermissionEnabled,
		}
		require.NoError(t, global.GVA_DB.Create(&anchor).Error)
		room := liveModel.LiveRoom{
			RoomNo: fmt.Sprintf("reconcile-room-%02d", i), AnchorId: anchor.ID,
			StreamName: fmt.Sprintf("reconcile-stream-%02d", i), Status: liveModel.LiveRoomStatusNormal,
			LiveStatus: liveModel.LiveRoomLiving, Visibility: liveModel.LiveRoomVisibilityPublic,
		}
		require.NoError(t, global.GVA_DB.Create(&room).Error)
		streamID := fmt.Sprintf("reconcile-srs-%02d", i)
		session := liveModel.LiveSession{
			SessionNo: fmt.Sprintf("reconcile-session-%02d", i), RoomId: room.ID, AnchorId: anchor.ID,
			Status: liveModel.LiveSessionLiving, StartedAt: now - 60_000,
			SRSServerID: "srs-1", SRSStreamID: streamID, SRSClientID: fmt.Sprintf("client-%02d", i),
			SRSVhost: "127.0.0.1", SRSApp: "live", StreamReconcileCheckedAt: now - 5000,
		}
		require.NoError(t, global.GVA_DB.Create(&session).Error)
		require.NoError(t, global.GVA_DB.Model(&room).Update("current_session_id", session.ID).Error)
		controller.snapshots = append(controller.snapshots, srsclient.PublisherSnapshot{
			StreamID: streamID, Vhost: "127.0.0.1", App: "live", Stream: room.StreamName, Active: true,
			VideoCodec: "H264", Width: 1280, Height: 720,
		})
	}

	require.NoError(t, service.ProcessLiveStreamReconciliation(now))
	require.Equal(t, 1, controller.listCallCount(), "同一轮跨三个数据库批次也必须复用一份SRS流列表")
	var seenCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND stream_reconcile_last_seen_at >= ?", liveModel.LiveSessionLiving, now).
		Count(&seenCount).Error)
	require.Equal(t, int64(sessionCount), seenCount)
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
	promoteReadyPublisher(t, &service, prepared.SessionNo)
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
	promoteReadyPublisher(t, &service, prepared.SessionNo)

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
	case liveModel.LiveSessionPreparing:
		require.Positive(t, successes)
		require.Equal(t, session.ID, room.CurrentSessionId)
		require.Equal(t, liveModel.LiveRoomPreparing, room.LiveStatus)
		require.Positive(t, session.StreamReadyDeadlineAt)
		require.Zero(t, session.StartedAt, "没有采集到尺寸时不能变为直播中")
	case liveModel.LiveSessionCancelled:
		require.Zero(t, room.CurrentSessionId)
		require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)
	case liveModel.LiveSessionEnding:
		// publish先保存SRS身份、超时扫描随后拿到锁时，必须进入可重试收尾，不能直接取消后遗留推流。
		require.Positive(t, successes)
		require.Equal(t, session.ID, room.CurrentSessionId)
		require.Equal(t, liveModel.LiveRoomEnding, room.LiveStatus)
		require.NotEmpty(t, session.SRSStreamID)
		require.Positive(t, session.StopNextRetryAt)
		require.Zero(t, session.StartedAt)
	default:
		t.Fatalf("unexpected session status after race: %d", session.Status)
	}
}

func TestPrepareTimeoutScanDrainsMultipleBoundedBatches(t *testing.T) {
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
	require.Equal(t, int64(31), cancelled)
	require.Zero(t, preparing)
}

func TestLiveTaskBatchSizesAreIndependentAndBounded(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndScanBatchSize = 25
	global.GVA_CONFIG.Live.StreamReadyBatchSize = 40
	global.GVA_CONFIG.Live.StreamReconcileBatchSize = 60
	global.GVA_CONFIG.Live.StreamReconcileMaxBatchesPerRun = 7
	global.GVA_CONFIG.Live.EndStopBatchSize = 80
	require.Equal(t, 25, liveEndScanBatchSize())
	require.Equal(t, 40, liveStreamReadyBatchSize())
	require.Equal(t, 60, liveStreamReconcileBatchSize())
	require.Equal(t, 7, liveStreamReconcileMaxBatchesPerRun())
	require.Equal(t, 80, liveEndStopBatchSize())

	global.GVA_CONFIG.Live.StreamReadyBatchSize = 50_000
	require.Equal(t, 1000, liveStreamReadyBatchSize(), "错误配置不能制造无界瞬时数据库压力")
}

func TestLiveTaskSingleRunCapacityCoversTenThousandStreams(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.MaxConcurrentStreams = 10_000
	global.GVA_CONFIG.Live.EndScanBatchSize = 1000
	global.GVA_CONFIG.Live.StreamReadyBatchSize = 1000
	global.GVA_CONFIG.Live.StreamReconcileBatchSize = 1000
	global.GVA_CONFIG.Live.EndStopBatchSize = 1000
	// 即使max-batches误配得过小，也不能低于声明的容量目标。
	global.GVA_CONFIG.Live.EndScanMaxBatchesPerRun = 1
	global.GVA_CONFIG.Live.StreamReadyMaxBatchesPerRun = 1
	global.GVA_CONFIG.Live.StreamReconcileMaxBatchesPerRun = 1
	global.GVA_CONFIG.Live.EndStopMaxBatchesPerRun = 1

	require.GreaterOrEqual(t, liveEndScanBatchSize()*liveEndScanMaxBatchesPerRun(), 10_000)
	require.GreaterOrEqual(t, liveStreamReadyBatchSize()*liveStreamReadyMaxBatchesPerRun(), 10_000)
	require.GreaterOrEqual(t, liveStreamReconcileBatchSize()*liveStreamReconcileMaxBatchesPerRun(), 10_000)
	require.GreaterOrEqual(t, liveEndStopBatchSize()*liveEndStopMaxBatchesPerRun(), 10_000)
}

func createEndingSessionsForTest(t *testing.T, count int, eligibleAt int64) []liveModel.LiveSession {
	t.Helper()
	sessions := make([]liveModel.LiveSession, 0, count)
	for i := 0; i < count; i++ {
		anchor := liveModel.LiveAnchor{
			UserId: uint64(50_000 + i), AnchorNo: fmt.Sprintf("ending-anchor-%03d", i),
			Nickname: "收尾测试主播", ApplyStatus: liveModel.AnchorApplyStatusApproved,
			Status: liveModel.AnchorStatusNormal, LivePermission: liveModel.AnchorPermissionEnabled,
		}
		require.NoError(t, global.GVA_DB.Create(&anchor).Error)
		room := liveModel.LiveRoom{
			RoomNo: fmt.Sprintf("ending-room-%03d", i), AnchorId: anchor.ID,
			StreamName: fmt.Sprintf("ending-stream-%03d", i), Status: liveModel.LiveRoomStatusNormal,
			LiveStatus: liveModel.LiveRoomEnding, Visibility: liveModel.LiveRoomVisibilityPublic,
		}
		require.NoError(t, global.GVA_DB.Create(&room).Error)
		session := liveModel.LiveSession{
			SessionNo: fmt.Sprintf("ending-session-%03d", i), RoomId: room.ID, AnchorId: room.AnchorId,
			Status: liveModel.LiveSessionEnding, EndReason: liveModel.LiveSessionEndByAdmin,
			EndedAt: eligibleAt - 1000, StopRequestedAt: eligibleAt - 1000,
			StopNextRetryAt: eligibleAt, SRSServerID: "srs-1",
			SRSStreamID: fmt.Sprintf("ending-srs-%03d", i), SRSVhost: "127.0.0.1", SRSApp: "live",
		}
		require.NoError(t, global.GVA_DB.Create(&session).Error)
		require.NoError(t, global.GVA_DB.Model(&room).Update("current_session_id", session.ID).Error)
		sessions = append(sessions, session)
	}
	return sessions
}

func TestPendingDeadlineScanNeverCallsSRS(t *testing.T) {
	setupRoomTestDB(t)
	now := time.Now().UnixMilli()
	controller := &fakeSRSController{stopBlock: make(chan struct{})}
	service := RoomService{srsController: controller}
	ending := createEndingSessionsForTest(t, 1, now-1)[0]

	room := liveModel.LiveRoom{
		RoomNo: "expired-prepare-room", AnchorId: 9001, StreamName: "expired-prepare-stream",
		Status: liveModel.LiveRoomStatusNormal, LiveStatus: liveModel.LiveRoomPreparing,
		Visibility: liveModel.LiveRoomVisibilityPublic,
	}
	require.NoError(t, global.GVA_DB.Create(&room).Error)
	expired := liveModel.LiveSession{
		SessionNo: "expired-prepare-session", RoomId: room.ID, AnchorId: room.AnchorId,
		Status: liveModel.LiveSessionPreparing, PrepareDeadlineAt: now - 1,
	}
	require.NoError(t, global.GVA_DB.Create(&expired).Error)
	require.NoError(t, global.GVA_DB.Model(&room).Update("current_session_id", expired.ID).Error)

	require.NoError(t, service.ProcessPendingSessions(now))
	require.Zero(t, controller.callCount(), "纯数据库生命周期扫描绝不能访问SRS")
	require.NoError(t, global.GVA_DB.First(&expired, expired.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, expired.Status)
	require.NoError(t, global.GVA_DB.First(&ending, ending.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnding, ending.Status)
}

func TestBlockedSRSStopDoesNotBlockDeadlineScan(t *testing.T) {
	setupRoomTestDB(t)
	now := time.Now().UnixMilli()
	stopBlock := make(chan struct{})
	controller := &fakeSRSController{started: make(chan struct{}), stopBlock: stopBlock}
	service := RoomService{srsController: controller}
	createEndingSessionsForTest(t, 1, now-1)

	stopDone := make(chan error, 1)
	go func() { stopDone <- service.ProcessEndingSessions(now) }()
	select {
	case <-controller.started:
	case <-time.After(2 * time.Second):
		t.Fatal("SRS收尾请求未启动")
	}

	room := liveModel.LiveRoom{
		RoomNo: "non-blocked-deadline-room", AnchorId: 9002, StreamName: "non-blocked-deadline-stream",
		Status: liveModel.LiveRoomStatusNormal, LiveStatus: liveModel.LiveRoomPreparing,
		Visibility: liveModel.LiveRoomVisibilityPublic,
	}
	require.NoError(t, global.GVA_DB.Create(&room).Error)
	expired := liveModel.LiveSession{
		SessionNo: "non-blocked-deadline-session", RoomId: room.ID, AnchorId: room.AnchorId,
		Status: liveModel.LiveSessionPreparing, PrepareDeadlineAt: now - 1,
	}
	require.NoError(t, global.GVA_DB.Create(&expired).Error)
	require.NoError(t, global.GVA_DB.Model(&room).Update("current_session_id", expired.ID).Error)

	deadlineDone := make(chan error, 1)
	go func() { deadlineDone <- service.ProcessPendingSessions(now) }()
	select {
	case err := <-deadlineDone:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SRS请求阻塞了独立的数据库生命周期扫描")
	}
	require.NoError(t, global.GVA_DB.First(&expired, expired.ID).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, expired.Status)
	select {
	case err := <-stopDone:
		t.Fatalf("释放SRS前收尾任务不应结束: %v", err)
	default:
	}
	close(stopBlock)
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("释放SRS后收尾任务未结束")
	}
}

func TestEndingScanUsesBoundedConcurrency(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndStopConcurrency = 3
	now := time.Now().UnixMilli()
	stopBlock := make(chan struct{})
	controller := &fakeSRSController{stopBlock: stopBlock}
	service := RoomService{srsController: controller}
	sessions := createEndingSessionsForTest(t, 9, now-1)

	done := make(chan error, 1)
	go func() { done <- service.ProcessEndingSessions(now) }()
	require.Eventually(t, func() bool { return controller.callCount() == 3 }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 3, controller.callCount(), "阻塞期间不能启动超过配置上限的SRS请求")
	require.Equal(t, 3, controller.maxStopConcurrency())
	close(stopBlock)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("有界并发收尾任务未完成")
	}
	require.Equal(t, 9, controller.callCount())
	require.Equal(t, 3, controller.maxStopConcurrency())
	for _, session := range sessions {
		require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
		require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	}
}

func TestEndingScanDrainsMultipleBoundedBatches(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndScanBatchSize = 25
	global.GVA_CONFIG.Live.EndStopBatchSize = 5
	global.GVA_CONFIG.Live.EndStopConcurrency = 2
	now := time.Now().UnixMilli()
	controller := &fakeSRSController{}
	service := RoomService{srsController: controller}
	createEndingSessionsForTest(t, 7, now-1)

	require.NoError(t, service.ProcessEndingSessions(now))
	require.Equal(t, 7, controller.callCount())
	var endingCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ?", liveModel.LiveSessionEnding).Count(&endingCount).Error)
	require.Zero(t, endingCount)
}

func TestEndingScanUsesBatchSRSControllerOnce(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndStopBatchSize = 5
	global.GVA_CONFIG.Live.EndStopConcurrency = 2
	now := time.Now().UnixMilli()
	controller := &fakeBatchSRSController{fakeSRSController: &fakeSRSController{}}
	service := RoomService{srsController: controller}
	createEndingSessionsForTest(t, 7, now-1)

	require.NoError(t, service.ProcessEndingSessions(now))
	batchCalls, batchRefs := controller.batchCounts()
	require.Equal(t, 1, batchCalls)
	require.Equal(t, 7, batchRefs)
	require.Zero(t, controller.callCount(), "支持批量停止时不能再逐场查询SRS")
	var endingCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ?", liveModel.LiveSessionEnding).Count(&endingCount).Error)
	require.Zero(t, endingCount)
}

func TestEndingFailuresDoNotStarveLaterBacklog(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndScanBatchSize = 25
	global.GVA_CONFIG.Live.EndStopBatchSize = 5
	global.GVA_CONFIG.Live.EndStopConcurrency = 2
	now := time.Now().UnixMilli()
	stopErr := errors.New("SRS unavailable")
	controller := &fakeSRSController{errors: []error{stopErr, stopErr, stopErr, stopErr, stopErr}}
	service := RoomService{srsController: controller}
	createEndingSessionsForTest(t, 7, now-1)

	err := service.ProcessEndingSessions(now)
	require.ErrorContains(t, err, "SRS unavailable")
	require.Equal(t, 7, controller.callCount())

	// 失败的前五场按退避时间移出本轮到期队列；同一轮的下一有界批次必须继续推进后两场。
	var endingCount, cancelledCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ?", liveModel.LiveSessionEnding).Count(&endingCount).Error)
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ?", liveModel.LiveSessionCancelled).Count(&cancelledCount).Error)
	require.Equal(t, int64(5), endingCount)
	require.Equal(t, int64(2), cancelledCount)
}

func TestEndingFailureSchedulesRetryFromActualCompletionTime(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndRetryBaseSeconds = 5
	staleScanTime := time.Now().Add(-time.Minute).UnixMilli()
	controller := &fakeSRSController{errors: []error{errors.New("SRS unavailable")}}
	service := RoomService{srsController: controller}
	session := createEndingSessionsForTest(t, 1, staleScanTime-1)[0]
	startedAt := time.Now().UnixMilli()

	err := service.ProcessEndingSessions(staleScanTime)
	require.ErrorContains(t, err, "SRS unavailable")
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnding, session.Status)
	require.Equal(t, uint32(1), session.StopAttempts)
	require.GreaterOrEqual(t, session.StopNextRetryAt, startedAt+5000,
		"批次排队或SRS超时后，下一次重试必须从实际完成时间计算，不能立即形成重试风暴")
}

func TestEndStopConcurrencyConfigurationIsBounded(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndStopConcurrency = 0
	require.Equal(t, defaultEndStopConcurrency, liveEndStopConcurrency())
	global.GVA_CONFIG.Live.EndStopConcurrency = 4
	require.Equal(t, 4, liveEndStopConcurrency())
	global.GVA_CONFIG.Live.EndStopConcurrency = 10000
	require.Equal(t, 64, liveEndStopConcurrency())
}

func TestSummarizeLiveBatchErrorsBoundsOutputAndPreservesCause(t *testing.T) {
	sentinel := errors.New("SRS unavailable")
	runErrors := make([]error, 1000)
	for index := range runErrors {
		runErrors[index] = fmt.Errorf("stop publisher failed: %w", sentinel)
	}

	summary := summarizeLiveBatchErrors("SRS场次收尾", runErrors)
	require.ErrorIs(t, summary, sentinel, "压缩日志后仍要保留错误链供监控和上层判断")
	require.ErrorContains(t, summary, "失败1000项")
	require.Less(t, len(summary.Error()), 700, "批量同类故障不能让单条定时任务日志随场次数量无限增长")
}

func TestOwnerEndRetriesSRSFailureAndIgnoresUnpublishForEndingSession(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{
		errors:    []error{errors.New("SRS timeout"), nil},
		snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	promoteReadyPublisher(t, &service, prepared.SessionNo)

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

	require.NoError(t, service.ProcessEndingSessions(session.StopNextRetryAt-1))
	require.Equal(t, 1, controller.callCount(), "未到重试时间不能调用SRS")
	require.NoError(t, service.ProcessEndingSessions(session.StopNextRetryAt))
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
	controller := &fakeSRSController{
		snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
		started:   make(chan struct{}), release: make(chan struct{}),
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	promoteReadyPublisher(t, &service, prepared.SessionNo)

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
	require.NoError(t, service.ProcessEndingSessions(time.Now().UnixMilli()))
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
	require.NoError(t, service.ProcessEndingSessions(time.Now().Add(time.Minute).UnixMilli()))
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
	promoteReadyPublisher(t, &service, prepared.SessionNo)
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
	require.JSONEq(t, `{}`, string(publicStreamInfo([]byte(`{"width":1920}`))))
	require.JSONEq(t, `{}`, string(publicStreamInfo([]byte(`{"width":1920,"height":0}`))))
	require.JSONEq(t, `{"width":1920,"height":1080}`, string(publicStreamInfo([]byte(`{"width":1920,"height":1080}`))))
	require.JSONEq(t, `{"videoCodec":"H264","width":1920,"height":1080}`,
		string(publicStreamInfo([]byte(`{"videoCodec":"H264","width":1920,"height":1080,"token":"secret","clientId":"internal"}`))))
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

func TestPrepareSessionRejectsEveryBlockedAnchorStateWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name    string
		updates map[string]interface{}
	}{
		{name: "申请尚未审核", updates: map[string]interface{}{"apply_status": liveModel.AnchorApplyStatusPending}},
		{name: "申请被拒绝", updates: map[string]interface{}{"apply_status": liveModel.AnchorApplyStatusRejected}},
		{name: "主播被停用", updates: map[string]interface{}{"status": liveModel.AnchorStatusDisabled}},
		{name: "主播被封禁", updates: map[string]interface{}{"status": liveModel.AnchorStatusBanned}},
		{name: "主播已注销", updates: map[string]interface{}{"status": liveModel.AnchorStatusCancelled}},
		{name: "高风险", updates: map[string]interface{}{"risk_level": liveModel.AnchorRiskHigh}},
		{name: "开播权限关闭", updates: map[string]interface{}{"live_permission": liveModel.AnchorPermissionDisabled}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupRoomTestDB(t)
			anchor, category := createRoomTestAnchor(t)
			require.NoError(t, global.GVA_DB.Model(&liveModel.LiveAnchor{}).
				Where("id = ?", anchor.ID).Updates(test.updates).Error)

			_, err := (&RoomService{}).PrepareSession(anchor.UserId, roomTestPrepareRequest(category.ID))
			require.ErrorIs(t, err, ErrLiveRoomUnavailable)

			var sessionCount, roomCount int64
			require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&sessionCount).Error)
			require.NoError(t, global.GVA_DB.Model(&liveModel.LiveRoom{}).Count(&roomCount).Error)
			require.Zero(t, sessionCount, "权限拒绝不得残留准备中场次")
			require.Zero(t, roomCount, "权限拒绝发生在确保房间之前，不应产生附带写入")
		})
	}
}

func TestPrepareSessionWaitsForConcurrentPermissionDisableThenRejects(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	queryStarted, unblock := blockNextQueryForTable(t, "live_anchor")

	disabled := liveModel.AnchorPermissionDisabled
	permissionDone := make(chan error, 1)
	go func() {
		permissionDone <- (&AnchorService{}).UpdateAnchorPermission(liveReq.AnchorPermissionUpdateReq{
			AnchorId: anchor.ID, LivePermission: &disabled, PkPermission: &disabled,
			RecommendPermission: &disabled, WithdrawPermission: &disabled,
		})
	}()
	select {
	case <-queryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("后台权限事务没有进入主播锁查询")
	}

	prepareDone := make(chan error, 1)
	go func() {
		_, err := (&RoomService{}).PrepareSession(anchor.UserId, roomTestPrepareRequest(category.ID))
		prepareDone <- err
	}()
	select {
	case err := <-prepareDone:
		t.Fatalf("权限事务提交前 prepare 不应越过主播锁，提前返回: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	unblock()
	select {
	case err := <-permissionDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("后台权限事务未完成")
	}
	select {
	case err := <-prepareDone:
		require.ErrorIs(t, err, ErrLiveRoomUnavailable)
	case <-time.After(2 * time.Second):
		t.Fatal("prepare 等待权限事务后未返回")
	}

	var sessionCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&sessionCount).Error)
	require.Zero(t, sessionCount)
}

func TestConcurrentPermissionDisableAfterPrepareCancelsNewSessionAndRevokesToken(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	queryStarted, unblock := blockNextQueryForTable(t, "live_anchor")
	service := newRoomTestService()

	type prepareResult struct {
		fixture liveResFixture
		err     error
	}
	prepareDone := make(chan prepareResult, 1)
	go func() {
		prepared, err := service.PrepareSession(anchor.UserId, roomTestPrepareRequest(category.ID))
		prepareDone <- prepareResult{fixture: liveResFixture{
			RoomNo: prepared.RoomNo, SessionNo: prepared.SessionNo, StreamName: prepared.StreamName,
			Token: prepared.PublishToken, PushURL: prepared.PushURL, PlayURL: prepared.PlayURL,
			PrepareDeadlineAt: prepared.PrepareDeadlineAt,
		}, err: err}
	}()
	select {
	case <-queryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("prepare 没有进入事务内主播锁查询")
	}

	disabled := liveModel.AnchorPermissionDisabled
	permissionDone := make(chan error, 1)
	go func() {
		permissionDone <- (&AnchorService{}).UpdateAnchorPermission(liveReq.AnchorPermissionUpdateReq{
			AnchorId: anchor.ID, LivePermission: &disabled, PkPermission: &disabled,
			RecommendPermission: &disabled, WithdrawPermission: &disabled,
		})
	}()
	select {
	case err := <-permissionDone:
		t.Fatalf("prepare 提交前后台权限修改不应越过主播锁，提前返回: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	unblock()
	var prepared prepareResult
	select {
	case prepared = <-prepareDone:
		require.NoError(t, prepared.err)
	case <-time.After(2 * time.Second):
		t.Fatal("prepare 未完成")
	}
	select {
	case err := <-permissionDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("后台权限修改未完成")
	}

	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", prepared.fixture.SessionNo).First(&session).Error)
	require.Equal(t, liveModel.LiveSessionCancelled, session.Status)
	require.Equal(t, liveModel.LiveSessionEndAnchorBlocked, session.EndReason)
	require.Equal(t, "后台关闭主播开播权限", session.FailureReason)
	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("room_no = ?", prepared.fixture.RoomNo).First(&room).Error)
	require.Zero(t, room.CurrentSessionId)
	require.Equal(t, liveModel.LiveRoomOffline, room.LiveStatus)

	_, err := service.OnPublish(roomTestPublishRequest(prepared.fixture, "?pt="+prepared.fixture.Token))
	require.Error(t, err, "后台关闭权限后，已经签发但尚未使用的旧凭证也必须失效")
}

func TestPrepareSessionWaitsForConcurrentCategoryDisableThenRejects(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	queryStarted, unblock := blockNextQueryForTable(t, "live_category")

	disabled := liveModel.LiveCategoryStatusDisabled
	categoryDone := make(chan error, 1)
	go func() {
		categoryDone <- (&CategoryService{}).UpdateCategoryStatus(liveReq.LiveCategoryStatusUpdateReq{
			ID: category.ID, Status: &disabled,
		})
	}()
	select {
	case <-queryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("分类停用事务没有进入分类锁查询")
	}

	prepareDone := make(chan error, 1)
	go func() {
		_, err := (&RoomService{}).PrepareSession(anchor.UserId, roomTestPrepareRequest(category.ID))
		prepareDone <- err
	}()
	select {
	case err := <-prepareDone:
		t.Fatalf("分类事务提交前 prepare 不应越过分类锁，提前返回: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	unblock()
	select {
	case err := <-categoryDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("分类停用事务未完成")
	}
	select {
	case err := <-prepareDone:
		require.ErrorIs(t, err, ErrLiveCategoryNotFound)
	case <-time.After(2 * time.Second):
		t.Fatal("prepare 等待分类事务后未返回")
	}

	var sessionCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&sessionCount).Error)
	require.Zero(t, sessionCount)
}

func TestConcurrentPrepareCreatesOnlyOneActiveSession(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := service.PrepareSession(anchor.UserId, roomTestPrepareRequest(category.ID))
			results <- err
		}()
	}
	close(start)

	var successCount, activeExistsCount int
	for range 2 {
		select {
		case err := <-results:
			switch {
			case err == nil:
				successCount++
			case errors.Is(err, ErrLiveRoomActiveSessionExists):
				activeExistsCount++
			default:
				t.Fatalf("并发 prepare 返回了非预期错误: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("并发 prepare 未完成")
		}
	}
	require.Equal(t, 1, successCount)
	require.Equal(t, 1, activeExistsCount)

	var sessions []liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Find(&sessions).Error)
	require.Len(t, sessions, 1)
	require.Equal(t, liveModel.LiveSessionPreparing, sessions[0].Status)
	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.Where("anchor_id = ?", anchor.ID).First(&room).Error)
	require.Equal(t, sessions[0].ID, room.CurrentSessionId)
}

func TestPrepareSessionDoesNotClearCurrentSessionOnDatabaseError(t *testing.T) {
	setupRoomTestDB(t)
	anchor, category := createRoomTestAnchor(t)
	room, err := ensureRoomForAnchor(global.GVA_DB, anchor)
	require.NoError(t, err)
	staleSession := liveModel.LiveSession{
		SessionNo: "stale-session", RoomId: room.ID, AnchorId: anchor.ID,
		CategoryId: uint64(category.ID), Status: liveModel.LiveSessionEnded,
	}
	require.NoError(t, global.GVA_DB.Create(&staleSession).Error)
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).
		Updates(map[string]interface{}{
			"current_session_id": staleSession.ID,
			"live_status":        liveModel.LiveRoomPreparing,
		}).Error)

	sentinel := errors.New("injected session lookup failure")
	var injectOnce sync.Once
	const callbackName = "test:fail_current_session_lookup"
	require.NoError(t, global.GVA_DB.Callback().Query().Before("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Table == "live_session" {
			injectOnce.Do(func() { _ = db.AddError(sentinel) })
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, global.GVA_DB.Callback().Query().Remove(callbackName))
	})

	_, err = (&RoomService{}).PrepareSession(anchor.UserId, roomTestPrepareRequest(category.ID))
	require.ErrorIs(t, err, sentinel)
	require.NoError(t, global.GVA_DB.First(&room, room.ID).Error)
	require.Equal(t, staleSession.ID, room.CurrentSessionId, "查询故障必须回滚，不能被当成可清理的旧指针")
	require.Equal(t, liveModel.LiveRoomPreparing, room.LiveStatus)
	var sessionCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)
}

func TestSRSRestartAllowsImmediateControlledPublisherTakeover(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	controller := &runtimeFakeSRSController{
		fakeSRSController: &fakeSRSController{snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()}},
		runtime:           srsclient.RuntimeIdentity{ServerID: "srs-1", ServiceID: "service-1"},
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	require.NoError(t, func() error {
		_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
		return err
	}())
	session := promoteReadyPublisher(t, &service, prepared.SessionNo)
	require.Equal(t, uint64(1), session.SRSGeneration)
	oldEpoch := session.PublisherEpoch

	controller.setRuntime("srs-2")
	republish := roomTestPublishRequest(prepared, "?pt="+prepared.Token)
	republish.ServerID = "srs-2"
	republish.StreamID = "stream-2"
	republish.ClientID = "client-2"
	published, err := service.OnPublish(republish)
	require.NoError(t, err, "可信HTTP API确认的新SRS代际应立即接管，不等待三次缺失")
	require.Equal(t, liveModel.LiveMediaPublishing, published.MediaState)
	require.Positive(t, published.MediaStateChangedAt)
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, "srs-2", session.SRSServerID)
	require.Equal(t, "stream-2", session.SRSStreamID)
	require.Equal(t, uint64(2), session.SRSGeneration)
	require.Equal(t, oldEpoch+1, session.PublisherEpoch)
	require.Positive(t, session.StreamReadyDeadlineAt)

	// 旧进程迟到的unpublish不得影响新连接。
	require.NoError(t, service.OnUnpublish(roomTestUnpublishRequest(prepared, "client-1")))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Zero(t, session.ReconnectDeadlineAt)
	require.Equal(t, "stream-2", session.SRSStreamID)

	// 同一代际的另一连接仍然必须拒绝，不能把“重启恢复”变成任意连接覆盖。
	conflict := republish
	conflict.StreamID = "stream-3"
	conflict.ClientID = "client-3"
	_, err = service.OnPublish(conflict)
	require.ErrorIs(t, err, ErrLiveSessionStateInvalid)

	// 旧server_id即使迟到，也会被当前SRS API身份校验拒绝，不能回滚generation。
	stale := roomTestPublishRequest(prepared, "?pt="+prepared.Token)
	_, err = service.OnPublish(stale)
	require.ErrorIs(t, err, ErrLiveSessionStateInvalid)
}

func TestConfirmMediaStoppedRequiresAttentionAndCreatesAudit(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.EndWarningSeconds = 1
	global.GVA_CONFIG.Live.EndCriticalSeconds = 2
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{
		snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
		errors:    []error{errors.New("SRS unavailable")},
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	session := promoteReadyPublisher(t, &service, prepared.SessionNo)
	require.NoError(t, service.RequestAdminEnd(session.ID, "违规内容"))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnding, session.Status)

	epoch := session.PublisherEpoch
	earlyReq := liveReq.LiveSessionConfirmMediaStoppedReq{
		SessionID: session.ID, ExpectedPublisherEpoch: &epoch,
		Reason: "已在媒体机确认停止", Evidence: "incident-2026-001",
	}
	err = service.ConfirmMediaStopped(earlyReq, MediaStopActor{UserID: 99, Username: "root"})
	require.ErrorIs(t, err, ErrLiveManualConfirmationTooEarly)
	wrongEpoch := epoch + 1
	wrongEpochReq := earlyReq
	wrongEpochReq.ExpectedPublisherEpoch = &wrongEpoch
	err = service.ConfirmMediaStopped(wrongEpochReq, MediaStopActor{UserID: 99, Username: "root"})
	require.ErrorIs(t, err, ErrLiveSessionStateInvalid)

	stalledAt := time.Now().UnixMilli() - 3000
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Where("id = ?", session.ID).
		Updates(map[string]interface{}{"stop_requested_at": stalledAt, "stop_next_retry_at": time.Now().Add(time.Hour).UnixMilli()}).Error)
	require.NoError(t, promoteEndingStallAlerts(time.Now().UnixMilli()))
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnding, session.Status, "严重告警只能转人工关注，不能按次数或时间自动结算")
	require.Equal(t, liveModel.LiveStopRecoveryManualAttention, session.StopRecoveryState)
	require.NoError(t, service.ConfirmMediaStopped(earlyReq, MediaStopActor{
		UserID: 99, Username: "root", ClientIP: "192.0.2.8",
		UserAgent: "admin-test", RequestID: "request-1",
	}))

	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnded, session.Status)
	require.Equal(t, liveModel.LiveMediaStopped, session.MediaState)
	require.Equal(t, liveModel.LiveMediaStopSourceAdmin, session.MediaStopSource)
	require.Equal(t, uint(99), session.MediaStopConfirmedBy)
	var room liveModel.LiveRoom
	require.NoError(t, global.GVA_DB.First(&room, session.RoomId).Error)
	require.Zero(t, room.CurrentSessionId)
	var audits []liveModel.LiveSessionMediaAudit
	require.NoError(t, global.GVA_DB.Where("session_id = ?", session.ID).Find(&audits).Error)
	require.Len(t, audits, 1)
	require.Equal(t, "incident-2026-001", audits[0].Evidence)
	require.Equal(t, session.PublisherEpoch, audits[0].PublisherEpoch)
}

func TestConfirmedMediaStopIsReplayedWithoutSRSAfterCrashWindow(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	controller := &fakeSRSController{
		snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
		errors:    []error{errors.New("SRS unavailable")},
	}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	session := promoteReadyPublisher(t, &service, prepared.SessionNo)
	require.NoError(t, service.RequestAdminEnd(session.ID, "test crash window"))
	require.Equal(t, 1, controller.callCount())

	now := time.Now().UnixMilli()
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Where("id = ?", session.ID).
		Updates(map[string]interface{}{
			"media_state": liveModel.LiveMediaStopped, "media_stop_source": liveModel.LiveMediaStopSourceAdmin,
			"media_stop_confirmed_at": now, "media_stop_confirmed_by": uint(99), "stop_next_retry_at": now,
		}).Error)

	// 模拟服务在人工确认事务提交后、调用 finalize 前退出。恢复时即便 SRS 仍不可用，
	// 持久化的人工确认事实也必须足以完成结算，且不能再次调用 SRS。
	require.NoError(t, service.processEndingSessionWithController(
		session.ID, now, controller, errors.New("SRS still unavailable"),
	))
	require.Equal(t, 1, controller.callCount())
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnded, session.Status)
	require.Equal(t, liveModel.LiveMediaStopSourceAdmin, session.MediaStopSource)
}

func TestBatchEndingReplaySkipsSRSForConfirmedMediaStop(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	base := &fakeSRSController{
		snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
		errors:    []error{errors.New("SRS unavailable")},
	}
	controller := &fakeBatchSRSController{fakeSRSController: base}
	service := RoomService{srsController: controller}
	prepared := prepareRoomTestSession(t, &service, category.ID)
	_, err := service.OnPublish(roomTestPublishRequest(prepared, "?pt="+prepared.Token))
	require.NoError(t, err)
	session := promoteReadyPublisher(t, &service, prepared.SessionNo)
	require.NoError(t, service.RequestAdminEnd(session.ID, "test batch crash window"))

	now := time.Now().UnixMilli()
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Where("id = ?", session.ID).
		Updates(map[string]interface{}{
			"media_state": liveModel.LiveMediaStopped, "media_stop_source": liveModel.LiveMediaStopSourceAdmin,
			"media_stop_confirmed_at": now, "media_stop_confirmed_by": uint(99), "stop_next_retry_at": now,
		}).Error)
	require.NoError(t, service.ProcessEndingSessions(now))
	batchCalls, batchRefs := controller.batchCounts()
	require.Zero(t, batchCalls)
	require.Zero(t, batchRefs)
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Equal(t, liveModel.LiveSessionEnded, session.Status)
}

func TestSRSRecoveryPrioritizesAllEndingSessionsWithoutPerSessionHealthWrites(t *testing.T) {
	setupRoomTestDB(t)
	now := time.Now().UnixMilli()
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSRSRuntime{}).
		Where("id = ?", liveSRSRuntimeSingletonID).Updates(map[string]interface{}{
		"server_id": "srs-old", "generation": uint64(7),
		"health_state": liveModel.LiveSRSHealthDegraded, "degraded_at": now - 60_000,
	}).Error)
	const count = 1000
	sessions := make([]liveModel.LiveSession, 0, count)
	for index := range count {
		sessions = append(sessions, liveModel.LiveSession{
			SessionNo: fmt.Sprintf("recovery-%04d", index), RoomId: uint(index + 1), AnchorId: uint(index + 1),
			Status: liveModel.LiveSessionEnding, StopRequestedAt: now - int64(index+1),
			StopNextRetryAt: now + 300_000, MediaState: liveModel.LiveMediaStopping,
		})
	}
	require.NoError(t, global.GVA_DB.CreateInBatches(&sessions, 200).Error)

	observation, err := observeLiveSRSSuccess(srsclient.RuntimeIdentity{ServerID: "srs-new"}, now)
	require.NoError(t, err)
	require.True(t, observation.Restarted)
	require.True(t, observation.Recovered)
	var delayed int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND stop_next_retry_at > ?", liveModel.LiveSessionEnding, now).Count(&delayed).Error)
	require.Zero(t, delayed, "恢复事件必须一次批量唤醒全部积压，而不是逐场等待原退避")
}

func TestStalePublisherSnapshotCannotRollbackSRSGeneration(t *testing.T) {
	setupRoomTestDB(t)
	now := time.Now().UnixMilli()
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSRSRuntime{}).
		Where("id = ?", liveSRSRuntimeSingletonID).Updates(map[string]interface{}{
		"server_id": "srs-new", "service_id": "service-new", "generation": uint64(9),
		"health_state": liveModel.LiveSRSHealthHealthy, "last_success_at": now,
	}).Error)

	_, err := observePublisherBatch(srsclient.PublisherBatch{
		CapturedAt: now - 1000,
		Runtime:    srsclient.RuntimeIdentity{ServerID: "srs-old", ServiceID: "service-old"},
	}, now+1000, nil)
	require.ErrorIs(t, err, errLiveSRSStaleSnapshot)

	runtime, err := loadLiveSRSRuntime(global.GVA_DB)
	require.NoError(t, err)
	require.Equal(t, "srs-new", runtime.ServerID)
	require.Equal(t, "service-new", runtime.ServiceID)
	require.Equal(t, uint64(9), runtime.Generation)
	require.Equal(t, liveModel.LiveSRSHealthHealthy, runtime.HealthState)
}

func TestCachedSnapshotBeforeFailureCannotRecoverSRSHealth(t *testing.T) {
	setupRoomTestDB(t)
	now := time.Now().UnixMilli()
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSRSRuntime{}).
		Where("id = ?", liveSRSRuntimeSingletonID).Updates(map[string]interface{}{
		"server_id": "srs-1", "generation": uint64(3),
		"health_state": liveModel.LiveSRSHealthDegraded, "last_success_at": now - 2000,
		"last_failure_at": now, "degraded_at": now,
	}).Error)

	observation, err := observePublisherBatch(srsclient.PublisherBatch{
		CapturedAt: now - 1000, FromCache: true,
		Runtime: srsclient.RuntimeIdentity{ServerID: "srs-1"},
	}, now+1000, nil)
	require.NoError(t, err)
	require.False(t, observation.Recovered)
	require.Equal(t, liveModel.LiveSRSHealthDegraded, observation.Runtime.HealthState)

	runtime, err := loadLiveSRSRuntime(global.GVA_DB)
	require.NoError(t, err)
	require.Equal(t, liveModel.LiveSRSHealthDegraded, runtime.HealthState)
	require.Zero(t, runtime.RecoveredAt)
}
