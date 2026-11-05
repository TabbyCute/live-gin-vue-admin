package request

import common "tb_live_module/model/common/request"

// Binding materials are write-only. No verification/identity claims are accepted.
type CreatePayee struct {
	RequestID      string `json:"requestId"`
	Method         string `json:"method"`
	Currency       string `json:"currency"`
	AccountName    string `json:"accountName"`
	Account        string `json:"account"`
	BankCode       string `json:"bankCode"`
	IdentifierType string `json:"identifierType"`
	AppID          string `json:"appId"`
}
type PayeeAction struct {
	PayeeAccountNo string `json:"payeeAccountNo"`
}
type PayeeSearch struct {
	common.PageInfo
	Currency string `form:"currency"`
	Status   string `form:"status"`
}
