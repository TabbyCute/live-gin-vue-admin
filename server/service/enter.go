package service

import (
	"tb_live_module/service/example"
	"tb_live_module/service/live"
	"tb_live_module/service/social"
	"tb_live_module/service/system"
)

var ServiceGroupApp = &ServiceGroup{SocialServiceGroup: social.NewServiceGroup()}

type ServiceGroup struct {
	SocialServiceGroup  social.ServiceGroup
	SystemServiceGroup  system.ServiceGroup
	ExampleServiceGroup example.ServiceGroup
	LiveServiceGroup    live.ServiceGroup
}
