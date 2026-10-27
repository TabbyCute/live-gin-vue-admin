package social

import "tb_live_module/service"

type ApiGroup struct {
	SocialApi
	SocialAdminApi
}

var socialService = service.ServiceGroupApp.SocialServiceGroup.Service
