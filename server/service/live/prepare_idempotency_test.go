package live

import (
	"context"
	"testing"
	"time"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"

	"github.com/stretchr/testify/require"
)

func TestPrepareRequiresIdempotencyKeyBeforeDatabaseWrites(t *testing.T) {
	setupRoomTestDB(t)
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	req := roomTestPrepareRequest(category.ID)
	req.IdempotencyKey = ""

	_, err := service.PrepareSession(501, req)
	require.ErrorIs(t, err, ErrLiveIdempotencyKeyInvalid)

	var sessionCount, roomCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveSession{}).Count(&sessionCount).Error)
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveRoom{}).Count(&roomCount).Error)
	require.Zero(t, sessionCount)
	require.Zero(t, roomCount)
}

func TestPrepareWithoutRedisUsesEncryptedLocalCacheAndReplaysResult(t *testing.T) {
	setupRoomTestDB(t)
	previousRedis := global.GVA_REDIS
	global.GVA_REDIS = nil
	t.Cleanup(func() { global.GVA_REDIS = previousRedis })
	_, category := createRoomTestAnchor(t)
	service := newRoomTestService()
	req := roomTestPrepareRequest(category.ID)

	first, err := service.PrepareSession(501, req)
	require.NoError(t, err)
	// 模拟事务已提交、HTTP响应丢失后进程重启；新服务对象没有任何本地缓存。
	restartedService := newRoomTestService()
	second, err := restartedService.PrepareSession(501, req)
	require.NoError(t, err)
	require.Equal(t, first, second)

	cache, ok := service.resolvedPrepareResultCache.(*localLivePrepareResultCache)
	require.True(t, ok, "关闭 Redis 时必须固定使用单实例本地缓存")
	cacheKey := livePrepareCacheKey(global.GVA_CONFIG.Live.PublishTokenKey, 501, req.IdempotencyKey)
	cache.mu.Lock()
	ciphertext := cache.values[cacheKey].value
	cache.mu.Unlock()
	require.NotEmpty(t, ciphertext)
	require.NotContains(t, ciphertext, first.PublishToken, "本地缓存也不能保存明文推流凭证")
	var session liveModel.LiveSession
	require.NoError(t, global.GVA_DB.Where("session_no = ?", first.SessionNo).First(&session).Error)
	require.Equal(t,
		livePrepareIdempotencyHash(global.GVA_CONFIG.Live.PublishTokenKey, 501, req.IdempotencyKey),
		session.PublishIdempotencyKeyHash,
	)
	require.NotEqual(t, req.IdempotencyKey, session.PublishIdempotencyKeyHash, "数据库不能保存原始幂等键")
	require.NotEmpty(t, session.PublishResultCiphertext)
	require.NotContains(t, session.PublishResultCiphertext, first.PublishToken, "数据库不能保存明文推流凭证")

	fixture := liveResFixture{
		RoomNo: first.RoomNo, SessionNo: first.SessionNo, StreamName: first.StreamName,
		Token: first.PublishToken, PushURL: first.PushURL, PlayURL: first.PlayURL,
		PrepareDeadlineAt: first.PrepareDeadlineAt,
	}
	_, err = restartedService.OnPublish(roomTestPublishRequest(fixture, "?pt="+first.PublishToken))
	require.NoError(t, err)
	require.NoError(t, global.GVA_DB.First(&session, session.ID).Error)
	require.Empty(t, session.PublishResultCiphertext, "SRS已接受推流后应清理短期重放密文")
}

func TestLocalPrepareCacheIsBoundedAndUpdatesExpiryInPlace(t *testing.T) {
	cache := newLocalLivePrepareResultCache(2)
	now := time.Unix(100, 0)
	cache.now = func() time.Time { return now }
	ctx := context.Background()

	require.NoError(t, cache.Set(ctx, "first", "v1", time.Minute))
	// 反复更新同一个键必须原位调整堆节点，不能让过期索引持续增长。
	require.NoError(t, cache.Set(ctx, "first", "v2", 3*time.Minute))
	require.NoError(t, cache.Set(ctx, "second", "v2", 2*time.Minute))
	require.NoError(t, cache.Set(ctx, "third", "v3", 4*time.Minute))

	value, err := cache.Get(ctx, "first")
	require.NoError(t, err)
	require.Equal(t, "v2", value)
	_, err = cache.Get(ctx, "second")
	require.ErrorIs(t, err, errLivePrepareResultCacheMiss)
	_, err = cache.Get(ctx, "third")
	require.NoError(t, err)
	cache.mu.Lock()
	require.LessOrEqual(t, len(cache.values), 2)
	require.Equal(t, len(cache.values), cache.expiries.Len())
	cache.mu.Unlock()

	now = now.Add(5 * time.Minute)
	_, err = cache.Get(ctx, "first")
	require.ErrorIs(t, err, errLivePrepareResultCacheMiss)
	_, err = cache.Get(ctx, "third")
	require.ErrorIs(t, err, errLivePrepareResultCacheMiss)
	cache.mu.Lock()
	require.Empty(t, cache.values)
	cache.mu.Unlock()
}

func TestLocalPrepareCacheCapacityCoversTargetConcurrencyAndRefreshes(t *testing.T) {
	previousLive := global.GVA_CONFIG.Live
	global.GVA_CONFIG.Live.MaxConcurrentStreams = 10_000
	global.GVA_CONFIG.Live.PushURLRefreshLimit = 5
	t.Cleanup(func() { global.GVA_CONFIG.Live = previousLive })

	require.Equal(t, 60_000, localLivePrepareCacheCapacity())

	global.GVA_CONFIG.Live.MaxConcurrentStreams = 1_000_000
	global.GVA_CONFIG.Live.PushURLRefreshLimit = hardPushURLRefreshLimit
	require.Equal(t, hardLocalPrepareCacheEntries, localLivePrepareCacheCapacity(), "异常配置不能导致无界内存")
}
