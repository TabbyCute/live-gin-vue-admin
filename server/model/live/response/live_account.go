package response

import "time"

// Account 是可返回给客户端的安全账号信息，不包含密码哈希。
type Account struct {
	ID                 uint      `json:"id"`
	AccountNo          string    `json:"accountNo"`
	Username           string    `json:"username"`
	PhoneMasked        string    `json:"phoneMasked,omitempty"`
	PhoneVerified      bool      `json:"phoneVerified"`
	Nickname           string    `json:"nickname"`
	Avatar             string    `json:"avatar"`
	Signature          string    `json:"signature"`
	Gender             uint8     `json:"gender"`
	Birthday           string    `json:"birthday,omitempty"`
	CountryCode        string    `json:"countryCode"`
	RegionCode         string    `json:"regionCode"`
	CityCode           string    `json:"cityCode"`
	Language           string    `json:"language"`
	RealNameStatus     uint8     `json:"realNameStatus"`
	RealNameVerifiedAt int64     `json:"realNameVerifiedAt"`
	Status             uint8     `json:"status"`
	BanUntil           int64     `json:"banUntil,omitempty"`
	LastLoginAt        int64     `json:"lastLoginAt"`
	CreatedAt          time.Time `json:"createdAt"`
}

type RegisterResponse struct {
	Account Account `json:"account"`
}

type LoginResponse struct {
	Account   Account `json:"account"`
	Token     string  `json:"token"`
	ExpiresAt int64   `json:"expiresAt"`
}

type AccountResponse struct {
	Account Account `json:"account"`
}

type PasswordChangeResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

type AccountPublicProfileResp struct {
	AccountNo   string `json:"accountNo"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Signature   string `json:"signature"`
	Gender      uint8  `json:"gender"`
	CountryCode string `json:"countryCode"`
}

type AccountAdminListItemResp struct {
	ID                uint      `json:"id"`
	AccountNo         string    `json:"accountNo"`
	Username          string    `json:"username"`
	PhoneMasked       string    `json:"phoneMasked"`
	PhoneVerified     bool      `json:"phoneVerified"`
	Nickname          string    `json:"nickname"`
	Avatar            string    `json:"avatar"`
	Status            uint8     `json:"status"`
	BanUntil          int64     `json:"banUntil"`
	RealNameStatus    uint8     `json:"realNameStatus"`
	RegisterSource    string    `json:"registerSource"`
	RegisterChannelID uint64    `json:"registerChannelId"`
	LastLoginAt       int64     `json:"lastLoginAt"`
	CreatedAt         time.Time `json:"createdAt"`
}

type AccountAdminListResp struct {
	List     []AccountAdminListItemResp `json:"list"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"pageSize"`
}

type AccountAdminDetailResp struct {
	ID                 uint      `json:"id"`
	AccountNo          string    `json:"accountNo"`
	Username           string    `json:"username"`
	Phone              string    `json:"phone"`
	PhoneMasked        string    `json:"phoneMasked"`
	PhoneVerifiedAt    int64     `json:"phoneVerifiedAt"`
	Nickname           string    `json:"nickname"`
	Avatar             string    `json:"avatar"`
	Signature          string    `json:"signature"`
	Gender             uint8     `json:"gender"`
	Birthday           string    `json:"birthday,omitempty"`
	CountryCode        string    `json:"countryCode"`
	RegionCode         string    `json:"regionCode"`
	CityCode           string    `json:"cityCode"`
	Language           string    `json:"language"`
	RealNameStatus     uint8     `json:"realNameStatus"`
	RealNameVerifiedAt int64     `json:"realNameVerifiedAt"`
	Status             uint8     `json:"status"`
	StatusReason       string    `json:"statusReason"`
	BanUntil           int64     `json:"banUntil"`
	PasswordChangedAt  int64     `json:"passwordChangedAt"`
	LastLoginAt        int64     `json:"lastLoginAt"`
	RegisterSource     string    `json:"registerSource"`
	RegisterChannelID  uint64    `json:"registerChannelId"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
