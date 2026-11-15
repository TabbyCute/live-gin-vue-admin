package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestBoundedCountersAndConcurrentSnapshots(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				r.BeginRequest()
				r.EndRequest("POST", "/gift/send", 503, 100*time.Millisecond)
				if j%50 == 0 {
					_ = r.Snapshot()
				}
			}
		}()
	}
	wg.Wait()
	s := r.Snapshot()
	require.Zero(t, s.InFlight)
	require.Len(t, s.HTTP, 1)
	require.EqualValues(t, 16000, s.HTTP[0].Calls)
	require.EqualValues(t, 16000, s.HTTP[0].Rejected)
	require.EqualValues(t, 16000, s.HTTP[0].Errors)
	require.EqualValues(t, 16000, s.HTTP[0].Buckets[4])
	for i := 0; i < 2000; i++ {
		r.BeginRequest()
		r.EndRequest("GET", fmt.Sprintf("/route/%d", i), 200, time.Second)
	}
	require.LessOrEqual(t, len(r.Snapshot().HTTP), maxRoutes+1)
}

func TestWorkerSnapshotsNeverIncludeErrorContents(t *testing.T) {
	r := New()
	finish := r.StartWorker("event_delivery", time.Second)
	finish(errors.New("private-password-secret"))
	finish(nil)
	s := r.Snapshot()
	require.Len(t, s.Workers, 1)
	require.EqualValues(t, 1, s.Workers[0].Runs)
	require.Equal(t, "failed", s.Workers[0].ErrorKind)
	require.Zero(t, s.Workers[0].Running)
	r.StartWorker("private-password-secret", time.Second)(nil)
	r.StartWorker("event_delivery", time.Second)(context.DeadlineExceeded)
	r.StartWorker("event_delivery", time.Second)(nil)
	s = r.Snapshot()
	require.Len(t, s.Workers, 1)
	require.EqualValues(t, 2, s.Workers[0].Failures)
	require.Zero(t, s.Workers[0].ConsecutiveFailures)
	b, e := json.Marshal(s)
	require.NoError(t, e)
	require.NotContains(t, string(b), "private-")
	r.mu.Lock()
	r.workers["event_delivery"].LastFinishedAt = time.Now().Add(-2 * time.Second).UnixMilli()
	r.workers["event_delivery"].LastStartedAt = time.Now().Add(-3 * time.Second).UnixMilli()
	r.mu.Unlock()
	require.True(t, r.Snapshot().Workers[0].Stale)
	finish = r.StartWorker("event_delivery", time.Second)
	require.False(t, r.Snapshot().Workers[0].Stale, "new work must not inherit a stale previous finish")
	finish(nil)
}

func BenchmarkHTTPObserve(b *testing.B) {
	r := New()
	b.ReportAllocs()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			r.BeginRequest()
			r.EndRequest("POST", "/gift/send", 200, 20*time.Millisecond)
		}
	})
}
