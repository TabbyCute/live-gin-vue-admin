package router

import (
	"tb_live_module/router/earning"
	"tb_live_module/router/example"
	"tb_live_module/router/gift"
	"tb_live_module/router/guard"
	"tb_live_module/router/live"
	"tb_live_module/router/payout"
	"tb_live_module/router/ranking"
	"tb_live_module/router/social"
	"tb_live_module/router/system"
	"tb_live_module/router/wallet"
)

var RouterGroupApp = new(RouterGroup)

type RouterGroup struct {
	Guard   guard.RouterGroup
	Ranking ranking.RouterGroup
	Payout  payout.RouterGroup
	Earning earning.RouterGroup
	Gift    gift.RouterGroup
	Social  social.RouterGroup
	Wallet  wallet.RouterGroup
	System  system.RouterGroup
	Example example.RouterGroup
	Live    live.RouterGroup
}
