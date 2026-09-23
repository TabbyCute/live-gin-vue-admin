package live

import "github.com/gin-gonic/gin"

type HookRouter struct{}

// InitHookRouter 的接口由独立 X-Live-Hook-Token 鉴权，不使用客户端 JWT。
func (r *HookRouter) InitHookRouter(publicGroup *gin.RouterGroup) {
	hook := publicGroup.Group("hook")
	{
		hook.POST("publish", hookApi.Publish)
		hook.POST("unpublish", hookApi.Unpublish)
		hook.POST("session/stats", hookApi.UpdateStats)
	}
}
