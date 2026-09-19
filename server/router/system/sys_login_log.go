package system

import (
	"tb_live_module/api/v1"
	"tb_live_module/middleware"
	"github.com/gin-gonic/gin"
)

type LoginLogRouter struct{}

func (s *LoginLogRouter) InitLoginLogRouter(Router *gin.RouterGroup) {
	loginLogRouter := Router.Group("sysLoginLog").Use(middleware.OperationRecord())
	loginLogRouterWithoutRecord := Router.Group("sysLoginLog")
	sysLoginLogApi := v1.ApiGroupApp.SystemApiGroup.LoginLogApi
	{
		loginLogRouter.DELETE("deleteLoginLog", sysLoginLogApi.DeleteLoginLog)           // 删除登录日志
		loginLogRouter.DELETE("deleteLoginLogByIds", sysLoginLogApi.DeleteLoginLogByIds) // 批量删除登录日志
	}
	{
		loginLogRouterWithoutRecord.GET("findLoginLog", sysLoginLogApi.FindLoginLog)       // 根据ID获取登录日志(详情)
		loginLogRouterWithoutRecord.GET("getLoginLogList", sysLoginLogApi.GetLoginLogList) // 获取登录日志列表
	}
}
