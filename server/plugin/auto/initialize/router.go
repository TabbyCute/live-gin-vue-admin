package initialize

import (
	"tb_live_module/global"
	"tb_live_module/middleware"
	"tb_live_module/plugin/auto/router"
	"github.com/gin-gonic/gin"
)

func Router(engine *gin.Engine) {
	InitializeRouter(engine)
}

func InitializeRouter(engine *gin.Engine) {
	public := engine.Group(global.GVA_CONFIG.System.RouterPrefix).Group("")
	private := engine.Group(global.GVA_CONFIG.System.RouterPrefix).Group("")
	private.Use(middleware.JWTAuth()).Use(middleware.CasbinHandler())

	router.RouterGroupApp.InitAutoCodeRouter(private, public)
	router.RouterGroupApp.InitAutoCodeHistoryRouter(private)
	router.RouterGroupApp.InitSkillsRouter(private, public)
}
