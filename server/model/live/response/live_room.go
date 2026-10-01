package response

import (
	"encoding/json"
	"time"
)

// LiveRoomOwnerInfoResp 表示 APP 主播查询本人直播间时的响应。
// HasRoom 为 false 时 Room 为 nil，用于兼容审核已通过但直播间尚未补建的异常数据。
// 使用接口：GET /api/v1/app/live/room/info。
type LiveRoomOwnerInfoResp struct {
	HasRoom bool          `json:"hasRoom"`
	Room    *LiveRoomInfo `json:"room"`
}

// LiveRoomInfo 表示主播本人可查看的直播间资料和当前状态。
// 该结构只暴露 roomNo、anchorNo 等对外编号，不包含数据库内部主键。
// 使用接口：POST /api/v1/app/live/room/save 直接返回；GET /api/v1/app/live/room/info 通过 room 字段嵌套返回。
type LiveRoomInfo struct {
	RoomNo           string           `json:"roomNo"`
	AnchorNo         string           `json:"anchorNo"`
	CategoryId       uint64           `json:"categoryId"`
	Title            string           `json:"title"`
	CoverURL         string           `json:"coverUrl"`
	Notice           string           `json:"notice"`
	StreamName       string           `json:"streamName"`
	StreamKeyVersion uint32           `json:"streamKeyVersion"`
	Status           uint8            `json:"status"`
	StatusReason     string           `json:"statusReason"`
	LiveStatus       uint8            `json:"liveStatus"`
	LiveStartedAt    int64            `json:"liveStartedAt"`
	Visibility       uint8            `json:"visibility"`
	CurrentSession   *LiveSessionInfo `json:"currentSession,omitempty"`
}

// LiveSessionInfo 表示 APP 端可查看的一场直播场次详情。
// 其中时间点和时长字段均以毫秒为单位，不包含场次、房间或主播的内部主键。
// 使用接口：GET /api/v1/app/live/room/session/current 和 POST /api/v1/app/live/hook/publish 直接返回；
// GET /api/v1/app/live/room/info 通过 currentSession 字段返回；GET /api/v1/app/live/room/session/history 作为列表元素返回。
// 管理后台的 GET /api/v1/admin/live/session/detail 和 GET /api/v1/admin/live/session/list 通过 LiveSessionAdminItem 嵌入返回。
type LiveSessionInfo struct {
	SessionNo           string          `json:"sessionNo"`
	RoomNo              string          `json:"roomNo"`
	AnchorNo            string          `json:"anchorNo"`
	CategoryId          uint64          `json:"categoryId"`
	Title               string          `json:"title"`
	CoverURL            string          `json:"coverUrl"`
	StreamInfo          json.RawMessage `json:"streamInfo" swaggertype:"object"`
	Status              uint8           `json:"status"`
	PrepareDeadlineAt   int64           `json:"prepareDeadlineAt"`
	StartedAt           int64           `json:"startedAt"`
	EndedAt             int64           `json:"endedAt"`
	DurationMs          uint64          `json:"durationMs"`
	LastUnpublishAt     int64           `json:"lastUnpublishAt"`
	ReconnectDeadlineAt int64           `json:"reconnectDeadlineAt"`
	DisconnectCount     uint32          `json:"disconnectCount"`
	EndReason           uint8           `json:"endReason"`
	FailureReason       string          `json:"failureReason"`
	ViewCount           uint64          `json:"viewCount"`
	ViewerCount         uint64          `json:"viewerCount"`
	PeakOnlineCount     uint32          `json:"peakOnlineCount"`
	LikeCount           uint64          `json:"likeCount"`
	GiftCount           uint64          `json:"giftCount"`
	GiftCoinAmount      uint64          `json:"giftCoinAmount"`
	GiftUserCount       uint32          `json:"giftUserCount"`
	StatsFinalizedAt    int64           `json:"statsFinalizedAt"`
}

// LiveSessionPrepareResp 表示主播准备开播成功后的响应。
// PublishToken 是本次准备生成的 AES-256-GCM 加密推流凭证，只在该响应中返回一次。
// 使用接口：POST /api/v1/app/live/room/session/prepare。
type LiveSessionPrepareResp struct {
	RoomNo            string `json:"roomNo"`
	SessionNo         string `json:"sessionNo"`
	StreamName        string `json:"streamName"`
	PrepareDeadlineAt int64  `json:"prepareDeadlineAt"`
	PublishToken      string `json:"publishToken"`
	PushURL           string `json:"pushUrl"`
	PlayURL           string `json:"playUrl"`
}

