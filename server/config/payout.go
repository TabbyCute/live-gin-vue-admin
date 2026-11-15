package config

// Payout 由启动时选中的 YAML 提供。密钥只用于服务初始化，变更后须重启。
// json:"-" 阻止系统配置接口返回或接受密钥；后台保存配置也必须保留原值。
type Payout struct {
	// ActiveKeyID 指向 EncryptionKeys 中用于新数据加密的一把密钥。
	ActiveKeyID string `mapstructure:"active-key-id" json:"-" yaml:"active-key-id"`
	// EncryptionKeys 是 key ID → Base64(32字节AES密钥) 的 JSON 字符串。
	// 保留为字符串，避免 Viper 将 map 的 key ID 转小写而丢失历史密钥标识。
	EncryptionKeys string `mapstructure:"encryption-keys" json:"-" yaml:"encryption-keys"`
	// FingerprintKey 是独立的32字节HMAC密钥；不能随AES密钥轮换。
	FingerprintKey string `mapstructure:"fingerprint-key" json:"-" yaml:"fingerprint-key"`
}
