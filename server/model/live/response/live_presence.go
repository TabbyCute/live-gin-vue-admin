package response

import common "tb_live_module/model/common/response"

type PresenceStats struct {
	RoomNo               string `json:"roomNo"`
	SessionNo            string `json:"sessionNo"`
	ConnectionCount      string `json:"connectionCount"`
	OnlineCount          string `json:"onlineCount"` // logged-in unique accounts
	MemberCount          string `json:"memberCount"`
	GuestCount           string `json:"guestCount"` // signed guest sessions, not physical people
	TotalOnlineCount     string `json:"totalOnlineCount"`
	ViewCount            string `json:"viewCount"`
	ViewerCount          string `json:"viewerCount"`
	PeakOnlineCount      string `json:"peakOnlineCount"` // member + guest peak
	LikeCount            string `json:"likeCount"`
	TotalWatchDurationMs string `json:"totalWatchDurationMs"`
	UpdatedAt            int64  `json:"updatedAt"`
	CheckpointAt         int64  `json:"checkpointAt"`
	FinalizedAt          int64  `json:"finalizedAt"`
	Stale                bool   `json:"stale"`
	DataLost             bool   `json:"dataLost"`
	PendingCount         string `json:"pendingCount"`
	CleanupLagMs         string `json:"cleanupLagMs"`
	WorkerHealthy        bool   `json:"workerHealthy"`
}
type PresenceSession struct {
	PresenceStats
	PresenceToken            string `json:"presenceToken,omitempty"`
	ConnectionNo             string `json:"connectionNo,omitempty"`
	HeartbeatIntervalSeconds int    `json:"heartbeatIntervalSeconds"`
	ExpiresInSeconds         int    `json:"expiresInSeconds"`
	ConnectionDurationMs     string `json:"connectionDurationMs"`
	ViewerSessionDurationMs  string `json:"viewerSessionDurationMs"`
	AcceptedCount            string `json:"acceptedCount,omitempty"`
	Closed                   bool   `json:"closed"`
}
type PresenceGuest struct {
	GuestToken       string `json:"guestToken"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}
type PresenceViewer struct {
	User             common.UserPublicProfile `json:"user"`
	LastActiveAt     int64                    `json:"lastActiveAt"`
	LastActivityType string                   `json:"lastActivityType"`
}
type PresenceViewers struct {
	List       []PresenceViewer `json:"list"`
	NextCursor string           `json:"nextCursor"`
	HasMore    bool             `json:"hasMore"`
}
