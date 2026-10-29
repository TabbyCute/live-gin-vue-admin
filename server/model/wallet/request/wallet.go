package request

import common "tb_live_module/model/common/request"

type CreateAdjustment struct {
	RequestID       string `json:"requestId" binding:"required"`
	SourceType      string `json:"sourceType" example:"manual"`
	ExternalOrderNo string `json:"externalOrderNo" binding:"required" example:"BANK-20261028-0001"`
	AccountNo       string `json:"accountNo" binding:"required"`
	CurrencyCode    string `json:"currencyCode"`
	Amount          string `json:"amount" binding:"required" example:"100"`
	Reason          string `json:"reason" binding:"required"`
	EvidenceRef     string `json:"evidenceRef" binding:"required"`
}
type ReverseAdjustment struct {
	RequestID   string `json:"requestId" binding:"required"`
	OrderNo     string `json:"orderNo" binding:"required"`
	Reason      string `json:"reason" binding:"required"`
	EvidenceRef string `json:"evidenceRef"`
}
type ReviewAdjustment struct {
	OrderNo string `json:"orderNo" binding:"required"`
	Reason  string `json:"reason" binding:"required"`
}
type AccountQuery struct {
	AccountNo    string `form:"accountNo" json:"accountNo"`
	Phone        string `form:"phone" json:"phone"`
	CurrencyCode string `form:"currencyCode" json:"currencyCode"`
}
type OrderQuery struct {
	OrderNo string `form:"orderNo" json:"orderNo" binding:"required"`
}
type OrderSearch struct {
	common.PageInfo
	AccountNo       string `form:"accountNo" json:"accountNo"`
	Phone           string `form:"phone" json:"phone"`
	OrderNo         string `form:"orderNo" json:"orderNo"`
	Status          string `form:"status" json:"status"`
	Kind            string `form:"kind" json:"kind"`
	SourceType      string `form:"sourceType" json:"sourceType"`
	ExternalOrderNo string `form:"externalOrderNo" json:"externalOrderNo"`
}
type LedgerSearch struct {
	AccountQuery
	Limit  int    `form:"limit" json:"limit"`
	Cursor string `form:"cursor" json:"cursor"`
}
type AccountStatus struct {
	AccountNo    string `json:"accountNo"`
	Phone        string `json:"phone"`
	CurrencyCode string `json:"currencyCode"`
	Status       string `json:"status" binding:"required"`
	Reason       string `json:"reason" binding:"required"`
}

// CheckSearch 分批只读对账，按钱包版本向前扫描，最多1000条；不自动修账。
type CheckSearch struct {
	AccountQuery
	AfterVersion string `form:"afterVersion" json:"afterVersion"`
	AfterOrderID string `form:"afterOrderId" json:"afterOrderId"`
	Limit        int    `form:"limit" json:"limit"`
}
