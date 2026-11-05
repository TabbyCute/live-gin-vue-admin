package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"tb_live_module/global"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	"tb_live_module/model/system"
	"testing"
	"time"
)

func TestRedisRequestBudgetsWithColdScriptsAndLatency(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	require.NoError(t, s.refreshWorkerPressure(ctx))
	require.NoError(t, s.Redis.ScriptFlush(ctx).Err())
	hook := &historyLatencyHook{delay: 180 * time.Millisecond}
	s.Redis.AddHook(hook)
	start := time.Now()
	require.NoError(t, s.RateLimit(ctx, 1, false))
	require.EqualValues(t, 1, hook.pipelines.Load())
	require.Zero(t, hook.commands.Load())
	t.Logf("cold write guard at 180ms RTT: %s", time.Since(start))
	r := req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: "visible"}
	opened, err := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "latency-session"})
	require.NoError(t, err)
	report := req.HistoryReport{Resource: r, SessionToken: opened.SessionToken, SessionVersion: opened.SessionVersion, Epoch: opened.Epoch, Sequence: 1}
	_, err = s.ReportHistory(ctx, 1, report)
	require.NoError(t, err)
	s.cacheSet(ctx, "social:test:latency", []int{1})
	var cached []int
	require.True(t, s.cacheGet(ctx, "social:test:latency", &cached))
	require.Equal(t, []int{1}, cached)
}

func TestHistoryAdmissionRejectsForgedSessionsWithoutQueueGrowth(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: "visible"}
	opened, err := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "admission-session"})
	require.NoError(t, err)
	report := req.HistoryReport{Resource: r, SessionToken: opened.SessionToken, SessionVersion: opened.SessionVersion, Epoch: opened.Epoch, Sequence: 1}
	for _, actor := range []uint64{2, 3} {
		_, err = s.ReportHistory(ctx, actor, report)
		require.ErrorIs(t, err, ErrConflict)
	}
	for i := 0; i < 32; i++ {
		fake := report
		fake.ResourceID = fmt.Sprintf("invented-%d", i)
		fake.SessionToken = uuid.NewString()
		_, err = s.ReportHistory(ctx, 1, fake)
		require.ErrorIs(t, err, ErrConflict)
	}
	for shard := 0; shard < historyShards; shard++ {
		require.Zero(t, s.Redis.ZCard(ctx, fmt.Sprintf("%s{history-%02d}:pending", redisPrefix, shard)).Val())
	}
	queries := 0
	require.NoError(t, s.DB.Callback().Query().Before("gorm:query").Register("test:admission", func(*gorm.DB) { queries++ }))
	t.Cleanup(func() { s.DB.Callback().Query().Remove("test:admission") })
	for i := 1; i <= 20; i++ {
		report.Sequence = uint64(i)
		_, err = s.ReportHistory(ctx, 1, report)
		require.NoError(t, err)
	}
	require.Equal(t, 1, queries, "连续20次心跳仅首次验证会话，不逐次查询MySQL")
	key, _ := historyKeys(1, r)
	before := s.Redis.Get(ctx, key).Val()
	report.SessionVersion++
	_, err = s.ReportHistory(ctx, 1, report)
	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, before, s.Redis.Get(ctx, key).Val())
}

func TestHistoryAcknowledgementsBatchAndPreserveNewerReports(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	// 32条已失效但格式正确的会话在同一分片，证明确认不会发32次网络请求。
	queue := redisPrefix + "{history-00}:pending"
	for i, n := 0, 0; n < 32; i++ {
		r := req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: fmt.Sprintf("retired-%d", i)}
		key, q := historyKeys(1, r)
		if q != queue {
			continue
		}
		raw, _ := json.Marshal(bufferedHistory{Actor: 1, Report: req.HistoryReport{Resource: r, SessionToken: uuid.NewString(), SessionVersion: 1, Sequence: 1}})
		require.NoError(t, s.Redis.Set(ctx, key, raw, time.Minute).Err())
		require.NoError(t, s.Redis.ZAdd(ctx, q, redis.Z{Member: key, Score: 0}).Err())
		n++
	}
	hook := &historyLatencyHook{delay: 30 * time.Millisecond}
	s.Redis.AddHook(hook)
	start := time.Now()
	require.NoError(t, s.FlushHistory(ctx))
	require.EqualValues(t, 2, hook.pipelines.Load(), "一次探测+一次批量确认")
	require.EqualValues(t, 4, hook.commands.Load(), "加锁、读队列、MGET、释放租约")
	t.Logf("32 acknowledgements at 30ms RTT: %s", time.Since(start))
	key := "social:v1:{history-00}:item:cas-test"
	require.NoError(t, s.Redis.Set(ctx, key, "new", time.Minute).Err())
	require.NoError(t, s.Redis.ZAdd(ctx, queue, redis.Z{Member: key}).Err())
	require.NoError(t, ackHistoryScript.Eval(ctx, s.Redis, []string{key, queue}, "old").Err())
	require.Equal(t, "new", s.Redis.Get(ctx, key).Val())
	require.EqualValues(t, 1, s.Redis.ZCard(ctx, queue).Val())
}

