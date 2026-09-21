package live

import (
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type CategoryAdminRouter struct{}

func (r *CategoryAdminRouter) InitCategoryAdminRouter(privateGroup *gin.RouterGroup) {
	categoryWriteRouter := privateGroup.Group("live/category").Use(middleware.OperationRecord())
	categoryReadRouter := privateGroup.Group("live/category")
	{
		categoryReadRouter.GET("list", categoryAdminApi.List)
		categoryReadRouter.GET("tree", categoryAdminApi.Tree)
	}
	{
		categoryWriteRouter.POST("create", categoryAdminApi.Create)
		categoryWriteRouter.POST("update", categoryAdminApi.Update)
		categoryWriteRouter.POST("status/update", categoryAdminApi.UpdateStatus)
		categoryWriteRouter.POST("delete", categoryAdminApi.Delete)
	}
}
