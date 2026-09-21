package initialize

import (
	"tb_live_module/router"

	"github.com/gin-gonic/gin"
)

const MODULE_LIVE = "/app/live"

func registerLiveRoutes(publicRoot *gin.RouterGroup) {
	registerLiveRoutesV2(publicRoot)
	registerLiveRoutesV1(publicRoot)
}

func registerLiveRoutesV1(publicRoot *gin.RouterGroup) {
	publicGroup := publicRoot.Group("/v1" + MODULE_LIVE)
	privateGroup := publicRoot.Group("/v1" + MODULE_LIVE)
	liveRouter := router.RouterGroupApp.Live
	liveRouter.InitUserRouter(publicGroup, privateGroup)
}

func registerLiveRoutesV2(publicRoot *gin.RouterGroup) {
	publicGroup := publicRoot.Group("/v2" + MODULE_LIVE)
	privateGroup := publicRoot.Group("/v2" + MODULE_LIVE)
	liveRouter := router.RouterGroupApp.Live
	liveRouter.InitUserRouter(publicGroup, privateGroup)
}
