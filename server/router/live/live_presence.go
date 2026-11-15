package live

import (
	"github.com/gin-gonic/gin"
	liveAPI "tb_live_module/api/v1/live"
	socialAPI "tb_live_module/api/v1/social"
	"tb_live_module/middleware"
)

type PresenceRouter struct{}

func (r *PresenceRouter) InitPresenceRouter(root *gin.RouterGroup) {
	g := root.Group("presence").Use(liveAPI.PresenceBudget(), middleware.PresenceJWTSignature())
	g.POST("guest", presenceApi.Guest)
	g.POST("open", presenceApi.Open)
	g.POST("heartbeat", presenceApi.Heartbeat)
	g.POST("leave", presenceApi.Leave)
	g.GET("stats", presenceApi.Stats)
	g.GET("viewers", presenceApi.Viewers)
	g.GET("active-viewers", presenceApi.ActiveViewers)
	root.POST("room/like", liveAPI.PresenceBudget(), middleware.PresenceJWTSignature(), presenceApi.Like)
}
func (r *PresenceRouter) InitPresenceAdminRouter(root *gin.RouterGroup) {
	// Existing social administrator validation checks current enabled account and role
	// membership, including fixed root. No body-recording middleware for bearer payloads.
	root.Group("live/presence").Use(socialAPI.RequestBudget(), middleware.RequireAuthority(888), socialAPI.AdminGuard()).POST("stats", presenceApi.AdminStats)
}
