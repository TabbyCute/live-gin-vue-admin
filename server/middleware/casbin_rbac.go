package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"tb_live_module/model/common/response"
	"tb_live_module/utils"
)

// CasbinHandler 拦截器
func CasbinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		waitUse, _ := utils.GetClaims(c)
		//获取请求的PATH
		obj := utils.NormalizeAdminRoutePath(c.Request.URL.Path)
		// 获取请求方法
		act := c.Request.Method
		// 获取用户的角色
		sub := strconv.Itoa(int(waitUse.AuthorityId))
		e := utils.GetCasbin() // 判断策略中是否存在
		success, _ := e.Enforce(sub, obj, act)
		if !success {
			response.FailWithDetailed(gin.H{}, "权限不足", c)
			c.Abort()
			return
		}
		c.Next()
	}
}
