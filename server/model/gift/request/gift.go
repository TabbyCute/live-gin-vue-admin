package request

import common "tb_live_module/model/common/request"

type Send struct {
	RoomNo    string `json:"roomNo"`
	GiftNo    string `json:"giftNo"`
	Quantity  string `json:"quantity"`
	RequestID string `json:"requestId"`
}
type Refund struct {
	OrderNo   string `json:"orderNo"`
	Quantity  string `json:"quantity"`
	RequestID string `json:"requestId"`
	Reason    string `json:"reason"`
}
type Category struct {
	CategoryNo  string `json:"categoryNo"`
	Name        string `json:"name"`
	IconURL     string `json:"iconUrl"`
	Description string `json:"description"`
	SortOrder   int32  `json:"sortOrder"`
	Status      string `json:"status"`
	Version     string `json:"version"`
}
type Item struct {
	GiftNo              string `json:"giftNo"`
	CategoryNo          string `json:"categoryNo"`
	Name                string `json:"name"`
	IconURL             string `json:"iconUrl"`
	AnimationURL        string `json:"animationUrl"`
	AnimationType       string `json:"animationType"`
	CoinPrice           string `json:"coinPrice"`
	SortOrder           int32  `json:"sortOrder"`
	Status              string `json:"status"`
	IsPremium           bool   `json:"isPremium"`
	MaxQuantityPerOrder string `json:"maxQuantityPerOrder"`
	Description         string `json:"description"`
	BenefitType         string `json:"benefitType"`
	GuardPlanNo         string `json:"guardPlanNo"`
	GuardDurationDays   string `json:"guardDurationDays"`
	BenefitStatus       string `json:"benefitStatus"`
	EffectiveFrom       int64  `json:"effectiveFrom"`
	EffectiveTo         int64  `json:"effectiveTo"`
	Version             string `json:"version"`
}
type Sort struct {
	Kind       string   `json:"kind"`
	CategoryNo string   `json:"categoryNo"`
	Numbers    []string `json:"numbers"`
	Version    string   `json:"version"`
}
type Delete struct {
	Kind    string `json:"kind"`
	No      string `json:"no"`
	Version string `json:"version"`
}
type Query struct {
	common.PageInfo
	No         string `form:"no" json:"no"`
	RequestID  string `form:"requestId" json:"requestId"`
	AccountNo  string `form:"accountNo" json:"accountNo"`
	AnchorNo   string `form:"anchorNo" json:"anchorNo"`
	RoomNo     string `form:"roomNo" json:"roomNo"`
	SessionNo  string `form:"sessionNo" json:"sessionNo"`
	CategoryNo string `form:"categoryNo" json:"categoryNo"`
	GiftNo     string `form:"giftNo" json:"giftNo"`
	Name       string `form:"name" json:"name"`
	Status     string `form:"status" json:"status"`
	MinAmount  string `form:"minAmount" json:"minAmount"`
	MaxAmount  string `form:"maxAmount" json:"maxAmount"`
	Refunded   string `form:"refunded" json:"refunded"`
	From       int64  `form:"from" json:"from"`
	To         int64  `form:"to" json:"to"`
	Sort       string `form:"sort" json:"sort"`
	Desc       bool   `form:"desc" json:"desc"`
	Dimension  string `form:"dimension" json:"dimension"`
	Period     string `form:"period" json:"period"`
}
