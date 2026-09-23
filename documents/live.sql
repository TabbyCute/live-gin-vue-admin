CREATE TABLE `live_anchor` (
                               `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主播ID',

    /* ==================== 账号关联 ==================== */

                               `user_id` BIGINT UNSIGNED NOT NULL COMMENT '关联用户ID，一个用户最多对应一个主播',

                               `anchor_no` VARCHAR(32) NOT NULL COMMENT '主播对外编号，用于搜索/展示/客服查询',

    /* ==================== 基础资料 ==================== */

                               `nickname` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '主播昵称',

                               `avatar` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '主播头像',

                               `cover` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '主播主页封面/背景图',

                               `signature` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '主播签名/简介',

                               `gender` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '性别：0未知 1男 2女 3其他',

                               `birthday` INT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '生日，格式YYYYMMDD，例如19980101，0表示未设置',

                               `country_code` VARCHAR(16) NOT NULL DEFAULT ''
                                   COMMENT '国家/地区代码，例如CN/MY/TH',

                               `region_code` VARCHAR(32) NOT NULL DEFAULT ''
                                   COMMENT '省/州/地区编码',

                               `city_code` VARCHAR(32) NOT NULL DEFAULT ''
                                   COMMENT '城市编码',

                               `language` VARCHAR(16) NOT NULL DEFAULT ''
                                   COMMENT '主播主要语言，例如zh-CN/en-US',

    /* ==================== 主播类型 ==================== */

                               `anchor_type` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '主播类型：0普通 1签约 2官方 3公会 4内部运营',

                               `category_id` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '主播主要直播分类ID',

                               `level` INT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '主播等级',

                               `tags` JSON DEFAULT NULL
                                   COMMENT '主播标签，第一阶段使用JSON即可',

    /* ==================== 公会/机构 ==================== */

                               `agency_id` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '公会/机构ID，0表示无公会',

                               `agency_join_at` BIGINT NOT NULL DEFAULT 0
                                   COMMENT '加入公会时间，毫秒时间戳',

    /* ==================== 主播申请 ==================== */

                               `apply_status` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '主播申请状态：0未申请 1审核中 2通过 3拒绝',

                               `apply_at` BIGINT NOT NULL DEFAULT 0
                                   COMMENT '申请主播时间，毫秒时间戳',

                               `audit_at` BIGINT NOT NULL DEFAULT 0
                                   COMMENT '主播申请审核时间，毫秒时间戳',

                               `audit_user_id` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '审核后台管理员ID',

                               `reject_reason` VARCHAR(255) NOT NULL DEFAULT ''
                                   COMMENT '最近一次审核拒绝原因',

    /* ==================== 认证 ==================== */

                               `cert_status` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '认证状态：0未认证 1认证中 2已认证 3认证失败',

                               `cert_type` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '认证类型：0无 1个人 2机构 3官方',

                               `cert_name` VARCHAR(64) NOT NULL DEFAULT ''
                                   COMMENT '认证展示名称，例如官方主播/签约主播',

    /* ==================== 主播状态 ==================== */

                               `status` TINYINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '主播账号状态：0禁用 1正常 2封禁 3注销',

                               `status_reason` VARCHAR(255) NOT NULL DEFAULT ''
                                   COMMENT '状态变更原因，例如封禁原因',

                               `ban_until` BIGINT NOT NULL DEFAULT 0
                                   COMMENT '封禁截止时间，0表示非临时封禁或永久',

    /* ==================== 功能权限 ==================== */

                               `live_permission` TINYINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '开播权限：0禁止 1允许',

                               `pk_permission` TINYINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT 'PK权限：0禁止 1允许',

                               `recommend_permission` TINYINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '推荐权限：0禁止进入推荐 1允许',

                               `withdraw_permission` TINYINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '提现权限：0禁止 1允许',

    /* ==================== 运营属性 ==================== */

                               `is_official` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '是否官方主播：0否 1是',

                               `is_signed` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '是否签约主播：0否 1是',

                               `is_recommended` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '是否运营推荐：0否 1是',

                               `is_newcomer` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '是否新人主播标记：0否 1是',

                               `sort` INT NOT NULL DEFAULT 0
                                   COMMENT '人工排序权重，数值越大越靠前',

                               `recommend_weight` INT NOT NULL DEFAULT 0
                                   COMMENT '推荐基础权重，后续推荐算法可使用',

    /* ==================== 主播统计快照 ==================== */

                               `fans_count` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '粉丝数量缓存',

                               `follow_count` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '主播关注其他用户数量缓存',

                               `total_live_count` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '累计开播场次',

                               `total_live_duration_ms` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '累计逻辑直播时长，单位毫秒',

                               `max_online_count` INT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '历史最高在线人数',

                               `total_view_count` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '累计观看人次快照',

                               `last_live_at` BIGINT NOT NULL DEFAULT 0
                                   COMMENT '最近一次开播时间，毫秒时间戳',

                               `last_live_end_at` BIGINT NOT NULL DEFAULT 0
                                   COMMENT '最近一次下播时间，毫秒时间戳',

    /* ==================== 来源信息 ==================== */

                               `source` VARCHAR(32) NOT NULL DEFAULT ''
                                   COMMENT '主播来源，例如app/admin/import/old_platform',

                               `source_id` VARCHAR(64) NOT NULL DEFAULT ''
                                   COMMENT '第三方或老系统主播ID',

                               `channel_id` BIGINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '注册/引入渠道ID',

    /* ==================== 风控/后台 ==================== */

                               `risk_level` TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '风控等级：0正常 1低风险 2中风险 3高风险',

                               `remark` VARCHAR(500) NOT NULL DEFAULT ''
                                   COMMENT '运营后台备注，前台不可见',

    /* ==================== 扩展 ==================== */

                               `extra` JSON DEFAULT NULL
                                   COMMENT '低频扩展字段，不建议存高频查询字段',

    /* ==================== 时间 ==================== */

                               `created_at` BIGINT NOT NULL COMMENT '创建时间，毫秒时间戳',

                               `updated_at` BIGINT NOT NULL COMMENT '更新时间，毫秒时间戳',

                               `deleted_at` BIGINT NOT NULL DEFAULT 0
                                   COMMENT '软删除时间，0表示未删除',

                               PRIMARY KEY (`id`),

                               UNIQUE KEY `uk_user_id` (`user_id`),
                               UNIQUE KEY `uk_anchor_no` (`anchor_no`),

                               KEY `idx_status` (`status`),

                               KEY `idx_apply_created` (`apply_status`, `created_at`),

                               KEY `idx_cert_status` (`cert_status`),

                               KEY `idx_category_status` (`category_id`, `status`),

                               KEY `idx_gender_status` (`gender`, `status`),

                               KEY `idx_agency_id` (`agency_id`),

                               KEY `idx_anchor_type` (`anchor_type`),

                               KEY `idx_recommend`
                                   (`status`, `live_permission`, `recommend_permission`, `is_recommended`, `sort`),

                               KEY `idx_last_live_at` (`last_live_at`)

) ENGINE=InnoDB
DEFAULT CHARSET=utf8mb4
COMMENT='直播主播信息表';




package model

import (
	"gorm.io/gorm"
)

// LiveAnchor 直播主播信息表
type LiveAnchor struct {
	// 主键
	Id uint64 `gorm:"column:id;primaryKey;autoIncrement;type:bigint unsigned;comment:主播ID"`

	// ==================== 账号关联 ====================
	UserId    uint64 `gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_user_id;comment:关联用户ID，一个用户最多对应一个主播"`
	AnchorNo  string `gorm:"column:anchor_no;type:varchar(32);not null;uniqueIndex:uk_anchor_no;comment:主播对外编号，用于搜索/展示/客服查询"`

	// ==================== 基础资料 ====================
	Nickname    string `gorm:"column:nickname;type:varchar(64);not null;default:'';comment:主播昵称"`
	Avatar      string `gorm:"column:avatar;type:varchar(500);not null;default:'';comment:主播头像"`
	Cover       string `gorm:"column:cover;type:varchar(500);not null;default:'';comment:主播主页封面/背景图"`
	Signature   string `gorm:"column:signature;type:varchar(255);not null;default:'';comment:主播签名/简介"`
	Gender      uint8  `gorm:"column:gender;type:tinyint unsigned;not null;default:0;index:idx_gender_status;comment:性别：0未知 1男 2女 3其他"`
	Birthday    uint32 `gorm:"column:birthday;type:int unsigned;not null;default:0;comment:生日，格式YYYYMMDD，例如19980101，0表示未设置"`
	CountryCode string `gorm:"column:country_code;type:varchar(16);not null;default:'';comment:国家/地区代码，例如CN/MY/TH"`
	RegionCode  string `gorm:"column:region_code;type:varchar(32);not null;default:'';comment:省/州/地区编码"`
	CityCode    string `gorm:"column:city_code;type:varchar(32);not null;default:'';comment:城市编码"`
	Language    string `gorm:"column:language;type:varchar(16);not null;default:'';comment:主播主要语言，例如zh-CN/en-US"`

	// ==================== 主播类型 ====================
	AnchorType uint8   `gorm:"column:anchor_type;type:tinyint unsigned;not null;default:0;index:idx_anchor_type;comment:主播类型：0普通 1签约 2官方 3公会 4内部运营"`
	CategoryId uint64  `gorm:"column:category_id;type:bigint unsigned;not null;default:0;index:idx_category_status;comment:主播主要直播分类ID"`
	Level      uint32  `gorm:"column:level;type:int unsigned;not null;default:1;comment:主播等级"`
	Tags       *string `gorm:"column:tags;type:json;serializer:json;default:null;comment:主播标签，第一阶段使用JSON即可"`

	// ==================== 公会/机构 ====================
	AgencyId     uint64 `gorm:"column:agency_id;type:bigint unsigned;not null;default:0;index:idx_agency_id;comment:公会/机构ID，0表示无公会"`
	AgencyJoinAt int64  `gorm:"column:agency_join_at;type:bigint;not null;default:0;comment:加入公会时间，毫秒时间戳"`

	// ==================== 主播申请 ====================
	ApplyStatus   uint8  `gorm:"column:apply_status;type:tinyint unsigned;not null;default:0;index:idx_apply_created;comment:主播申请状态：0未申请 1审核中 2通过 3拒绝"`
	ApplyAt       int64  `gorm:"column:apply_at;type:bigint;not null;default:0;comment:申请主播时间，毫秒时间戳"`
	AuditAt       int64  `gorm:"column:audit_at;type:bigint;not null;default:0;comment:主播申请审核时间，毫秒时间戳"`
	AuditUserId   uint64 `gorm:"column:audit_user_id;type:bigint unsigned;not null;default:0;comment:审核后台管理员ID"`
	RejectReason  string `gorm:"column:reject_reason;type:varchar(255);not null;default:'';comment:最近一次审核拒绝原因"`

	// ==================== 认证 ====================
	CertStatus uint8  `gorm:"column:cert_status;type:tinyint unsigned;not null;default:0;index:idx_cert_status;comment:认证状态：0未认证 1认证中 2已认证 3认证失败"`
	CertType   uint8  `gorm:"column:cert_type;type:tinyint unsigned;not null;default:0;comment:认证类型：0无 1个人 2机构 3官方"`
	CertName   string `gorm:"column:cert_name;type:varchar(64);not null;default:'';comment:认证展示名称，例如官方主播/签约主播"`

	// ==================== 主播状态 ====================
	Status        uint8  `gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_status;index:idx_category_status;index:idx_gender_status;index:idx_recommend;comment:主播账号状态：0禁用 1正常 2封禁 3注销"`
	StatusReason  string `gorm:"column:status_reason;type:varchar(255);not null;default:'';comment:状态变更原因，例如封禁原因"`
	BanUntil      int64  `gorm:"column:ban_until;type:bigint;not null;default:0;comment:封禁截止时间，0表示非临时封禁或永久"`

	// ==================== 功能权限 ====================
	LivePermission     uint8 `gorm:"column:live_permission;type:tinyint unsigned;not null;default:0;index:idx_recommend;comment:开播权限：0禁止 1允许"`
	PkPermission      uint8 `gorm:"column:pk_permission;type:tinyint unsigned;not null;default:0;comment:PK权限：0禁止 1允许"`
	RecommendPermission uint8 `gorm:"column:recommend_permission;type:tinyint unsigned;not null;default:0;index:idx_recommend;comment:推荐权限：0禁止进入推荐 1允许"`
	WithdrawPermission uint8 `gorm:"column:withdraw_permission;type:tinyint unsigned;not null;default:0;comment:提现权限：0禁止 1允许"`

	// ==================== 运营属性 ====================
	IsOfficial     uint8 `gorm:"column:is_official;type:tinyint unsigned;not null;default:0;comment:是否官方主播：0否 1是"`
	IsSigned       uint8 `gorm:"column:is_signed;type:tinyint unsigned;not null;default:0;comment:是否签约主播：0否 1是"`
	IsRecommended  uint8 `gorm:"column:is_recommended;type:tinyint unsigned;not null;default:0;index:idx_recommend;comment:是否运营推荐：0否 1是"`
	IsNewcomer     uint8 `gorm:"column:is_newcomer;type:tinyint unsigned;not null;default:0;comment:是否新人主播标记：0否 1是"`
	Sort           int32 `gorm:"column:sort;type:int;not null;default:0;index:idx_recommend;comment:人工排序权重，数值越大越靠前"`
	RecommendWeight int32 `gorm:"column:recommend_weight;type:int;not null;default:0;comment:推荐基础权重，后续推荐算法可使用"`

	// ==================== 主播统计快照 ====================
	FansCount        uint64 `gorm:"column:fans_count;type:bigint unsigned;not null;default:0;comment:粉丝数量缓存"`
	FollowCount      uint64 `gorm:"column:follow_count;type:bigint unsigned;not null;default:0;comment:主播关注其他用户数量缓存"`
	TotalLiveCount   uint64 `gorm:"column:total_live_count;type:bigint unsigned;not null;default:0;comment:累计开播场次"`
	TotalLiveDurationMs uint64 `gorm:"column:total_live_duration_ms;type:bigint unsigned;not null;default:0;comment:累计逻辑直播时长，单位毫秒"`
	MaxOnlineCount   uint32 `gorm:"column:max_online_count;type:int unsigned;not null;default:0;comment:历史最高在线人数"`
	TotalViewCount   uint64 `gorm:"column:total_view_count;type:bigint unsigned;not null;default:0;comment:累计观看人次快照"`
	LastLiveAt       int64  `gorm:"column:last_live_at;type:bigint;not null;default:0;index:idx_last_live_at;comment:最近一次开播时间，毫秒时间戳"`
	LastLiveEndAt    int64  `gorm:"column:last_live_end_at;type:bigint;not null;default:0;comment:最近一次下播时间，毫秒时间戳"`

	// ==================== 来源信息 ====================
	Source     string `gorm:"column:source;type:varchar(32);not null;default:'';comment:主播来源，例如app/admin/import/old_platform"`
	SourceId   string `gorm:"column:source_id;type:varchar(64);not null;default:'';comment:第三方或老系统主播ID"`
	ChannelId  uint64 `gorm:"column:channel_id;type:bigint unsigned;not null;default:0;comment:注册/引入渠道ID"`

	// ==================== 风控/后台 ====================
	RiskLevel uint8  `gorm:"column:risk_level;type:tinyint unsigned;not null;default:0;comment:风控等级：0正常 1低风险 2中风险 3高风险"`
	Remark    string `gorm:"column:remark;type:varchar(500);not null;default:'';comment:运营后台备注，前台不可见"`

	// ==================== 扩展 ====================
	Extra *string `gorm:"column:extra;type:json;serializer:json;default:null;comment:低频扩展字段，不建议存高频查询字段"`

	// ==================== 时间 ====================
	CreatedAt int64 `gorm:"column:created_at;type:bigint;not null;autoCreateTime:milli;index:idx_apply_created;comment:创建时间，毫秒时间戳"`
	UpdatedAt int64 `gorm:"column:updated_at;type:bigint;not null;autoUpdateTime:milli;comment:更新时间，毫秒时间戳"`
	DeletedAt int64 `gorm:"column:deleted_at;type:bigint;not null;default:0;comment:软删除时间，0表示未删除"`
}

// TableName 指定表名
func (LiveAnchor) TableName() string {
	return "live_anchor"
}





K 权限可以根据运营策略决定是否同步开启。

同时后端不能只判断一个字段，建议统一成一个业务方法：

func (a *LiveAnchor) CanLive() bool {
    return a.DeletedAt == 0 &&
        a.Status == 1 &&
        a.ApplyStatus == 2 &&
        a.LivePermission == 1
}

以后所有：

获取推流地址
创建直播
开始直播
恢复直播

全部调用这个方法。

PK：

func (a *LiveAnchor) CanPK() bool {
    return a.CanLive() &&
        a.PkPermission == 1
}

这是我认为你第一版最应该修改的地方。


1，2，5，6，
FollowCount删除;
Birthday 使用 DATE；
CountryCode 标准化；
你的索引目前有一点“索引偏多”这里也帮我优化；

第3点也帮我按你的，TagIds []uint64 `gorm:"column:tag_ids;type:json;serializer:json;comment:主播标签ID列表"`

请给出优化后的表model




package model

import "time"


