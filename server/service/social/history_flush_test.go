package social

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
)

// 注入每次网络往返的延迟；pipeline只延迟一次，复现跨网络部署而非本机零延迟。
type historyLatencyHook struct {
	delay     time.Duration
	commands  atomic.Int64
	pipelines atomic.Int64
}

func (h *historyLatencyHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func waitHistoryTest(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (h *historyLatencyHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.commands.Add(1)
		if e := waitHistoryTest(ctx, h.delay); e != nil {
			return e
		}
		return next(ctx, cmd)
	}
}
func (h *historyLatencyHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		h.pipelines.Add(1)
		if e := waitHistoryTest(ctx, h.delay); e != nil {
			return e
		}
		return next(ctx, cmds)
	}
}

var _ redis.Hook = (*historyLatencyHook)(nil)

func TestRedisEmptyHistoryFlushLatencyBudget(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	hook := &historyLatencyHook{delay: 50 * time.Millisecond}
	s.Redis.AddHook(hook)
	// 原来的逐分片扫描，即使零记录也在短预算内超时。
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	var err error
	for shard := 0; shard < historyShards; shard++ {
		if err = s.flushHistoryShard(ctx, shard); err != nil {
			break
		}
	}
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	hook.commands.Store(0)
	hook.pipelines.Store(0)
	ctx, cancel = context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	require.NoError(t, s.FlushHistory(ctx))
	require.Zero(t, hook.commands.Load(), "空分片不能加锁/解锁或发单独命令")
	require.EqualValues(t, 1, hook.pipelines.Load())
}

func TestRedisHistoryProbeFailureDoesNotStarveOtherShards(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceShortVideo, ResourceID: "visible"}
	_, goodQueue := historyKeys(1, r)
	badShard := 0
	if goodQueue == redisPrefix+"{history-00}:pending" {
		badShard = 1
	}
	badQueue := fmt.Sprintf("%s{history-%02d}:pending", redisPrefix, badShard)
	opened, e := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "probe-failure-session"})
	require.NoError(t, e)
	_, e = s.ReportHistory(ctx, 1, req.HistoryReport{Resource: r, SessionToken: opened.SessionToken, SessionVersion: opened.SessionVersion, Epoch: opened.Epoch, Sequence: 1})
	require.NoError(t, e)
	key, _ := historyKeys(1, r)
	require.NoError(t, s.Redis.ZAdd(ctx, goodQueue, redis.Z{Score: 0, Member: key}).Err())
	require.NoError(t, s.Redis.Set(ctx, badQueue, "wrong-type", 0).Err())
	e = s.FlushHistory(ctx)
	var step *workerStepError
	require.ErrorAs(t, e, &step)
	require.Equal(t, "history_probe", step.stage)
	require.Equal(t, badShard, step.shard)
	require.EqualValues(t, 1, s.Metrics.HistoryPersisted.Load(), "坏分片不能阻塞健康分片")
	require.Zero(t, s.Redis.ZCard(ctx, goodQueue).Val())
	require.Equal(t, "wrong-type", s.Redis.Get(ctx, badQueue).Val(), "不能为消除告警擅自删除坏队列")
	require.NoError(t, s.Redis.Del(ctx, badQueue).Err())
	require.NoError(t, s.FlushHistory(ctx))
}

func TestRedisHistoryProbeDeadline(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	s.Redis.AddHook(&historyLatencyHook{delay: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.True(t, errors.Is(s.FlushHistory(ctx), context.Canceled))
}

func TestRedisHistoryColdLeaseReleaseWithNetworkLatency(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	require.NoError(t, s.Redis.ScriptFlush(ctx).Err())
	s.Redis.AddHook(&historyLatencyHook{delay: 180 * time.Millisecond})
	require.NoError(t, s.flushHistoryShard(ctx, 0))
	// 首次执行脚本且每次往返超过原150ms预算，仍立即释放自己的租约。
	require.Zero(t, s.Redis.Exists(ctx, redisPrefix+"{history-00}:lease").Val())
}
