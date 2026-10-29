package v1

import (
	"tb_live_module/api/v1/example"
	"tb_live_module/api/v1/live"
	"tb_live_module/api/v1/social"
	"tb_live_module/api/v1/system"
	"tb_live_module/api/v1/wallet"
)

var ApiGroupApp = new(ApiGroup)

type ApiGroup struct {
	SocialApiGroup  social.ApiGroup
	WalletApiGroup  wallet.ApiGroup
	SystemApiGroup  system.ApiGroup
	ExampleApiGroup example.ApiGroup
	LiveApiGroup    live.ApiGroup
}
