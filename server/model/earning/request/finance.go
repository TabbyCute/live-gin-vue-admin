package request

import common "tb_live_module/model/common/request"

type ExchangeRule struct {
	RequestID     string `json:"requestId"`
	Name          string `json:"name"`
	SourceUnit    string `json:"sourceUnit"`
	Currency      string `json:"currency"`
	CoinUnits     string `json:"coinUnits"`
	MinorUnits    string `json:"minorUnits"`
	RoundingMode  string `json:"roundingMode"`
	EffectiveFrom int64  `json:"effectiveFrom"`
	EffectiveTo   int64  `json:"effectiveTo"`
}
type FinanceReview struct {
	No      string `json:"no"`
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}
type CreateSettlement struct {
	RequestID   string `json:"requestId"`
	AnchorNo    string `json:"anchorNo"`
	RuleNo      string `json:"ruleNo"`
	PeriodStart int64  `json:"periodStart"`
	PeriodEnd   int64  `json:"periodEnd"`
}
type ApplyWithdrawal struct {
	PayeeAccountNo string `json:"payeeAccountNo"`
	RequestID      string `json:"requestId"`
	Currency       string `json:"currency"`
	Amount         string `json:"amount"`
}
type FinanceQuery struct {
	PayeeAccountNo string `form:"payeeAccountNo"`
	common.PageInfo
	No         string `form:"no"`
	AnchorNo   string `form:"anchorNo"`
	Currency   string `form:"currency"`
	SourceType string `form:"sourceType"`
	Status     string `form:"status"`
	From       int64  `form:"from"`
	To         int64  `form:"to"`
	Sort       string `form:"sort"`
	Desc       bool   `form:"desc"`
}
type Reconcile struct {
	WithdrawalNo string `json:"withdrawalNo"`
}

// AppWithdrawalResult accepts exactly one public identity for response-loss recovery.
type AppWithdrawalResult struct {
	No        string `form:"no"`
	RequestID string `form:"requestId"`
}
