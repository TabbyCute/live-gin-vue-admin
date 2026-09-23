package config

// Live 是直播间、SRS 回调和断流重连的运行配置。
type Live struct {
	ReconnectWindowSeconds int    `mapstructure:"reconnect-window-seconds" json:"reconnect-window-seconds" yaml:"reconnect-window-seconds"`
	SRSHookToken           string `mapstructure:"srs-hook-token" json:"srs-hook-token" yaml:"srs-hook-token"`
	PushBaseURL            string `mapstructure:"push-base-url" json:"push-base-url" yaml:"push-base-url"`
	PlayBaseURL            string `mapstructure:"play-base-url" json:"play-base-url" yaml:"play-base-url"`
}
