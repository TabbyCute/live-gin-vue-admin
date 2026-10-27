// Package social 定义社交事实表。关系使用硬删除，避免软删除破坏唯一约束。
package social

type Base struct {
	CreatedAt UTCTime `json:"-" gorm:"column:created_at;precision:3;not null;comment:UTC创建时间"`
	UpdatedAt UTCTime `json:"-" gorm:"column:updated_at;precision:3;not null;comment:UTC更新时间"`
}

type SocialFollow struct {
	ID uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键;index:idx_social_follow_out,priority:2,sort:desc;index:idx_social_follow_in,priority:2,sort:desc"`
	Base
	UserID       uint64 `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_follow_pair,priority:1;index:idx_social_follow_out,priority:1;check:ck_social_follow_1,user_id <> target_user_id AND user_id > 0 AND target_user_id > 0"`
	TargetUserID uint64 `json:"-" gorm:"column:target_user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_follow_pair,priority:2;index:idx_social_follow_in,priority:1"`
}

func (SocialFollow) TableName() string { return "social_follow" }

type SocialUserCounter struct {
	UserID         uint64  `json:"-" gorm:"column:user_id;type:bigint unsigned;primaryKey;autoIncrement:false"`
	FollowingCount uint64  `json:"-" gorm:"column:following_count;type:bigint unsigned;not null;default:0;check:ck_social_user_counter_1,following_count <= 9223372036854775807 AND fans_count <= 9223372036854775807"`
	FansCount      uint64  `json:"-" gorm:"column:fans_count;type:bigint unsigned;not null;default:0"`
	Version        uint64  `json:"-" gorm:"column:version;type:bigint unsigned;not null;default:0;comment:关系展示缓存版本"`
	HistoryEpoch   uint64  `json:"-" gorm:"column:history_epoch;type:bigint unsigned;not null;default:0;comment:清空历史的撤销代际"`
	HistoryVersion uint64  `json:"-" gorm:"column:history_version;type:bigint unsigned;not null;default:0;comment:用户观看会话递增版本，删除记录不重置"`
	CreatedAt      UTCTime `json:"-" gorm:"column:created_at;precision:3;not null"`
	UpdatedAt      UTCTime `json:"-" gorm:"column:updated_at;precision:3;not null"`
}

func (SocialUserCounter) TableName() string { return "social_user_counter" }

type SocialBlock struct {
	ID uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键;index:idx_social_block_list,priority:2,sort:desc"`
	Base
	UserID       uint64 `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_block_pair,priority:1;index:idx_social_block_list,priority:1;check:ck_social_block_1,user_id <> target_user_id AND user_id > 0 AND target_user_id > 0"`
	TargetUserID uint64 `json:"-" gorm:"column:target_user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_block_pair,priority:2"`
}

func (SocialBlock) TableName() string { return "social_block" }

type MuteScope string

const (
	MuteContent          MuteScope = "content"
	MuteLiveNotification MuteScope = "live_notification"
	MuteRecommendation   MuteScope = "recommendation"
	MuteDirectMessage    MuteScope = "direct_message"
)

func (s MuteScope) Valid() bool {
	return s == MuteContent || s == MuteLiveNotification || s == MuteRecommendation || s == MuteDirectMessage
}

type SocialMute struct {
	ID uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键;index:idx_social_mute_list,priority:2,sort:desc"`
	Base
	UserID       uint64    `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_mute_scope,priority:1;index:idx_social_mute_list,priority:1;check:ck_social_mute_1,user_id <> target_user_id AND user_id > 0 AND target_user_id > 0"`
	TargetUserID uint64    `json:"-" gorm:"column:target_user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_mute_scope,priority:2"`
	Scope        MuteScope `json:"-" gorm:"column:scope;type:varchar(32);not null;uniqueIndex:uk_social_mute_scope,priority:3;check:ck_social_mute_2,scope IN ('content','live_notification','recommendation','direct_message')"`
}

