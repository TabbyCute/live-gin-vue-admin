package live

import (
	"time"

	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
)

type UserRouter struct{}

func (r *UserRouter) InitUserRouter(publicGroup, privateGroup *gin.RouterGroup) {
	userPublicRouter := publicGroup.Group("user")
	userPrivateRouter := privateGroup.Group("user").Use(middleware.JWTAppAuth())
	authGuard := middleware.NewAccountAuthGuard()
	profileLookupLimit := middleware.LimitConfig{
		GenerationKey: func(c *gin.Context) string { return "LiveAccountPublicProfile:" + c.ClientIP() },
		CheckOrMark:   middleware.DefaultCheckOrMark,
		Expire:        60,
		Limit:         120,
	}.LimitWithTime()
	{
		userPublicRouter.POST("register", authGuard.Handler("register", 10, 5, 10*time.Minute), userApi.Register)
		userPublicRouter.POST("login", authGuard.Handler("login", 20, 10, time.Minute), userApi.Login)
		userPublicRouter.POST("sms/send", authGuard.Handler("sms", 10, 1, time.Minute), userApi.SendSMS)
	}
	{
		userPrivateRouter.GET("info", userApi.Info)
		userPrivateRouter.GET("public-profile", profileLookupLimit, userApi.PublicProfile)
		userPrivateRouter.PATCH("profile", userApi.UpdateProfile)
		userPrivateRouter.POST("password/change", userApi.ChangePassword)
	}
}
