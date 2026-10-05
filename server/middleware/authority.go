package middleware

import (
	"tb_live_module/model/common/response"
	"tb_live_module/utils"

	"github.com/gin-gonic/gin"
)

// RequireAuthority 为极少数不可逆高风险接口提供真正的服务端权限边界。
// 普通动态菜单只控制可见性，不能替代这里的授权判断。
func RequireAuthority(authorityIDs ...uint) gin.HandlerFunc {
	allowed := make(map[uint]struct{}, len(authorityIDs))
	for _, authorityID := range authorityIDs {
		allowed[authorityID] = struct{}{}
	}
	return func(c *gin.Context) {
		if _, ok := allowed[utils.GetUserAuthorityId(c)]; !ok {
			response.NoAuth("权限不足：该操作仅限受信任的高级管理员", c)
			c.Abort()
			return
		}
		c.Next()
	}
}
