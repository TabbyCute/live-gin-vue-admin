package live

import (
	"time"

	"tb_live_module/global"
)

const (
	AnchorApplyStatusNone uint8 = iota
	AnchorApplyStatusPending
	AnchorApplyStatusApproved
	AnchorApplyStatusRejected
)

const (
	AnchorStatusDisabled uint8 = iota
	AnchorStatusNormal
	AnchorStatusBanned
	AnchorStatusCancelled
)

const (
	AnchorCertStatusNone uint8 = iota
	AnchorCertStatusPending
	AnchorCertStatusApproved
	AnchorCertStatusFailed
)

const (
	AnchorCertTypeNone uint8 = iota
	AnchorCertTypePersonal
	AnchorCertTypeOrganization
	AnchorCertTypeOfficial
)

const (
	AnchorRiskNormal uint8 = iota
	AnchorRiskLow
	AnchorRiskMedium
	AnchorRiskHigh
)

const (
	AnchorPermissionDisabled uint8 = iota
	AnchorPermissionEnabled
)

const (
	AnchorTypeNormal uint8 = iota
	AnchorTypeOfficial
	AnchorTypeInternal
)

// LiveAnchor 保存主播身份、审核状态和运营快照。
// 实时开播资格不能只读取 JWT，必须结合本表当前状态重新计算。
type LiveAnchor struct {
	global.GVA_MODEL

	UserId   uint64 `json:"userId" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_user_id;comment:关联用户ID，一个用户最多对应一个主播"`
	AnchorNo string `json:"anchorNo" gorm:"column:anchor_no;type:varchar(32);not null;uniqueIndex:uk_anchor_no;comment:主播对外编号，用于搜索/展示/客服查询"`

	Nickname    string     `json:"nickname" gorm:"column:nickname;type:varchar(64);not null;default:'';comment:主播昵称"`
	Avatar      string     `json:"avatar" gorm:"column:avatar;type:varchar(500);not null;default:'';comment:主播头像"`
	Cover       string     `json:"cover" gorm:"column:cover;type:varchar(500);not null;default:'';comment:主播主页封面/背景图"`
	Signature   string     `json:"signature" gorm:"column:signature;type:varchar(255);not null;default:'';comment:主播签名/简介"`
	Gender      uint8      `json:"gender" gorm:"column:gender;type:tinyint unsigned;not null;default:0;comment:性别：0未知 1男 2女 3其他"`
	Birthday    *time.Time `json:"birthday" gorm:"column:birthday;type:date;default:null;comment:生日，格式YYYY-MM-DD"`
	CountryCode string     `json:"countryCode" gorm:"column:country_code;type:char(2);not null;default:'';comment:国家/地区代码，ISO 3166-1 alpha-2，例如CN/MY/TH"`
	RegionCode  string     `json:"regionCode" gorm:"column:region_code;type:varchar(32);not null;default:'';comment:省/州/地区编码"`
	CityCode    string     `json:"cityCode" gorm:"column:city_code;type:varchar(32);not null;default:'';comment:城市编码"`
	Language    string     `json:"language" gorm:"column:language;type:varchar(16);not null;default:'';comment:主播主要语言，例如zh-CN/en-US/ms-MY"`

	// AnchorType 只描述账号性质；签约和公会关系分别由 IsSigned、AgencyId 表示。
	AnchorType uint8    `json:"anchorType" gorm:"column:anchor_type;type:tinyint unsigned;not null;default:0;comment:主播类型：0普通 1官方 2内部运营"`
	CategoryId uint64   `json:"categoryId" gorm:"column:category_id;type:bigint unsigned;not null;default:0;index:idx_category_status,priority:1;comment:主播主要直播分类ID"`
	Level      uint32   `json:"level" gorm:"column:level;type:int unsigned;not null;default:1;comment:主播等级"`
	TagIds     []uint64 `json:"tagIds" gorm:"column:tag_ids;type:json;serializer:json;default:null;comment:主播标签ID列表"`

	AgencyId     uint64 `json:"agencyId" gorm:"column:agency_id;type:bigint unsigned;not null;default:0;index:idx_agency_status,priority:1;comment:公会/机构ID，0表示无公会"`
	AgencyJoinAt int64  `json:"agencyJoinAt" gorm:"column:agency_join_at;type:bigint;not null;default:0;comment:加入公会时间，毫秒时间戳"`

	ApplyStatus  uint8  `json:"applyStatus" gorm:"column:apply_status;type:tinyint unsigned;not null;default:0;index:idx_apply_created,priority:1;comment:主播申请状态：0未申请 1审核中 2通过 3拒绝"`
	ApplyAt      int64  `json:"applyAt" gorm:"column:apply_at;type:bigint;not null;default:0;comment:申请主播时间，毫秒时间戳"`
	AuditAt      int64  `json:"auditAt" gorm:"column:audit_at;type:bigint;not null;default:0;comment:主播申请审核时间，毫秒时间戳"`
	AuditUserId  uint64 `json:"auditUserId" gorm:"column:audit_user_id;type:bigint unsigned;not null;default:0;comment:审核后台管理员ID"`
	RejectReason string `json:"rejectReason" gorm:"column:reject_reason;type:varchar(255);not null;default:'';comment:最近一次审核拒绝原因"`

	CertStatus uint8  `json:"certStatus" gorm:"column:cert_status;type:tinyint unsigned;not null;default:0;comment:认证状态：0未认证 1认证中 2已认证 3认证失败"`
	CertType   uint8  `json:"certType" gorm:"column:cert_type;type:tinyint unsigned;not null;default:0;comment:认证类型：0无 1个人 2机构 3官方"`
	CertName   string `json:"certName" gorm:"column:cert_name;type:varchar(64);not null;default:'';comment:认证展示名称，例如官方主播/签约主播"`

	Status       uint8  `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_category_status,priority:2;index:idx_agency_status,priority:2;index:idx_recommend,priority:2;comment:主播账号状态：0禁用 1正常 2封禁 3注销"`
	StatusReason string `json:"statusReason" gorm:"column:status_reason;type:varchar(255);not null;default:'';comment:状态变更原因，例如封禁原因"`
	BanUntil     int64  `json:"banUntil" gorm:"column:ban_until;type:bigint;not null;default:0;comment:封禁截止时间，0表示非临时封禁或永久"`

	LivePermission      uint8 `json:"livePermission" gorm:"column:live_permission;type:tinyint unsigned;not null;default:0;index:idx_recommend,priority:4;comment:开播权限：0禁止 1允许，主播审核通过后开启"`
	PkPermission        uint8 `json:"pkPermission" gorm:"column:pk_permission;type:tinyint unsigned;not null;default:0;comment:PK权限：0禁止 1允许"`
	RecommendPermission uint8 `json:"recommendPermission" gorm:"column:recommend_permission;type:tinyint unsigned;not null;default:1;index:idx_recommend,priority:3;comment:推荐权限：0禁止进入推荐 1允许"`
	WithdrawPermission  uint8 `json:"withdrawPermission" gorm:"column:withdraw_permission;type:tinyint unsigned;not null;default:0;comment:主播运营提现权限：0禁止 1允许，最终提现资格由钱包和风控系统共同判断"`

	IsSigned        uint8 `json:"isSigned" gorm:"column:is_signed;type:tinyint unsigned;not null;default:0;comment:是否签约主播：0否 1是"`
	IsRecommended   uint8 `json:"isRecommended" gorm:"column:is_recommended;type:tinyint unsigned;not null;default:0;index:idx_recommend,priority:1;comment:是否运营推荐：0否 1是"`
	NewcomerUntil   int64 `json:"newcomerUntil" gorm:"column:newcomer_until;type:bigint;not null;default:0;comment:新人保护/新人推荐截止时间，毫秒时间戳，0表示非新人"`
	Sort            int32 `json:"sort" gorm:"column:sort;type:int;not null;default:0;index:idx_recommend,priority:5,sort:desc;comment:人工排序权重，数值越大越靠前"`
	RecommendWeight int32 `json:"recommendWeight" gorm:"column:recommend_weight;type:int;not null;default:0;comment:推荐基础权重，后续推荐算法可使用"`

	FansCount           uint64 `json:"fansCount" gorm:"column:fans_count;type:bigint unsigned;not null;default:0;comment:粉丝数量缓存"`
	TotalLiveCount      uint64 `json:"totalLiveCount" gorm:"column:total_live_count;type:bigint unsigned;not null;default:0;comment:累计开播场次"`
	TotalLiveDurationMs uint64 `json:"totalLiveDurationMs" gorm:"column:total_live_duration_ms;type:bigint unsigned;not null;default:0;comment:累计逻辑直播时长，单位毫秒"`
	MaxOnlineCount      uint32 `json:"maxOnlineCount" gorm:"column:max_online_count;type:int unsigned;not null;default:0;comment:历史最高在线人数"`
	TotalViewCount      uint64 `json:"totalViewCount" gorm:"column:total_view_count;type:bigint unsigned;not null;default:0;comment:累计观看人次快照"`
	LastLiveAt          int64  `json:"lastLiveAt" gorm:"column:last_live_at;type:bigint;not null;default:0;index:idx_last_live_at,sort:desc;comment:最近一次开播时间，毫秒时间戳"`
	LastLiveEndAt       int64  `json:"lastLiveEndAt" gorm:"column:last_live_end_at;type:bigint;not null;default:0;comment:最近一次下播时间，毫秒时间戳"`

	Source    string `json:"source" gorm:"column:source;type:varchar(32);not null;default:'';index:idx_source_ref,priority:1;comment:主播来源，例如app/admin/import/old_platform"`
	SourceId  string `json:"sourceId" gorm:"column:source_id;type:varchar(64);not null;default:'';index:idx_source_ref,priority:2;comment:第三方或老系统主播ID"`
	ChannelId uint64 `json:"channelId" gorm:"column:channel_id;type:bigint unsigned;not null;default:0;comment:注册/引入渠道ID"`

	RiskLevel uint8   `json:"riskLevel" gorm:"column:risk_level;type:tinyint unsigned;not null;default:0;comment:风控等级：0正常 1低风险 2中风险 3高风险"`
	Remark    string  `json:"remark" gorm:"column:remark;type:varchar(500);not null;default:'';comment:运营后台备注，前台不可见"`
	Extra     *string `json:"extra" gorm:"column:extra;type:json;serializer:json;default:null;comment:低频扩展字段，不建议存高频查询字段"`
}

func (LiveAnchor) TableName() string {
	return "live_anchor"
}
