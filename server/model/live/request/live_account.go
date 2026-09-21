package request

// Register 客户端账号注册参数。
type Register struct {
	Username string `json:"username" binding:"required,min=3,max=32" example:"demo_user"`
	Password string `json:"password" binding:"required,min=8,max=72" example:"password123"`
	Nickname string `json:"nickname" binding:"omitempty,max=64" example:"Demo"`
	Avatar   string `json:"avatar" binding:"omitempty,max=500" example:"https://example.com/avatar.png"`
}

// Login 客户端账号登录参数。
type Login struct {
	Username string `json:"username" binding:"required,min=3,max=32" example:"demo_user"`
	Password string `json:"password" binding:"required,min=8,max=72" example:"password123"`
}
