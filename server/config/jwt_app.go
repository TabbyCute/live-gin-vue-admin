package config

// JWTApp 是客户端账号专用的 JWT 配置，不与管理后台 JWT 共用密钥或签发者。
type JWTApp struct {
	SigningKey  string `mapstructure:"signing-key" json:"signing-key" yaml:"signing-key"`
	ExpiresTime string `mapstructure:"expires-time" json:"expires-time" yaml:"expires-time"`
	Issuer      string `mapstructure:"issuer" json:"issuer" yaml:"issuer"`
}
