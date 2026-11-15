package bankreconcile

import "tb_live_module/service"

type API struct{}
type ApiGroup struct{ API }

var bankService = service.ServiceGroupApp.BankReconcileServiceGroup
