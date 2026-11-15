package anchorhome

import api "tb_live_module/api/v1"

type RouterGroup struct{ AlbumRouter }

var publicAPI = api.ApiGroupApp.AnchorHomeApiGroup.PublicAPI
var albumAPI = api.ApiGroupApp.AnchorHomeApiGroup.AlbumAPI
