package response

import "time"

type LiveRoomOwnerInfoResp struct {
	HasRoom bool          `json:"hasRoom"`
	Room    *LiveRoomInfo `json:"room"`
}

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

type LiveSessionInfo struct {
	SessionNo           string `json:"sessionNo"`
	RoomNo              string `json:"roomNo"`
	AnchorNo            string `json:"anchorNo"`
	CategoryId          uint64 `json:"categoryId"`
	Title               string `json:"title"`
	CoverURL            string `json:"coverUrl"`
	Status              uint8  `json:"status"`
	StartedAt           int64  `json:"startedAt"`
	EndedAt             int64  `json:"endedAt"`
	DurationMs          uint64 `json:"durationMs"`
	LastUnpublishAt     int64  `json:"lastUnpublishAt"`
	ReconnectDeadlineAt int64  `json:"reconnectDeadlineAt"`
	DisconnectCount     uint32 `json:"disconnectCount"`
	EndReason           uint8  `json:"endReason"`
	FailureReason       string `json:"failureReason"`
	ViewCount           uint64 `json:"viewCount"`
	ViewerCount         uint64 `json:"viewerCount"`
	PeakOnlineCount     uint32 `json:"peakOnlineCount"`
	LikeCount           uint64 `json:"likeCount"`
	GiftCount           uint64 `json:"giftCount"`
	GiftCoinAmount      uint64 `json:"giftCoinAmount"`
	GiftUserCount       uint32 `json:"giftUserCount"`
	StatsFinalizedAt    int64  `json:"statsFinalizedAt"`
}

type LiveSessionPrepareResp struct {
	RoomNo       string `json:"roomNo"`
	SessionNo    string `json:"sessionNo"`
	StreamName   string `json:"streamName"`
	PublishToken string `json:"publishToken"`
	PushURL      string `json:"pushUrl"`
	PlayURL      string `json:"playUrl"`
}

type LiveSessionHistoryResp struct {
	List     []LiveSessionInfo `json:"list"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
}

type LiveRoomPublicItem struct {
	RoomNo         string `json:"roomNo"`
	AnchorNo       string `json:"anchorNo"`
	AnchorNickname string `json:"anchorNickname"`
	AnchorAvatar   string `json:"anchorAvatar"`
	CategoryId     uint64 `json:"categoryId"`
	Title          string `json:"title"`
	CoverURL       string `json:"coverUrl"`
	Notice         string `json:"notice,omitempty"`
	LiveStartedAt  int64  `json:"liveStartedAt"`
	SessionNo      string `json:"sessionNo"`
	ViewCount      uint64 `json:"viewCount"`
	LikeCount      uint64 `json:"likeCount"`
}

type LiveRoomPublicListResp struct {
	List     []LiveRoomPublicItem `json:"list"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
}

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

type LiveRoomAdminListResp struct {
	List     []LiveRoomAdminItem `json:"list"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

type LiveSessionAdminItem struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	LiveSessionInfo
	RoomID   uint `json:"roomId"`
	AnchorID uint `json:"anchorId"`
}

type LiveSessionAdminListResp struct {
	List     []LiveSessionAdminItem `json:"list"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
}
