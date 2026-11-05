package earning

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	api "tb_live_module/api/v1"
	"tb_live_module/middleware"
	svc "tb_live_module/service/earning"
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
func (RouterGroup) InitEarningRouter(root *gin.RouterGroup) {
	g := root.Group("earning").Use(guard(32, 8*time.Second), middleware.JWTAppAuth(), userRateLimit())
	a := api.ApiGroupApp.EarningApiGroup.API
	g.GET("overview", a.AppOverview)
	g.GET("balance", a.AppBalance)
	g.GET("settlement/list", a.AppSettlementList)
	g.GET("withdrawal/list", a.AppWithdrawalList)
	g.GET("withdrawal/detail", a.AppWithdrawalDetail)
	g.POST("withdrawal/apply", a.AppApplyWithdrawal)
}
func (RouterGroup) InitEarningAdminRouter(root *gin.RouterGroup) {
	g := root.Group("earning").Use(guard(16, 8*time.Second))
	a := api.ApiGroupApp.EarningApiGroup.API
	g.GET("income/list", a.IncomeList)
	g.GET("exchange/list", a.ExchangeList)
	g.GET("settlement/list", a.SettlementList)
	g.GET("account/list", a.AccountList)
	g.GET("withdrawal/list", a.WithdrawalList)
	g.GET("reconciliation/list", a.ReconciliationList)
	g.POST("exchange/create", a.CreateExchange)
	g.POST("exchange/review", a.ReviewExchange)
	g.POST("settlement/create", a.CreateSettlement)
	g.POST("settlement/review", a.ReviewSettlement)
	g.POST("withdrawal/review", a.ReviewWithdrawal)
	g.POST("reconciliation/run", a.RunReconciliation)
	g.GET("overview", a.Overview)
	g.GET("settlement/detail", a.SettlementDetail)
	g.GET("withdrawal/detail", a.WithdrawalDetail)
	g.GET("account/ledger", a.AccountLedger)
	g.GET("account/check", a.AccountCheck)
}
