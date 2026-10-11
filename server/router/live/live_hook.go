package live

import "github.com/gin-gonic/gin"

type HookRouter struct{}

// InitHookRouter 的接口使用启动时冻结的SRS共享密钥认证，并由部署层IP白名单提供第二层保护。
// Hook 不使用客户端JWT；认证中间件必须在读取请求体和进入数据库前执行。
func (r *HookRouter) InitHookRouter(publicGroup *gin.RouterGroup, hookAuth gin.HandlerFunc) {
	hook := publicGroup.Group("hook").Use(hookAuth)
	{
		hook.POST("publish", hookApi.Publish)
		hook.POST("unpublish", hookApi.Unpublish)
		hook.POST("session/stats", hookApi.UpdateStats)
	}
}
