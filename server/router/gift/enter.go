package gift

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	api "tb_live_module/api/v1"
	"tb_live_module/middleware"
	svc "tb_live_module/service/gift"
	"time"
)

type RouterGroup struct{}

func guard(limit int, budget time.Duration) gin.HandlerFunc {
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
		if c.Request.ContentLength > 64<<10 || len(c.Request.URL.RawQuery) > 2048 || len(c.GetHeader("Authorization")) > 8192 || len(c.GetHeader("x-token")) > 8192 {
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
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		// 不在Handler退出时清除：net/http随后还可能丢弃未读正文。
		// 清除会使超时请求再次无限阻塞；下次请求由HTTP server重置期限。
		c.Next()
	}
}
func (RouterGroup) InitGiftRouter(root *gin.RouterGroup) {
	g := root.Group("gift").Use(guard(128, 8*time.Second), middleware.JWTAppAuth(), userRateLimit())
	a := api.ApiGroupApp.GiftApiGroup.API
	g.GET("category/list", a.Categories)
	g.GET("item/list", a.Items)
	g.POST("send", a.Send)
	g.GET("result", a.Result)
	g.GET("order/list", a.History)
}
func (RouterGroup) InitGiftAdminRouter(root *gin.RouterGroup) {
	g := root.Group("gift").Use(guard(32, 8*time.Second))
	a := api.ApiGroupApp.GiftApiGroup.API
	g.GET("category/list", a.AdminCategories)
	g.GET("item/list", a.AdminItems)
	g.POST("category/save", a.SaveCategory)
	g.POST("item/save", a.SaveItem)
	g.POST("sort", a.Sort)
	g.POST("delete", a.Delete)
	g.GET("order/list", a.Orders)
	g.GET("order/detail", a.Detail)
	g.POST("refund", a.Refund)
	g.GET("ranking", a.Ranking)
	g.GET("rule/list", a.Rules)
	g.POST("rule/create", a.CreateRule)
	g.POST("rule/review", a.ReviewRule)
	g.POST("rule/terminate", a.TerminateRule)
	g.POST("rule/terminate/review", a.ReviewTermination)
}
