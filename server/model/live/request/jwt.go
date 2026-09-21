package request

import jwt "github.com/golang-jwt/jwt/v5"

const AppTokenType = "live_app_access"

// AppClaims 是客户端账号专用 Claims，不复用管理后台 CustomClaims。
type AppClaims struct {
	AccountID uint   `json:"accountId"`
	Username  string `json:"username"`
	TokenType string `json:"tokenType"`
	jwt.RegisteredClaims
}
