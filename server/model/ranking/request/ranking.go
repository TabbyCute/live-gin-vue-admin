package request

type Query struct {
	RankType  string `form:"rankType" json:"rankType"`
	Period    string `form:"period" json:"period"`
	PeriodKey string `form:"periodKey" json:"periodKey"`
	ScopeType string `form:"scopeType" json:"scopeType"`
	ScopeNo   string `form:"scopeNo" json:"scopeNo"`
	GiftNo    string `form:"giftNo" json:"giftNo"`
	Page      int    `form:"page" json:"page"`
	PageSize  int    `form:"pageSize" json:"pageSize"`
}
type Action struct {
	RequestID string `json:"requestId"`
	Scope     string `json:"scope"`
}