func (SocialMute) TableName() string { return "social_mute" }

type ResourceType string

const (
	ResourceLiveRoom   ResourceType = "live_room"
	ResourceLiveReplay ResourceType = "live_replay"
	ResourceShortVideo ResourceType = "short_video"
)

func (t ResourceType) Valid() bool {
	return t == ResourceLiveRoom || t == ResourceLiveReplay || t == ResourceShortVideo
}

type SocialFavorite struct {
	ID uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键;index:idx_social_favorite_list,priority:2,sort:desc;index:idx_social_favorite_type,priority:3,sort:desc"`
	Base
	UserID       uint64       `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_favorite_resource,priority:1;index:idx_social_favorite_list,priority:1;index:idx_social_favorite_type,priority:1"`
	ResourceType ResourceType `json:"-" gorm:"column:resource_type;type:varchar(32);not null;uniqueIndex:uk_social_favorite_resource,priority:2;index:idx_social_favorite_type,priority:2;check:ck_social_favorite_1,resource_type IN ('live_room','live_replay','short_video')"`
	ResourceID   string       `json:"-" gorm:"column:resource_id;type:varchar(64);not null;uniqueIndex:uk_social_favorite_resource,priority:3"`
}

func (SocialFavorite) TableName() string { return "social_favorite" }

type SocialWatchHistory struct {
	ID uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键;index:idx_social_history_recent,priority:3,sort:desc;index:idx_social_history_retention,priority:2"`
	Base
	UserID         uint64       `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_history_resource,priority:1;index:idx_social_history_recent,priority:1"`
	ResourceType   ResourceType `json:"-" gorm:"column:resource_type;type:varchar(32);not null;uniqueIndex:uk_social_history_resource,priority:2;check:ck_social_watch_history_1,resource_type IN ('live_room','live_replay','short_video')"`
	ResourceID     string       `json:"-" gorm:"column:resource_id;type:varchar(64);not null;uniqueIndex:uk_social_history_resource,priority:3"`
	LastWatchedAt  int64        `json:"-" gorm:"column:last_watched_at;type:bigint;not null;default:0;index:idx_social_history_recent,priority:2,sort:desc;index:idx_social_history_retention,priority:1;check:ck_social_watch_history_2,position_ms >= 0 AND last_watched_at >= 0"`
	PositionMs     int64        `json:"-" gorm:"column:position_ms;type:bigint;not null;default:0"`
	Completed      bool         `json:"-" gorm:"column:completed;not null;default:false"`
	SessionToken   string       `json:"-" gorm:"column:session_token;type:char(36);not null;default:''"`
	SessionVersion uint64       `json:"-" gorm:"column:session_version;type:bigint unsigned;not null;default:0"`
	Epoch          uint64       `json:"-" gorm:"column:epoch;type:bigint unsigned;not null;default:0"`
	Sequence       uint64       `json:"-" gorm:"column:sequence;type:bigint unsigned;not null;default:0"`
	OpenRequestID  string       `json:"-" gorm:"column:open_request_id;type:varchar(64);not null;default:''"`
	Hidden         bool         `json:"-" gorm:"column:hidden;not null;default:false"`
}

func (SocialWatchHistory) TableName() string { return "social_watch_history" }

type SocialLiveSubscription struct {
	ID uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键;index:idx_social_subscription_list,priority:2,sort:desc;index:idx_social_subscription_fanout,priority:3"`
	Base
	UserID       uint64 `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_subscription_pair,priority:1;index:idx_social_subscription_list,priority:1;check:ck_social_live_subscription_1,user_id <> target_user_id AND user_id > 0 AND target_user_id > 0"`
	TargetUserID uint64 `json:"-" gorm:"column:target_user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_subscription_pair,priority:2;index:idx_social_subscription_fanout,priority:1"`
	Enabled      bool   `json:"-" gorm:"column:enabled;not null;default:false;index:idx_social_subscription_fanout,priority:2"`
}

