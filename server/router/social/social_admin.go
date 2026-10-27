package social

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	api "tb_live_module/api/v1"
	"tb_live_module/middleware"
	"time"
)

type SocialAdminRouter struct{}

func (r *SocialAdminRouter) InitSocialAdminRouter(root *gin.RouterGroup) {
	group := root.Group("social").Use(middleware.RequireAuthority(888), func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}, middleware.OperationRecord())
	a := api.ApiGroupApp.SocialApiGroup.SocialAdminApi
	group.GET("counter/check", a.Check)
	group.POST("counter/repair", a.Repair)
	group.GET("event/list", a.Events)
	group.POST("event/replay", a.Replay)
	group.GET("block/list", a.Blocks)
	group.GET("relationship/check", a.Anomalies)
	group.GET("health", a.Health)
}
