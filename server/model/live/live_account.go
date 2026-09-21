package live

import "tb_live_module/global"

const (
	LiveAccountStatusNormal   uint8 = 1
	LiveAccountStatusDisabled uint8 = 2
)

// LiveAccount 客户端账号。该账号体系与管理后台 sys_users 完全独立。
type LiveAccount struct {
	global.GVA_MODEL
	Username string `json:"username" gorm:"type:varchar(32);not null;uniqueIndex:uk_live_account_username;comment:客户端登录名"`
	Password string `json:"-" gorm:"type:varchar(255);not null;comment:客户端登录密码哈希"`
	Nickname string `json:"nickname" gorm:"type:varchar(64);not null;default:'';comment:客户端昵称"`
	Avatar   string `json:"avatar" gorm:"type:varchar(500);not null;default:'';comment:客户端头像"`
	Status   uint8  `json:"status" gorm:"type:tinyint unsigned;not null;default:1;index;comment:账号状态：1正常 2禁用"`
}

func (LiveAccount) TableName() string {
	return "live_account"
}
