package announcement

import (
	"context"
	"github.com/gin-gonic/gin"
	"tb_live_module/plugin/announcement/initialize"
	interfaces "tb_live_module/utils/plugin/v2"
)

var _ interfaces.Plugin = (*plugin)(nil)

var Plugin = new(plugin)

type plugin struct{}

func init() {
	interfaces.Register(Plugin)
}

func (p *plugin) Register(group *gin.Engine) {
	ctx := context.Background()
	// 如果需要配置文件，请到 config.Config 中填充配置结构，并在当前环境配置文件中填写对应 key
	// initialize.Viper()
	// 安装插件时候自动注册的api数据请到下方法.Api方法中实现
	initialize.Api(ctx)
	// 安装插件时候自动注册的Menu数据请到下方法.Menu方法中实现
	initialize.Menu(ctx)
	// 安装插件时候自动注册的Dictionary数据请到下方法.Dictionary方法中实现
	initialize.Dictionary(ctx)
	initialize.Gorm(ctx)
	initialize.Router(group)
}
