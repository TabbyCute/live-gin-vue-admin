package config

// SRS 是媒体服务器应用、HTTP API、推流地址和播放地址配置。
type SRS struct {
	App                      string `mapstructure:"app" json:"app" yaml:"app"`
	HTTPAPIs                 string `mapstructure:"http_apis" json:"http_apis" yaml:"http_apis"`
	APIUsername              string `mapstructure:"api-username" json:"api-username" yaml:"api-username"`
	APIPassword              string `mapstructure:"api-password" json:"api-password" yaml:"api-password"`
	APIRequestTimeoutSeconds int    `mapstructure:"api-request-timeout-seconds" json:"api-request-timeout-seconds" yaml:"api-request-timeout-seconds"`
	PushBaseURL              string `mapstructure:"push-base-url" json:"push-base-url" yaml:"push-base-url"`
	PlayBaseURL              string `mapstructure:"play-base-url" json:"play-base-url" yaml:"play-base-url"`
}
