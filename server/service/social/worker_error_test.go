package social

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type testRedisWorkerError string

func (e testRedisWorkerError) Error() string { return string(e) }
func (e testRedisWorkerError) RedisError()   {}

func TestWorkerErrorDiagnosticsAreRedacted(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		kind, code string
	}{
		{"timeout", context.DeadlineExceeded, "deadline_exceeded", ""},
		{"redis", testRedisWorkerError("WRONGTYPE secret-session-and-cache-payload"), "redis", "WRONGTYPE"},
		{"unrecognized", testRedisWorkerError("SECRET-PASSWORD secret-session-and-cache-payload"), "redis", ""},
		{"mysql", &mysql.MySQLError{Number: 1146, Message: "secret-session-and-cache-payload"}, "mysql", ""},
		{"unknown", errors.New("secret-session-and-cache-payload"), "internal", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)
			err := errors.Join(&workerStepError{stage: "history_persist", shard: 3, err: tc.err})
			zap.New(core).Warn("probe", workerErrorFields(err)...)
			fields := logs.All()[0].ContextMap()
			require.Equal(t, tc.kind, fields["error_kind"])
			require.EqualValues(t, 3, fields["shard"])
			require.Equal(t, "history_persist", fields["stage"])
			if tc.code != "" {
				require.Equal(t, tc.code, fields["redis_code"])
			} else {
				require.NotContains(t, fields, "redis_code")
			}
			if tc.name == "mysql" {
				require.EqualValues(t, 1146, fields["mysql_code"])
			}
			require.NotContains(t, fmt.Sprint(fields), "secret")
			require.NotContains(t, fmt.Sprint(fields), "SECRET-PASSWORD")
		})
	}
}

func TestWorkerLoopLogsDeadlineAndStopsCleanly(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	s := &Service{Log: zap.New(core)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runLoop(ctx, time.Hour, time.Second, "HISTORY_FLUSH_FAILED", func(context.Context) error {
			return &workerStepError{stage: "history_queue", shard: 2, err: context.DeadlineExceeded}
		})
	}()
	require.Eventually(t, func() bool { return logs.Len() == 1 }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	fields := logs.All()[0].ContextMap()
	require.Equal(t, "HISTORY_FLUSH_FAILED", fields["code"])
	require.Equal(t, "deadline_exceeded", fields["error_kind"])
	require.Contains(t, fields, "elapsed")
}
