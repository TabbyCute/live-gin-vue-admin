package response

import "time"

// Account 是可返回给客户端的安全账号信息，不包含密码哈希。
type Account struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	Status    uint8     `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
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
