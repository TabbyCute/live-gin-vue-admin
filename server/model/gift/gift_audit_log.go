package gift

// Audit 不使用软删除；财务事实由数据库触发器保护。
type Audit struct {
	ActorIP    string `json:"actorIp" gorm:"size:45;not null"`
	ID         uint64 `json:"-" gorm:"primaryKey"`
	ObjectNo   string `json:"objectNo" gorm:"size:64;not null;index"`
	Action     string `json:"action" gorm:"size:40;not null"`
	OperatorID uint64 `json:"operatorId,string" gorm:"not null"`
	Snapshot   string `json:"snapshot" gorm:"type:text;not null"`
	CreatedAt  int64  `json:"createdAt" gorm:"not null"`
}

func (Audit) TableName() string { return "gift_audit_log" }
