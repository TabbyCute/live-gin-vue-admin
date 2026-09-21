package initialize

import (
	"tb_live_module/router"

	"github.com/gin-gonic/gin"
)

const MODULE_LIVE = "/app/live"

func registerLiveRoutes(publicRoot *gin.RouterGroup) {
	registerLiveRoutesV1(publicRoot)
}

func registerLiveRoutesV1(publicRoot *gin.RouterGroup) {
	publicGroup := publicRoot.Group("/v1" + MODULE_LIVE)
	privateGroup := publicRoot.Group("/v1" + MODULE_LIVE)
	liveRouter := router.RouterGroupApp.Live
	liveRouter.InitUserRouter(publicGroup, privateGroup)
	liveRouter.InitAnchorRouter(publicGroup, privateGroup)
	liveRouter.InitCategoryRouter(publicGroup)
}
