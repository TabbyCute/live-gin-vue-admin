package social

import (
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"os"
	"sort"
	"strings"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	repo "tb_live_module/repository/social"
	"testing"
	"time"
)

func TestRedisHistoryDeleteReopenAndLargeSequence(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: "visible"}
	first, err := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "first-session"})
	require.NoError(t, err)
	report := req.HistoryReport{Resource: r, SessionToken: first.SessionToken, SessionVersion: first.SessionVersion, Epoch: first.Epoch, Sequence: repo.MaxSafeInteger}
	_, err = s.ReportHistory(ctx, 1, report)
	require.NoError(t, err)
	key, queue := historyKeys(1, r)
	raw, err := s.Redis.Get(ctx, key).Bytes()
	require.NoError(t, err)
	var buffered bufferedHistory
	require.NoError(t, json.Unmarshal(raw, &buffered))
	require.Equal(t, repo.MaxSafeInteger, buffered.Report.Sequence)
	require.NoError(t, s.DeleteHistory(ctx, 1, &r))
	second, err := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "second-session"})
	require.NoError(t, err)
	require.Greater(t, second.SessionVersion, first.SessionVersion)
	report.SessionToken, report.SessionVersion, report.Sequence = second.SessionToken, second.SessionVersion, 1
	accepted, err := s.ReportHistory(ctx, 1, report)
	require.NoError(t, err)
	require.True(t, accepted.Accepted)
	require.NoError(t, s.Redis.ZAdd(ctx, queue, redis.Z{Score: 0, Member: key}).Err())
	require.NoError(t, s.FlushHistory(ctx))
	var row model.SocialWatchHistory
	require.NoError(t, s.DB.First(&row).Error)
	require.Equal(t, second.SessionToken, row.SessionToken)
	require.Equal(t, uint64(1), row.Sequence)
}

func TestPrivacyRevocationRemainsRateLimitedWithoutWorker(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	require.ErrorIs(t, s.RateLimitRequest(ctx, 1, false, true), ErrUnavailable)
	for i := 0; i < 60; i++ {
		require.NoError(t, s.RateLimitRequest(ctx, 1, false, false))
	}
	require.ErrorIs(t, s.RateLimitRequest(ctx, 1, false, false), ErrRateLimited)
}

func TestHistoryFlushRespectsShardLease(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: "visible"}
	opened, err := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "lease-session"})
	require.NoError(t, err)
	_, err = s.ReportHistory(ctx, 1, req.HistoryReport{Resource: r, SessionToken: opened.SessionToken, SessionVersion: opened.SessionVersion, Epoch: opened.Epoch, Sequence: 1})
	require.NoError(t, err)
	key, queue := historyKeys(1, r)
	lease := strings.TrimSuffix(queue, ":pending") + ":lease"
	require.NoError(t, s.Redis.ZAdd(ctx, queue, redis.Z{Score: 0, Member: key}).Err())
	require.NoError(t, s.Redis.Set(ctx, lease, "other-worker", time.Minute).Err())
	require.NoError(t, s.FlushHistory(ctx))
	require.Zero(t, s.Metrics.HistoryPersisted.Load())
	require.Equal(t, "other-worker", s.Redis.Get(ctx, lease).Val())
	require.NoError(t, s.Redis.Del(ctx, lease).Err())
	require.NoError(t, s.FlushHistory(ctx))
	require.Equal(t, uint64(1), s.Metrics.HistoryPersisted.Load())
}

func isolatedRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("SOCIAL_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set SOCIAL_TEST_REDIS_ADDR=127.0.0.1:16389 for isolated compose Redis")
	}
	require.Equal(t, "127.0.0.1:16389", addr, "refuse non-isolated Redis")
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	require.NoError(t, client.FlushDB(context.Background()).Err())
	t.Cleanup(func() { client.Close() })
	return client
}
func TestRedisStaleFillCannotOverwriteNewVersion(t *testing.T) {
	client := isolatedRedis(t)
	s := testService(t)
	s.Redis = client
	ctx := context.Background()
	changeForTest(t, s, 1, 2, Follow)
	oldKey, e := s.relationshipCacheKey(ctx, 1, []uint64{2})
	require.NoError(t, e)
	rows, e := s.BatchStatus(ctx, 1, []req.Target{target(2)})
	require.NoError(t, e)
	require.True(t, rows[0].Following)
	changeForTest(t, s, 2, 1, Block)
	stale, _ := json.Marshal([]res.Relationship{{User: res.User{AccountNo: "2"}, Following: true}})
	require.NoError(t, client.Set(ctx, oldKey, stale, time.Minute).Err())
	rows, e = s.BatchStatus(ctx, 1, []req.Target{target(2)})
	require.NoError(t, e)
	require.False(t, rows[0].Following)
	require.True(t, rows[0].Blocked)
	require.NoError(t, s.DB.Model(&testUser{}).Where("id=2").Update("normal", false).Error)
	rows, e = s.BatchStatus(ctx, 1, []req.Target{target(2)})
	require.NoError(t, e)
	require.Empty(t, rows)
}
func TestRedisAggregationClearFenceAndQueueBound(t *testing.T) {
	client := isolatedRedis(t)
	s := testService(t)
	s.Redis = client
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceShortVideo, ResourceID: "visible"}
	opened, e := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "redis-open-0001"})
	require.NoError(t, e)
	report := req.HistoryReport{Resource: r, SessionToken: opened.SessionToken, SessionVersion: opened.SessionVersion, Epoch: opened.Epoch, Sequence: 1, PositionMs: 3000}
	for i := uint64(1); i <= 20; i++ {
		report.Sequence = i
		report.PositionMs = int64(i * 100)
		out, e := s.ReportHistory(ctx, 1, report)
		require.NoError(t, e)
		require.True(t, out.Accepted)
		require.False(t, out.Persisted)
	}
	key, queue := historyKeys(1, r)
	require.Equal(t, int64(1), client.ZCard(ctx, queue).Val())
	report.Sequence = 19
	report.PositionMs = 9000
	out, e := s.ReportHistory(ctx, 1, report)
	require.NoError(t, e)
	require.False(t, out.Accepted)
	require.NoError(t, client.ZAdd(ctx, queue, redis.Z{Score: 0, Member: key}).Err())
	require.NoError(t, s.FlushHistory(ctx))
	rows, e := s.HistoryProgress(ctx, 1, []req.Resource{r})
	require.NoError(t, e)
	require.Equal(t, int64(2000), rows.List[0].PositionMs)
	require.Equal(t, int64(0), client.ZCard(ctx, queue).Val())
	report.Sequence = 21
	_, e = s.ReportHistory(ctx, 1, report)
	require.NoError(t, e)
	require.NoError(t, s.DeleteHistory(ctx, 1, nil))
	require.NoError(t, client.ZAdd(ctx, queue, redis.Z{Score: 0, Member: key}).Err())
	require.NoError(t, s.FlushHistory(ctx))
	history, e := s.HistoryList(ctx, 1, req.CursorSearch{})
	require.NoError(t, e)
	require.Empty(t, history.List)
	// 有界队列满时明确拒绝，不通过无限创建key继续增长。
	members := make([]redis.Z, 1000)
	for i := range members {
		members[i] = redis.Z{Score: float64(time.Now().UnixMilli()), Member: formatID(uint64(i + 1))}
	}
	require.NoError(t, client.ZAdd(ctx, queue, members...).Err())
	report.Sequence = 22
	_, e = s.ReportHistory(ctx, 1, report)
	require.ErrorIs(t, e, ErrRateLimited)
}

func TestRedisHeartbeatLoadProfile(t *testing.T) {
	if os.Getenv("SOCIAL_RUN_LOAD") != "1" {
		t.Skip("SOCIAL_RUN_LOAD=1")
	}
	client := isolatedRedis(t)
	s := testService(t)
	s.Redis = client
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceShortVideo, ResourceID: "visible"}
	opened, err := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "redis-load-001"})
	require.NoError(t, err)
	times := make([]time.Duration, 1000)
	start := time.Now()
	for i := range times {
		at := time.Now()
		_, err := s.ReportHistory(ctx, 1, req.HistoryReport{Resource: r, SessionToken: opened.SessionToken, SessionVersion: opened.SessionVersion, Epoch: opened.Epoch, Sequence: uint64(i + 1), PositionMs: int64(i)})
		require.NoError(t, err)
		times[i] = time.Since(at)
	}
	elapsed := time.Since(start)
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	t.Logf("buffered_heartbeat requests=1000 qps=%.2f p95_ms=%.3f p99_ms=%.3f errors=0", 1000/elapsed.Seconds(), float64(times[949].Microseconds())/1000, float64(times[989].Microseconds())/1000)
	key, queue := historyKeys(1, r)
	require.Equal(t, int64(1), client.ZCard(ctx, queue).Val())
	require.NoError(t, client.ZAdd(ctx, queue, redis.Z{Score: 0, Member: key}).Err())
	require.NoError(t, s.FlushHistory(ctx))
	require.Equal(t, uint64(1), s.Metrics.HistoryPersisted.Load())
}

func TestWorkerReadinessAndBackpressureFailClosed(t *testing.T) {
	client := isolatedRedis(t)
	s := testService(t)
	s.Redis = client
	ctx := context.Background()
	require.ErrorIs(t, s.RateLimit(ctx, 1, false), ErrUnavailable)
	require.NoError(t, s.refreshWorkerPressure(ctx))
	require.NoError(t, s.RateLimit(ctx, 1, false))
	require.NoError(t, client.Set(ctx, redisPrefix+"worker:pressure", 1, time.Second).Err())
	require.ErrorIs(t, s.RateLimit(ctx, 1, false), ErrRateLimited)
}