func (SocialLiveSubscription) TableName() string { return "social_live_subscription" }

type EventStatus string

const (
	EventPending      EventStatus = "pending"
	EventDone         EventStatus = "done"
	EventFailed       EventStatus = "failed"
	EventSkipped      EventStatus = "skipped"
	EventRelationship             = "relationship.changed"
	EventLiveStarted              = "live.started"
)

type SocialOutboxEvent struct {
	ID            uint64      `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键;index:idx_social_event_claim,priority:3;index:idx_social_event_audit,priority:3;index:idx_social_event_retention,priority:3"`
	CreatedAt     UTCTime     `json:"-" gorm:"column:created_at;precision:3;not null"`
	UpdatedAt     UTCTime     `json:"-" gorm:"column:updated_at;precision:3;not null;index:idx_social_event_retention,priority:2"`
	EventID       string      `json:"-" gorm:"column:event_id;type:char(36);not null;uniqueIndex:uk_social_event_id"`
	BusinessKey   string      `json:"-" gorm:"column:business_key;type:varchar(128);not null;uniqueIndex:uk_social_event_business"`
	EventType     string      `json:"-" gorm:"column:event_type;type:varchar(40);not null;index:idx_social_event_audit,priority:1"`
	Version       uint32      `json:"-" gorm:"column:version;type:int unsigned;not null;default:1"`
	UserID        uint64      `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;default:0"`
	TargetUserID  uint64      `json:"-" gorm:"column:target_user_id;type:bigint unsigned;not null;default:0"`
	ObjectNo      string      `json:"-" gorm:"column:object_no;type:varchar(96);not null;default:''"`
	Action        string      `json:"-" gorm:"column:action;type:varchar(32);not null;default:''"`
	Status        EventStatus `json:"-" gorm:"column:status;type:varchar(16);not null;default:pending;index:idx_social_event_claim,priority:1;index:idx_social_event_audit,priority:2;index:idx_social_event_retention,priority:1;check:ck_social_outbox_event_1,status IN ('pending','done','failed','skipped')"`
	NextAttemptAt int64       `json:"-" gorm:"column:next_attempt_at;type:bigint;not null;default:0;index:idx_social_event_claim,priority:2"`
	Attempts      uint32      `json:"-" gorm:"column:attempts;type:int unsigned;not null;default:0"`
	LeaseToken    string      `json:"-" gorm:"column:lease_token;type:char(36);not null;default:''"`
	Cursor        uint64      `json:"-" gorm:"column:cursor;type:bigint unsigned;not null;default:0"`
	LastError     string      `json:"-" gorm:"column:last_error;type:varchar(64);not null;default:'';comment:脱敏错误代码"`
}

func (SocialOutboxEvent) TableName() string { return "social_outbox_event" }

type SocialEventConsumer struct {
	ID uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;comment:内部主键"`
	Base
	EventID  string      `json:"-" gorm:"column:event_id;type:char(36);not null;uniqueIndex:uk_social_consumer_delivery,priority:1"`
	Consumer string      `json:"-" gorm:"column:consumer;type:varchar(32);not null;uniqueIndex:uk_social_consumer_delivery,priority:2"`
	UserID   uint64      `json:"-" gorm:"column:user_id;type:bigint unsigned;not null;uniqueIndex:uk_social_consumer_delivery,priority:3"`
	Status   EventStatus `json:"-" gorm:"column:status;type:varchar(16);not null;check:ck_social_event_consumer_1,status IN ('pending','done','failed','skipped')"`
}

func (SocialEventConsumer) TableName() string { return "social_event_consumer" }

func Models() []interface{} {
	return []interface{}{&SocialFollow{}, &SocialUserCounter{}, &SocialBlock{}, &SocialMute{}, &SocialFavorite{}, &SocialWatchHistory{}, &SocialLiveSubscription{}, &SocialOutboxEvent{}, &SocialEventConsumer{}}
}
