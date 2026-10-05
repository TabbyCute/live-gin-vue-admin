package initialize

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"tb_live_module/global"

	"github.com/redis/go-redis/v9"
)

const (
	defaultLiveTaskLeaderLease = 60 * time.Second
	liveTaskLeaseOperationTime = 2 * time.Second
)

var errLiveTaskLeaderLost = errors.New("直播定时任务Leader租约已丢失")

var (
	liveTaskLeaderFallbackLogMu sync.Mutex
	liveTaskLeaderFallbackLogAt = make(map[string]time.Time)
)

type liveTaskLeaseStore interface {
	TryAcquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Renew(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key, token string) error
}

type redisLiveTaskLeaseStore struct {
	client redis.UniversalClient
}

var liveTaskLeaseStoreProvider = func() liveTaskLeaseStore {
	if global.GVA_REDIS == nil {
		return nil
	}
	return redisLiveTaskLeaseStore{client: global.GVA_REDIS}
}

func (s redisLiveTaskLeaseStore) TryAcquire(
	ctx context.Context, key, token string, ttl time.Duration,
) (bool, error) {
	return s.client.SetNX(ctx, key, token, ttl).Result()
}

func (s redisLiveTaskLeaseStore) Renew(
	ctx context.Context, key, token string, ttl time.Duration,
) (bool, error) {
	result, err := s.client.Eval(ctx, `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("pexpire", KEYS[1], ARGV[2])
end
return 0`, []string{key}, token, ttl.Milliseconds()).Int64()
	return result == 1, err
}

func (s redisLiveTaskLeaseStore) Release(ctx context.Context, key, token string) error {
	_, err := s.client.Eval(ctx, `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0`, []string{key}, token).Result()
	return err
}

// runLiveTaskWithLeader 为每类Cron任务持有独立的Redis租约。
// 多个任务仍可并行，但同一任务在所有后端实例中同一时刻只有一个执行者。
// 任务执行时间超过租约时会自动续期。开启fail-open后，Redis不可用会退回各实例有界执行；
// 各任务仍依靠数据库租约、条件更新和事务保证幂等，避免直播状态机整体冻结。
func runLiveTaskWithLeader(taskName string, run func() error) error {
	if !global.GVA_CONFIG.Live.TaskLeaderEnabled {
		return run()
	}
	store := liveTaskLeaseStoreProvider()
	if store == nil {
		leaderErr := errors.New("直播定时任务Leader已启用，但Redis不可用")
		return runLiveTaskLeaderFallback(taskName, run, leaderErr)
	}
	token, err := newLiveTaskLeaseToken()
	if err != nil {
		return err
	}
	lease := liveTaskLeaderLease()
	key := liveTaskLeaderKey(taskName)
	ctx, cancel := context.WithTimeout(context.Background(), liveTaskLeaseOperationTime)
	acquired, err := store.TryAcquire(ctx, key, token, lease)
	cancel()
	if err != nil {
		return runLiveTaskLeaderFallback(taskName, run, fmt.Errorf("获取%s Leader租约失败: %w", taskName, err))
	}
	if !acquired {
		return nil
	}

	stopRenew := make(chan struct{})
	renewDone := make(chan error, 1)
	go renewLiveTaskLease(store, key, token, lease, stopRenew, renewDone)
	runErr := run()
	close(stopRenew)
	renewErr := <-renewDone

	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), liveTaskLeaseOperationTime)
	releaseErr := store.Release(releaseCtx, key, token)
	releaseCancel()
	if releaseErr != nil {
		releaseErr = fmt.Errorf("释放%s Leader租约失败: %w", taskName, releaseErr)
	}
	return errors.Join(runErr, renewErr, releaseErr)
}

func runLiveTaskLeaderFallback(taskName string, run func() error, leaderErr error) error {
	if !global.GVA_CONFIG.Live.TaskLeaderFailOpen {
		return leaderErr
	}
	runErr := run()
	// Redis持续故障时readiness每秒运行一次；同类Leader告警每分钟最多一条，避免日志反向放大。
	if shouldReportLiveTaskLeaderFallback(taskName, time.Now()) {
		return errors.Join(leaderErr, runErr)
	}
	return runErr
}

func shouldReportLiveTaskLeaderFallback(taskName string, now time.Time) bool {
	liveTaskLeaderFallbackLogMu.Lock()
	defer liveTaskLeaderFallbackLogMu.Unlock()
	last := liveTaskLeaderFallbackLogAt[taskName]
	if !last.IsZero() && now.Sub(last) < time.Minute {
		return false
	}
	liveTaskLeaderFallbackLogAt[taskName] = now
	return true
}

func renewLiveTaskLease(
	store liveTaskLeaseStore,
	key, token string,
	lease time.Duration,
	stop <-chan struct{},
	done chan<- error,
) {
	ticker := time.NewTicker(lease / 3)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			done <- nil
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), liveTaskLeaseOperationTime)
			renewed, err := store.Renew(ctx, key, token, lease)
			cancel()
			if err != nil {
				done <- fmt.Errorf("续期Leader租约失败: %w", err)
				return
			}
			if !renewed {
				done <- errLiveTaskLeaderLost
				return
			}
		}
	}
}

func liveTaskLeaderLease() time.Duration {
	seconds := global.GVA_CONFIG.Live.TaskLeaderLeaseSeconds
	if seconds <= 0 {
		return defaultLiveTaskLeaderLease
	}
	if seconds < 10 {
		seconds = 10
	}
	if seconds > 600 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
}

func newLiveTaskLeaseToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("生成Leader租约令牌失败: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func liveTaskLeaderKey(taskName string) string {
	identity := strings.TrimSpace(global.GVA_CONFIG.Live.SRS.HTTPAPIs) + "\x00" +
		strings.Trim(strings.TrimSpace(global.GVA_CONFIG.Live.SRS.App), "/")
	digest := sha256.Sum256([]byte(identity))
	return "live:task-leader:v1:" + hex.EncodeToString(digest[:]) + ":" + taskName
}
