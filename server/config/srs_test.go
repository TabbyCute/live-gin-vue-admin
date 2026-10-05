package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLiveAndSRSConfigUnmarshal(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`
live:
  max-concurrent-streams: 10000
  reconnect-window-seconds: 20
  stream-ready-timeout-seconds: 15
  stream-ready-scan-interval-seconds: 1
  stream-ready-retry-base-seconds: 1
  stream-ready-retry-max-seconds: 4
  stream-reconcile-interval-seconds: 5
  stream-reconcile-missing-threshold: 3
  stream-ready-batch-size: 1000
  stream-ready-max-batches-per-run: 10
  state-transition-concurrency: 16
  stream-reconcile-batch-size: 900
  stream-reconcile-max-batches-per-run: 5
  srs-snapshot-cache-milliseconds: 1500
  task-leader-enabled: true
  task-leader-fail-open: true
  task-leader-lease-seconds: 60
  push-token-seconds: 180
  publish-token-key: publish-token-secret
  push-url-refresh-limit: 5
  end-scan-interval-seconds: 5
  end-scan-batch-size: 100
  end-scan-max-batches-per-run: 10
  end-stop-batch-size: 80
  end-stop-max-batches-per-run: 10
  end-stop-concurrency: 8
  end-retry-base-seconds: 5
  end-retry-max-seconds: 60
  end-warning-seconds: 120
  end-critical-seconds: 900
  end-manual-retry-seconds: 300
  log-srs-hook-raw-body: true
  srs-hook-max-body-bytes: 65536
  srs-hook-log-max-body-bytes: 4096
  srs-hook-token: hook-secret
  srs:
    app: live
    http_apis: http://127.0.0.1:1985
    api-username: admin
    api-password: secret
    api-request-timeout-seconds: 3
    snapshot-page-size: 1000
    snapshot-max-streams: 20000
    push-base-url: rtmp://127.0.0.1/live
    play-base-url: https://127.0.0.1/live
`)))

	var cfg Server
	require.NoError(t, v.Unmarshal(&cfg))
	require.Equal(t, 10000, cfg.Live.MaxConcurrentStreams)
	require.Equal(t, 20, cfg.Live.ReconnectWindowSeconds)
	require.Equal(t, 15, cfg.Live.StreamReadyTimeoutSeconds)
	require.Equal(t, 1, cfg.Live.StreamReadyScanIntervalSeconds)
	require.Equal(t, 1, cfg.Live.StreamReadyRetryBaseSeconds)
	require.Equal(t, 4, cfg.Live.StreamReadyRetryMaxSeconds)
	require.Equal(t, 5, cfg.Live.StreamReconcileIntervalSeconds)
	require.Equal(t, 3, cfg.Live.StreamReconcileMissingThreshold)
	require.Equal(t, 1000, cfg.Live.StreamReadyBatchSize)
	require.Equal(t, 10, cfg.Live.StreamReadyMaxBatchesPerRun)
	require.Equal(t, 16, cfg.Live.StateTransitionConcurrency)
	require.Equal(t, 900, cfg.Live.StreamReconcileBatchSize)
	require.Equal(t, 5, cfg.Live.StreamReconcileMaxBatchesPerRun)
	require.Equal(t, 1500, cfg.Live.SRSSnapshotCacheMilliseconds)
	require.True(t, cfg.Live.TaskLeaderEnabled)
	require.True(t, cfg.Live.TaskLeaderFailOpen)
	require.Equal(t, 60, cfg.Live.TaskLeaderLeaseSeconds)
	require.Equal(t, 180, cfg.Live.PushTokenSeconds)
	require.Equal(t, "publish-token-secret", cfg.Live.PublishTokenKey)
	require.Equal(t, 5, cfg.Live.PushURLRefreshLimit)
	require.Equal(t, 5, cfg.Live.EndScanIntervalSeconds)
	require.Equal(t, 100, cfg.Live.EndScanBatchSize)
	require.Equal(t, 10, cfg.Live.EndScanMaxBatchesPerRun)
	require.Equal(t, 80, cfg.Live.EndStopBatchSize)
	require.Equal(t, 10, cfg.Live.EndStopMaxBatchesPerRun)
	require.Equal(t, 8, cfg.Live.EndStopConcurrency)
	require.Equal(t, 5, cfg.Live.EndRetryBaseSeconds)
	require.Equal(t, 60, cfg.Live.EndRetryMaxSeconds)
	require.Equal(t, 120, cfg.Live.EndWarningSeconds)
	require.Equal(t, 900, cfg.Live.EndCriticalSeconds)
	require.Equal(t, 300, cfg.Live.EndManualRetrySeconds)
	require.True(t, cfg.Live.LogSRSHookRawBody)
	require.Equal(t, int64(65536), cfg.Live.SRSHookMaxBodyBytes)
	require.Equal(t, int64(4096), cfg.Live.SRSHookLogMaxBodyBytes)
	require.Equal(t, "hook-secret", cfg.Live.SRSHookToken)
	require.Equal(t, "live", cfg.Live.SRS.App)
	require.Equal(t, "http://127.0.0.1:1985", cfg.Live.SRS.HTTPAPIs)
	require.Equal(t, "admin", cfg.Live.SRS.APIUsername)
	require.Equal(t, "secret", cfg.Live.SRS.APIPassword)
	require.Equal(t, 3, cfg.Live.SRS.APIRequestTimeoutSeconds)
	require.Equal(t, 1000, cfg.Live.SRS.SnapshotPageSize)
	require.Equal(t, 20000, cfg.Live.SRS.SnapshotMaxStreams)
	require.Equal(t, "rtmp://127.0.0.1/live", cfg.Live.SRS.PushBaseURL)
	require.Equal(t, "https://127.0.0.1/live", cfg.Live.SRS.PlayBaseURL)
}

