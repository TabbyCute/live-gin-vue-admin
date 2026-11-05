package request

type CreateChannelAccount struct {
	RequestID       string `json:"requestId"`
	ChannelCode     string `json:"channelCode"`
	AccountName     string `json:"accountName"`
	InstitutionName string `json:"institutionName"`
	Currency        string `json:"currency"`
	Account         string `json:"account"`
	SingleLimit     string `json:"singleLimit"`
	DailyLimit      string `json:"dailyLimit"`
}
type ChannelAccountAction struct {
	RequestID        string `json:"requestId"`
	ChannelAccountNo string `json:"channelAccountNo"`
	Reason           string `json:"reason"`
}
type ChannelAccountSearch struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Currency string `form:"currency"`
	Status   string `form:"status"`
}
type StartOrder struct {
	RequestID        string `json:"requestId"`
	PayoutNo         string `json:"payoutNo"`
	ChannelAccountNo string `json:"channelAccountNo"`
	Reason           string `json:"reason"`
}
type UnpaidAction struct {
	RequestID         string `json:"requestId"`
	AttemptNo         string `json:"attemptNo"`
	Reason            string `json:"reason"`
	ConfirmNoTransfer bool   `json:"confirmNoTransfer"`
}
type AttemptAction struct {
	RequestID string `json:"requestId"`
	AttemptNo string `json:"attemptNo"`
	Reason    string `json:"reason"`
}
type SubmitAttempt struct {
	RequestID           string `json:"requestId"`
	AttemptNo           string `json:"attemptNo"`
	PaymentReference    string `json:"paymentReference"`
	PaymentTime         int64  `json:"paymentTime"`
	EvidenceNo          string `json:"evidenceNo"`
	OperatorNote        string `json:"operatorNote"`
	ConfirmRealTransfer bool   `json:"confirmRealTransfer"`
}
type ReviewAttempt struct {
	RequestID             string `json:"requestId"`
	AttemptNo             string `json:"attemptNo"`
	Reason                string `json:"reason"`
	ConfirmExternalResult bool   `json:"confirmExternalResult"`
}
type ResolveAttempt struct {
	RequestID             string `json:"requestId"`
	AttemptNo             string `json:"attemptNo"`
	InvestigationNo       string `json:"investigationNo"`
	Decision              string `json:"decision"`
	Reason                string `json:"reason"`
	ConfirmExternalResult bool   `json:"confirmExternalResult"`
}
type OrderAction struct {
	RequestID string `json:"requestId"`
	PayoutNo  string `json:"payoutNo"`
	Reason    string `json:"reason"`
}
type CancelOrder struct {
	RequestID         string `json:"requestId"`
	PayoutNo          string `json:"payoutNo"`
	Reason            string `json:"reason"`
	ConfirmNoTransfer bool   `json:"confirmNoTransfer"`
}
type Investigate struct {
	RequestID  string `json:"requestId"`
	PayoutNo   string `json:"payoutNo"`
	Note       string `json:"note"`
	EvidenceNo string `json:"evidenceNo"`
}
type RevealGrant struct {
	RequestID string `json:"requestId"`
	TargetNo  string `json:"targetNo"`
	Reason    string `json:"reason"`
}
type RevealConsume struct {
	RequestID string `json:"requestId"`
	GrantNo   string `json:"grantNo"`
	Token     string `json:"token"`
}
type UploadEvidence struct {
	RequestID string `json:"requestId"`
	AttemptNo string `json:"attemptNo"`
	MIME      string `json:"mime"`
	Content   string `json:"content"`
}
type ReadEvidence struct {
	RequestID  string `json:"requestId"`
	EvidenceNo string `json:"evidenceNo"`
	Reason     string `json:"reason"`
}
