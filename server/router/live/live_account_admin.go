package live

import (
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type AccountAdminRouter struct{}

func (r *AccountAdminRouter) InitAccountAdminRouter(privateGroup *gin.RouterGroup) {
	readRouter := privateGroup.Group("live/account").Use(middleware.RequireAuthority(888))
	writeRouter := privateGroup.Group("live/account").Use(middleware.OperationRecord(), middleware.RequireAuthority(888))
	{
		readRouter.GET("list", accountAdminApi.List)
		readRouter.GET("detail", accountAdminApi.Detail)
	}
	{
		writeRouter.POST("profile/update", accountAdminApi.UpdateProfile)
		writeRouter.POST("status/update", accountAdminApi.UpdateStatus)
		writeRouter.POST("phone/unbind", accountAdminApi.UnbindPhone)
		writeRouter.POST("logout-all", accountAdminApi.LogoutAll)
	}
}
