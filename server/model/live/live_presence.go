package live

// These append/checkpoint facts deliberately contain no bearer token, network address,
// wallet amount or mutable social relationship. Times are UTC Unix milliseconds.
type LiveViewerVisit struct {
	VisitNo           string `gorm:"type:varchar(72);primaryKey" json:"visitNo"`
	SessionNo         string `gorm:"type:varchar(64);not null;index:idx_presence_visit_session,priority:1;index:idx_presence_visit_viewer,priority:1" json:"sessionNo"`
	RoomNo            string `gorm:"type:varchar(64);not null;index" json:"roomNo"`
	ViewerID          uint64 `gorm:"not null;default:0;index:idx_presence_visit_viewer,priority:2" json:"-"`
	VisitorKey        string `gorm:"type:varchar(72);not null" json:"-"`
	JoinedAt          int64  `gorm:"not null" json:"joinedAt"`
	LastSeenAt        int64  `gorm:"not null" json:"lastSeenAt"`
	LeftAt            int64  `gorm:"not null;default:0" json:"leftAt"`
	WatchDurationMs   uint64 `gorm:"not null;default:0" json:"watchDurationMs,string"`
	LikeCount         uint64 `gorm:"not null;default:0" json:"likeCount,string"`
	EventCheckpointAt int64  `gorm:"not null;default:0" json:"-"`
	ExitReason        string `gorm:"type:varchar(32);not null" json:"exitReason"`
	Status            string `gorm:"type:varchar(16);not null;index:idx_presence_visit_session,priority:2" json:"status"`
	Revision          uint64 `gorm:"not null" json:"-"`
	CreatedAt         int64  `gorm:"not null" json:"-"`
	UpdatedAt         int64  `gorm:"not null" json:"-"`
}

func (LiveViewerVisit) TableName() string { return "live_viewer_visit" }

type LiveViewerSession struct {
	SessionNo       string `gorm:"type:varchar(64);primaryKey" json:"-"`
	VisitorKey      string `gorm:"type:varchar(72);primaryKey" json:"-"`
	ViewerID        uint64 `gorm:"not null;default:0;index" json:"-"`
	WatchDurationMs uint64 `gorm:"not null;default:0" json:"watchDurationMs,string"`
	LastSeenAt      int64  `gorm:"not null;default:0" json:"lastSeenAt"`
}

func (LiveViewerSession) TableName() string { return "live_viewer_session" }

type LivePresenceCheckpoint struct {
	SessionNo            string `gorm:"type:varchar(64);primaryKey" json:"sessionNo"`
	RoomNo               string `gorm:"type:varchar(64);not null" json:"roomNo"`
	Epoch                string `gorm:"type:char(32);not null" json:"-"`
	Revision             uint64 `gorm:"not null;default:0" json:"-"`
	ViewCount            uint64 `gorm:"not null;default:0" json:"viewCount,string"`
	ViewerCount          uint64 `gorm:"not null;default:0" json:"viewerCount,string"`
	PeakOnlineCount      uint64 `gorm:"not null;default:0" json:"peakOnlineCount,string"`
	LikeCount            uint64 `gorm:"not null;default:0" json:"likeCount,string"`
	TotalWatchDurationMs uint64 `gorm:"not null;default:0" json:"totalWatchDurationMs,string"`
	UpdatedAt            int64  `gorm:"not null;default:0;index:idx_presence_updated" json:"updatedAt"`
	AttemptAt            int64  `gorm:"not null;default:0;index:idx_presence_pending_attempt,priority:2" json:"-"`
	FinalizedAt          int64  `gorm:"not null;default:0;index:idx_presence_pending_attempt,priority:1" json:"finalizedAt"`
	DataLost             bool   `gorm:"not null;default:false" json:"dataLost"`
	LastError            string `gorm:"type:varchar(32);not null;default:''" json:"lastError"`
}

func (LivePresenceCheckpoint) TableName() string { return "live_presence_checkpoint" }

type LiveRoomEvent struct {
	EventID       string `gorm:"type:varchar(128);primaryKey" json:"eventId"`
	EventType     string `gorm:"type:varchar(48);not null" json:"eventType"`
	SchemaVersion uint8  `gorm:"not null;default:1" json:"schemaVersion"`
	RoomNo        string `gorm:"type:varchar(64);not null;index" json:"roomNo"`
	SessionNo     string `gorm:"type:varchar(64);not null;index:idx_room_event_session,priority:1" json:"sessionNo"`
	ActorID       uint64 `gorm:"not null;default:0" json:"-"`
	SourceType    string `gorm:"type:varchar(24);not null" json:"sourceType"`
	SourceNo      string `gorm:"type:varchar(96);not null" json:"sourceNo"`
	OccurredAt    int64  `gorm:"not null;index:idx_room_event_session,priority:2" json:"occurredAt"`
	Payload       string `gorm:"type:text;not null" json:"payload"`
}

func (LiveRoomEvent) TableName() string { return "live_room_event" }
func PresenceModels() []any {
	return []any{&LiveViewerVisit{}, &LiveViewerSession{}, &LivePresenceCheckpoint{}, &LiveRoomEvent{}}
}
