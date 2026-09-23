package response

import "time"

// MyAnchorInfoResp 明确列出 APP 可见字段，避免模型后续新增内部字段时被自动暴露。
type MyAnchorInfoResp struct {
	IsAnchor bool          `json:"isAnchor"`
	Anchor   *MyAnchorInfo `json:"anchor"`
}

type MyAnchorInfo struct {
	AnchorNo            string   `json:"anchorNo"`
	Nickname            string   `json:"nickname"`
	Avatar              string   `json:"avatar"`
	Cover               string   `json:"cover"`
	Signature           string   `json:"signature"`
	Gender              uint8    `json:"gender"`
	Birthday            string   `json:"birthday,omitempty"`
	CountryCode         string   `json:"countryCode"`
	RegionCode          string   `json:"regionCode"`
	CityCode            string   `json:"cityCode"`
	Language            string   `json:"language"`
	AnchorType          uint8    `json:"anchorType"`
	CategoryId          uint64   `json:"categoryId"`
	Level               uint32   `json:"level"`
	TagIds              []uint64 `json:"tagIds"`
	AgencyId            uint64   `json:"agencyId"`
	ApplyStatus         uint8    `json:"applyStatus"`
	ApplyAt             int64    `json:"applyAt"`
	AuditAt             int64    `json:"auditAt"`
	RejectReason        string   `json:"rejectReason"`
	CertStatus          uint8    `json:"certStatus"`
	CertType            uint8    `json:"certType"`
	CertName            string   `json:"certName"`
	Status              uint8    `json:"status"`
	StatusReason        string   `json:"statusReason"`
	BanUntil            int64    `json:"banUntil"`
	LivePermission      uint8    `json:"livePermission"`
	PkPermission        uint8    `json:"pkPermission"`
	RecommendPermission uint8    `json:"recommendPermission"`
	WithdrawPermission  uint8    `json:"withdrawPermission"`
	IsSigned            uint8    `json:"isSigned"`
	FansCount           uint64   `json:"fansCount"`
	TotalLiveCount      uint64   `json:"totalLiveCount"`
	TotalLiveDurationMs uint64   `json:"totalLiveDurationMs"`
	MaxOnlineCount      uint32   `json:"maxOnlineCount"`
	TotalViewCount      uint64   `json:"totalViewCount"`
	LastLiveAt          int64    `json:"lastLiveAt"`
	LastLiveEndAt       int64    `json:"lastLiveEndAt"`
}

type AnchorApplyStatusResp struct {
	IsApplied    bool   `json:"isApplied"`
	AnchorNo     string `json:"anchorNo"`
	ApplyStatus  uint8  `json:"applyStatus"`
	ApplyAt      int64  `json:"applyAt"`
	AuditAt      int64  `json:"auditAt"`
	RejectReason string `json:"rejectReason"`
}

type AnchorPublicDetailResp struct {
	AnchorNo            string   `json:"anchorNo"`
	Nickname            string   `json:"nickname"`
	Avatar              string   `json:"avatar"`
	Cover               string   `json:"cover"`
	Signature           string   `json:"signature"`
	Gender              uint8    `json:"gender"`
	CountryCode         string   `json:"countryCode"`
	RegionCode          string   `json:"regionCode"`
	CityCode            string   `json:"cityCode"`
	Language            string   `json:"language"`
	AnchorType          uint8    `json:"anchorType"`
	CategoryId          uint64   `json:"categoryId"`
	Level               uint32   `json:"level"`
	TagIds              []uint64 `json:"tagIds"`
	CertStatus          uint8    `json:"certStatus"`
	CertType            uint8    `json:"certType"`
	CertName            string   `json:"certName"`
	IsSigned            uint8    `json:"isSigned"`
	FansCount           uint64   `json:"fansCount"`
	TotalLiveCount      uint64   `json:"totalLiveCount"`
	TotalLiveDurationMs uint64   `json:"totalLiveDurationMs"`
	MaxOnlineCount      uint32   `json:"maxOnlineCount"`
	TotalViewCount      uint64   `json:"totalViewCount"`
	LastLiveAt          int64    `json:"lastLiveAt"`
	IsRecommended       uint8    `json:"isRecommended"`
}

