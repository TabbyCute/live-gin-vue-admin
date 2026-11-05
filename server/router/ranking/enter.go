package ranking

import (
	"github.com/gin-gonic/gin"
	api "tb_live_module/api/v1"
	"tb_live_module/global"
	"tb_live_module/internal/httpkit"
	"tb_live_module/middleware"
	"time"
)

type RouterGroup struct{}

func (RouterGroup) InitRankingRouter(root *gin.RouterGroup) {
	g := root.Group("ranking").Use(httpkit.Budget(64, 8*time.Second), middleware.JWTAppAuth(), httpkit.Ready(&global.RankingReady), httpkit.UserRateLimit())
	a := api.ApiGroupApp.RankingApiGroup.API
	g.GET("gift", a.Boardgift)
	g.GET("wealth", a.Boardwealth)
	g.GET("charm", a.Boardcharm)
}
func (RouterGroup) InitRankingAdminRouter(root *gin.RouterGroup) {
	g := root.Group("ranking").Use(httpkit.Budget(16, 8*time.Second), httpkit.Ready(&global.RankingReady))
	a := api.ApiGroupApp.RankingApiGroup.API
	g.GET("admin/status", a.Status)
	g.GET("admin/board", a.AdminBoard)
	g.GET("admin/job/list", a.Jobs)
	g.POST("admin/rebuild", a.Actionrebuild)
	g.POST("admin/reconcile", a.Actionreconcile)
}
