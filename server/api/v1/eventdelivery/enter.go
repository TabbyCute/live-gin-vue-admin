package eventdelivery

import "tb_live_module/service"

type API struct{}
type ApiGroup struct{ API }

var deliveryService = service.ServiceGroupApp.EventDeliveryServiceGroup
