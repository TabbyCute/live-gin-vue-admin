package gift

// Event 不使用软删除；财务事实由数据库触发器保护。
type Event struct {
	ID             uint64 `json:"-" gorm:"primaryKey"`
	EventID        string `json:"eventId" gorm:"size:40;not null;uniqueIndex"`
	EventType      string `json:"eventType" gorm:"size:32;not null"`
	GiftOrderNo    string `json:"giftOrderNo" gorm:"size:40;not null;uniqueIndex"`
	RoomNo         string `json:"roomNo" gorm:"size:40;not null"`
	SessionNo      string `json:"sessionNo" gorm:"size:40;not null"`
	Payload        string `json:"payload" gorm:"type:text;not null"`
	DeliveryStatus string `json:"deliveryStatus" gorm:"size:20;not null;index:idx_gift_event_delivery,priority:1"`
	RetryCount     int    `json:"retryCount" gorm:"not null"`
	LastError      string `json:"lastError" gorm:"size:500;not null"`
	CreatedAt      int64  `json:"createdAt" gorm:"not null;index:idx_gift_event_delivery,priority:2"`
	DeliveredAt    int64  `json:"deliveredAt" gorm:"not null"`
}

func (Event) TableName() string { return "gift_event_outbox" }
