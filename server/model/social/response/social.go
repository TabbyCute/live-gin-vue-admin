package response

import (
	commonRes "tb_live_module/model/common/response"
	model "tb_live_module/model/social"
)

type Change struct {
	Changed bool `json:"changed"`
}
type User = commonRes.UserPublicProfile
type Relationship struct {
	User
	Following   bool              `json:"following"`
	FollowsYou  bool              `json:"followsYou"`
	Mutual      bool              `json:"mutual"`
	Blocked     bool              `json:"blocked"`
	BlockedByMe bool              `json:"blockedByMe"`
	MutedScopes []model.MuteScope `json:"mutedScopes"`
	Subscribed  bool              `json:"subscribed"`
}
type Counters struct {
	AccountNo      string `json:"accountNo"`
	FollowingCount uint64 `json:"followingCount"`
	FansCount      uint64 `json:"fansCount"`
}
type UserItem struct {
	User
	CreatedAt int64           `json:"createdAt"`
	Scope     model.MuteScope `json:"scope,omitempty"`
	Enabled   *bool           `json:"enabled,omitempty"`
}
type UserList struct {
	List       []UserItem `json:"list"`
	NextCursor string     `json:"nextCursor"`
	HasMore    bool       `json:"hasMore"`
}
type Resource struct {
	OwnerUserID  uint64             `json:"-"` // 仅用于跨模块批量关联公开用户资料，不对外暴露。
	ResourceType model.ResourceType `json:"resourceType"`
	ResourceID   string             `json:"resourceId"`
	Title        string             `json:"title"`
	CoverURL     string             `json:"coverUrl"`
	DurationMs   int64              `json:"durationMs"`
}
type FavoriteStatus struct {
	ResourceType model.ResourceType `json:"resourceType"`
	ResourceID   string             `json:"resourceId"`
	Favorite     bool               `json:"favorite"`
}
type ResourceItem struct {
	Resource
	CreatedAt     int64 `json:"createdAt"`
	LastWatchedAt int64 `json:"lastWatchedAt,omitempty"`
	PositionMs    int64 `json:"positionMs"`
	Completed     bool  `json:"completed"`
}
type ResourceList struct {
	List       []ResourceItem `json:"list"`
	NextCursor string         `json:"nextCursor"`
	HasMore    bool           `json:"hasMore"`
}

// HistoryUserItem 以用户公开资料展示历史，保留资源键用于删除原记录。
type HistoryUserItem struct {
	User
	ResourceType  model.ResourceType `json:"resourceType"`
	ResourceID    string             `json:"resourceId"`
	CreatedAt     int64              `json:"createdAt"`
	LastWatchedAt int64              `json:"lastWatchedAt"`
}
type HistoryUserList struct {
	List       []HistoryUserItem `json:"list"`
	NextCursor string            `json:"nextCursor"`
	HasMore    bool              `json:"hasMore"`
}
type WatchSession struct {
	SessionToken   string `json:"sessionToken"`
	SessionVersion uint64 `json:"sessionVersion"`
	Epoch          uint64 `json:"epoch"`
	PositionMs     int64  `json:"positionMs"`
	Completed      bool   `json:"completed"`
}
type WatchAccepted struct {
	Persisted      bool  `json:"persisted"`
	Accepted       bool  `json:"accepted"`
	MaxBufferAgeMs int64 `json:"maxBufferAgeMs"`
}
type CounterAudit struct {
	Counters
	ActualFollowing uint64 `json:"actualFollowing"`
	ActualFans      uint64 `json:"actualFans"`
	Mismatch        bool   `json:"mismatch"`
}
type Event struct {
	EventID       string            `json:"eventId"`
	EventType     string            `json:"eventType"`
	Status        model.EventStatus `json:"status"`
	CreatedAt     int64             `json:"createdAt"`
	Attempts      uint32            `json:"attempts"`
	NextAttemptAt int64             `json:"nextAttemptAt"`
	LastError     string            `json:"lastError"`
}
type EventList struct {
	List       []Event `json:"list"`
	NextCursor string  `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}
type OnlineRoom struct {
	User
	Title    string `json:"title"`
	CoverURL string `json:"coverUrl"`
}
type OnlineList struct {
	List       []OnlineRoom `json:"list"`
	NextCursor string       `json:"nextCursor"`
	HasMore    bool         `json:"hasMore"`
}
