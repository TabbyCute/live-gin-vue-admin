package live

import "tb_live_module/service"

type ApiGroup struct {
	UserApi
}

var accountService = service.ServiceGroupApp.LiveServiceGroup.AccountService
