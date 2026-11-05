package gift

// AdminPermission is an exact operation grant, independent of menu visibility.
type AdminPermission struct {
	ID          uint64 `json:"-" gorm:"primaryKey"`
	AuthorityID uint   `json:"authorityId" gorm:"not null;uniqueIndex:uk_gift_permission,priority:1"`
	Path        string `json:"path" gorm:"size:80;not null;uniqueIndex:uk_gift_permission,priority:2"`
	Method      string `json:"method" gorm:"size:8;not null;uniqueIndex:uk_gift_permission,priority:3"`
}

func (AdminPermission) TableName() string { return "gift_admin_permission" }
