package service

import (
	"tb_live_module/service/anchorhome"
	"tb_live_module/service/bankreconcile"
	"tb_live_module/service/earning"
	"tb_live_module/service/eventdelivery"
	"tb_live_module/service/example"
	"tb_live_module/service/gift"
	"tb_live_module/service/guard"
	"tb_live_module/service/live"
	"tb_live_module/service/payout"
	"tb_live_module/service/ranking"
	"tb_live_module/service/social"
	"tb_live_module/service/system"
	"tb_live_module/service/wallet"
)

var ServiceGroupApp = &ServiceGroup{AnchorHomeServiceGroup: anchorhome.NewServiceGroup(), BankReconcileServiceGroup: bankreconcile.NewServiceGroup(), EventDeliveryServiceGroup: eventdelivery.NewServiceGroup(), LiveServiceGroup: live.ServiceGroup{PresenceService: live.DefaultPresence}, GuardServiceGroup: guard.NewServiceGroup(), RankingServiceGroup: ranking.NewServiceGroup(), PayoutServiceGroup: payout.NewServiceGroup(), EarningServiceGroup: earning.NewServiceGroup(), GiftServiceGroup: gift.NewServiceGroup(), SocialServiceGroup: social.NewServiceGroup(), WalletServiceGroup: wallet.NewServiceGroup()}

type ServiceGroup struct {
	AnchorHomeServiceGroup    anchorhome.ServiceGroup
	EventDeliveryServiceGroup eventdelivery.ServiceGroup
	BankReconcileServiceGroup bankreconcile.ServiceGroup
	GuardServiceGroup         guard.ServiceGroup
	RankingServiceGroup       ranking.ServiceGroup
	PayoutServiceGroup        payout.ServiceGroup
	EarningServiceGroup       earning.ServiceGroup
	GiftServiceGroup          gift.ServiceGroup
	SocialServiceGroup        social.ServiceGroup
	WalletServiceGroup        wallet.ServiceGroup
	SystemServiceGroup        system.ServiceGroup
	ExampleServiceGroup       example.ServiceGroup
	LiveServiceGroup          live.ServiceGroup
}
