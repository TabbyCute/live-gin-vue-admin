package social

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
	"math/rand/v2"
	"sort"
	model "tb_live_module/model/social"
	"time"
)

const redisPrefix = "social:v1:"

var fallbackReadLimiter = rate.NewLimiter(100, 100)

func fallbackReadLimit() error {
	if fallbackReadLimiter.Allow() {
		return nil
	}
	return ErrRateLimited
}

func (s *Service) relationshipCacheKey(ctx context.Context, actor uint64, ids []uint64) (string, error) {
	all := uniqueIDs(append([]uint64{actor}, ids...))
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	var rows []model.SocialUserCounter
	if err := s.db(ctx).Select("user_id", "version").Where("user_id IN ?", all).Order("user_id").Find(&rows).Error; err != nil {
		return "", err
	}
	versions := map[uint64]uint64{}
	for _, row := range rows {
		versions[row.UserID] = row.Version
	}
	b, _ := json.Marshal(struct {
		Actor    uint64
		IDs      []uint64
		Versions map[uint64]uint64
	}{actor, ids, versions})
	sum := sha256.Sum256(b)
	return redisPrefix + "relationship:" + hex.EncodeToString(sum[:]), nil
}
func (s *Service) cacheGet(ctx context.Context, key string, out interface{}) bool {
	if s.Redis == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	raw, err := s.Redis.Get(ctx, key).Bytes()
	if err != nil || json.Unmarshal(raw, out) != nil {
		s.Metrics.CacheMisses.Add(1)
		return false
	}
	s.Metrics.CacheHits.Add(1)
	return true
}
func (s *Service) cacheSet(ctx context.Context, key string, value interface{}) {
	if s.Redis == nil {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 65536 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_ = s.Redis.Set(ctx, key, raw, time.Duration(5+rand.IntN(6))*time.Second).Err()
}

var rateScript = redis.NewScript(`local n=redis.call('INCR',KEYS[1]);if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[2]) end;return n<=tonumber(ARGV[1]) and 1 or 0`)

// RateLimit Redis 不可用时拒绝敏感写；不使用本地计数冒充多实例限流。
func (s *Service) RateLimit(ctx context.Context, actor uint64, read bool) error {
	return s.RateLimitRequest(ctx, actor, read, true)
}

// RateLimitRequest 隐私撤销操作保留Redis限流，但不依赖异步Worker的健康状态。
func (s *Service) RateLimitRequest(ctx context.Context, actor uint64, read, workerRequired bool) error {
	if s.Redis == nil {
		if read {
			return fallbackReadLimit()
		}
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	if !read && workerRequired {
		pressure, err := s.Redis.Get(ctx, redisPrefix+"worker:pressure").Int()
		if err != nil {
			return ErrUnavailable
		}
		if pressure > 0 {
			return ErrRateLimited
		}
	}
	limit := 60
	bucket := "write"
	if read {
		limit = 600
		bucket = "read"
	}
	n, err := rateScript.Run(ctx, s.Redis, []string{fmt.Sprintf("%srate:%s:%d", redisPrefix, bucket, actor)}, limit, 60000).Int()
	if err != nil {
		if read {
			return fallbackReadLimit()
		}
		return ErrUnavailable
	}
	if n != 1 {
		return ErrRateLimited
	}
	return nil
}
