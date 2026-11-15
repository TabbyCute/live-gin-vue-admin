package request

type StartAudit struct {
	RequestID string `json:"requestId"`
	AnchorNo  string `json:"anchorNo"`
	Currency  string `json:"currency"`
	RetryOf   string `json:"retryOf"`
}

type CancelAudit struct {
	JobNo string `json:"jobNo"`
}