// LiveSessionHistoryResp 表示 APP 主播本人的历史直播场次分页响应。
// 使用接口：GET /api/v1/app/live/room/session/history。
type LiveSessionHistoryResp struct {
	List     []LiveSessionInfo `json:"list"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
}

// LiveRoomPublicItem 表示公开直播列表中的单个房间。
// 使用接口：GET /api/v1/app/live/room/list 作为列表元素返回。
type LiveRoomPublicItem struct {
	RoomNo         string `json:"roomNo"`           // RoomNo 来源于 MySQL live_room.room_no，是直播间对外编号。
	AnchorNo       string `json:"anchorNo"`         // AnchorNo 来源于 MySQL live_anchor.anchor_no，通过 live_room.anchor_id 关联主播。
	AnchorNickname string `json:"anchorNickname"`   // AnchorNickname 来源于 MySQL live_anchor.nickname，通过 live_room.anchor_id 关联主播。
	AnchorAvatar   string `json:"anchorAvatar"`     // AnchorAvatar 来源于 MySQL live_anchor.avatar，通过 live_room.anchor_id 关联主播。
	CategoryId     uint64 `json:"categoryId"`       // CategoryId 来源于 MySQL live_room.category_id，表示直播间当前分类，不是场次快照。
	Title          string `json:"title"`            // Title 来源于 MySQL live_room.title，表示直播间当前标题，不是场次快照。
	CoverURL       string `json:"coverUrl"`         // CoverURL 来源于 MySQL live_room.cover_url，表示直播间当前封面，不是场次快照。
	Notice         string `json:"notice,omitempty"` // Notice 来源于 MySQL live_room.notice；值为空时可能因 omitempty 不出现在 JSON 中。
	LiveStatus     uint8  `json:"liveStatus"`       // LiveStatus 来源于 MySQL live_room.live_status：0未开播、1准备中、2直播中、3结束中。
	LiveStartedAt  int64  `json:"liveStartedAt"`    // LiveStartedAt 来源于 MySQL live_room.live_started_at，是当前场次首次有效推流的毫秒时间戳。
	SessionNo      string `json:"sessionNo"`        // SessionNo 来源于 MySQL live_session.session_no，通过 live_room.current_session_id 关联；没有当前场次时返回空字符串。
	ViewCount      uint64 `json:"viewCount"`        // ViewCount 来源于 MySQL live_session.view_count，通过 live_room.current_session_id 关联；没有当前场次时返回 0。
	LikeCount      uint64 `json:"likeCount"`        // LikeCount 来源于 MySQL live_session.like_count，通过 live_room.current_session_id 关联；没有当前场次时返回 0。
}

// LiveSessionPublicSummary 表示公开直播间详情中的当前场次或最近一次已结束场次。
// 只包含观众端展示需要的业务快照和汇总数据，不暴露内部主键、断流重连、失败原因、结算时间或礼物金额。
// 使用接口：GET /api/v1/app/live/room/detail，通过 latestSession 字段返回。
type LiveSessionPublicSummary struct {
	SessionNo       string          `json:"sessionNo"`                       // SessionNo 来源于 live_session.session_no，是场次对外编号。
	CategoryId      uint64          `json:"categoryId"`                      // CategoryId 来源于 live_session.category_id，是本场分类快照。
	Title           string          `json:"title"`                           // Title 来源于 live_session.title，是本场标题快照。
	CoverURL        string          `json:"coverUrl"`                        // CoverURL 来源于 live_session.cover_url，是本场封面快照。
	StreamInfo      json.RawMessage `json:"streamInfo" swaggertype:"object"` // StreamInfo 来源于 live_session.stream_info，只保存允许公开的视频信息白名单；暂无信息时返回 {}。
	Status          uint8           `json:"status"`                          // Status 来源于 live_session.status：0准备中、1直播中、2结束中、3已结束。
	StartedAt       int64           `json:"startedAt"`                       // StartedAt 来源于 live_session.started_at，单位毫秒。
	EndedAt         int64           `json:"endedAt"`                         // EndedAt 来源于 live_session.ended_at，未结束时为0，单位毫秒。
	DurationMs      uint64          `json:"durationMs"`                      // DurationMs 来源于 live_session.duration_ms，单位毫秒。
	ViewCount       uint64          `json:"viewCount"`                       // ViewCount 来源于 live_session.view_count。
	ViewerCount     uint64          `json:"viewerCount"`                     // ViewerCount 来源于 live_session.viewer_count。
	PeakOnlineCount uint32          `json:"peakOnlineCount"`                 // PeakOnlineCount 来源于 live_session.peak_online_count。
	LikeCount       uint64          `json:"likeCount"`                       // LikeCount 来源于 live_session.like_count。
}

// MarshalJSON 保证没有当前或历史场次时响应为 {}，而不是 null、字段缺失或带有歧义的全零场次。
func (s LiveSessionPublicSummary) MarshalJSON() ([]byte, error) {
	if s.SessionNo == "" {
		return []byte("{}"), nil
	}
	type liveSessionPublicSummaryAlias LiveSessionPublicSummary
	return json.Marshal(liveSessionPublicSummaryAlias(s))
}

// LiveRoomPublicItemV2 表示公开直播间详情。
// 房间未开播时仍返回房间和完整 AnchorInfo；LatestSession 返回最近一次已结束直播，从未完成过直播时返回空对象。
// 房间存在准备中、直播中或结束中的活动场次时，LatestSession 返回该活动场次。
// 使用接口：GET /api/v1/app/live/room/detail。
type LiveRoomPublicItemV2 struct {
	RoomNo        string                   `json:"roomNo"`        // RoomNo 来源于 live_room.room_no。
	CategoryId    uint64                   `json:"categoryId"`    // CategoryId 来源于 live_room.category_id，是房间当前分类。
	Title         string                   `json:"title"`         // Title 来源于 live_room.title，是房间当前标题。
	CoverURL      string                   `json:"coverUrl"`      // CoverURL 来源于 live_room.cover_url，是房间当前封面。
	Notice        string                   `json:"notice"`        // Notice 来源于 live_room.notice。
	StreamName    string                   `json:"streamName"`    // StreamName 来源于 live_room.stream_name，是稳定流名称，不是推流凭证。
	LiveStatus    uint8                    `json:"liveStatus"`    // LiveStatus 来源于 live_room.live_status：0未开播、1准备中、2直播中、3结束中。
	AnchorInfo    AnchorPublicDetailResp   `json:"anchorInfo"`    // AnchorInfo 来源于 live_anchor，保持完整公开主播详情结构。
	LatestSession LiveSessionPublicSummary `json:"latestSession"` // LatestSession 优先取当前活动场次，否则取最近一次已结束直播；没有时返回 {}。
}

// LiveRoomPublicListResp 表示公开的正在直播房间分页响应。
// 使用接口：GET /api/v1/app/live/room/list。
type LiveRoomPublicListResp struct {
	List     []LiveRoomPublicItem `json:"list"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
}

