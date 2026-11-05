package gift

// Control 不使用软删除；财务事实由数据库触发器保护。
type Control struct {
	ID      uint64 `json:"-" gorm:"primaryKey"`
	Version int64  `json:"version,string" gorm:"not null"`
}

func (Control) TableName() string { return "gift_control" }
