package initialize

import (
	"net/http"
	"os"

	"tb_live_module/docs"
	"tb_live_module/global"
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type justFilesFilesystem struct {
	fs http.FileSystem
}

func (fs justFilesFilesystem) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err == nil && stat.IsDir() {
		return nil, os.ErrPermission
	}

	return f, nil
}

// 初始化总路由

func Routers() *gin.Engine {
	Router := gin.New()
	// 使用自定义的 Recovery 中间件，记录 panic 并入库
	Router.Use(middleware.GinRecovery(true))
	if gin.Mode() == gin.DebugMode {
		Router.Use(gin.Logger())
	}
	// 开发环境直接请求后端，放行全部跨域请求。
	Router.Use(middleware.Cors())
	global.GVA_LOG.Info("use middleware cors allow all")

	// 如果想要不使用nginx代理前端网页，可以修改 web/.env.production 下的
	// VUE_APP_BASE_API = /
	// VUE_APP_BASE_PATH = http://localhost
	// 然后执行打包命令 npm run build。在打开下面3行注释
	// Router.StaticFile("/favicon.ico", "./dist/favicon.ico")
	// Router.Static("/assets", "./dist/assets")   // dist里面的静态资源
	// Router.StaticFile("/", "./dist/index.html") // 前端网页入口页面

	Router.StaticFS(global.GVA_CONFIG.Local.StorePath, justFilesFilesystem{http.Dir(global.GVA_CONFIG.Local.StorePath)})
	// Router.Use(middleware.LoadTls())  // 如果需要使用https 请打开此中间件 然后前往 core/server.go 将启动模式 更变为 Router.RunTLS("端口","你的cre/pem文件","你的key文件")

	// 如果需要恢复白名单模式，请禁用上面的 Cors，再启用下面这行。
	// Router.Use(middleware.CorsByRules())

	//swagger
	docs.SwaggerInfo.BasePath = global.GVA_CONFIG.System.RouterPrefix + adminV1RoutePrefix
	Router.GET(global.GVA_CONFIG.System.RouterPrefix+"/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	global.GVA_LOG.Info("register swagger handler")

	//App主要是为了提供前缀。
	AppGroup := Router.Group(global.GVA_CONFIG.System.RouterPrefix)

	// 健康监测：开发环境路径为 /api/health。
	Router.GET(global.GVA_CONFIG.System.RouterPrefix+"/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, "ok")
	})

	//注册：system相关的路由
	registerSystemRoutes(AppGroup, Router)

	//注册：live 相关路由
	registerLiveRoutes(AppGroup)

	global.GVA_ROUTERS = Router.Routes()

	global.GVA_LOG.Info("router register success")
	return Router
}
