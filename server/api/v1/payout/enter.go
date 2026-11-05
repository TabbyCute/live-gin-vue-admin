package payout

import "tb_live_module/service"

type API struct{}
type ApiGroup struct{ API }

var payoutService = service.ServiceGroupApp.PayoutServiceGroup
