package social

import (
	"go.uber.org/zap"
	"sync/atomic"
	"time"
)

type Metrics struct {
	Changes            atomic.Uint64
	Failures           atomic.Uint64
	Retries            atomic.Uint64
	Deadlocks          atomic.Uint64
	CacheHits          atomic.Uint64
	CacheMisses        atomic.Uint64
	WorkerFailures     atomic.Uint64
	Delivered          atomic.Uint64
	Deduplicated       atomic.Uint64
	HistoryBuffered    atomic.Uint64
	HistoryPersisted   atomic.Uint64
	HistoryDropped     atomic.Uint64
	AccessDenied       atomic.Uint64
	CredentialRejected atomic.Uint64
	CounterDifferences atomic.Uint64
	OperationCalls     [9]atomic.Uint64
	OperationFailures  [9]atomic.Uint64
	LatencyBuckets     [9][8]atomic.Uint64
}

func (s *Service) observe(operation string, start time.Time) func() {
	return func() {
		if i := operationIndex(operation); i >= 0 {
			s.Metrics.OperationCalls[i].Add(1)
			ms := time.Since(start).Milliseconds()
			bounds := []int64{1, 5, 10, 25, 50, 100, 500}
			bucket := 7
			for j, bound := range bounds {
				if ms <= bound {
					bucket = j
					break
				}
			}
			s.Metrics.LatencyBuckets[i][bucket].Add(1)
		}
		if s.Log != nil {
			s.Log.Debug("social operation", zap.String("operation", operation), zap.Int64("duration_ms", time.Since(start).Milliseconds()))
		}
	}
}
func (s *Service) MetricSnapshot() map[string]uint64 {
	out := map[string]uint64{
		"counterDifferences": s.Metrics.CounterDifferences.Load(), "changes": s.Metrics.Changes.Load(), "failures": s.Metrics.Failures.Load(), "transactionRetries": s.Metrics.Retries.Load(), "deadlocks": s.Metrics.Deadlocks.Load(), "cacheHits": s.Metrics.CacheHits.Load(), "cacheMisses": s.Metrics.CacheMisses.Load(), "workerFailures": s.Metrics.WorkerFailures.Load(), "delivered": s.Metrics.Delivered.Load(), "deduplicated": s.Metrics.Deduplicated.Load(), "historyBuffered": s.Metrics.HistoryBuffered.Load(), "historyPersisted": s.Metrics.HistoryPersisted.Load(), "historyDropped": s.Metrics.HistoryDropped.Load(), "accessDenied": s.Metrics.AccessDenied.Load(), "credentialRejected": s.Metrics.CredentialRejected.Load()}
	for i, name := range operationNames {
		out[name+".calls"] = s.Metrics.OperationCalls[i].Load()
		out[name+".failures"] = s.Metrics.OperationFailures[i].Load()
		for j, label := range []string{"1ms", "5ms", "10ms", "25ms", "50ms", "100ms", "500ms", "overflow"} {
			out[name+".latency."+label] = s.Metrics.LatencyBuckets[i][j].Load()
		}
	}
	return out
}

var operationNames = []string{"follow", "unfollow", "block", "unblock", "mute", "unmute", "subscribe", "unsubscribe", "batch_status"}

func operationIndex(name string) int {
	for i, value := range operationNames {
		if name == value {
			return i
		}
	}
	return -1
}
