package request

type PresenceOpen struct {
	RoomNo    string `json:"roomNo"`
	RequestID string `json:"requestId"`
}
type PresenceHeartbeat struct {
	PresenceToken string `json:"presenceToken"`
	Sequence      string `json:"sequence"`
	Playing       bool   `json:"playing"`
	Visible       bool   `json:"visible"`
}
type PresenceLeave struct {
	PresenceToken string `json:"presenceToken"`
	Sequence      string `json:"sequence"`
	Reason        string `json:"reason"`
}
type PresenceLike struct {
	PresenceToken string `json:"presenceToken"`
	RequestID     string `json:"requestId"`
	Sequence      string `json:"sequence"`
	Count         string `json:"count"`
}
type PresenceQuery struct {
	RoomNo string `form:"roomNo"`
	Cursor string `form:"cursor"`
	Limit  int    `form:"limit"`
}
type PresenceAdminBatch struct {
	SessionNos []string `json:"sessionNos"`
	RoomNos    []string `json:"roomNos"`
}
