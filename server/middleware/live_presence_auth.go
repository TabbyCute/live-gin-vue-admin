package middleware

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/model/common/response"
	"tb_live_module/utils"
)

// PresenceJWTSignature verifies APP issuer/audience/signature/expiry on EVERY request.
// Only presence routes may use this middleware: PresenceService performs database
// account/version/access checks at open and at most 30 seconds between renewals.
// Invalid supplied JWT never falls back to guest access.
func PresenceJWTSignature() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" {
			c.Next()
			return
		}
		token, ok := appBearerToken(h)
		if !ok {
			response.NoAuth("客户端登录无效", c)
			c.Abort()
			return
		}
		claims, e := utils.NewAppJWT().ParseToken(token)
		if e != nil {
			response.NoAuth("客户端登录已失效", c)
			c.Abort()
			return
		}
		c.Set(appClaimsContextKey, claims)
		c.Next()
	}
}

// PresenceClientIP uses the same explicit proxy trust policy as account authentication.
func (g *AccountAuthGuard) PresenceClientIP(c *gin.Context) string { return g.clientIP(c) }
