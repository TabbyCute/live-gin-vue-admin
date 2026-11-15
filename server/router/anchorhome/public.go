package anchorhome

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/global"
	"tb_live_module/internal/httpkit"
	"tb_live_module/middleware"
	"time"
)

func (RouterGroup) InitAnchorHomePublicRouter(root *gin.RouterGroup) {
	g := root.Group("anchor-home").Use(httpkit.Budget(32, 5*time.Second), middleware.AnchorHomeReadLimit("json", global.GVA_CONFIG.AnchorHome.PublicReadPerMinute), middleware.OptionalJWTAppAuth())
	g.GET("profile", publicAPI.Profile)
	g.GET("supporters", httpkit.Ready(&global.RankingReady), publicAPI.Supporters)
	g.GET("guards", httpkit.Ready(&global.GuardReady), publicAPI.Guards)
	p := root.Group("anchor-home").Use(httpkit.Budget(8, 5*time.Second), middleware.JWTAppAuth(), httpkit.UserRateLimit())
	p.GET("settings", publicAPI.Settings)
	p.POST("settings", publicAPI.SaveSettings)
}
