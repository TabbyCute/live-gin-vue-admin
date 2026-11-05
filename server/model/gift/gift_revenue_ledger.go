package gift

// Revenue 不使用软删除；财务事实由数据库触发器保护。
type Revenue struct {
	ID              uint64 `json:"-" gorm:"primaryKey"`
	RevenueNo       string `json:"revenueNo" gorm:"size:40;not null;uniqueIndex"`
	GiftOrderNo     string `json:"giftOrderNo" gorm:"size:40;not null;index"`
	BusinessNo      string `json:"businessNo" gorm:"size:40;not null;uniqueIndex"`
	EntryType       string `json:"entryType" gorm:"size:16;not null;check:ck_gift_revenue_type,entry_type IN ('gift','refund')"`
	GrossCoinAmount int64  `json:"grossCoinAmount,string" gorm:"not null"`
	Quantity        int64  `json:"quantity,string" gorm:"not null"`
	CreatedAt       int64  `json:"createdAt" gorm:"not null;index"`
}

func (Revenue) TableName() string { return "gift_revenue_ledger" }
