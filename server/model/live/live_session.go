package live

import "tb_live_module/global"

const (
	LiveSessionPreparing uint8 = iota
	LiveSessionLiving
	LiveSessionEnding
	LiveSessionEnded
	LiveSessionCancelled
	LiveSessionFailed
)

const (
	LiveSessionEndUnknown uint8 = iota
	LiveSessionEndByAnchor
	LiveSessionEndByAdmin
	LiveSessionEndDisconnectTimeout
	LiveSessionEndAnchorBlocked
	LiveSessionEndSystemError
)

// LiveSession 保存一次逻辑直播场次。短暂断流重连继续使用原场次。
// ActiveRoomId 是数据库生成列，用唯一索引保证同一直播间最多一个活动场次。
type LiveSession struct {
	global.GVA_MODEL
	SessionNo string `json:"sessionNo" gorm:"column:session_no;type:varchar(32);not null;uniqueIndex:uk_live_session_no;comment:直播场次对外编号"`
	RoomId    uint   `json:"-" gorm:"column:room_id;type:bigint unsigned;not null;index:idx_live_session_room_created,priority:1;comment:直播间内部ID"`
	AnchorId  uint   `json:"-" gorm:"column:anchor_id;type:bigint unsigned;not null;index:idx_live_session_anchor_started,priority:1;comment:主播内部ID快照"`

	CategoryId uint64 `json:"categoryId" gorm:"column:category_id;type:bigint unsigned;not null;default:0;index:idx_live_session_category_started,priority:1;comment:本场直播分类ID快照"`
	Title      string `json:"title" gorm:"column:title;type:varchar(128);not null;default:'';comment:本场标题快照"`
	CoverURL   string `json:"coverUrl" gorm:"column:cover_url;type:varchar(500);not null;default:'';comment:本场封面快照"`

	Status       uint8 `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:0;index:idx_live_session_reconnect,priority:1;comment:场次状态：0准备中 1直播中 2结束中 3已结束 4已取消 5失败"`
	ActiveRoomId *uint `json:"-" gorm:"->;type:bigint unsigned GENERATED ALWAYS AS (CASE WHEN status IN (0,1,2) THEN room_id ELSE NULL END) STORED;uniqueIndex:uk_live_session_active_room;comment:活动场次房间ID生成列"`

	StartedAt  int64  `json:"startedAt" gorm:"column:started_at;type:bigint;not null;default:0;index:idx_live_session_anchor_started,priority:2,sort:desc;index:idx_live_session_category_started,priority:2,sort:desc;comment:首次成功发布流时间，毫秒时间戳"`
	EndedAt    int64  `json:"endedAt" gorm:"column:ended_at;type:bigint;not null;default:0;comment:实际最终断流或主动结束时间，毫秒时间戳"`
	DurationMs uint64 `json:"durationMs" gorm:"column:duration_ms;type:bigint unsigned;not null;default:0;comment:逻辑直播时长，等于ended_at-started_at，包含成功重连前的短暂断流"`

	LastUnpublishAt     int64  `json:"lastUnpublishAt" gorm:"column:last_unpublish_at;type:bigint;not null;default:0;comment:最近一次SRS on_unpublish时间，毫秒时间戳"`
	ReconnectDeadlineAt int64  `json:"reconnectDeadlineAt" gorm:"column:reconnect_deadline_at;type:bigint;not null;default:0;index:idx_live_session_reconnect,priority:2;comment:等待重连截止时间，0表示不等待"`
	DisconnectCount     uint32 `json:"disconnectCount" gorm:"column:disconnect_count;type:int unsigned;not null;default:0;comment:本场累计断流次数"`

	EndReason     uint8  `json:"endReason" gorm:"column:end_reason;type:tinyint unsigned;not null;default:0;comment:结束原因：0未知 1主播结束 2管理员结束 3断流超时 4主播封禁 5系统异常"`
	FailureReason string `json:"failureReason" gorm:"column:failure_reason;type:varchar(500);not null;default:'';comment:取消或失败原因"`

	ViewCount        uint64 `json:"viewCount" gorm:"column:view_count;type:bigint unsigned;not null;default:0;comment:累计进入直播间次数"`
	ViewerCount      uint64 `json:"viewerCount" gorm:"column:viewer_count;type:bigint unsigned;not null;default:0;comment:累计去重观看人数"`
	PeakOnlineCount  uint32 `json:"peakOnlineCount" gorm:"column:peak_online_count;type:int unsigned;not null;default:0;comment:最高同时在线人数"`
	LikeCount        uint64 `json:"likeCount" gorm:"column:like_count;type:bigint unsigned;not null;default:0;comment:本场最终点赞次数"`
	GiftCount        uint64 `json:"giftCount" gorm:"column:gift_count;type:bigint unsigned;not null;default:0;comment:本场有效礼物总件数"`
	GiftCoinAmount   uint64 `json:"giftCoinAmount" gorm:"column:gift_coin_amount;type:bigint unsigned;not null;default:0;comment:本场有效礼物金币总额"`
	GiftUserCount    uint32 `json:"giftUserCount" gorm:"column:gift_user_count;type:int unsigned;not null;default:0;comment:本场去重送礼人数"`
	StatsFinalizedAt int64  `json:"statsFinalizedAt" gorm:"column:stats_finalized_at;type:bigint;not null;default:0;comment:最终统计汇总完成时间，毫秒时间戳"`

	Extra *string `json:"extra,omitempty" gorm:"column:extra;type:json;serializer:json;default:null;comment:场次低频扩展信息"`
}

func (LiveSession) TableName() string { return "live_session" }
