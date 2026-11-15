package bankreconcile

import api "tb_live_module/api/v1"

type RouterGroup struct{ Router }

var bankAPI = api.ApiGroupApp.BankReconcileApiGroup.API