type AnchorPermissionCheckResp struct {
	Allow    bool   `json:"allow"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	BanUntil int64  `json:"banUntil,omitempty"`
}

type AnchorAdminListItemResp struct {
	ID                  uint      `json:"id"`
	CreatedAt           time.Time `json:"createdAt"`
	UserId              uint64    `json:"userId"`
	AnchorNo            string    `json:"anchorNo"`
	Nickname            string    `json:"nickname"`
	Avatar              string    `json:"avatar"`
	AnchorType          uint8     `json:"anchorType"`
	CategoryId          uint64    `json:"categoryId"`
	AgencyId            uint64    `json:"agencyId"`
	ApplyStatus         uint8     `json:"applyStatus"`
	CertStatus          uint8     `json:"certStatus"`
	Status              uint8     `json:"status"`
	LivePermission      uint8     `json:"livePermission"`
	PkPermission        uint8     `json:"pkPermission"`
	RecommendPermission uint8     `json:"recommendPermission"`
	WithdrawPermission  uint8     `json:"withdrawPermission"`
	IsSigned            uint8     `json:"isSigned"`
	IsRecommended       uint8     `json:"isRecommended"`
	RiskLevel           uint8     `json:"riskLevel"`
	Source              string    `json:"source"`
	ChannelId           uint64    `json:"channelId"`
}

// AnchorAdminListResp 与项目统一 PageResult JSON 结构一致，仅用于让 Swagger 明确 list 元素类型。
type AnchorAdminListResp struct {
	List     []AnchorAdminListItemResp `json:"list"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"pageSize"`
}

type AnchorAdminDetailResp struct {
	ID                  uint      `json:"id"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	UserId              uint64    `json:"userId"`
	AnchorNo            string    `json:"anchorNo"`
	Nickname            string    `json:"nickname"`
	Avatar              string    `json:"avatar"`
	Cover               string    `json:"cover"`
	Signature           string    `json:"signature"`
	Gender              uint8     `json:"gender"`
	Birthday            string    `json:"birthday,omitempty"`
	CountryCode         string    `json:"countryCode"`
	RegionCode          string    `json:"regionCode"`
	CityCode            string    `json:"cityCode"`
	Language            string    `json:"language"`
	AnchorType          uint8     `json:"anchorType"`
	CategoryId          uint64    `json:"categoryId"`
	Level               uint32    `json:"level"`
	TagIds              []uint64  `json:"tagIds"`
	AgencyId            uint64    `json:"agencyId"`
	AgencyJoinAt        int64     `json:"agencyJoinAt"`
	ApplyStatus         uint8     `json:"applyStatus"`
	ApplyAt             int64     `json:"applyAt"`
	AuditAt             int64     `json:"auditAt"`
	AuditUserId         uint64    `json:"auditUserId"`
	RejectReason        string    `json:"rejectReason"`
	CertStatus          uint8     `json:"certStatus"`
	CertType            uint8     `json:"certType"`
	CertName            string    `json:"certName"`
	Status              uint8     `json:"status"`
	StatusReason        string    `json:"statusReason"`
	BanUntil            int64     `json:"banUntil"`
	LivePermission      uint8     `json:"livePermission"`
	PkPermission        uint8     `json:"pkPermission"`
	RecommendPermission uint8     `json:"recommendPermission"`
	WithdrawPermission  uint8     `json:"withdrawPermission"`
	IsSigned            uint8     `json:"isSigned"`
	IsRecommended       uint8     `json:"isRecommended"`
	NewcomerUntil       int64     `json:"newcomerUntil"`
	Sort                int32     `json:"sort"`
	RecommendWeight     int32     `json:"recommendWeight"`
	FansCount           uint64    `json:"fansCount"`
	TotalLiveCount      uint64    `json:"totalLiveCount"`
	TotalLiveDurationMs uint64    `json:"totalLiveDurationMs"`
	MaxOnlineCount      uint32    `json:"maxOnlineCount"`
	TotalViewCount      uint64    `json:"totalViewCount"`
	LastLiveAt          int64     `json:"lastLiveAt"`
	LastLiveEndAt       int64     `json:"lastLiveEndAt"`
	Source              string    `json:"source"`
	SourceId            string    `json:"sourceId"`
	ChannelId           uint64    `json:"channelId"`
	RiskLevel           uint8     `json:"riskLevel"`
	Remark              string    `json:"remark"`
	Extra               *string   `json:"extra"`
}
