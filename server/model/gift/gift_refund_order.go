package gift

// Refund 不使用软删除；财务事实由数据库触发器保护。
type Refund struct {
	ID                        uint64 `json:"-" gorm:"primaryKey"`
	RefundNo                  string `json:"refundNo" gorm:"size:40;not null;uniqueIndex"`
	RequestID                 string `json:"requestId" gorm:"size:64;not null;uniqueIndex"`
	GiftOrderNo               string `json:"giftOrderNo" gorm:"size:40;not null;index"`
	RefundQuantity            int64  `json:"refundQuantity,string" gorm:"not null;check:ck_gift_refund_quantity,refund_quantity > 0"`
	RefundCoinAmount          int64  `json:"refundCoinAmount,string" gorm:"not null;check:ck_gift_refund_amount,refund_coin_amount > 0"`
	Reason                    string `json:"reason" gorm:"size:500;not null"`
	OperatorID                uint64 `json:"operatorId,string" gorm:"not null"`
	WalletRefundTransactionNo string `json:"walletRefundTransactionNo" gorm:"size:40;not null;uniqueIndex"`
	WalletRefundLedgerNo      string `json:"walletRefundLedgerNo" gorm:"size:40;not null;uniqueIndex"`
	BenefitCompensationStatus string `json:"benefitCompensationStatus" gorm:"size:20;not null"`
	EarningReversalNo         string `json:"earningReversalNo" gorm:"size:40;not null"`
	Status                    string `json:"status" gorm:"size:20;not null"`
	CreatedAt                 int64  `json:"createdAt" gorm:"not null"`
}

func (Refund) TableName() string { return "gift_refund_order" }
