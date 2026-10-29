package wallet

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	api "tb_live_module/api/v1"
	"tb_live_module/middleware"
	"time"
)

type WalletRouter struct{}
type WalletAdminRouter struct{}

func guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
func (*WalletRouter) InitWalletRouter(root *gin.RouterGroup) {
	group := root.Group("wallet").Use(guard(), middleware.JWTAppAuth())
	a := api.ApiGroupApp.WalletApiGroup.WalletApi
	group.GET("balance", a.Balance)
	group.GET("ledger", a.Ledger)
}
func (*WalletAdminRouter) InitWalletAdminRouter(root *gin.RouterGroup) {
	// root继承后台JWT；每个Service入口另核验当前数据库权限，不能仅靠菜单。
	group := root.Group("wallet")
	group.Use(guard())
	a := api.ApiGroupApp.WalletApiGroup.WalletAdminApi
	group.GET("account/detail", a.Balance)
	group.GET("account/check", a.Check)
	group.GET("ledger/list", a.Ledger)
	group.GET("adjustment/list", a.List)
	group.GET("adjustment/detail", a.Detail)
	writes := group.Group("").Use(middleware.OperationRecord())
	writes.POST("account/status", a.SetStatus)
	writes.POST("adjustment/create", a.Create)
	writes.POST("adjustment/approve", a.Approve)
	writes.POST("adjustment/reject", a.Reject)
	writes.POST("adjustment/reverse", a.Reverse)
}
