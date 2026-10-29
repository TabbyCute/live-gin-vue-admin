package router

import (
	"tb_live_module/router/example"
	"tb_live_module/router/live"
	"tb_live_module/router/social"
	"tb_live_module/router/system"
	"tb_live_module/router/wallet"
)

var RouterGroupApp = new(RouterGroup)

type RouterGroup struct {
	Social  social.RouterGroup
	Wallet  wallet.RouterGroup
	System  system.RouterGroup
	Example example.RouterGroup
	Live    live.RouterGroup
}
