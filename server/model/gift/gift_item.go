package gift

// Item 不使用软删除；财务事实由数据库触发器保护。
type Item struct {
	ID                  uint64 `json:"-" gorm:"primaryKey"`
	GiftNo              string `json:"giftNo" gorm:"size:40;not null;uniqueIndex"`
	CategoryNo          string `json:"categoryNo" gorm:"size:40;not null;index:idx_gift_item_catalog,priority:1"`
	Name                string `json:"name" gorm:"size:64;not null"`
	IconURL             string `json:"iconUrl" gorm:"size:500;not null"`
	AnimationURL        string `json:"animationUrl" gorm:"size:500;not null"`
	AnimationType       string `json:"animationType" gorm:"size:16;not null"`
	CoinPrice           int64  `json:"coinPrice,string" gorm:"not null;check:ck_gift_price,coin_price > 0"`
	SortOrder           int32  `json:"sortOrder" gorm:"not null;index:idx_gift_item_catalog,priority:3"`
	Status              string `json:"status" gorm:"size:16;not null;index:idx_gift_item_catalog,priority:2;check:ck_gift_item_status,status IN ('draft','enabled','disabled')"`
	IsPremium           bool   `json:"isPremium" gorm:"not null"`
	MaxQuantityPerOrder int64  `json:"maxQuantityPerOrder,string" gorm:"not null;check:ck_gift_quantity,max_quantity_per_order BETWEEN 1 AND 10000"`
	Description         string `json:"description" gorm:"size:500;not null"`
	BenefitType         string `json:"benefitType" gorm:"size:20;not null"`
	GuardPlanNo         string `json:"guardPlanNo" gorm:"size:40;not null"`
	GuardDurationDays   int64  `json:"guardDurationDays,string" gorm:"not null"`
	BenefitStatus       string `json:"benefitStatus" gorm:"size:16;not null"`
	EffectiveFrom       int64  `json:"effectiveFrom" gorm:"not null"`
	EffectiveTo         int64  `json:"effectiveTo" gorm:"not null"`
	Version             int64  `json:"version,string" gorm:"not null"`
	CreatedAt           int64  `json:"createdAt" gorm:"not null;autoCreateTime:milli"`
	UpdatedAt           int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:milli"`
}

func (Item) TableName() string { return "gift_item" }
