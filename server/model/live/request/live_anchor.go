package request

import commonReq "tb_live_module/model/common/request"

// AnchorApplyReq 不接受 userId；账号身份始终由客户端 JWT 决定。
type AnchorApplyReq struct {
	Nickname    string   `json:"nickname" binding:"required,max=64" example:"Alice"`
	Avatar      string   `json:"avatar" binding:"omitempty,max=500" example:"https://example.com/avatar.png"`
	Cover       string   `json:"cover" binding:"omitempty,max=500" example:"https://example.com/cover.png"`
	Signature   string   `json:"signature" binding:"omitempty,max=255" example:"欢迎来到我的直播间"`
	Gender      uint8    `json:"gender" binding:"oneof=0 1 2 3" example:"2"`
	Birthday    *string  `json:"birthday" binding:"omitempty,datetime=2006-01-02" example:"1998-01-01"`
	CountryCode string   `json:"countryCode" binding:"omitempty,len=2,alpha" example:"MY"`
	RegionCode  string   `json:"regionCode" binding:"omitempty,max=32" example:"10"`
	CityCode    string   `json:"cityCode" binding:"omitempty,max=32" example:"1001"`
	Language    string   `json:"language" binding:"omitempty,max=16" example:"ms-MY"`
	CategoryId  uint64   `json:"categoryId" example:"1"`
	TagIds      []uint64 `json:"tagIds" binding:"max=100,dive,gt=0" example:"1,3,8"`
	ChannelId   uint64   `json:"channelId" binding:"required,gt=0,lte=999999" example:"1"`
}

// AnchorProfileUpdateReq 是 APP 端公开资料白名单；缺省/零值也会被显式写入。
type AnchorProfileUpdateReq struct {
	Nickname    string   `json:"nickname" binding:"required,max=64" example:"Alice"`
	Avatar      string   `json:"avatar" binding:"omitempty,max=500"`
	Cover       string   `json:"cover" binding:"omitempty,max=500"`
	Signature   string   `json:"signature" binding:"omitempty,max=255"`
	Gender      uint8    `json:"gender" binding:"oneof=0 1 2 3"`
	Birthday    *string  `json:"birthday" binding:"omitempty,datetime=2006-01-02" example:"1998-01-01"`
	CountryCode string   `json:"countryCode" binding:"omitempty,len=2,alpha"`
	RegionCode  string   `json:"regionCode" binding:"omitempty,max=32"`
	CityCode    string   `json:"cityCode" binding:"omitempty,max=32"`
	Language    string   `json:"language" binding:"omitempty,max=16"`
	CategoryId  uint64   `json:"categoryId"`
	TagIds      []uint64 `json:"tagIds" binding:"max=100,dive,gt=0"`
}

// AnchorPublicDetailReq 使用对外主播编号查询，客户端不接触 LiveAnchor 数据库主键。
type AnchorPublicDetailReq struct {
	AnchorNo string `json:"anchorNo" form:"anchorNo" binding:"required,max=32" example:"1100001"`
}

type AnchorAdminListReq struct {
	commonReq.PageInfo
	AnchorId            uint   `json:"anchorId" form:"anchorId"`
	AnchorNo            string `json:"anchorNo" form:"anchorNo" binding:"omitempty,max=32"`
	UserId              uint64 `json:"userId" form:"userId"`
	Nickname            string `json:"nickname" form:"nickname" binding:"omitempty,max=64"`
	AnchorType          *uint8 `json:"anchorType" form:"anchorType" binding:"omitempty,oneof=0 1 2"`
	CategoryId          uint64 `json:"categoryId" form:"categoryId"`
	AgencyId            uint64 `json:"agencyId" form:"agencyId"`
	ApplyStatus         *uint8 `json:"applyStatus" form:"applyStatus" binding:"omitempty,oneof=0 1 2 3"`
	CertStatus          *uint8 `json:"certStatus" form:"certStatus" binding:"omitempty,oneof=0 1 2 3"`
	Status              *uint8 `json:"status" form:"status" binding:"omitempty,oneof=0 1 2 3"`
	LivePermission      *uint8 `json:"livePermission" form:"livePermission" binding:"omitempty,oneof=0 1"`
	PkPermission        *uint8 `json:"pkPermission" form:"pkPermission" binding:"omitempty,oneof=0 1"`
	RecommendPermission *uint8 `json:"recommendPermission" form:"recommendPermission" binding:"omitempty,oneof=0 1"`
	WithdrawPermission  *uint8 `json:"withdrawPermission" form:"withdrawPermission" binding:"omitempty,oneof=0 1"`
	IsSigned            *uint8 `json:"isSigned" form:"isSigned" binding:"omitempty,oneof=0 1"`
	IsRecommended       *uint8 `json:"isRecommended" form:"isRecommended" binding:"omitempty,oneof=0 1"`
	RiskLevel           *uint8 `json:"riskLevel" form:"riskLevel" binding:"omitempty,oneof=0 1 2 3"`
	Source              string `json:"source" form:"source" binding:"omitempty,max=32"`
	ChannelId           uint64 `json:"channelId" form:"channelId"`
	CreatedAtStart      string `json:"createdAtStart" form:"createdAtStart" binding:"omitempty,datetime=2006-01-02" example:"2026-01-01"`
	CreatedAtEnd        string `json:"createdAtEnd" form:"createdAtEnd" binding:"omitempty,datetime=2006-01-02" example:"2026-12-31"`
}