// LiveRoomAdminItem 表示管理后台中的单个直播间。
// 后台管理需要精确关联和操作数据，因此该结构会返回内部主键和当前场次主键。
// 使用接口：GET /api/v1/admin/live/room/list 作为列表元素返回；
// GET /api/v1/admin/live/room/detail 通过 LiveRoomAdminDetailResp 嵌入返回。
type LiveRoomAdminItem struct {
	ID               uint      `json:"id"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	RoomNo           string    `json:"roomNo"`
	AnchorID         uint      `json:"anchorId"`
	AnchorNo         string    `json:"anchorNo"`
	AnchorNickname   string    `json:"anchorNickname"`
	CategoryId       uint64    `json:"categoryId"`
	Title            string    `json:"title"`
	CoverURL         string    `json:"coverUrl"`
	Notice           string    `json:"notice"`
	StreamName       string    `json:"streamName"`
	StreamKeyVersion uint32    `json:"streamKeyVersion"`
	Status           uint8     `json:"status"`
	StatusReason     string    `json:"statusReason"`
	StatusChangedAt  int64     `json:"statusChangedAt"`
	LiveStatus       uint8     `json:"liveStatus"`
	CurrentSessionID uint      `json:"currentSessionId"`
	LiveStartedAt    int64     `json:"liveStartedAt"`
	Visibility       uint8     `json:"visibility"`
	RecommendWeight  int32     `json:"recommendWeight"`
}

// LiveRoomAdminDetailResp 表示管理后台的直播间详情。
// CurrentSession 只在存在当前活动场次时返回；列表接口不查询该快照。
type LiveRoomAdminDetailResp struct {
	LiveRoomAdminItem
	CurrentSession *LiveSessionAdminItem `json:"currentSession,omitempty"`
}

// LiveRoomAdminListResp 表示管理后台的直播间分页响应。
// 使用接口：GET /api/v1/admin/live/room/list。
type LiveRoomAdminListResp struct {
	List     []LiveRoomAdminItem `json:"list"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

// LiveSessionAdminItem 表示管理后台中的单个直播场次。
// 它在通用场次信息上增加场次、直播间和主播的内部主键，仅供后台使用。
// 使用接口：GET /api/v1/admin/live/session/detail 直接返回；GET /api/v1/admin/live/session/list 作为列表元素返回。
type LiveSessionAdminItem struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	LiveSessionInfo
	RoomID    uint   `json:"roomId"`
	AnchorID  uint   `json:"anchorId"`
	PublishIP string `json:"publishIp"`
}

// LiveSessionAdminListResp 表示管理后台的直播场次分页响应。
// 使用接口：GET /api/v1/admin/live/session/list。
type LiveSessionAdminListResp struct {
	List     []LiveSessionAdminItem `json:"list"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
}
