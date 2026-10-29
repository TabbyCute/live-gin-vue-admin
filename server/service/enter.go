package service

import (
	"tb_live_module/service/example"
	"tb_live_module/service/live"
	"tb_live_module/service/social"
	"tb_live_module/service/system"
	"tb_live_module/service/wallet"
)

var ServiceGroupApp = &ServiceGroup{SocialServiceGroup: social.NewServiceGroup(), WalletServiceGroup: wallet.NewServiceGroup()}

type ServiceGroup struct {
	SocialServiceGroup  social.ServiceGroup
	WalletServiceGroup  wallet.ServiceGroup
	SystemServiceGroup  system.ServiceGroup
	ExampleServiceGroup example.ServiceGroup
	LiveServiceGroup    live.ServiceGroup
}
