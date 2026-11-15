package anchorhome

import "tb_live_module/service"

type PublicAPI struct{}
type ApiGroup struct {
	PublicAPI
	AlbumAPI
}

var publicService = service.ServiceGroupApp.AnchorHomeServiceGroup
