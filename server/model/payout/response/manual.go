package response

import model "tb_live_module/model/payout"

type ActionResult struct {
	PayoutNo         string `json:"payoutNo,omitempty"`
	AttemptNo        string `json:"attemptNo,omitempty"`
	Status           string `json:"status,omitempty"`
	ChannelAccountNo string `json:"channelAccountNo,omitempty"`
	EvidenceNo       string `json:"evidenceNo,omitempty"`
	EvidenceHash     string `json:"evidenceHash,omitempty"`
	GrantNo          string `json:"grantNo,omitempty"`
	ExpiresAt        int64  `json:"expiresAt,omitempty"`
	InvestigationNo  string `json:"investigationNo,omitempty"`
	ReconciliationNo string `json:"reconciliationNo,omitempty"`
}
type ChannelAccount struct {
	ChannelAccountNo string `json:"channelAccountNo"`
	ChannelCode      string `json:"channelCode"`
	AccountName      string `json:"accountName"`
	InstitutionName  string `json:"institutionName"`
	Currency         string `json:"currency"`
	MaskedAccount    string `json:"maskedAccount"`
	Status           string `json:"status"`
	SingleLimit      int64  `json:"singleLimit,string"`
	DailyLimit       int64  `json:"dailyLimit,string"`
	CreatedBy        uint64 `json:"createdBy,string"`
	CreatedAt        int64  `json:"createdAt"`
	UpdatedAt        int64  `json:"updatedAt"`
	DisabledAt       int64  `json:"disabledAt"`
}

func PublicChannelAccount(a model.ChannelAccount) ChannelAccount {
	return ChannelAccount{a.ChannelAccountNo, a.ChannelCode, a.AccountName, a.InstitutionName, a.Currency, a.MaskedAccount, a.Status, a.SingleLimit, a.DailyLimit, a.CreatedBy, a.CreatedAt, a.UpdatedAt, a.DisabledAt}
}

type ChannelAccountPage struct {
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total,string"`
	List     []ChannelAccount `json:"list"`
}
type Attempt struct {
	AttemptNo         string `json:"attemptNo"`
	PayoutNo          string `json:"payoutNo"`
	Sequence          int64  `json:"sequence,string"`
	ChannelCode       string `json:"channelCode"`
	ChannelAccountNo  string `json:"channelAccountNo"`
	CompanyMasked     string `json:"companyMasked"`
	Amount            int64  `json:"amount,string"`
	Currency          string `json:"currency"`
	Status            string `json:"status"`
	OperatorID        uint64 `json:"operatorId,string"`
	ReviewerID        uint64 `json:"reviewerId,string"`
	PaymentReference  string `json:"paymentReference"`
	PaymentTime       int64  `json:"paymentTime"`
	EvidenceReference string `json:"evidenceReference"`
	EvidenceHash      string `json:"evidenceHash"`
	OperatorNote      string `json:"operatorNote"`
	ReviewReason      string `json:"reviewReason"`
	BankSubmittedAt   int64  `json:"bankSubmittedAt"`
	SubmittedAt       int64  `json:"submittedAt"`
	ReviewedAt        int64  `json:"reviewedAt"`
	CompletedAt       int64  `json:"completedAt"`
	FailureCode       string `json:"failureCode"`
	FailureReason     string `json:"failureReason"`
	CreatedAt         int64  `json:"createdAt"`
	UpdatedAt         int64  `json:"updatedAt"`
}

func PublicAttempt(a model.Attempt) Attempt {
	return Attempt{a.AttemptNo, a.PayoutNo, a.Sequence, a.ChannelCode, a.ChannelAccountNo, a.CompanyMasked, a.Amount, a.Currency, a.Status, a.OperatorID, a.ReviewerID, a.PaymentReference, a.PaymentTime, a.EvidenceReference, a.EvidenceHash, a.OperatorNote, a.ReviewReason, a.BankSubmittedAt, a.SubmittedAt, a.ReviewedAt, a.CompletedAt, a.FailureCode, a.FailureReason, a.CreatedAt, a.UpdatedAt}
}

type RevealTicket struct {
	GrantNo   string `json:"grantNo"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

// This DTO is returned only by the single-use reveal capability, never a detail.
type RevealedAccount struct {
	TargetNo          string `json:"targetNo"`
	AccountName       string `json:"accountName"`
	Account           string `json:"account"`
	Method            string `json:"method"`
	Currency          string `json:"currency"`
	BankCode          string `json:"bankCode,omitempty"`
	InstitutionName   string `json:"institutionName,omitempty"`
	AppID             string `json:"appId,omitempty"`
	IdentifierType    string `json:"identifierType,omitempty"`
	ClearAfterSeconds int    `json:"clearAfterSeconds"`
}
type Reconciliation struct {
	PayoutNo   string   `json:"payoutNo"`
	Status     string   `json:"status"`
	Consistent bool     `json:"consistent"`
	Issues     []string `json:"issues"`
	CheckedAt  int64    `json:"checkedAt"`
}
type EvidenceInfo struct {
	EvidenceNo string `json:"evidenceNo"`
	AttemptNo  string `json:"attemptNo"`
	SHA256     string `json:"sha256"`
	MIME       string `json:"mime"`
	Size       int64  `json:"size,string"`
	CreatedBy  uint64 `json:"createdBy,string"`
	CreatedAt  int64  `json:"createdAt"`
}
