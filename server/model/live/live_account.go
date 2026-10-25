package live

import (
	"time"

	"tb_live_module/global"
)

const (
	// LiveAccountStatusNormal（1）表示客户端账号状态正常，允许继续登录和访问 APP 接口。
	LiveAccountStatusNormal uint8 = 1
	// LiveAccountStatusDisabled（2）表示客户端账号已被禁用，不允许继续登录和访问受保护接口。
	LiveAccountStatusDisabled uint8 = 2
	// LiveAccountStatusBanned（3）表示客户端账号被临时或永久封禁。
	LiveAccountStatusBanned uint8 = 3
	// LiveAccountStatusCancelled（4）表示客户端账号已注销，不能通过普通状态接口恢复。
	LiveAccountStatusCancelled uint8 = 4
)

const (
	LiveAccountRealNameUnverified uint8 = iota
	LiveAccountRealNamePending
	LiveAccountRealNameVerified
	LiveAccountRealNameRejected
	LiveAccountRealNameExpired
)

// LiveAccount 客户端账号。该账号体系与管理后台 sys_users 完全独立。
type LiveAccount struct {
	global.GVA_MODEL
	AccountNo string `json:"accountNo" gorm:"column:account_no;type:varchar(32);not null;uniqueIndex:uk_live_account_account_no;comment:客户端数字账号编号"`
	Username  string `json:"username" gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_live_account_username;comment:系统生成的内部用户名，不用于登录"`
	Password  string `json:"-" gorm:"column:password;type:varchar(255);not null;comment:客户端登录密码哈希"`

	Phone           *string `json:"-" gorm:"column:phone;type:varchar(20);default:null;uniqueIndex:uk_live_account_phone;comment:验证通过的E.164手机号"`
	PhoneVerifiedAt int64   `json:"-" gorm:"column:phone_verified_at;type:bigint;not null;default:0;comment:手机号验证完成时间，毫秒时间戳"`

	Nickname    string     `json:"nickname" gorm:"column:nickname;type:varchar(64);not null;default:'';comment:客户端昵称"`
	Avatar      string     `json:"avatar" gorm:"column:avatar;type:varchar(500);not null;default:'';comment:客户端头像"`
	Signature   string     `json:"signature" gorm:"column:signature;type:varchar(255);not null;default:'';comment:个性签名"`
	Gender      uint8      `json:"gender" gorm:"column:gender;type:tinyint unsigned;not null;default:0;comment:性别：0未知 1男 2女 3其他"`
	Birthday    *time.Time `json:"birthday" gorm:"column:birthday;type:date;default:null;comment:出生日期"`
	CountryCode string     `json:"countryCode" gorm:"column:country_code;type:char(2);not null;default:'';comment:国家或地区代码"`
	RegionCode  string     `json:"regionCode" gorm:"column:region_code;type:varchar(32);not null;default:'';comment:省州地区编码"`
	CityCode    string     `json:"cityCode" gorm:"column:city_code;type:varchar(32);not null;default:'';comment:城市编码"`
	Language    string     `json:"language" gorm:"column:language;type:varchar(16);not null;default:'';comment:首选语言"`

	RealNameStatus     uint8 `json:"realNameStatus" gorm:"column:real_name_status;type:tinyint unsigned;not null;default:0;index:idx_live_account_real_name_status;comment:实名认证状态：0未认证 1审核中 2已认证 3未通过 4已过期或撤销"`
	RealNameVerifiedAt int64 `json:"realNameVerifiedAt" gorm:"column:real_name_verified_at;type:bigint;not null;default:0;comment:实名认证通过时间，毫秒时间戳"`

	Status       uint8  `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_live_account_status_ban,priority:1;comment:账号状态：1正常 2禁用 3封禁 4注销"`
	StatusReason string `json:"-" gorm:"column:status_reason;type:varchar(255);not null;default:'';comment:当前账号状态原因，仅后台可见"`
	BanUntil     int64  `json:"banUntil" gorm:"column:ban_until;type:bigint;not null;default:0;index:idx_live_account_status_ban,priority:2;comment:封禁截止毫秒时间戳，0表示永久或未设置"`

	TokenVersion      uint64 `json:"-" gorm:"column:token_version;type:bigint unsigned;not null;default:1;comment:客户端JWT撤销版本"`
	PasswordChangedAt int64  `json:"-" gorm:"column:password_changed_at;type:bigint;not null;default:0;comment:最近修改密码时间，毫秒时间戳"`
	LastLoginAt       int64  `json:"lastLoginAt" gorm:"column:last_login_at;type:bigint;not null;default:0;comment:最近成功登录时间，毫秒时间戳"`

	RegisterSource    string `json:"-" gorm:"column:register_source;type:varchar(32);not null;default:'app';comment:注册来源"`
	RegisterChannelID uint64 `json:"-" gorm:"column:register_channel_id;type:bigint unsigned;not null;default:0;index:idx_live_account_register_channel;comment:注册渠道ID"`
}

func (LiveAccount) TableName() string {
	return "live_account"
}
