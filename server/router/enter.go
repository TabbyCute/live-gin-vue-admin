package router

import (
	"tb_live_module/router/example"
	"tb_live_module/router/system"
)

var RouterGroupApp = new(RouterGroup)

type RouterGroup struct {
	System  system.RouterGroup
	Example example.RouterGroup
}
