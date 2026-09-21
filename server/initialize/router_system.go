package initialize

import (
	"tb_live_module/middleware"
	"tb_live_module/router"

	"github.com/gin-gonic/gin"
)

const adminV1RoutePrefix = "/v1/admin"

func registerSystemRoutes(apiRoot *gin.RouterGroup, engine *gin.Engine) {
	registerSystemRoutesV1(apiRoot, engine)
}

func registerSystemRoutesV1(apiRoot *gin.RouterGroup, engine *gin.Engine) {
	adminPublicGroup := apiRoot.Group(adminV1RoutePrefix)
	adminPrivateGroup := apiRoot.Group(adminV1RoutePrefix)
	adminPrivateGroup.Use(middleware.JWTAuth())

	systemRouter := router.RouterGroupApp.System
	// 无需登录
	systemRouter.InitBaseRouter(adminPublicGroup)
	systemRouter.InitInitRouter(adminPublicGroup)

	// 后台接口只校验管理员登录；角色可见范围由动态菜单分配控制。
	systemRouter.InitJwtRouter(adminPrivateGroup)
	systemRouter.InitUserRouter(adminPrivateGroup)
	systemRouter.InitMenuRouter(adminPrivateGroup)
	systemRouter.InitSystemRouter(adminPrivateGroup)
	systemRouter.InitSysVersionRouter(adminPrivateGroup)
	systemRouter.InitAuthorityRouter(adminPrivateGroup)
	systemRouter.InitSysDictionaryRouter(adminPrivateGroup)
	systemRouter.InitSysOperationRecordRouter(adminPrivateGroup)
	systemRouter.InitSysDictionaryDetailRouter(adminPrivateGroup)
	systemRouter.InitSysExportTemplateRouter(adminPrivateGroup, adminPublicGroup)
	systemRouter.InitSysParamsRouter(adminPrivateGroup, adminPublicGroup)
	systemRouter.InitSysErrorRouter(adminPrivateGroup, adminPublicGroup)
	systemRouter.InitLoginLogRouter(adminPrivateGroup)
	systemRouter.InitApiTokenRouter(adminPrivateGroup)

	// 示例模块也属于后台私有路由。
	exampleRouter := router.RouterGroupApp.Example
	exampleRouter.InitCustomerRouter(adminPrivateGroup)                 // 客户路由
	exampleRouter.InitFileUploadAndDownloadRouter(adminPrivateGroup)    // 文件上传下载功能路由
	exampleRouter.InitAttachmentCategoryRouterRouter(adminPrivateGroup) // 文件上传下载分类

	// 直播后台接口继承 adminPrivateGroup 的 JWT，并由 live Router 继续区分读写操作日志。
	liveRouter := router.RouterGroupApp.Live
	liveRouter.InitAnchorAdminRouter(adminPrivateGroup)
	liveRouter.InitCategoryAdminRouter(adminPrivateGroup)

	//注册：插件路由安装
	InstallPlugin(adminPrivateGroup, adminPublicGroup, engine)

	////注册：注册业务路由。是“业务模块路由统一注册入口”，主要给自动代码生成器使用
	initBizRouter(adminPrivateGroup, adminPublicGroup)
}
