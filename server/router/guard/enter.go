package guard

import (
	"github.com/gin-gonic/gin"
	api "tb_live_module/api/v1"
	"tb_live_module/global"
	"tb_live_module/internal/httpkit"
	"tb_live_module/middleware"
	"time"
)

type RouterGroup struct{}

func (RouterGroup) InitGuardRouter(root *gin.RouterGroup) {
	g := root.Group("guard").Use(httpkit.Budget(64, 8*time.Second), middleware.JWTAppAuth(), httpkit.Ready(&global.GuardReady), httpkit.UserRateLimit())
	a := api.ApiGroupApp.GuardApiGroup.API
	g.GET("plan/list", a.Plans)
	g.GET("status", a.Status)
	g.GET("mine", a.Mine)
	g.GET("anchor/members", a.Members)
}
func (RouterGroup) InitGuardAdminRouter(root *gin.RouterGroup) {
	g := root.Group("guard").Use(httpkit.Budget(16, 8*time.Second), httpkit.Ready(&global.GuardReady))
	a := api.ApiGroupApp.GuardApiGroup.API
	g.GET("plan/list", a.AdminPlans)
	g.POST("plan/save", a.SavePlan)
	g.GET("member/list", a.AdminMemberlist)
	g.GET("member/detail", a.AdminMemberdetail)
	g.GET("grant/list", a.Admingrant)
	g.GET("compensation/list", a.Admincompensation)
	g.GET("ledger/list", a.Adminledger)
	g.GET("task/list", a.Admintask)
	g.POST("task/retry", a.Actiontaskretry)
	g.POST("rebuild", a.Actionrebuild)
	g.POST("reconcile", a.Actionreconcile)
}
