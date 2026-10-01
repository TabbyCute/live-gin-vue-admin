package request

import (
	"encoding/json"

	commonReq "tb_live_module/model/common/request"
)

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

// SRSHookReq 接收 SRS 6.x 及更高版本发布/停止发布回调的完整已知字段。
// Extra 会保留未来版本或自定义 SRS 模块增加的字段，避免反序列化时静默丢失。
type SRSHookReq struct {
	ServerID  string                     `json:"server_id" binding:"omitempty,max=128"`
	ServiceID string                     `json:"service_id" binding:"omitempty,max=128"`
	Action    string                     `json:"action" binding:"required,oneof=on_publish on_unpublish"`
	ClientID  string                     `json:"client_id" binding:"omitempty,max=128"`
	IP        string                     `json:"ip" binding:"omitempty,max=64"`
	Vhost     string                     `json:"vhost" binding:"omitempty,max=255"`
	App       string                     `json:"app" binding:"omitempty,max=128"`
	TCURL     string                     `json:"tcUrl" binding:"omitempty,max=2048"`
	Stream    string                     `json:"stream" binding:"required,max=64"`
	Param     string                     `json:"param" binding:"omitempty,max=8192"`
	StreamURL string                     `json:"stream_url" binding:"omitempty,max=2048"`
	StreamID  string                     `json:"stream_id" binding:"omitempty,max=128"`
	Extra     map[string]json.RawMessage `json:"-" swaggerignore:"true"`
}

// UnmarshalJSON 在解析标准字段的同时保留 SRS 后续版本增加的未知字段。
func (r *SRSHookReq) UnmarshalJSON(data []byte) error {
	type plainSRSHookReq SRSHookReq
	var known plainSRSHookReq
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	*r = SRSHookReq(known)

	var extra map[string]json.RawMessage
	if err := json.Unmarshal(data, &extra); err != nil {
		return err
	}
	for _, key := range []string{
		"server_id", "service_id", "action", "client_id", "ip", "vhost", "app",
		"tcUrl", "stream", "param", "stream_url", "stream_id",
	} {
		delete(extra, key)
	}
	if len(extra) > 0 {
		r.Extra = extra
	}
	return nil
}

type LiveSessionStatsReq struct {
	SessionNo       string `json:"sessionNo" binding:"required,max=96"`
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
	SessionNo      string `json:"sessionNo" form:"sessionNo" binding:"omitempty,max=96"`
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
