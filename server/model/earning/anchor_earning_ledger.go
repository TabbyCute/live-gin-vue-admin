package earning

// Ledger 不使用软删除；财务事实由数据库触发器保护。
type Ledger struct {
	ID                uint64 `json:"-" gorm:"primaryKey"`
	EarningNo         string `json:"earningNo" gorm:"size:40;not null;uniqueIndex"`
	AnchorID          uint64 `json:"-" gorm:"not null;index"`
	GiftOrderNo       string `json:"giftOrderNo" gorm:"size:40;not null;index"`
	BusinessNo        string `json:"businessNo" gorm:"size:40;not null;uniqueIndex"`
	EntryType         string `json:"entryType" gorm:"size:16;not null;check:ck_earning_type,entry_type IN ('earning','reversal')"`
	RevenueRuleNo     string `json:"revenueRuleNo" gorm:"size:40;not null"`
	ShareBPS          int64  `json:"shareBps,string" gorm:"not null"`
	EarningCoinAmount int64  `json:"earningCoinAmount,string" gorm:"not null"`
	Status            string `json:"status" gorm:"size:16;not null"`
	CreatedAt         int64  `json:"createdAt" gorm:"not null"`
}

func (Ledger) TableName() string { return "anchor_earning_ledger" }
