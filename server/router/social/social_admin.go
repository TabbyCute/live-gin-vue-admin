package social

import (
	"github.com/gin-gonic/gin"
	api "tb_live_module/api/v1"
	socialAPI "tb_live_module/api/v1/social"
	"tb_live_module/middleware"
)

type SocialAdminRouter struct{}

func (r *SocialAdminRouter) InitSocialAdminRouter(root *gin.RouterGroup) {
	group := root.Group("social").Use(socialAPI.RequestBudget(), middleware.RequireAuthority(888), socialAPI.AdminGuard(), middleware.OperationRecord())
	a := api.ApiGroupApp.SocialApiGroup.SocialAdminApi
	group.GET("counter/check", a.Check)
	group.POST("counter/repair", a.Repair)
	group.GET("event/list", a.Events)
	group.POST("event/replay", a.Replay)
	group.GET("block/list", a.Blocks)
	group.GET("relationship/check", a.Anomalies)
	group.GET("health", a.Health)
}
