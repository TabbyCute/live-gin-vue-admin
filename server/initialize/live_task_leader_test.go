package initialize

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tb_live_module/config"
	"tb_live_module/global"

	"github.com/stretchr/testify/require"
)

type memoryLiveTaskLeaseStore struct {
	mu     sync.Mutex
	key    string
	token  string
	renews int
}

func (s *memoryLiveTaskLeaseStore) TryAcquire(
	_ context.Context, key, token string, _ time.Duration,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != "" {
		return false, nil
	}
	s.key, s.token = key, token
	return true, nil
}

func (s *memoryLiveTaskLeaseStore) Renew(
	_ context.Context, key, token string, _ time.Duration,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != key || s.token != token {
		return false, nil
	}
	s.renews++
	return true, nil
}

func (s *memoryLiveTaskLeaseStore) Release(_ context.Context, key, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == key && s.token == token {
		s.key, s.token = "", ""
	}
	return nil
}

func TestLiveTaskLeaderAllowsOnlyOneInstancePerTask(t *testing.T) {
	previousLive := global.GVA_CONFIG.Live
	previousProvider := liveTaskLeaseStoreProvider
	store := &memoryLiveTaskLeaseStore{}
	global.GVA_CONFIG.Live = config.Live{TaskLeaderEnabled: true, TaskLeaderLeaseSeconds: 30}
	liveTaskLeaseStoreProvider = func() liveTaskLeaseStore { return store }
	t.Cleanup(func() {
		global.GVA_CONFIG.Live = previousLive
		liveTaskLeaseStoreProvider = previousProvider
	})

	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	var runs atomic.Int32
	go func() {
		firstDone <- runLiveTaskWithLeader("reconcile", func() error {
			runs.Add(1)
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	require.NoError(t, runLiveTaskWithLeader("reconcile", func() error {
		runs.Add(1)
		return nil
	}))
	require.Equal(t, int32(1), runs.Load(), "未取得租约的实例必须跳过本轮")
	close(release)
	require.NoError(t, <-firstDone)

	require.NoError(t, runLiveTaskWithLeader("reconcile", func() error {
		runs.Add(1)
		return nil
	}))
	require.Equal(t, int32(2), runs.Load(), "前一轮释放后下一实例应能接管")
}

func TestLiveTaskLeaderFailOpenKeepsLifecycleMovingWithoutRedis(t *testing.T) {
	previousLive := global.GVA_CONFIG.Live
	previousProvider := liveTaskLeaseStoreProvider
	global.GVA_CONFIG.Live = config.Live{TaskLeaderEnabled: true, TaskLeaderFailOpen: true}
	liveTaskLeaseStoreProvider = func() liveTaskLeaseStore { return nil }
	liveTaskLeaderFallbackLogMu.Lock()
	delete(liveTaskLeaderFallbackLogAt, "fail-open-test")
	liveTaskLeaderFallbackLogMu.Unlock()
	t.Cleanup(func() {
		global.GVA_CONFIG.Live = previousLive
		liveTaskLeaseStoreProvider = previousProvider
	})

	var runs atomic.Int32
	err := runLiveTaskWithLeader("fail-open-test", func() error {
		runs.Add(1)
		return nil
	})
	require.ErrorContains(t, err, "Redis不可用", "降级执行仍必须保留可观察告警")
	require.Equal(t, int32(1), runs.Load())
}

func TestLiveTaskLeaderFailClosedSkipsLifecycleWithoutRedis(t *testing.T) {
	previousLive := global.GVA_CONFIG.Live
	previousProvider := liveTaskLeaseStoreProvider
	global.GVA_CONFIG.Live = config.Live{TaskLeaderEnabled: true, TaskLeaderFailOpen: false}
	liveTaskLeaseStoreProvider = func() liveTaskLeaseStore { return nil }
	t.Cleanup(func() {
		global.GVA_CONFIG.Live = previousLive
		liveTaskLeaseStoreProvider = previousProvider
	})

	var runs atomic.Int32
	err := runLiveTaskWithLeader("reconcile", func() error {
		runs.Add(1)
		return nil
	})
	require.ErrorContains(t, err, "Redis不可用")
	require.Zero(t, runs.Load())
}

func TestLiveTaskLeaderKeyIsStableDuringPublishTokenRotation(t *testing.T) {
	previousLive := global.GVA_CONFIG.Live
	t.Cleanup(func() { global.GVA_CONFIG.Live = previousLive })
	global.GVA_CONFIG.Live.SRS.HTTPAPIs = "http://srs:1985"
	global.GVA_CONFIG.Live.SRS.App = "live"
	global.GVA_CONFIG.Live.PublishTokenKey = "old-key"
	before := liveTaskLeaderKey("reconcile")
	global.GVA_CONFIG.Live.PublishTokenKey = "new-key"
	require.Equal(t, before, liveTaskLeaderKey("reconcile"), "凭证轮换期间不能产生两个Leader命名空间")
}