func TestEnvironmentConfigsUseNestedSRS(t *testing.T) {
	for _, filename := range []string{"config.dev.yaml", "config.prod.yaml", "config.docker.yaml"} {
		t.Run(filename, func(t *testing.T) {
			v := viper.New()
			v.SetConfigFile("../" + filename)
			require.NoError(t, v.ReadInConfig())
			require.True(t, v.InConfig("live.max-concurrent-streams"))
			require.True(t, v.InConfig("live.srs.app"))
			require.True(t, v.InConfig("live.push-token-seconds"))
			require.True(t, v.InConfig("live.stream-ready-timeout-seconds"))
			require.True(t, v.InConfig("live.stream-ready-scan-interval-seconds"))
			require.True(t, v.InConfig("live.stream-ready-retry-base-seconds"))
			require.True(t, v.InConfig("live.stream-ready-retry-max-seconds"))
			require.True(t, v.InConfig("live.stream-reconcile-interval-seconds"))
			require.True(t, v.InConfig("live.stream-reconcile-missing-threshold"))
			require.True(t, v.InConfig("live.stream-ready-batch-size"))
			require.True(t, v.InConfig("live.stream-ready-max-batches-per-run"))
			require.True(t, v.InConfig("live.state-transition-concurrency"))
			require.True(t, v.InConfig("live.stream-reconcile-batch-size"))
			require.True(t, v.InConfig("live.stream-reconcile-max-batches-per-run"))
			require.True(t, v.InConfig("live.srs-snapshot-cache-milliseconds"))
			require.True(t, v.InConfig("live.task-leader-enabled"))
			require.True(t, v.InConfig("live.task-leader-fail-open"))
			require.True(t, v.InConfig("live.task-leader-lease-seconds"))
			require.True(t, v.InConfig("live.publish-token-key"))
			require.True(t, v.InConfig("live.push-url-refresh-limit"))
			require.True(t, v.InConfig("live.end-scan-interval-seconds"))
			require.True(t, v.InConfig("live.end-scan-batch-size"))
			require.True(t, v.InConfig("live.end-scan-max-batches-per-run"))
			require.True(t, v.InConfig("live.end-stop-batch-size"))
			require.True(t, v.InConfig("live.end-stop-max-batches-per-run"))
			require.True(t, v.InConfig("live.end-stop-concurrency"))
			require.True(t, v.InConfig("live.end-retry-base-seconds"))
			require.True(t, v.InConfig("live.end-retry-max-seconds"))
			require.True(t, v.InConfig("live.end-warning-seconds"))
			require.True(t, v.InConfig("live.end-critical-seconds"))
			require.True(t, v.InConfig("live.end-manual-retry-seconds"))
			require.True(t, v.InConfig("live.log-srs-hook-raw-body"))
			require.True(t, v.InConfig("live.srs-hook-max-body-bytes"))
			require.True(t, v.InConfig("live.srs-hook-log-max-body-bytes"))
			require.True(t, v.InConfig("live.srs.http_apis"))
			require.True(t, v.InConfig("live.srs.api-username"))
			require.True(t, v.InConfig("live.srs.api-password"))
			require.True(t, v.InConfig("live.srs.api-request-timeout-seconds"))
			require.True(t, v.InConfig("live.srs.snapshot-page-size"))
			require.True(t, v.InConfig("live.srs.snapshot-max-streams"))
			require.True(t, v.InConfig("live.srs.push-base-url"))
			require.True(t, v.InConfig("live.srs.play-base-url"))
			require.True(t, v.InConfig("live.srs-hook-token"))
		})
	}
}
