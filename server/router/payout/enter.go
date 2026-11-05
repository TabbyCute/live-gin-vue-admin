package payout

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	api "tb_live_module/api/v1"
	"tb_live_module/middleware"
	svc "tb_live_module/service/payout"
	"time"
)

type RouterGroup struct{}

func guard(limit int, budget time.Duration) gin.HandlerFunc {
	return guardSized(limit, budget, 8<<10)
}
func guardSized(limit int, budget time.Duration, maxBytes int64) gin.HandlerFunc {
	// 容量在路由初始化时固定。无等待队列、无按IP增长的map；APP与后台独立。
	slots := make(chan struct{}, limit)
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(c.Request.Context(), budget)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		// 包括拒绝请求后的HTTP正文清理，也不能继承上传接口的长超时。
		controller := http.NewResponseController(c.Writer)
		deadline, _ := ctx.Deadline()
		_ = controller.SetReadDeadline(deadline)
		if c.Request.ContentLength > maxBytes || len(c.Request.URL.RawQuery) > 1024 || len(c.GetHeader("Authorization")) > 8192 || len(c.GetHeader("x-token")) > 8192 {
			if c.Request.ProtoMajor == 1 {
				c.Header("Connection", "close")
			}
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"code": svc.ErrInvalid.Code, "msg": svc.ErrInvalid.Message, "data": gin.H{}})
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			if c.Request.ProtoMajor == 1 {
				c.Header("Connection", "close")
			}
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": svc.ErrUnavailable.Code, "msg": svc.ErrUnavailable.Message, "data": gin.H{}})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		// 不在Handler退出时清除：net/http随后还可能丢弃未读正文。
		// 清除会使超时请求再次无限阻塞；下次请求由HTTP server重置期限。
		c.Next()
	}
}
func (RouterGroup) InitPayoutRouter(root *gin.RouterGroup) {
	g := root.Group("payout/payee").Use(guard(16, 8*time.Second), middleware.JWTAppAuth(), userRateLimit())
	a := api.ApiGroupApp.PayoutApiGroup.API
	g.GET("list", a.List)
	g.POST("create", a.Create)
	g.POST("set-default", a.SetDefault)
	g.POST("disable", a.Disable)
}
func (RouterGroup) InitPayoutAdminRouter(root *gin.RouterGroup) {
	initManualRoutes(root)
	g := root.Group("payout/order").Use(guard(8, 8*time.Second), middleware.JWTAuth(), adminRateLimit())
	a := api.ApiGroupApp.PayoutApiGroup.API
	g.POST("create", a.CreateOrder)
	g.GET("list", a.OrderList)
	g.GET("detail", a.OrderDetail)
}
