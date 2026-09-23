package live

import "tb_live_module/global"

const (
	LiveRoomStatusDisabled uint8 = iota
	LiveRoomStatusNormal
	LiveRoomStatusClosed
)

const (
	LiveRoomOffline uint8 = iota
	LiveRoomPreparing
	LiveRoomLiving
	LiveRoomEnding
)

const (
	LiveRoomVisibilityPrivate uint8 = iota
	LiveRoomVisibilityPublic
	LiveRoomVisibilityFollowers
)

// LiveRoom 保存一名主播唯一直播间的固定配置和当前运行快照。
// 历史开播时间和统计以 LiveSession 为准，PublishSecretHash 不得通过接口返回。
type LiveRoom struct {
	global.GVA_MODEL
	RoomNo   string `json:"roomNo" gorm:"column:room_no;type:varchar(32);not null;uniqueIndex:uk_live_room_no;comment:直播间对外编号，默认主播编号，后台可修改"`
	AnchorId uint   `json:"-" gorm:"column:anchor_id;type:bigint unsigned;not null;uniqueIndex:uk_live_room_anchor;comment:主播内部ID，一名主播一个直播间"`

	CategoryId uint64 `json:"categoryId" gorm:"column:category_id;type:bigint unsigned;not null;default:0;index:idx_live_room_category,priority:1;comment:当前直播分类ID"`
	Title      string `json:"title" gorm:"column:title;type:varchar(128);not null;default:'';comment:直播间标题"`
	CoverURL   string `json:"coverUrl" gorm:"column:cover_url;type:varchar(500);not null;default:'';comment:直播间封面地址"`
	Notice     string `json:"notice" gorm:"column:notice;type:varchar(500);not null;default:'';comment:直播间公告"`

	StreamName        string `json:"streamName" gorm:"column:stream_name;type:varchar(64);not null;uniqueIndex:uk_live_room_stream;comment:SRS稳定流名称，不是场次ID"`
	PublishSecretHash string `json:"-" gorm:"column:publish_secret_hash;type:char(64);not null;default:'';comment:推流密钥SHA-256哈希"`
	StreamKeyVersion  uint32 `json:"streamKeyVersion" gorm:"column:stream_key_version;type:int unsigned;not null;default:1;comment:推流密钥版本"`

	Status           uint8  `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_live_room_discovery,priority:2;index:idx_live_room_category,priority:3;comment:直播间状态：0禁用 1正常 2关闭"`
	StatusReason     string `json:"statusReason" gorm:"column:status_reason;type:varchar(255);not null;default:'';comment:禁用或关闭原因"`
	StatusChangedAt  int64  `json:"statusChangedAt" gorm:"column:status_changed_at;type:bigint;not null;default:0;comment:直播间状态最近变更时间，毫秒时间戳"`
	LiveStatus       uint8  `json:"liveStatus" gorm:"column:live_status;type:tinyint unsigned;not null;default:0;index:idx_live_room_discovery,priority:1;index:idx_live_room_category,priority:2;comment:当前直播状态：0未开播 1准备中 2直播中 3结束中"`
	CurrentSessionId uint   `json:"-" gorm:"column:current_session_id;type:bigint unsigned;not null;default:0;comment:当前场次内部ID，0表示没有活动场次"`
	LiveStartedAt    int64  `json:"liveStartedAt" gorm:"column:live_started_at;type:bigint;not null;default:0;index:idx_live_room_discovery,priority:4,sort:desc;comment:当前场次开始时间，毫秒时间戳；结束后清零"`

	Visibility      uint8   `json:"visibility" gorm:"column:visibility;type:tinyint unsigned;not null;default:1;comment:可见范围：0私密 1公开 2仅关注者"`
	RecommendWeight int32   `json:"recommendWeight" gorm:"column:recommend_weight;type:int;not null;default:0;index:idx_live_room_discovery,priority:3,sort:desc;index:idx_live_room_category,priority:4,sort:desc;comment:直播间推荐权重"`
	Extra           *string `json:"extra,omitempty" gorm:"column:extra;type:json;serializer:json;default:null;comment:低频扩展配置"`
}

func (LiveRoom) TableName() string { return "live_room" }
