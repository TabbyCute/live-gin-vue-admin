package request

import commonReq "tb_live_module/model/common/request"

type LiveRoomSaveReq struct {
	CategoryId uint64 `json:"categoryId"`
	Title      string `json:"title" binding:"required,max=128"`
	CoverURL   string `json:"coverUrl" binding:"omitempty,max=500"`
	Notice     string `json:"notice" binding:"omitempty,max=500"`
	Visibility *uint8 `json:"visibility" binding:"required,oneof=0 1 2"`
}

type LiveRoomOwnerStatusReq struct {
	Status *uint8 `json:"status" binding:"required,oneof=1 2"`
}

type LiveSessionPrepareReq struct {
	CategoryId uint64 `json:"categoryId"`
	Title      string `json:"title" binding:"required,max=128"`
	CoverURL   string `json:"coverUrl" binding:"omitempty,max=500"`
	Visibility *uint8 `json:"visibility" binding:"required,oneof=0 1 2"`
}

type LiveSessionEndReq struct {
	SessionNo string `json:"sessionNo" binding:"required,max=32"`
}

type LiveSessionHistoryReq struct {
	commonReq.PageInfo
	Status *uint8 `json:"status" form:"status" binding:"omitempty,oneof=0 1 2 3 4 5"`
}

type LiveRoomPublicDetailReq struct {
	RoomNo string `json:"roomNo" form:"roomNo" binding:"required,max=32"`
}

type LiveRoomPublicListReq struct {
	commonReq.PageInfo
	CategoryId uint64 `json:"categoryId" form:"categoryId"`
}

// SRSHookReq 兼容 SRS HTTP callback 的核心字段。
type SRSHookReq struct {
	Action   string `json:"action"`
	ClientID string `json:"client_id"`
	IP       string `json:"ip"`
	Vhost    string `json:"vhost"`
	App      string `json:"app"`
	Stream   string `json:"stream" binding:"required,max=64"`
	Param    string `json:"param"`
}

type LiveSessionStatsReq struct {
	SessionNo       string `json:"sessionNo" binding:"required,max=32"`
	ViewCount       uint64 `json:"viewCount"`
	ViewerCount     uint64 `json:"viewerCount"`
	PeakOnlineCount uint32 `json:"peakOnlineCount"`
	LikeCount       uint64 `json:"likeCount"`
	GiftCount       uint64 `json:"giftCount"`
	GiftCoinAmount  uint64 `json:"giftCoinAmount"`
	GiftUserCount   uint32 `json:"giftUserCount"`
}

type LiveRoomAdminListReq struct {
	commonReq.PageInfo
	RoomNo     string `json:"roomNo" form:"roomNo" binding:"omitempty,max=32"`
	AnchorNo   string `json:"anchorNo" form:"anchorNo" binding:"omitempty,max=32"`
	Title      string `json:"title" form:"title" binding:"omitempty,max=128"`
	CategoryId uint64 `json:"categoryId" form:"categoryId"`
	Status     *uint8 `json:"status" form:"status" binding:"omitempty,oneof=0 1 2"`
	LiveStatus *uint8 `json:"liveStatus" form:"liveStatus" binding:"omitempty,oneof=0 1 2 3"`
}

type LiveRoomAdminDetailReq struct {
	RoomID uint `json:"roomId" form:"roomId" binding:"required,gt=0"`
}

type LiveRoomAdminUpdateReq struct {
	RoomID          uint   `json:"roomId" binding:"required,gt=0"`
	RoomNo          string `json:"roomNo" binding:"required,max=32"`
	CategoryId      uint64 `json:"categoryId"`
	Title           string `json:"title" binding:"required,max=128"`
	CoverURL        string `json:"coverUrl" binding:"omitempty,max=500"`
	Notice          string `json:"notice" binding:"omitempty,max=500"`
	Visibility      *uint8 `json:"visibility" binding:"required,oneof=0 1 2"`
	RecommendWeight int32  `json:"recommendWeight"`
}

type LiveRoomAdminStatusReq struct {
	RoomID       uint   `json:"roomId" binding:"required,gt=0"`
	Status       *uint8 `json:"status" binding:"required,oneof=0 1 2"`
	StatusReason string `json:"statusReason" binding:"omitempty,max=255"`
}

type LiveSessionAdminListReq struct {
	commonReq.PageInfo
	SessionNo      string `json:"sessionNo" form:"sessionNo" binding:"omitempty,max=32"`
	RoomNo         string `json:"roomNo" form:"roomNo" binding:"omitempty,max=32"`
	AnchorNo       string `json:"anchorNo" form:"anchorNo" binding:"omitempty,max=32"`
	CategoryId     uint64 `json:"categoryId" form:"categoryId"`
	Status         *uint8 `json:"status" form:"status" binding:"omitempty,oneof=0 1 2 3 4 5"`
	StartedAtStart int64  `json:"startedAtStart" form:"startedAtStart"`
	StartedAtEnd   int64  `json:"startedAtEnd" form:"startedAtEnd"`
}

type LiveSessionAdminDetailReq struct {
	SessionID uint `json:"sessionId" form:"sessionId" binding:"required,gt=0"`
}

type LiveSessionAdminEndReq struct {
	SessionID uint   `json:"sessionId" binding:"required,gt=0"`
	Reason    string `json:"reason" binding:"required,max=500"`
}
