package live

import "tb_live_module/service"

type ApiGroup struct {
	UserApi
	AnchorApi
	AnchorAdminApi
	CategoryApi
	CategoryAdminApi
}

var (
	accountService  = service.ServiceGroupApp.LiveServiceGroup.AccountService
	anchorService   = service.ServiceGroupApp.LiveServiceGroup.AnchorService
	categoryService = service.ServiceGroupApp.LiveServiceGroup.CategoryService
)
