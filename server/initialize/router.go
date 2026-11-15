package initialize

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	serverassets "tb_live_module/assets"
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

const publicHTMLRoute = "/app/v1/assets/html"

const (
	publicAdminRoute     = "/app/v1/assets/admin"
	publicAdminAssetsDir = "assets/admin"
	publicH5Route        = "/app/v1/assets/h5"
	publicH5AssetsDir    = "assets/h5"
)

func (fs justFilesFilesystem) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if stat.IsDir() {
		f.Close()
		return nil, os.ErrPermission
	}

	return f, nil
}

func registerPublicHTML(router *gin.Engine) {
	htmlFiles, err := fs.Sub(serverassets.HTML, "html")
	if err != nil {
		panic("initialize embedded HTML assets: " + err.Error())
	}

	router.StaticFS(publicHTMLRoute, justFilesFilesystem{fs: http.FS(htmlFiles)})
}

func registerPublicAdmin(router *gin.Engine, adminFiles http.FileSystem) {
	registerPublicFrontend(router, publicAdminRoute, adminFiles, false)
}

func registerPublicH5(router *gin.Engine, h5Files http.FileSystem) {
	registerPublicFrontend(router, publicH5Route, h5Files, true)
}

func registerPublicFrontend(router *gin.Engine, route string, files http.FileSystem, historyFallback bool) {
	filesOnly := justFilesFilesystem{fs: files}
	serve := func(c *gin.Context) {
		name := c.Param("filepath")
		// 不公开隐藏文件，也不接受包含目录穿越或反斜杠的路径。
		for _, segment := range strings.Split(name, "/") {
			if strings.HasPrefix(segment, ".") || strings.Contains(segment, "\\") {
				c.Status(http.StatusNotFound)
				return
			}
		}
		if name == "" || name == "/" {
			name = "/index.html"
		}

		file, err := filesOnly.Open(name)
		// H5 使用 history 路由。页面刷新回退到入口，丢失的静态资源仍返回 404。
		if historyFallback && os.IsNotExist(err) && path.Ext(name) == "" &&
			name != "/assets" && !strings.HasPrefix(name, "/assets/") &&
			name != "/cdn_scripts" && !strings.HasPrefix(name, "/cdn_scripts/") {
			name = "/index.html"
			file, err = filesOnly.Open(name)
		}
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil || stat.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}

		if name == "/index.html" {
			c.Header("Cache-Control", "no-cache")
		} else if strings.HasPrefix(name, "/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache")
		}
		http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), file)
	}

	router.GET(route, serve)
	router.HEAD(route, serve)
	router.GET(route+"/*filepath", serve)
	router.HEAD(route+"/*filepath", serve)
}

// 初始化总路由

func Routers() *gin.Engine {
	Router := gin.New()
	// All ClientIP consumers share the explicit proxy allowlist; an empty list
	// trusts only the socket peer, rather than Gin's default of every proxy.
	if err := Router.SetTrustedProxies(global.GVA_CONFIG.LiveAccountAuth.TrustedProxies); err != nil {
		panic("invalid trusted proxy configuration")
	}
	Router.Use(middleware.RequestObservability())
	// 使用自定义的 Recovery 中间件，记录 panic 并入库
	Router.Use(middleware.GinRecovery(true))
	// 浏览器跨域请求统一按配置处理；无 Origin 的 SRS hook 和原生客户端不受 CORS 限制。
	Router.Use(middleware.CorsByRules())
	//global.GVA_LOG.Info("use middleware cors by configured rules")

	// 如果想要不使用nginx代理前端网页，可以修改 web/.env.production 下的
	// VUE_APP_BASE_API = /
	// VUE_APP_BASE_PATH = http://localhost
	// 然后执行打包命令 npm run build。在打开下面3行注释
	// Router.StaticFile("/favicon.ico", "./dist/favicon.ico")
	// Router.Static("/assets", "./dist/assets")   // dist里面的静态资源
	// Router.StaticFile("/", "./dist/index.html") // 前端网页入口页面

	Router.StaticFS(global.GVA_CONFIG.Local.StorePath, justFilesFilesystem{http.Dir(global.GVA_CONFIG.Local.StorePath)})
	if gin.Mode() != gin.ReleaseMode {
		registerPublicHTML(Router)
	}
	registerPublicAdmin(Router, http.Dir(publicAdminAssetsDir))
	registerPublicH5(Router, http.Dir(publicH5AssetsDir))
	// Router.Use(middleware.LoadTls())  // 如果需要使用https 请打开此中间件 然后前往 core/server.go 将启动模式 更变为 Router.RunTLS("端口","你的cre/pem文件","你的key文件")

	//swagger
	docs.SwaggerInfo.BasePath = global.GVA_CONFIG.System.RouterPrefix
	registerProjectSwagger()
	if gin.Mode() != gin.ReleaseMode {
		Router.GET(global.GVA_CONFIG.System.RouterPrefix+"/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName(projectSwaggerInstance)))
		global.GVA_LOG.Info("register swagger handler")
	}

	//App主要是为了提供前缀。
	AppGroup := Router.Group(global.GVA_CONFIG.System.RouterPrefix)

	// 健康监测：开发环境路径为 /api/health。
	Router.GET(global.GVA_CONFIG.System.RouterPrefix+"/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, "ok")
	})
	Router.GET(global.GVA_CONFIG.System.RouterPrefix+"/ready", readiness)

	//注册：system相关的路由
	registerSystemRoutes(AppGroup, Router)

	//注册：live 相关路由
	registerLiveRoutes(AppGroup)

	global.GVA_ROUTERS = Router.Routes()

	global.GVA_LOG.Info("router register success")
	return Router
}
