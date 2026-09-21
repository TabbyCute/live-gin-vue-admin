package live

import (
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type AnchorAdminRouter struct{}

// InitAnchorAdminRouter 接收已经挂载后台 JWT 的父路由。
// 写接口额外挂操作日志；读接口不记录操作日志，保持与现有 GVA Router 风格一致。
func (r *AnchorAdminRouter) InitAnchorAdminRouter(privateGroup *gin.RouterGroup) {
	anchorWriteRouter := privateGroup.Group("live/anchor").Use(middleware.OperationRecord())
	anchorReadRouter := privateGroup.Group("live/anchor")
	{
		anchorReadRouter.GET("list", anchorAdminApi.List)
		anchorReadRouter.GET("detail", anchorAdminApi.Detail)
	}
	{
		anchorWriteRouter.POST("audit", anchorAdminApi.Audit)
		anchorWriteRouter.POST("status/update", anchorAdminApi.UpdateStatus)
		anchorWriteRouter.POST("permission/update", anchorAdminApi.UpdatePermission)
		anchorWriteRouter.POST("profile/update", anchorAdminApi.UpdateProfile)
		anchorWriteRouter.POST("recommend/update", anchorAdminApi.UpdateRecommend)
		anchorWriteRouter.POST("signed/update", anchorAdminApi.UpdateSigned)
		anchorWriteRouter.POST("agency/update", anchorAdminApi.UpdateAgency)
		anchorWriteRouter.POST("risk/update", anchorAdminApi.UpdateRisk)
		anchorWriteRouter.POST("remark/update", anchorAdminApi.UpdateRemark)
		anchorWriteRouter.POST("cert/update", anchorAdminApi.UpdateCert)
	}
}
