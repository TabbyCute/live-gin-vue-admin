package live

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"tb_live_module/global"
	srsclient "tb_live_module/internal/srs"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var errLivePublisherSnapshotCacheMiss = errors.New("SRS发布流快照缓存不存在")

const maxLivePublisherSnapshotCacheBytes = 16 << 20

type livePublisherSnapshotCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	TryLock(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, key, token string) error
}

type redisLivePublisherSnapshotCache struct {
	client redis.UniversalClient
}

type localLivePublisherSnapshotCache struct {
	mu     sync.Mutex
	values map[string]localLivePublisherSnapshotValue
	locks  map[string]localLivePublisherSnapshotLock
}

type localLivePublisherSnapshotValue struct {
	value     string
	expiresAt time.Time
}

type localLivePublisherSnapshotLock struct {
	token     string
	expiresAt time.Time
}

var processLivePublisherSnapshotCache = &localLivePublisherSnapshotCache{
	values: make(map[string]localLivePublisherSnapshotValue),
	locks:  make(map[string]localLivePublisherSnapshotLock),
}

type cachedLivePublisherSnapshot struct {
	CapturedAt int64                         `json:"capturedAt"`
	ServerID   string                        `json:"serverId"`
	ServiceID  string                        `json:"serviceId"`
	Publishers []srsclient.PublisherSnapshot `json:"publishers"`
}

func (c redisLivePublisherSnapshotCache) Get(ctx context.Context, key string) (string, error) {
	value, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", errLivePublisherSnapshotCacheMiss
	}
	return value, err
}

func (c redisLivePublisherSnapshotCache) Set(
	ctx context.Context, key, value string, ttl time.Duration,
) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c redisLivePublisherSnapshotCache) TryLock(
	ctx context.Context, key, token string, ttl time.Duration,
) (bool, error) {
	return c.client.SetNX(ctx, key, token, ttl).Result()
}

func (c redisLivePublisherSnapshotCache) Unlock(ctx context.Context, key, token string) error {
	_, err := c.client.Eval(ctx, `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0`, []string{key}, token).Result()
	return err
}

func (c *localLivePublisherSnapshotCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, exists := c.values[key]
	if !exists || time.Now().After(value.expiresAt) {
		delete(c.values, key)
		return "", errLivePublisherSnapshotCacheMiss
	}
	return value.value, nil
}

func (c *localLivePublisherSnapshotCache) Set(
	_ context.Context, key, value string, ttl time.Duration,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = localLivePublisherSnapshotValue{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (c *localLivePublisherSnapshotCache) TryLock(
	_ context.Context, key, token string, ttl time.Duration,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if lock, exists := c.locks[key]; exists && time.Now().Before(lock.expiresAt) {
		return false, nil
	}
	c.locks[key] = localLivePublisherSnapshotLock{token: token, expiresAt: time.Now().Add(ttl)}
	return true, nil
}

func (c *localLivePublisherSnapshotCache) Unlock(_ context.Context, key, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if lock, exists := c.locks[key]; exists && lock.token == token {
		delete(c.locks, key)
	}
	return nil
}

func (s *RoomService) publisherSnapshotCacheStore() livePublisherSnapshotCache {
	if s.snapshotCache != nil {
		return s.snapshotCache
	}
	if global.GVA_REDIS == nil {
		return processLivePublisherSnapshotCache
	}
	return redisLivePublisherSnapshotCache{client: global.GVA_REDIS}
}

// loadPublisherSnapshots 使用短时进程/Redis快照和刷新锁协调readiness、reconcile及多后端实例。
// 过期快照绝不会用于断流判断；Redis异常时退回直接查询SRS，保证单实例可用性。
func (s *RoomService) loadPublisherSnapshots(
	ctx context.Context, controller srsclient.Controller,
) ([]srsclient.PublisherSnapshot, error) {
	batch, err := s.loadPublisherSnapshotBatch(ctx, controller)
	return batch.Publishers, err
}

func (s *RoomService) loadPublisherSnapshotBatch(
	ctx context.Context, controller srsclient.Controller,
) (srsclient.PublisherBatch, error) {
	maxAge := liveSRSSnapshotCacheDuration()
	cache := s.publisherSnapshotCacheStore()
	if maxAge <= 0 || cache == nil {
		return queryPublisherSnapshotBatch(ctx, controller)
	}
	cacheKey := livePublisherSnapshotCacheKey()
	if batch, found, err := loadFreshPublisherSnapshot(ctx, cache, cacheKey, maxAge); err == nil && found {
		return batch, nil
	} else if err != nil {
		logLiveSnapshotCacheError("读取SRS发布流快照缓存失败，退回直接查询", err)
		return queryPublisherSnapshotBatch(ctx, controller)
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return srsclient.PublisherBatch{}, err
	}
	token := hex.EncodeToString(tokenBytes)
	lockKey := cacheKey + ":refresh"
	lockTTL := liveSRSRequestTimeout() + 2*time.Second
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		acquired, err := cache.TryLock(ctx, lockKey, token, lockTTL)
		if err != nil {
			logLiveSnapshotCacheError("获取SRS发布流快照刷新锁失败，退回直接查询", err)
			return queryPublisherSnapshotBatch(ctx, controller)
		}
		if acquired {
			return s.refreshPublisherSnapshotBatch(ctx, controller, cache, cacheKey, lockKey, token, maxAge)
		}
		select {
		case <-ctx.Done():
			return srsclient.PublisherBatch{}, ctx.Err()
		case <-ticker.C:
			batch, found, getErr := loadFreshPublisherSnapshot(ctx, cache, cacheKey, maxAge)
			if getErr != nil {
				logLiveSnapshotCacheError("等待共享SRS快照时Redis读取失败，退回直接查询", getErr)
				return queryPublisherSnapshotBatch(ctx, controller)
			}
			if found {
				return batch, nil
			}
		}
	}
}

func (s *RoomService) refreshPublisherSnapshots(
	ctx context.Context,
	controller srsclient.Controller,
	cache livePublisherSnapshotCache,
	cacheKey, lockKey, token string,
	maxAge time.Duration,
) ([]srsclient.PublisherSnapshot, error) {
	batch, err := s.refreshPublisherSnapshotBatch(ctx, controller, cache, cacheKey, lockKey, token, maxAge)
	return batch.Publishers, err
}

func (s *RoomService) refreshPublisherSnapshotBatch(
	ctx context.Context,
	controller srsclient.Controller,
	cache livePublisherSnapshotCache,
	cacheKey, lockKey, token string,
	maxAge time.Duration,
) (srsclient.PublisherBatch, error) {
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := cache.Unlock(unlockCtx, lockKey, token); err != nil {
			logLiveSnapshotCacheError("释放SRS发布流快照刷新锁失败", err)
		}
	}()

	// 拿到锁后再读一次，避免锁竞争期间另一个实例已经完成刷新。
	if batch, found, err := loadFreshPublisherSnapshot(ctx, cache, cacheKey, maxAge); err == nil && found {
		return batch, nil
	}
	batch, err := queryPublisherSnapshotBatch(ctx, controller)
	if err != nil {
		return srsclient.PublisherBatch{}, err
	}
	if batch.CapturedAt <= 0 {
		batch.CapturedAt = time.Now().UnixMilli()
	}
	payload, err := json.Marshal(cachedLivePublisherSnapshot{
		CapturedAt: batch.CapturedAt,
		ServerID:   batch.Runtime.ServerID, ServiceID: batch.Runtime.ServiceID,
		Publishers: batch.Publishers,
	})
	if err != nil {
		return srsclient.PublisherBatch{}, err
	}
	if len(payload) > maxLivePublisherSnapshotCacheBytes {
		// SRS结果仍可用于本轮；只拒绝把异常大的聚合值写入Redis，避免缓存和网络被放大。
		logLiveSnapshotCacheError("SRS发布流快照超过共享缓存上限，跳过缓存", fmt.Errorf(
			"payload=%d limit=%d", len(payload), maxLivePublisherSnapshotCacheBytes,
		))
		return batch, nil
	}
	if err = cache.Set(ctx, cacheKey, string(payload), 2*maxAge); err != nil {
		// SRS结果本身有效，不因共享缓存写入失败把本轮业务处理判为失败。
		logLiveSnapshotCacheError("保存SRS发布流共享快照失败", err)
	}
	return batch, nil
}

