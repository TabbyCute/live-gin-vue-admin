package wallet

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	api "tb_live_module/api/v1"
	"tb_live_module/global"
	"tb_live_module/middleware"
	svc "tb_live_module/service/wallet"
	"time"
)

type WalletRouter struct{}
type WalletAdminRouter struct{}

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
		if c.Request.ContentLength > 16<<10 || len(c.Request.URL.RawQuery) > 2048 || len(c.GetHeader("Authorization")) > 8192 || len(c.GetHeader("x-token")) > 8192 {
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
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		// 不在Handler退出时清除：net/http随后还可能丢弃未读正文。
		// 清除会使超时请求再次无限阻塞；下次请求由HTTP server重置期限。
		c.Next()
	}
}
func (*WalletRouter) InitWalletRouter(root *gin.RouterGroup) {
	cfg, err := global.GVA_CONFIG.Wallet.Normalize()
	if err != nil {
		panic(err)
	}
	group := root.Group("wallet").Use(guard(cfg.AppMaxInflight, 8*time.Second), middleware.JWTAppAuth())
	a := api.ApiGroupApp.WalletApiGroup.WalletApi
	group.GET("balance", a.Balance)
	group.GET("ledger", a.Ledger)
}
func (*WalletAdminRouter) InitWalletAdminRouter(root *gin.RouterGroup) {
	// root继承后台JWT；每个Service入口另核验当前数据库权限，不能仅靠菜单。
	group := root.Group("wallet")
	cfg, err := global.GVA_CONFIG.Wallet.Normalize()
	if err != nil {
		panic(err)
	}
	group.Use(guard(cfg.AdminMaxInflight, 8*time.Second))
	a := api.ApiGroupApp.WalletApiGroup.WalletAdminApi
	group.GET("account/detail", a.Balance)
	group.GET("account/check", a.Check)
	group.GET("ledger/list", a.Ledger)
	group.GET("transaction/trace", a.Trace)
	group.GET("adjustment/list", a.List)
	group.GET("adjustment/detail", a.Detail)
	writes := group.Group("").Use(middleware.OperationRecord())
	writes.POST("account/status", a.SetStatus)
	writes.POST("adjustment/create", a.Create)
	writes.POST("adjustment/approve", a.Approve)
	writes.POST("adjustment/reject", a.Reject)
	writes.POST("adjustment/reverse", a.Reverse)
}