type AnchorAdminDetailReq struct {
	AnchorId uint `json:"anchorId" form:"anchorId" binding:"required,gt=0"`
}

type AnchorAuditReq struct {
	AnchorId     uint   `json:"anchorId" binding:"required,gt=0"`
	ApplyStatus  uint8  `json:"applyStatus" binding:"required,oneof=2 3" example:"2"`
	RejectReason string `json:"rejectReason" binding:"omitempty,max=255"`
}

type AnchorStatusUpdateReq struct {
	AnchorId     uint   `json:"anchorId" binding:"required,gt=0"`
	Status       *uint8 `json:"status" binding:"required,oneof=0 1 2 3"`
	StatusReason string `json:"statusReason" binding:"omitempty,max=255"`
	BanUntil     int64  `json:"banUntil" example:"1790000000000"`
}

type AnchorPermissionUpdateReq struct {
	AnchorId            uint   `json:"anchorId" binding:"required,gt=0"`
	LivePermission      *uint8 `json:"livePermission" binding:"required,oneof=0 1"`
	PkPermission        *uint8 `json:"pkPermission" binding:"required,oneof=0 1"`
	RecommendPermission *uint8 `json:"recommendPermission" binding:"required,oneof=0 1"`
	WithdrawPermission  *uint8 `json:"withdrawPermission" binding:"required,oneof=0 1"`
}

type AnchorAdminProfileUpdateReq struct {
	AnchorId    uint     `json:"anchorId" binding:"required,gt=0"`
	Nickname    string   `json:"nickname" binding:"required,max=64"`
	Avatar      string   `json:"avatar" binding:"omitempty,max=500"`
	Cover       string   `json:"cover" binding:"omitempty,max=500"`
	Signature   string   `json:"signature" binding:"omitempty,max=255"`
	Gender      uint8    `json:"gender" binding:"oneof=0 1 2 3"`
	Birthday    *string  `json:"birthday" binding:"omitempty,datetime=2006-01-02"`
	CountryCode string   `json:"countryCode" binding:"omitempty,len=2,alpha"`
	RegionCode  string   `json:"regionCode" binding:"omitempty,max=32"`
	CityCode    string   `json:"cityCode" binding:"omitempty,max=32"`
	Language    string   `json:"language" binding:"omitempty,max=16"`
	AnchorType  uint8    `json:"anchorType" binding:"oneof=0 1 2"`
	CategoryId  uint64   `json:"categoryId"`
	TagIds      []uint64 `json:"tagIds" binding:"max=100,dive,gt=0"`
}

type AnchorRecommendUpdateReq struct {
	AnchorId        uint   `json:"anchorId" binding:"required,gt=0"`
	IsRecommended   *uint8 `json:"isRecommended" binding:"required,oneof=0 1"`
	NewcomerUntil   int64  `json:"newcomerUntil"`
	Sort            int32  `json:"sort"`
	RecommendWeight int32  `json:"recommendWeight"`
}

type AnchorSignedUpdateReq struct {
	AnchorId uint   `json:"anchorId" binding:"required,gt=0"`
	IsSigned *uint8 `json:"isSigned" binding:"required,oneof=0 1"`
}

type AnchorAgencyUpdateReq struct {
	AnchorId uint   `json:"anchorId" binding:"required,gt=0"`
	AgencyId uint64 `json:"agencyId"`
}

type AnchorRiskUpdateReq struct {
	AnchorId  uint   `json:"anchorId" binding:"required,gt=0"`
	RiskLevel *uint8 `json:"riskLevel" binding:"required,oneof=0 1 2 3"`
}

type AnchorRemarkUpdateReq struct {
	AnchorId uint   `json:"anchorId" binding:"required,gt=0"`
	Remark   string `json:"remark" binding:"max=500"`
}

type AnchorCertUpdateReq struct {
	AnchorId   uint   `json:"anchorId" binding:"required,gt=0"`
	CertStatus *uint8 `json:"certStatus" binding:"required,oneof=0 1 2 3"`
	CertType   *uint8 `json:"certType" binding:"required,oneof=0 1 2 3"`
	CertName   string `json:"certName" binding:"omitempty,max=64"`
}