func TestAdminCurrentStateAndMembership(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	require.NoError(t, s.DB.AutoMigrate(&system.SysUser{}, &system.SysUserAuthority{}))
	root := system.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 1}, Enable: 1}
	require.NoError(t, s.DB.Create(&root).Error)
	rootAllowed, rootErr := s.AdminAllowed(ctx, 1, 0)
	require.NoError(t, rootErr)
	require.True(t, rootAllowed)
	require.NoError(t, s.DB.Model(&root).Update("enable", 2).Error)
	rootAllowed, rootErr = s.AdminAllowed(ctx, 1, 0)
	require.NoError(t, rootErr)
	require.False(t, rootAllowed)
	u := system.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 10}, AuthorityId: 888, Enable: 1}
	require.NoError(t, s.DB.Create(&u).Error)
	allowed, err := s.AdminAllowed(ctx, 10, 888)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, s.DB.Model(&u).Update("enable", 2).Error)
	allowed, err = s.AdminAllowed(ctx, 10, 888)
	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t, s.DB.Model(&u).Updates(map[string]interface{}{"enable": 1, "authority_id": 9528}).Error)
	allowed, err = s.AdminAllowed(ctx, 10, 888)
	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t, s.DB.Exec("INSERT INTO sys_user_authority (sys_user_id,sys_authority_authority_id) VALUES (?,?)", 10, 888).Error)
	allowed, err = s.AdminAllowed(ctx, 10, 888)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, s.DB.Delete(&u).Error)
	allowed, err = s.AdminAllowed(ctx, 10, 888)
	require.NoError(t, err)
	require.False(t, allowed)
}

type cancelLeaseHook struct {
	cancel context.CancelFunc
	seen   map[string]bool
}

func (h *cancelLeaseHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *cancelLeaseHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *cancelLeaseHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "set" && strings.HasSuffix(fmt.Sprint(cmd.Args()[1]), ":lease") {
			h.seen[fmt.Sprint(cmd.Args()[1])] = true
			h.cancel()
			return ctx.Err()
		}
		return next(ctx, cmd)
	}
}

func TestHistoryShardsMakeProgressAcrossShortBudgets(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	for shard := 0; shard < historyShards; shard++ {
		queue := fmt.Sprintf("%s{history-%02d}:pending", redisPrefix, shard)
		require.NoError(t, s.Redis.ZAdd(context.Background(), queue, redis.Z{Member: "due"}).Err())
	}
	hook := &cancelLeaseHook{seen: map[string]bool{}}
	s.Redis.AddHook(hook)
	// 每轮只够尝试一个分片即取消；持续繁忙不能永远只处理固定的前缀。
	for i := 0; i < historyShards; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		hook.cancel = cancel
		require.ErrorIs(t, s.FlushHistory(ctx), context.Canceled)
		cancel()
	}
	require.Len(t, hook.seen, historyShards)
}

func TestHistoryPartialFailureAcknowledgesOnlyProcessedPrefix(t *testing.T) {
	s := testService(t)
	s.Redis = isolatedRedis(t)
	ctx := context.Background()
	queue := redisPrefix + "{history-00}:pending"
	keys := []string{}
	for i := 0; len(keys) < 3; i++ {
		r := req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: fmt.Sprintf("retired-%d", i)}
		key, q := historyKeys(1, r)
		if q != queue {
			continue
		}
		p := bufferedHistory{Actor: 1, Report: req.HistoryReport{Resource: r, SessionToken: uuid.NewString(), SessionVersion: 1, Sequence: 1}}
		if len(keys) == 0 {
			p.Actor = 2 // 故障注入：载荷身份与Redis key不符，不得持久化或阻塞后面的记录。
		}
		raw, err := json.Marshal(p)
		require.NoError(t, err)
		require.NoError(t, s.Redis.Set(ctx, key, raw, time.Minute).Err())
		require.NoError(t, s.Redis.ZAdd(ctx, queue, redis.Z{Member: key, Score: float64(len(keys))}).Err())
		keys = append(keys, key)
	}
	queries := 0
	failure := errors.New("injected database failure")
	require.NoError(t, s.DB.Callback().Query().Before("gorm:query").Register("test:history-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "social_watch_history" {
			queries++
			if queries == 2 {
				tx.AddError(failure)
			}
		}
	}))
	t.Cleanup(func() { s.DB.Callback().Query().Remove("test:history-failure") })
	require.ErrorIs(t, s.flushHistoryShard(ctx, 0), failure)
	require.Equal(t, []string{keys[2]}, s.Redis.ZRange(ctx, queue, 0, -1).Val(), "损坏/失效前缀已确认，数据库失败记录保留重试")
	require.Zero(t, s.Redis.Exists(ctx, keys[0], keys[1]).Val())
	require.EqualValues(t, 1, s.Redis.Exists(ctx, keys[2]).Val())
	require.EqualValues(t, 2, s.Metrics.HistoryDropped.Load())
	require.Zero(t, s.Redis.Exists(ctx, redisPrefix+"{history-00}:lease").Val())
}
