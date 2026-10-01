package live

import "tb_live_module/global"

const (
	// LiveRoomStatusDisabled（0）表示直播间被后台或风控禁用，主播不能自行恢复或开播。
	LiveRoomStatusDisabled uint8 = iota
	// LiveRoomStatusNormal（1）表示直播间可以正常使用；实际开播仍需通过主播资格检查。
	LiveRoomStatusNormal
	// LiveRoomStatusClosed（2）表示直播间由主播主动关闭或由运营停用，但不属于违规封禁。
	LiveRoomStatusClosed
)

const (
	// LiveRoomOffline（0）表示直播间当前没有活动直播场次。
	LiveRoomOffline uint8 = iota
	// LiveRoomPreparing（1）表示直播间已创建准备中场次，正在等待有效推流。
	LiveRoomPreparing
	// LiveRoomLiving（2）表示直播间当前正在直播。
	LiveRoomLiving
	// LiveRoomEnding（3）表示直播间正在执行停止推流、资源清理或场次结算。
	LiveRoomEnding
)

const (
	// LiveRoomVisibilityPrivate（0）表示直播间私密，不进入公开直播列表。
	LiveRoomVisibilityPrivate uint8 = iota
	// LiveRoomVisibilityPublic（1）表示直播间公开，直播中可以进入公开直播列表。
	LiveRoomVisibilityPublic
	// LiveRoomVisibilityFollowers（2）表示直播间仅允许关注主播的用户观看。
	LiveRoomVisibilityFollowers
)

// LiveRoom 保存一名主播唯一直播间的固定配置和当前运行快照。
// 历史开播时间和统计以 LiveSession 为准，PublishSecretHash 不得通过接口返回。
// rtmp://xxx/tb_live/[StreamName]?pt=xxxx
type LiveRoom struct {
	global.GVA_MODEL
	RoomNo   string `json:"roomNo" gorm:"column:room_no;type:varchar(32);not null;uniqueIndex:uk_live_room_no;comment:直播间对外编号，默认主播编号，后台可修改"`
	AnchorId uint   `json:"-" gorm:"column:anchor_id;type:bigint unsigned;not null;uniqueIndex:uk_live_room_anchor;comment:主播内部ID，一名主播一个直播间"`

	CategoryId uint64 `json:"categoryId" gorm:"column:category_id;type:bigint unsigned;not null;default:0;index:idx_live_room_category,priority:1;comment:当前直播分类ID"`
	Title      string `json:"title" gorm:"column:title;type:varchar(128);not null;default:'';comment:直播间标题"`
	CoverURL   string `json:"coverUrl" gorm:"column:cover_url;type:varchar(500);not null;default:'';comment:直播间封面地址"`
	Notice     string `json:"notice" gorm:"column:notice;type:varchar(500);not null;default:'';comment:直播间公告"`

	StreamName        string `json:"streamName" gorm:"column:stream_name;type:varchar(64);not null;uniqueIndex:uk_live_room_stream;comment:SRS稳定流名称，不是场次ID"`
	PublishSecretHash string `json:"-" gorm:"column:publish_secret_hash;type:char(64);not null;default:'';comment:当前pt字符串SHA-256哈希"`
	StreamKeyVersion  uint32 `json:"streamKeyVersion" gorm:"column:stream_key_version;type:int unsigned;not null;default:1;comment:推流凭证代数，对应pt的credential_version"`

	Status           uint8  `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_live_room_discovery,priority:2;index:idx_live_room_category,priority:3;comment:直播间状态：0禁用 1正常 2关闭"`
	StatusReason     string `json:"statusReason" gorm:"column:status_reason;type:varchar(255);not null;default:'';comment:禁用或关闭原因"`
	StatusChangedAt  int64  `json:"statusChangedAt" gorm:"column:status_changed_at;type:bigint;not null;default:0;comment:直播间状态最近变更时间，毫秒时间戳"`
	LiveStatus       uint8  `json:"liveStatus" gorm:"column:live_status;type:tinyint unsigned;not null;default:0;index:idx_live_room_discovery,priority:1;index:idx_live_room_category,priority:2;comment:当前直播状态：0未开播 1准备中 2直播中 3结束中"`
	CurrentSessionId uint   `json:"-" gorm:"column:current_session_id;type:bigint unsigned;not null;default:0;comment:当前场次内部ID，0表示没有活动场次"`
	LastSessionId    uint   `json:"-" gorm:"column:last_session_id;type:bigint unsigned;not null;default:0;comment:最近一次已完成直播场次内部ID，0表示从未完成直播"`
	LiveStartedAt    int64  `json:"liveStartedAt" gorm:"column:live_started_at;type:bigint;not null;default:0;index:idx_live_room_discovery,priority:4,sort:desc;comment:当前场次开始时间，毫秒时间戳；结束后清零"`

	Visibility      uint8   `json:"visibility" gorm:"column:visibility;type:tinyint unsigned;not null;default:1;comment:可见范围：0私密 1公开 2仅关注者"`
	RecommendWeight int32   `json:"recommendWeight" gorm:"column:recommend_weight;type:int;not null;default:0;index:idx_live_room_discovery,priority:3,sort:desc;index:idx_live_room_category,priority:4,sort:desc;comment:直播间推荐权重"`
	Extra           *string `json:"extra,omitempty" gorm:"column:extra;type:json;serializer:json;default:null;comment:低频扩展配置"`
}

func (LiveRoom) TableName() string { return "live_room" }
