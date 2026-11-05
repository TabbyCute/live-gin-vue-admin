package initialize

import (
	"tb_live_module/router"

	"github.com/gin-gonic/gin"
)

const MODULE_LIVE = "/app/live"

func registerLiveRoutes(publicRoot *gin.RouterGroup) {
	router.RouterGroupApp.Guard.InitGuardRouter(publicRoot.Group("/v1/app"))
	router.RouterGroupApp.Ranking.InitRankingRouter(publicRoot.Group("/v1/app"))
	router.RouterGroupApp.Payout.InitPayoutRouter(publicRoot.Group("/v1/app"))
	registerLiveRoutesV1(publicRoot)
	router.RouterGroupApp.Social.InitSocialRouter(publicRoot.Group("/v1/app"))
	router.RouterGroupApp.Wallet.InitWalletRouter(publicRoot.Group("/v1/app"))
	router.RouterGroupApp.Gift.InitGiftRouter(publicRoot.Group("/v1/app"))
	router.RouterGroupApp.Earning.InitEarningRouter(publicRoot.Group("/v1/app"))
}

func registerLiveRoutesV1(publicRoot *gin.RouterGroup) {
	publicGroup := publicRoot.Group("/v1" + MODULE_LIVE)
	privateGroup := publicRoot.Group("/v1" + MODULE_LIVE)
	liveRouter := router.RouterGroupApp.Live
	liveRouter.InitUserRouter(publicGroup, privateGroup)
	liveRouter.InitAnchorRouter(publicGroup, privateGroup)
	liveRouter.InitCategoryRouter(publicGroup)
	liveRouter.InitRoomRouter(publicGroup, privateGroup)
	liveRouter.InitHookRouter(publicGroup, requireSRSHookAuth())
}
