package service

import (
	"tb_live_module/service/example"
	"tb_live_module/service/live"
	"tb_live_module/service/system"
)

var ServiceGroupApp = new(ServiceGroup)

type ServiceGroup struct {
	SystemServiceGroup  system.ServiceGroup
	ExampleServiceGroup example.ServiceGroup
	LiveServiceGroup    live.ServiceGroup
}
