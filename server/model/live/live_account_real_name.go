package live

import "tb_live_module/global"

const (
	RealNameDocumentNationalID uint8 = 1
	RealNameDocumentPassport   uint8 = 2
	RealNameDocumentOther      uint8 = 3
)

// LiveAccountRealName 是实名认证保留表。当前版本不提供提交或审核流程。
// 姓名和证件号只允许保存由业务层加密后的密文；DocumentNoHash 必须是对
// “国家代码|证件类型|规范化证件号”执行带服务端密钥的HMAC-SHA256后得到的32字节原值。
type LiveAccountRealName struct {
	global.GVA_MODEL
	AccountID uint `json:"-" gorm:"column:account_id;type:bigint unsigned;not null;uniqueIndex:uk_live_account_real_name_account;comment:关联客户端账号ID"`

	CountryCode          string `json:"countryCode" gorm:"column:country_code;type:char(2);not null;comment:证件签发国家或地区"`
	DocumentType         uint8  `json:"documentType" gorm:"column:document_type;type:tinyint unsigned;not null;comment:证件类型：1居民身份证件 2护照 3其他"`
	LegalNameCiphertext  string `json:"-" gorm:"column:legal_name_ciphertext;type:text;not null;comment:真实姓名密文"`
	LegalNameMasked      string `json:"legalNameMasked" gorm:"column:legal_name_masked;type:varchar(64);not null;default:'';comment:脱敏姓名"`
	DocumentNoCiphertext string `json:"-" gorm:"column:document_no_ciphertext;type:text;not null;comment:证件号码密文"`
	DocumentNoHash       []byte `json:"-" gorm:"column:document_no_hash;type:binary(32);not null;uniqueIndex:uk_live_account_real_name_document;comment:规范化证件标识的HMAC-SHA256原值"`
	DocumentNoMasked     string `json:"documentNoMasked" gorm:"column:document_no_masked;type:varchar(64);not null;default:'';comment:脱敏证件号码"`
	EncryptionKeyID      string `json:"-" gorm:"column:encryption_key_id;type:varchar(32);not null;default:'';comment:密文加密密钥版本标识，用于安全轮换"`

	Status            uint8   `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_live_account_real_name_status_expire,priority:1;comment:认证状态：1审核中 2已认证 3未通过 4已过期或撤销"`
	Provider          string  `json:"provider" gorm:"column:provider;type:varchar(32);not null;default:'manual';uniqueIndex:uk_live_account_real_name_provider_ref,priority:1;comment:认证服务提供方"`
	ProviderReference *string `json:"-" gorm:"column:provider_reference;type:varchar(128);default:null;uniqueIndex:uk_live_account_real_name_provider_ref,priority:2;comment:第三方认证流水号"`
	SubmittedAt       int64   `json:"submittedAt" gorm:"column:submitted_at;type:bigint;not null;default:0;comment:提交时间，毫秒时间戳"`
	ReviewedAt        int64   `json:"reviewedAt" gorm:"column:reviewed_at;type:bigint;not null;default:0;comment:审核完成时间，毫秒时间戳"`
	ExpiresAt         int64   `json:"expiresAt" gorm:"column:expires_at;type:bigint;not null;default:0;index:idx_live_account_real_name_status_expire,priority:2;comment:认证或证件到期时间，毫秒时间戳"`
	ReviewUserID      uint64  `json:"-" gorm:"column:review_user_id;type:bigint unsigned;not null;default:0;comment:审核管理员ID，自动审核为0"`
	RejectReason      string  `json:"rejectReason,omitempty" gorm:"column:reject_reason;type:varchar(255);not null;default:'';comment:未通过原因"`
}

func (LiveAccountRealName) TableName() string { return "live_account_real_name" }
