package config

// Live 是直播间业务生命周期的运行配置。
type Live struct {
	MaxConcurrentStreams            int    `mapstructure:"max-concurrent-streams" json:"max-concurrent-streams" yaml:"max-concurrent-streams"`
	ReconnectWindowSeconds          int    `mapstructure:"reconnect-window-seconds" json:"reconnect-window-seconds" yaml:"reconnect-window-seconds"`
	PrepareTimeoutSeconds           int    `mapstructure:"prepare-timeout-seconds" json:"prepare-timeout-seconds" yaml:"prepare-timeout-seconds"`
	StreamReadyTimeoutSeconds       int    `mapstructure:"stream-ready-timeout-seconds" json:"stream-ready-timeout-seconds" yaml:"stream-ready-timeout-seconds"`
	StreamReadyScanIntervalSeconds  int    `mapstructure:"stream-ready-scan-interval-seconds" json:"stream-ready-scan-interval-seconds" yaml:"stream-ready-scan-interval-seconds"`
	StreamReadyRetryBaseSeconds     int    `mapstructure:"stream-ready-retry-base-seconds" json:"stream-ready-retry-base-seconds" yaml:"stream-ready-retry-base-seconds"`
	StreamReadyRetryMaxSeconds      int    `mapstructure:"stream-ready-retry-max-seconds" json:"stream-ready-retry-max-seconds" yaml:"stream-ready-retry-max-seconds"`
	StreamReconcileIntervalSeconds  int    `mapstructure:"stream-reconcile-interval-seconds" json:"stream-reconcile-interval-seconds" yaml:"stream-reconcile-interval-seconds"`
	StreamReconcileMissingThreshold int    `mapstructure:"stream-reconcile-missing-threshold" json:"stream-reconcile-missing-threshold" yaml:"stream-reconcile-missing-threshold"`
	StreamReadyBatchSize            int    `mapstructure:"stream-ready-batch-size" json:"stream-ready-batch-size" yaml:"stream-ready-batch-size"`
	StreamReadyMaxBatchesPerRun     int    `mapstructure:"stream-ready-max-batches-per-run" json:"stream-ready-max-batches-per-run" yaml:"stream-ready-max-batches-per-run"`
	StateTransitionConcurrency      int    `mapstructure:"state-transition-concurrency" json:"state-transition-concurrency" yaml:"state-transition-concurrency"`
	StreamReconcileBatchSize        int    `mapstructure:"stream-reconcile-batch-size" json:"stream-reconcile-batch-size" yaml:"stream-reconcile-batch-size"`
	StreamReconcileMaxBatchesPerRun int    `mapstructure:"stream-reconcile-max-batches-per-run" json:"stream-reconcile-max-batches-per-run" yaml:"stream-reconcile-max-batches-per-run"`
	SRSSnapshotCacheMilliseconds    int    `mapstructure:"srs-snapshot-cache-milliseconds" json:"srs-snapshot-cache-milliseconds" yaml:"srs-snapshot-cache-milliseconds"`
	TaskLeaderEnabled               bool   `mapstructure:"task-leader-enabled" json:"task-leader-enabled" yaml:"task-leader-enabled"`
	TaskLeaderFailOpen              bool   `mapstructure:"task-leader-fail-open" json:"task-leader-fail-open" yaml:"task-leader-fail-open"`
	TaskLeaderLeaseSeconds          int    `mapstructure:"task-leader-lease-seconds" json:"task-leader-lease-seconds" yaml:"task-leader-lease-seconds"`
	PushTokenSeconds                int    `mapstructure:"push-token-seconds" json:"push-token-seconds" yaml:"push-token-seconds"`
	PublishTokenKey                 string `mapstructure:"publish-token-key" json:"publish-token-key" yaml:"publish-token-key"`
	PushURLRefreshLimit             int    `mapstructure:"push-url-refresh-limit" json:"push-url-refresh-limit" yaml:"push-url-refresh-limit"`
	EndScanIntervalSeconds          int    `mapstructure:"end-scan-interval-seconds" json:"end-scan-interval-seconds" yaml:"end-scan-interval-seconds"`
	EndScanBatchSize                int    `mapstructure:"end-scan-batch-size" json:"end-scan-batch-size" yaml:"end-scan-batch-size"`
	EndScanMaxBatchesPerRun         int    `mapstructure:"end-scan-max-batches-per-run" json:"end-scan-max-batches-per-run" yaml:"end-scan-max-batches-per-run"`
	EndStopBatchSize                int    `mapstructure:"end-stop-batch-size" json:"end-stop-batch-size" yaml:"end-stop-batch-size"`
	EndStopMaxBatchesPerRun         int    `mapstructure:"end-stop-max-batches-per-run" json:"end-stop-max-batches-per-run" yaml:"end-stop-max-batches-per-run"`
	EndStopConcurrency              int    `mapstructure:"end-stop-concurrency" json:"end-stop-concurrency" yaml:"end-stop-concurrency"`
	EndRetryBaseSeconds             int    `mapstructure:"end-retry-base-seconds" json:"end-retry-base-seconds" yaml:"end-retry-base-seconds"`
	EndRetryMaxSeconds              int    `mapstructure:"end-retry-max-seconds" json:"end-retry-max-seconds" yaml:"end-retry-max-seconds"`
	EndWarningSeconds               int    `mapstructure:"end-warning-seconds" json:"end-warning-seconds" yaml:"end-warning-seconds"`
	EndCriticalSeconds              int    `mapstructure:"end-critical-seconds" json:"end-critical-seconds" yaml:"end-critical-seconds"`
	EndManualRetrySeconds           int    `mapstructure:"end-manual-retry-seconds" json:"end-manual-retry-seconds" yaml:"end-manual-retry-seconds"`
	LogSRSHookRawBody               bool   `mapstructure:"log-srs-hook-raw-body" json:"log-srs-hook-raw-body" yaml:"log-srs-hook-raw-body"`
	SRSHookMaxBodyBytes             int64  `mapstructure:"srs-hook-max-body-bytes" json:"srs-hook-max-body-bytes" yaml:"srs-hook-max-body-bytes"`
	SRSHookLogMaxBodyBytes          int64  `mapstructure:"srs-hook-log-max-body-bytes" json:"srs-hook-log-max-body-bytes" yaml:"srs-hook-log-max-body-bytes"`
	SRSHookToken                    string `mapstructure:"srs-hook-token" json:"-" yaml:"srs-hook-token"`
	SRS                             SRS    `mapstructure:"srs" json:"srs" yaml:"srs"`
}
