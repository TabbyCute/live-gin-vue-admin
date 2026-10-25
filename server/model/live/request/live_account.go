package request

import commonReq "tb_live_module/model/common/request"

// Register 客户端账号注册参数。
type Register struct {
	Phone    string `json:"phone" binding:"required,max=20" example:"+60123456789"`
	SmsCode  string `json:"smsCode" binding:"required,len=6,numeric" example:"123456"`
	Password string `json:"password" binding:"required,min=8,max=72" example:"password123"`
	Nickname string `json:"nickname" binding:"omitempty,max=64" example:"Demo"`
	Avatar   string `json:"avatar" binding:"omitempty,max=500" example:"https://example.com/avatar.png"`
}

// Login 客户端账号登录参数。
type Login struct {
	Phone    string `json:"phone" binding:"required,max=20" example:"+60123456789"`
	Password string `json:"password" binding:"required,min=8,max=72" example:"password123"`
}

// AccountSMSSendReq 仅占位注册短信，不生成或发送验证码。
type AccountSMSSendReq struct {
	Phone   string `json:"phone" binding:"required,max=20" example:"+60123456789"`
	Purpose string `json:"purpose" binding:"omitempty,oneof=register" example:"register"`
}

type AccountProfileUpdateReq struct {
	Nickname    string  `json:"nickname" binding:"required,max=64"`
	Avatar      string  `json:"avatar" binding:"omitempty,max=500"`
	Signature   string  `json:"signature" binding:"omitempty,max=255"`
	Gender      uint8   `json:"gender" binding:"oneof=0 1 2 3"`
	Birthday    *string `json:"birthday" binding:"omitempty,datetime=2006-01-02"`
	CountryCode string  `json:"countryCode" binding:"omitempty,len=2,alpha"`
	RegionCode  string  `json:"regionCode" binding:"omitempty,max=32"`
	CityCode    string  `json:"cityCode" binding:"omitempty,max=32"`
	Language    string  `json:"language" binding:"omitempty,max=16"`
}

type AccountPasswordChangeReq struct {
	OldPassword string `json:"oldPassword" binding:"required,min=8,max=72"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=72"`
}

type AccountPublicProfileReq struct {
	AccountNo string `json:"accountNo" form:"accountNo" binding:"required,max=32"`
}

type AccountAdminListReq struct {
	commonReq.PageInfo
	AccountNo         string `json:"accountNo" form:"accountNo" binding:"omitempty,max=32"`
	Username          string `json:"username" form:"username" binding:"omitempty,max=32"`
	Phone             string `json:"phone" form:"phone" binding:"omitempty,max=20"`
	Nickname          string `json:"nickname" form:"nickname" binding:"omitempty,max=64"`
	Status            *uint8 `json:"status" form:"status" binding:"omitempty,oneof=1 2 3 4"`
	RealNameStatus    *uint8 `json:"realNameStatus" form:"realNameStatus" binding:"omitempty,oneof=0 1 2 3 4"`
	RegisterSource    string `json:"registerSource" form:"registerSource" binding:"omitempty,max=32"`
	RegisterChannelID uint64 `json:"registerChannelId" form:"registerChannelId"`
	CreatedAtStart    string `json:"createdAtStart" form:"createdAtStart" binding:"omitempty,datetime=2006-01-02"`
	CreatedAtEnd      string `json:"createdAtEnd" form:"createdAtEnd" binding:"omitempty,datetime=2006-01-02"`
}

type AccountAdminDetailReq struct {
	AccountID uint `json:"accountId" form:"accountId" binding:"required,gt=0"`
}

type AccountAdminStatusUpdateReq struct {
	AccountID    uint   `json:"accountId" binding:"required,gt=0"`
	Status       *uint8 `json:"status" binding:"required,oneof=1 2 3 4"`
	StatusReason string `json:"statusReason" binding:"omitempty,max=255"`
	BanUntil     int64  `json:"banUntil"`
}

type AccountAdminProfileUpdateReq struct {
	AccountID uint `json:"accountId" binding:"required,gt=0"`
	// Phone为nil表示不修改；提供时必须为非空E.164格式，后台设置后视为已验证。
	Phone *string `json:"phone" binding:"omitempty,max=20"`
	AccountProfileUpdateReq
}

type AccountAdminActionReq struct {
	AccountID uint `json:"accountId" binding:"required,gt=0"`
}
