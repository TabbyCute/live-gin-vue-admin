package bankreconcile

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"tb_live_module/middleware"
	"time"
)

type Router struct{}

func (Router) InitBankReconcileRouter(root *gin.RouterGroup) {
	// Fixed global capacity prevents memory pressure from simultaneous CSV bodies.
	slots := make(chan struct{}, 2)
	budget := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		controller := http.NewResponseController(c.Writer)
		_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
		// Keep the deadline while net/http drains unread bytes after the handler.
		// Clearing it here permits a timed-out peer to stall response completion.
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			if c.Request.ProtoMajor == 1 {
				c.Header("Connection", "close")
			}
			c.AbortWithStatusJSON(429, gin.H{"code": 49303, "data": gin.H{}, "msg": "核对服务繁忙，请稍后使用原请求键重试"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		if c.Request.ContentLength > 1<<20 {
			if c.Request.ProtoMajor == 1 {
				c.Header("Connection", "close")
			}
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Next()
	}
	g := root.Group("payout/bank-reconciliation").Use(budget, middleware.JWTAuth())
	g.POST("import", bankAPI.Import)
	g.POST("run", bankAPI.Run)
	g.POST("review", bankAPI.Review)
	g.GET("list", bankAPI.List)
	g.GET("detail", bankAPI.Detail)
}
