package initialize

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/global"
	"tb_live_module/middleware"
	"tb_live_module/plugin/announcement/router"
)

func Router(engine *gin.Engine) {
	public := engine.Group(global.GVA_CONFIG.System.RouterPrefix).Group("")
	private := engine.Group(global.GVA_CONFIG.System.RouterPrefix).Group("")
	private.Use(middleware.JWTAuth())
	router.Router.Info.Init(public, private)
}
