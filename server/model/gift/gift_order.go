package gift

// Order 不使用软删除；财务事实由数据库触发器保护。
type Order struct {
	ID                          uint64 `json:"-" gorm:"primaryKey"`
	OrderNo                     string `json:"orderNo" gorm:"size:40;not null;uniqueIndex"`
	RequestID                   string `json:"requestId" gorm:"size:64;not null;uniqueIndex:uk_gift_request,priority:2"`
	SenderUserID                uint64 `json:"-" gorm:"not null;uniqueIndex:uk_gift_request,priority:1;index:idx_gift_sender_time,priority:1"`
	SenderAccountNo             string `json:"senderAccountNo" gorm:"size:40;not null;index"`
	AnchorID                    uint64 `json:"-" gorm:"not null;index"`
	AnchorNo                    string `json:"anchorNo" gorm:"size:40;not null;index:idx_gift_anchor_time,priority:1"`
	RoomID                      uint64 `json:"-" gorm:"not null;index"`
	RoomNo                      string `json:"roomNo" gorm:"size:40;not null;index:idx_gift_room_time,priority:1"`
	SessionID                   uint64 `json:"-" gorm:"not null;index"`
	SessionNo                   string `json:"sessionNo" gorm:"size:40;not null;index:idx_gift_session_time,priority:1"`
	CategoryID                  uint64 `json:"-" gorm:"not null"`
	CategoryNo                  string `json:"categoryNo" gorm:"size:40;not null;index"`
	CategoryName                string `json:"categoryName" gorm:"size:64;not null"`
	GiftID                      uint64 `json:"-" gorm:"not null"`
	GiftNo                      string `json:"giftNo" gorm:"size:40;not null;index"`
	GiftName                    string `json:"giftName" gorm:"size:64;not null"`
	IconURL                     string `json:"iconUrl" gorm:"size:500;not null"`
	AnimationURL                string `json:"animationUrl" gorm:"size:500;not null"`
	AnimationType               string `json:"animationType" gorm:"size:16;not null"`
	UnitPrice                   int64  `json:"unitPrice,string" gorm:"not null;check:ck_gift_order_price,unit_price > 0"`
	Quantity                    int64  `json:"quantity,string" gorm:"not null;check:ck_gift_order_quantity,quantity BETWEEN 1 AND 10000"`
	GrossCoinAmount             int64  `json:"grossCoinAmount,string" gorm:"not null;check:ck_gift_order_amount,gross_coin_amount > 0 AND gross_coin_amount % unit_price = 0 AND gross_coin_amount / unit_price = quantity"`
	RefundedCoinAmount          int64  `json:"refundedCoinAmount,string" gorm:"not null;check:ck_gift_order_refund,refunded_coin_amount >= 0 AND refunded_coin_amount <= gross_coin_amount AND refunded_coin_amount % unit_price = 0"`
	WalletTransactionNo         string `json:"walletTransactionNo" gorm:"size:40;not null;uniqueIndex"`
	WalletLedgerNo              string `json:"walletLedgerNo" gorm:"size:40;not null;uniqueIndex"`
	RevenueRuleNo               string `json:"revenueRuleNo" gorm:"size:40;not null"`
	AnchorShareBPS              int64  `json:"anchorShareBps,string" gorm:"not null;check:ck_gift_order_share,anchor_share_bps BETWEEN 0 AND 10000"`
	EstimatedAnchorEarningCoin  int64  `json:"estimatedAnchorEarningCoin,string" gorm:"not null"`
	EstimatedPlatformIncomeCoin int64  `json:"estimatedPlatformIncomeCoin,string" gorm:"not null"`
	BenefitSnapshot             string `json:"benefitSnapshot" gorm:"type:text;not null"`
	Status                      string `json:"status" gorm:"size:20;not null;check:ck_gift_order_status,status IN ('succeeded','partial_refund','refunded')"`
	ActorIP                     string `json:"-" gorm:"size:45;not null"`
	TraceID                     string `json:"-" gorm:"size:64;not null"`
	CreatedAt                   int64  `json:"createdAt" gorm:"not null;index;index:idx_gift_sender_time,priority:2;index:idx_gift_anchor_time,priority:2;index:idx_gift_room_time,priority:2;index:idx_gift_session_time,priority:2"`
}

func (Order) TableName() string { return "gift_order" }
