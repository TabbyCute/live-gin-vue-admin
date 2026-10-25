package config

// LiveAccountAuth 仅用于短信接入前的客户端注册联调。
// SuperCode不通过JSON配置接口读取或修改，避免泄漏通用注册凭证。
type LiveAccountAuth struct {
	SuperCode      string   `mapstructure:"super-code" json:"-" yaml:"super-code"`
	TrustedProxies []string `mapstructure:"trusted-proxies" json:"-" yaml:"trusted-proxies"`
}