func loadFreshPublisherSnapshot(
	ctx context.Context,
	cache livePublisherSnapshotCache,
	key string,
	maxAge time.Duration,
) (srsclient.PublisherBatch, bool, error) {
	value, err := cache.Get(ctx, key)
	if errors.Is(err, errLivePublisherSnapshotCacheMiss) {
		return srsclient.PublisherBatch{}, false, nil
	}
	if err != nil {
		return srsclient.PublisherBatch{}, false, err
	}
	var cached cachedLivePublisherSnapshot
	if err = json.Unmarshal([]byte(value), &cached); err != nil {
		return srsclient.PublisherBatch{}, false, err
	}
	ageMillis := time.Now().UnixMilli() - cached.CapturedAt
	if cached.CapturedAt <= 0 || ageMillis < 0 || ageMillis > maxAge.Milliseconds() {
		return srsclient.PublisherBatch{}, false, nil
	}
	return srsclient.PublisherBatch{
		CapturedAt: cached.CapturedAt,
		FromCache:  true,
		Runtime:    srsclient.RuntimeIdentity{ServerID: cached.ServerID, ServiceID: cached.ServiceID},
		Publishers: cached.Publishers,
	}, true, nil
}

func queryPublisherSnapshotBatch(
	ctx context.Context, controller srsclient.Controller,
) (srsclient.PublisherBatch, error) {
	if runtimeController, ok := controller.(srsclient.RuntimeController); ok {
		return runtimeController.ListPublishersWithRuntime(ctx)
	}
	publishers, err := controller.ListPublishers(ctx)
	return srsclient.PublisherBatch{CapturedAt: time.Now().UnixMilli(), Publishers: publishers}, err
}

func livePublisherSnapshotCacheKey() string {
	identity := strings.TrimSpace(global.GVA_CONFIG.Live.SRS.HTTPAPIs) + "\x00" +
		strings.Trim(strings.TrimSpace(global.GVA_CONFIG.Live.SRS.App), "/")
	digest := sha256.Sum256([]byte(identity))
	return "live:srs-publishers:v1:" + hex.EncodeToString(digest[:])
}

func liveSRSSnapshotCacheDuration() time.Duration {
	milliseconds := global.GVA_CONFIG.Live.SRSSnapshotCacheMilliseconds
	if milliseconds <= 0 {
		return 0
	}
	if milliseconds < 100 {
		milliseconds = 100
	}
	if milliseconds > 5000 {
		milliseconds = 5000
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func logLiveSnapshotCacheError(message string, err error) {
	if global.GVA_LOG != nil {
		global.GVA_LOG.Warn(message, zap.Error(err))
	}
}
