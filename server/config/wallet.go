package config

import "fmt"

// Wallet 配置在启动时冻结，限额调整须滚动重启；零值采用保守默认，负数拒绝启动。
type Wallet struct {
	AppMaxInflight     int   `mapstructure:"app-max-inflight" json:"app-max-inflight" yaml:"app-max-inflight"`
	AdminMaxInflight   int   `mapstructure:"admin-max-inflight" json:"admin-max-inflight" yaml:"admin-max-inflight"`
	MaxGrantAmount     int64 `mapstructure:"max-grant-amount" json:"max-grant-amount" yaml:"max-grant-amount"`
	AccountDailyLimit  int64 `mapstructure:"account-daily-limit" json:"account-daily-limit" yaml:"account-daily-limit"`
	OperatorDailyLimit int64 `mapstructure:"operator-daily-limit" json:"operator-daily-limit" yaml:"operator-daily-limit"`
	// EvidenceThreshold 兼容旧配置；人工充值现在无论金额大小都要求凭证。
	EvidenceThreshold int64  `mapstructure:"evidence-threshold" json:"evidence-threshold" yaml:"evidence-threshold"`
	ReadAuthorities   []uint `mapstructure:"read-authorities" json:"read-authorities" yaml:"read-authorities"`
	CreateAuthorities []uint `mapstructure:"create-authorities" json:"create-authorities" yaml:"create-authorities"`
	ReviewAuthorities []uint `mapstructure:"review-authorities" json:"review-authorities" yaml:"review-authorities"`
}

func (c Wallet) Normalize() (Wallet, error) {
	if c.AppMaxInflight < 0 || c.AppMaxInflight > 4096 || c.AdminMaxInflight < 0 || c.AdminMaxInflight > 4096 {
		return c, fmt.Errorf("wallet inflight limits must be within 1..4096, or zero for defaults")
	}
	if c.AppMaxInflight == 0 {
		c.AppMaxInflight = 64
	}
	if c.AdminMaxInflight == 0 {
		c.AdminMaxInflight = 16
	}
	if c.MaxGrantAmount < 0 || c.AccountDailyLimit < 0 || c.OperatorDailyLimit < 0 || c.EvidenceThreshold < 0 {
		return c, fmt.Errorf("wallet limits must be positive")
	}
	if c.MaxGrantAmount == 0 {
		c.MaxGrantAmount = 1000000
	}
	if c.AccountDailyLimit == 0 {
		c.AccountDailyLimit = 10000000
	}
	if c.OperatorDailyLimit == 0 {
		c.OperatorDailyLimit = 10000000
	}
	if c.EvidenceThreshold == 0 {
		c.EvidenceThreshold = 100000
	}
	if c.MaxGrantAmount > c.AccountDailyLimit || c.MaxGrantAmount > c.OperatorDailyLimit {
		return c, fmt.Errorf("wallet daily limits must cover max grant")
	}
	roles := []*[]uint{&c.ReadAuthorities, &c.CreateAuthorities, &c.ReviewAuthorities}
	for _, p := range roles {
		if len(*p) == 0 {
			*p = []uint{888}
		}
		for _, id := range *p {
			if id == 0 {
				return c, fmt.Errorf("wallet authority cannot be zero")
			}
		}
		*p = append([]uint(nil), (*p)...)
	}
	return c, nil
}
