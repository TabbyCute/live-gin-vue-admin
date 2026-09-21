package live

import (
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type AnchorRouter struct{}

func (r *AnchorRouter) InitAnchorRouter(publicGroup, privateGroup *gin.RouterGroup) {
	anchorPublicRouter := publicGroup.Group("anchor")
	anchorPrivateRouter := privateGroup.Group("anchor").Use(middleware.JWTAppAuth())
	{
		// 公共主页只返回审核通过、状态正常主播的公开 DTO，可供游客访问。
		anchorPublicRouter.GET("detail", anchorApi.Detail)
	}
	{
		anchorPrivateRouter.GET("info", anchorApi.Info)
		anchorPrivateRouter.POST("apply", anchorApi.Apply)
		anchorPrivateRouter.GET("apply/status", anchorApi.ApplyStatus)
		anchorPrivateRouter.POST("profile/update", anchorApi.UpdateProfile)
		anchorPrivateRouter.GET("live/check", anchorApi.CheckLive)
		anchorPrivateRouter.GET("pk/check", anchorApi.CheckPk)
	}
}
