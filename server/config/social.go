package config

// Social 核心能力默认启用；媒体门禁仍需独立配置并完成实际链路验收。
type Social struct {
	AutoSubscribe            bool   `mapstructure:"auto-subscribe" json:"auto-subscribe" yaml:"auto-subscribe"`
	HistoryRetentionDays     int    `mapstructure:"history-retention-days" json:"history-retention-days" yaml:"history-retention-days"`
	ProtectedPlaybackEnabled bool   `mapstructure:"protected-playback-enabled" json:"protected-playback-enabled" yaml:"protected-playback-enabled"`
	MediaGatewayVerified     bool   `mapstructure:"media-gateway-verified" json:"media-gateway-verified" yaml:"media-gateway-verified"`
	PlaybackSigningKey       string `mapstructure:"playback-signing-key" json:"-" yaml:"playback-signing-key"`
	PlaybackBaseURL          string `mapstructure:"playback-base-url" json:"playback-base-url" yaml:"playback-base-url"`
}
