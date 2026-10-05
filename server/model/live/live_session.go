package live

import (
	"tb_live_module/global"

	"gorm.io/datatypes"
)

const (
	// LiveSessionPreparing（0）表示场次正在准备开播；即使已经收到on_publish，也必须等SRS采集到有效视频宽高后才能进入直播中。
	LiveSessionPreparing uint8 = iota
	// LiveSessionLiving（1）表示SRS已经确认发布流活跃且取得有效视频宽高，场次正式直播中。
	LiveSessionLiving
	// LiveSessionEnding（2）表示场次正在停止和结算，尚未完成最终统计汇总。
	LiveSessionEnding
	// LiveSessionEnded（3）表示场次已经结束并完成最终结算。
	LiveSessionEnded
	// LiveSessionCancelled（4）表示准备中的场次在成功开播前被取消，不计入已完成直播统计。
	LiveSessionCancelled
	// LiveSessionFailed（5）表示场次因系统或业务异常失败。
	LiveSessionFailed
)

const (
	// LiveMediaUnknown 表示尚未从SRS取得可靠的媒体状态。
	LiveMediaUnknown uint8 = iota
	// LiveMediaPublishing 表示最近一次可靠观察确认发布流存在。
	LiveMediaPublishing
	// LiveMediaDisconnected 表示最近一次可靠观察确认发布流已断开。
	LiveMediaDisconnected
	// LiveMediaStopping 表示业务已经请求结束，但尚未确认媒体停止。
	LiveMediaStopping
	// LiveMediaStopped 表示SRS或经授权的人工操作已确认媒体停止。
	LiveMediaStopped
	// LiveMediaDegraded 仅用于对外表示全局SRS不可观测；不会在故障时批量写入所有场次。
	LiveMediaDegraded
)

const (
	LiveStopRecoveryNormal uint8 = iota
	LiveStopRecoveryManualAttention
)

const (
	LiveEndingAlertNone uint8 = iota
	LiveEndingAlertWarning
	LiveEndingAlertCritical
)

const (
	LiveMediaStopSourceUnknown uint8 = iota
	LiveMediaStopSourceSRS
	LiveMediaStopSourceAdmin
)

const (
	// LiveSessionEndUnknown（0）表示尚未确定或未记录场次结束原因。
	LiveSessionEndUnknown uint8 = iota
	// LiveSessionEndByAnchor（1）表示主播主动结束直播。
	LiveSessionEndByAnchor
	// LiveSessionEndByAdmin（2）表示后台管理员强制结束直播。
	LiveSessionEndByAdmin
	// LiveSessionEndDisconnectTimeout（3）表示断流超过重连窗口后由系统结束直播。
	LiveSessionEndDisconnectTimeout
	// LiveSessionEndAnchorBlocked（4）表示主播被禁用、封禁、判定高风险或关闭开播权限而结束直播。
	LiveSessionEndAnchorBlocked
	// LiveSessionEndSystemError（5）表示系统异常导致直播结束。
	LiveSessionEndSystemError
	// LiveSessionEndPrepareTimeout（6）表示取得推流凭证后未在期限内成功开播。
	LiveSessionEndPrepareTimeout
)

