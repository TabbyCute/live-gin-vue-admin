package config

// Live 是直播间业务生命周期的运行配置。
type Live struct {
	ReconnectWindowSeconds int    `mapstructure:"reconnect-window-seconds" json:"reconnect-window-seconds" yaml:"reconnect-window-seconds"`
	PrepareTimeoutSeconds  int    `mapstructure:"prepare-timeout-seconds" json:"prepare-timeout-seconds" yaml:"prepare-timeout-seconds"`
	PushTokenSeconds       int    `mapstructure:"push-token-seconds" json:"push-token-seconds" yaml:"push-token-seconds"`
	PublishTokenKey        string `mapstructure:"publish-token-key" json:"publish-token-key" yaml:"publish-token-key"`
	EndScanIntervalSeconds int    `mapstructure:"end-scan-interval-seconds" json:"end-scan-interval-seconds" yaml:"end-scan-interval-seconds"`
	EndScanBatchSize       int    `mapstructure:"end-scan-batch-size" json:"end-scan-batch-size" yaml:"end-scan-batch-size"`
	EndRetryBaseSeconds    int    `mapstructure:"end-retry-base-seconds" json:"end-retry-base-seconds" yaml:"end-retry-base-seconds"`
	EndRetryMaxSeconds     int    `mapstructure:"end-retry-max-seconds" json:"end-retry-max-seconds" yaml:"end-retry-max-seconds"`
	SRS                    SRS    `mapstructure:"srs" json:"srs" yaml:"srs"`
}
