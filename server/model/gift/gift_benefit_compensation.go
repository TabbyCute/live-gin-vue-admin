package gift

// BenefitCompensation 不使用软删除；财务事实由数据库触发器保护。
type BenefitCompensation struct {
	ID             uint64 `json:"-" gorm:"primaryKey"`
	CompensationNo string `json:"compensationNo" gorm:"size:40;not null;uniqueIndex"`
	RefundNo       string `json:"refundNo" gorm:"size:40;not null;uniqueIndex"`
	GiftOrderNo    string `json:"giftOrderNo" gorm:"size:40;not null;index"`
	ReceiverUserID uint64 `json:"-" gorm:"not null"`
	AnchorID       uint64 `json:"-" gorm:"not null"`
	DurationDays   int64  `json:"durationDays,string" gorm:"not null"`
	Status         string `json:"status" gorm:"size:20;not null;check:ck_compensation_status,status IN ('pending','processing','failed','succeeded')"`
	RetryCount     int64  `json:"retryCount,string" gorm:"not null;default:0;check:ck_compensation_retry,retry_count BETWEEN 0 AND 10000"`
	LastError      string `json:"lastError" gorm:"size:500;not null;default:''"`
	LeaseUntil     int64  `json:"leaseUntil" gorm:"not null;default:0"`
	UpdatedAt      int64  `json:"updatedAt" gorm:"autoUpdateTime:milli;not null;default:0"`
	CompletedAt    int64  `json:"completedAt" gorm:"not null;default:0"`
	CreatedAt      int64  `json:"createdAt" gorm:"not null"`
}

func (BenefitCompensation) TableName() string { return "gift_benefit_compensation" }
