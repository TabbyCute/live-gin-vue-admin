package live

import api "tb_live_module/api/v1"

type RouterGroup struct {
	UserRouter
}

var userApi = api.ApiGroupApp.LiveApiGroup.UserApi