// LiveSession 保存一次逻辑直播场次。短暂断流重连继续使用原场次。
// ActiveRoomId 是数据库生成列，用唯一索引保证同一直播间最多一个活动场次。
type LiveSession struct {
	global.GVA_MODEL
	SessionNo string `json:"sessionNo" gorm:"column:session_no;type:varchar(96);not null;uniqueIndex:uk_live_session_no;comment:直播场次对外编号，格式为yyyyMMddHHmmssSSS-anchorNo-roomNo"`
	RoomId    uint   `json:"-" gorm:"column:room_id;type:bigint unsigned;not null;index:idx_live_session_room_created,priority:1;comment:直播间内部ID"`
	AnchorId  uint   `json:"-" gorm:"column:anchor_id;type:bigint unsigned;not null;index:idx_live_session_anchor_started,priority:1;comment:主播内部ID快照"`

	CategoryId uint64         `json:"categoryId" gorm:"column:category_id;type:bigint unsigned;not null;default:0;index:idx_live_session_category_started,priority:1;comment:本场直播分类ID快照"`
	Title      string         `json:"title" gorm:"column:title;type:varchar(128);not null;default:'';comment:本场标题快照"`
	CoverURL   string         `json:"coverUrl" gorm:"column:cover_url;type:varchar(500);not null;default:'';comment:本场封面快照"`
	StreamInfo datatypes.JSON `json:"streamInfo" gorm:"column:stream_info;type:json;default:null;comment:取得有效视频宽高后保存的公开流媒体信息白名单JSON快照"`

	Status       uint8 `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:0;index:idx_live_session_prepare_timeout,priority:1;index:idx_live_session_reconnect,priority:1;index:idx_live_session_end_retry,priority:1;index:idx_live_session_end_stall,priority:1;index:idx_live_session_stream_ready,priority:1;index:idx_live_session_stream_probe,priority:1;comment:场次状态：0准备中 1直播中 2结束中 3已结束 4已取消 5失败"`
	ActiveRoomId *uint `json:"-" gorm:"->;type:bigint unsigned GENERATED ALWAYS AS (CASE WHEN status IN (0,1,2) THEN room_id ELSE NULL END) STORED;uniqueIndex:uk_live_session_active_room;comment:活动场次房间ID生成列"`

	PrepareDeadlineAt           int64  `json:"prepareDeadlineAt" gorm:"column:prepare_deadline_at;type:bigint;not null;default:0;index:idx_live_session_prepare_timeout,priority:2;comment:等待首次publish回调的截止时间，毫秒时间戳；0仅兼容待清理旧数据或表示已正式开播"`
	PublishCredentialIssueCount uint32 `json:"-" gorm:"column:publish_credential_issue_count;type:int unsigned;not null;default:1;comment:本场推流凭证累计签发次数，prepare首次签发计为1"`
	StartedAt                   int64  `json:"startedAt" gorm:"column:started_at;type:bigint;not null;default:0;index:idx_live_session_anchor_started,priority:2,sort:desc;index:idx_live_session_category_started,priority:2,sort:desc;comment:SRS首次取得有效视频宽高并确认正式开播的时间，毫秒时间戳"`
	PublishIP                   string `json:"-" gorm:"column:publish_ip;type:varchar(45);not null;default:'';comment:本场首次接受publish回调的推流IP，仅管理后台可见"`
	EndedAt                     int64  `json:"endedAt" gorm:"column:ended_at;type:bigint;not null;default:0;comment:实际最终断流或主动结束时间，毫秒时间戳"`
	DurationMs                  uint64 `json:"durationMs" gorm:"column:duration_ms;type:bigint unsigned;not null;default:0;comment:逻辑直播时长，等于ended_at-started_at，包含成功重连前的短暂断流"`

	LastUnpublishAt             int64  `json:"lastUnpublishAt" gorm:"column:last_unpublish_at;type:bigint;not null;default:0;comment:最近一次通过on_unpublish或SRS对账确认断流的时间，毫秒时间戳"`
	ReconnectDeadlineAt         int64  `json:"reconnectDeadlineAt" gorm:"column:reconnect_deadline_at;type:bigint;not null;default:0;index:idx_live_session_reconnect,priority:2;comment:等待重连截止时间，0表示不等待"`
	DisconnectCount             uint32 `json:"disconnectCount" gorm:"column:disconnect_count;type:int unsigned;not null;default:0;comment:本场累计断流次数"`
	SRSServerID                 string `json:"-" gorm:"column:srs_server_id;type:varchar(128);not null;default:'';comment:最近一次发布回调携带的SRS服务实例标识"`
	SRSStreamID                 string `json:"-" gorm:"column:srs_stream_id;type:varchar(128);not null;default:'';comment:最近一次发布回调携带的SRS流标识"`
	SRSClientID                 string `json:"-" gorm:"column:srs_client_id;type:varchar(128);not null;default:'';comment:最近一次发布回调携带的SRS客户端标识"`
	SRSVhost                    string `json:"-" gorm:"column:srs_vhost;type:varchar(255);not null;default:'';comment:最近一次有效发布使用的SRS vhost"`
	SRSApp                      string `json:"-" gorm:"column:srs_app;type:varchar(128);not null;default:'';comment:最近一次有效发布使用的SRS app"`
	SRSGeneration               uint64 `json:"-" gorm:"column:srs_generation;type:bigint unsigned;not null;default:0;comment:接受最近一次发布连接时的单SRS实例代际"`
	PublisherEpoch              uint64 `json:"-" gorm:"column:publisher_epoch;type:bigint unsigned;not null;default:0;comment:每次发布连接接管时递增的防迟到事件版本"`
	MediaState                  uint8  `json:"mediaState" gorm:"column:media_state;type:tinyint unsigned;not null;default:0;index:idx_live_session_media_state,priority:1;comment:最后可靠媒体状态：0未知 1发布中 2已断流 3停止中 4已停止"`
	MediaStateChangedAt         int64  `json:"mediaStateChangedAt" gorm:"column:media_state_changed_at;type:bigint;not null;default:0;comment:媒体状态最近变化时间，毫秒时间戳"`
	MediaLastConfirmedAt        int64  `json:"mediaLastConfirmedAt" gorm:"column:media_last_confirmed_at;type:bigint;not null;default:0;comment:最近一次从SRS可靠确认媒体状态的时间"`
	StreamReadyDeadlineAt       int64  `json:"-" gorm:"column:stream_ready_deadline_at;type:bigint;not null;default:0;index:idx_live_session_stream_ready,priority:2;comment:等待SRS返回有效视频宽高的截止时间，毫秒时间戳"`
	StreamProbeAttempts         uint32 `json:"-" gorm:"column:stream_probe_attempts;type:int unsigned;not null;default:0;comment:SRS流信息退避探测次数"`
	StreamProbeNextAt           int64  `json:"-" gorm:"column:stream_probe_next_at;type:bigint;not null;default:0;index:idx_live_session_stream_probe,priority:2;comment:下一次SRS流信息探测时间或处理租约截止时间"`
	StreamProbeLastError        string `json:"-" gorm:"column:stream_probe_last_error;type:varchar(500);not null;default:'';comment:最近一次SRS流信息探测错误"`
	StreamReconcileCheckedAt    int64  `json:"-" gorm:"column:stream_reconcile_checked_at;type:bigint;not null;default:0;index:idx_live_session_stream_reconcile,priority:2;comment:最近一次直播中SRS对账完成时间，毫秒时间戳"`
	StreamReconcileMissingCount uint32 `json:"-" gorm:"column:stream_reconcile_missing_count;type:int unsigned;not null;default:0;comment:直播中连续成功查询SRS但未发现当前发布流的次数"`
	StreamReconcileLastSeenAt   int64  `json:"-" gorm:"column:stream_reconcile_last_seen_at;type:bigint;not null;default:0;comment:最近一次从SRS确认当前发布流仍存在的时间，毫秒时间戳"`
	StreamReconcileLastError    string `json:"-" gorm:"column:stream_reconcile_last_error;type:varchar(500);not null;default:'';comment:最近一次直播中SRS对账请求或身份校验错误"`

	StopRequestedAt      int64  `json:"-" gorm:"column:stop_requested_at;type:bigint;not null;default:0;index:idx_live_session_end_stall,priority:2;comment:进入结束中的请求时间，毫秒时间戳"`
	StopAttempts         uint32 `json:"-" gorm:"column:stop_attempts;type:int unsigned;not null;default:0;comment:SRS停止推流累计尝试次数"`
	StopNextRetryAt      int64  `json:"-" gorm:"column:stop_next_retry_at;type:bigint;not null;default:0;index:idx_live_session_end_retry,priority:2;comment:下次停止推流重试时间或当前处理租约截止时间"`
	StopLastError        string `json:"-" gorm:"column:stop_last_error;type:varchar(500);not null;default:'';comment:最近一次停止推流失败原因"`
	StopRecoveryState    uint8  `json:"stopRecoveryState" gorm:"column:stop_recovery_state;type:tinyint unsigned;not null;default:0;index:idx_live_session_end_attention,priority:1;comment:收尾恢复状态：0自动恢复 1需要人工关注"`
	EndingAlertLevel     uint8  `json:"endingAlertLevel" gorm:"column:ending_alert_level;type:tinyint unsigned;not null;default:0;index:idx_live_session_end_attention,priority:2;comment:结束中滞留告警级别：0无 1警告 2严重"`
	EndingAlertedAt      int64  `json:"endingAlertedAt" gorm:"column:ending_alerted_at;type:bigint;not null;default:0;comment:最近一次提升结束中告警级别的时间"`
	MediaStopSource      uint8  `json:"mediaStopSource" gorm:"column:media_stop_source;type:tinyint unsigned;not null;default:0;comment:媒体停止确认来源：0未知 1SRS 2管理员"`
	MediaStopConfirmedAt int64  `json:"mediaStopConfirmedAt" gorm:"column:media_stop_confirmed_at;type:bigint;not null;default:0;comment:媒体停止确认时间"`
	MediaStopConfirmedBy uint   `json:"mediaStopConfirmedBy" gorm:"column:media_stop_confirmed_by;type:bigint unsigned;not null;default:0;comment:人工确认媒体停止的管理员ID"`

	EndReason     uint8  `json:"endReason" gorm:"column:end_reason;type:tinyint unsigned;not null;default:0;comment:结束原因：0未知 1主播结束 2管理员结束 3断流超时 4主播封禁 5系统异常 6准备开播超时"`
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
