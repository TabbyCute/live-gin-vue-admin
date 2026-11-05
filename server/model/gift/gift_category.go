package gift

// Category 不使用软删除；财务事实由数据库触发器保护。
type Category struct {
	ID          uint64 `json:"-" gorm:"primaryKey"`
	CategoryNo  string `json:"categoryNo" gorm:"size:40;not null;uniqueIndex"`
	Name        string `json:"name" gorm:"size:64;not null;uniqueIndex"`
	IconURL     string `json:"iconUrl" gorm:"size:500;not null"`
	Description string `json:"description" gorm:"size:500;not null"`
	SortOrder   int32  `json:"sortOrder" gorm:"not null;index:idx_gift_category_sort,priority:2"`
	Status      string `json:"status" gorm:"size:16;not null;index:idx_gift_category_sort,priority:1;check:ck_gift_category_status,status IN ('enabled','disabled')"`
	Version     int64  `json:"version,string" gorm:"not null"`
	CreatedAt   int64  `json:"createdAt" gorm:"not null;autoCreateTime:milli"`
	UpdatedAt   int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:milli"`
}

func (Category) TableName() string { return "gift_category" }
