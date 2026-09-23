package live

import api "tb_live_module/api/v1"

type RouterGroup struct {
	UserRouter
	AnchorRouter
	AnchorAdminRouter
	CategoryRouter
	CategoryAdminRouter
	RoomRouter
	RoomAdminRouter
	HookRouter
}

var (
	userApi          = api.ApiGroupApp.LiveApiGroup.UserApi
	anchorApi        = api.ApiGroupApp.LiveApiGroup.AnchorApi
	anchorAdminApi   = api.ApiGroupApp.LiveApiGroup.AnchorAdminApi
	categoryApi      = api.ApiGroupApp.LiveApiGroup.CategoryApi
	categoryAdminApi = api.ApiGroupApp.LiveApiGroup.CategoryAdminApi
	roomApi          = api.ApiGroupApp.LiveApiGroup.RoomApi
	roomAdminApi     = api.ApiGroupApp.LiveApiGroup.RoomAdminApi
	hookApi          = api.ApiGroupApp.LiveApiGroup.HookApi
)
