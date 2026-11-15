package eventdelivery

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/api/v1"
	"tb_live_module/internal/httpkit"
	"tb_live_module/middleware"
	"time"
)

type RouterGroup struct{}

// Pass the admin URL group before authentication so budget/no-store cover JWT rejection.
func (RouterGroup) InitEventDeliveryRouter(private *gin.RouterGroup) {
	api := v1.ApiGroupApp.EventDeliveryApiGroup
	r := private.Group("event-delivery").Use(httpkit.Budget(4, 5*time.Second), middleware.JWTAuth())
	r.GET("list", api.List)
	r.GET("detail", api.Detail)
	r.POST("retry", api.Retry)
}
