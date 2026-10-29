package wallet

import "tb_live_module/service"

type ApiGroup struct {
	WalletApi
	WalletAdminApi
}

var walletService = service.ServiceGroupApp.WalletServiceGroup.Service
