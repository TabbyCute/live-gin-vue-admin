package gift

// BenefitGrant 不使用软删除；财务事实由数据库触发器保护。
type BenefitGrant struct {
	ID              uint64 `json:"-" gorm:"primaryKey"`
	GrantNo         string `json:"grantNo" gorm:"size:40;not null;uniqueIndex"`
	GiftOrderNo     string `json:"giftOrderNo" gorm:"size:40;not null;uniqueIndex:uk_gift_benefit,priority:1"`
	BenefitType     string `json:"benefitType" gorm:"size:20;not null;uniqueIndex:uk_gift_benefit,priority:2"`
	BenefitSnapshot string `json:"benefitSnapshot" gorm:"type:text;not null"`
	ReceiverUserID  uint64 `json:"-" gorm:"not null"`
	AnchorID        uint64 `json:"-" gorm:"not null"`
	Status          string `json:"status" gorm:"size:20;not null"`
	RetryCount      int    `json:"retryCount" gorm:"not null"`
	LastError       string `json:"lastError" gorm:"size:500;not null"`
	CreatedAt       int64  `json:"createdAt" gorm:"not null"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"not null"`
}

func (BenefitGrant) TableName() string { return "gift_benefit_grant" }
