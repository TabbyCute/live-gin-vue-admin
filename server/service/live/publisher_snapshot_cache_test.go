package live

import (
	"context"
	"sync"
	"testing"
	"time"

	"tb_live_module/global"
	srsclient "tb_live_module/internal/srs"

	"github.com/stretchr/testify/require"
)

type memoryPublisherSnapshotCache struct {
	mu      sync.Mutex
	values  map[string]memoryLivePrepareResult
	locks   map[string]string
	setErr  error
	lockErr error
}

func newMemoryPublisherSnapshotCache() *memoryPublisherSnapshotCache {
	return &memoryPublisherSnapshotCache{
		values: make(map[string]memoryLivePrepareResult),
		locks:  make(map[string]string),
	}
}

func (c *memoryPublisherSnapshotCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[key]
	if !ok || time.Now().After(value.expiresAt) {
		return "", errLivePublisherSnapshotCacheMiss
	}
	return value.value, nil
}

func (c *memoryPublisherSnapshotCache) Set(
	_ context.Context, key, value string, ttl time.Duration,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	c.values[key] = memoryLivePrepareResult{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (c *memoryPublisherSnapshotCache) TryLock(
	_ context.Context, key, token string, _ time.Duration,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lockErr != nil {
		return false, c.lockErr
	}
	if _, exists := c.locks[key]; exists {
		return false, nil
	}
	c.locks[key] = token
	return true, nil
}

func (c *memoryPublisherSnapshotCache) Unlock(_ context.Context, key, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locks[key] == token {
		delete(c.locks, key)
	}
	return nil
}

func TestPublisherSnapshotCacheCoalescesConcurrentTaskRequests(t *testing.T) {
	setupRoomTestDB(t)
	global.GVA_CONFIG.Live.SRSSnapshotCacheMilliseconds = 1000
	controller := &fakeSRSController{
		snapshots: []srsclient.PublisherSnapshot{roomTestPublisherSnapshot()},
		listDelay: 30 * time.Millisecond,
	}
	cache := newMemoryPublisherSnapshotCache()
	first := RoomService{srsController: controller, snapshotCache: cache}
	second := RoomService{srsController: controller, snapshotCache: cache}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	results := make(chan []srsclient.PublisherSnapshot, 2)
	errs := make(chan error, 2)
	for _, service := range []*RoomService{&first, &second} {
		go func(service *RoomService) {
			snapshots, err := service.loadPublisherSnapshots(ctx, controller)
			results <- snapshots
			errs <- err
		}(service)
	}
	for range 2 {
		require.NoError(t, <-errs)
		require.Len(t, <-results, 1)
	}
	require.Equal(t, 1, controller.listCallCount(), "并发任务和实例必须共用一次SRS快照刷新")

	// 缓存新鲜期内的后续任务仍然复用同一份结果。
	snapshots, err := first.loadPublisherSnapshots(ctx, controller)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, 1, controller.listCallCount())
}
