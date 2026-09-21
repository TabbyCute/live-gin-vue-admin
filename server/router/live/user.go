package live

import (
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type UserRouter struct{}

func (r *UserRouter) InitUserRouter(publicGroup, privateGroup *gin.RouterGroup) {
	userPublicRouter := publicGroup.Group("user")
	userPrivateRouter := privateGroup.Group("user").Use(middleware.JWTAppAuth())
	{
		userPublicRouter.POST("register", userApi.Register)
		userPublicRouter.POST("login", userApi.Login)
	}
	{
		userPrivateRouter.GET("info", userApi.Info)
	}
}
