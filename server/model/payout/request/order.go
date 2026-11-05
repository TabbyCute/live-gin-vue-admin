package request

type CreateOrder struct {
	RequestID    string `json:"requestId"`
	WithdrawalNo string `json:"withdrawalNo"`
}
type OrderSearch struct {
	ChannelAccountNo string `form:"channelAccountNo"`
	OperatorID       string `form:"operatorId"`
	ReviewerID       string `form:"reviewerId"`
	PaymentReference string `form:"paymentReference"`
	Page             int    `form:"page"`
	PageSize         int    `form:"pageSize"`
	PayoutNo         string `form:"payoutNo"`
	WithdrawalNo     string `form:"withdrawalNo"`
	AnchorNo         string `form:"anchorNo"`
	Currency         string `form:"currency"`
	PayeeMethod      string `form:"payeeMethod"`
	Status           string `form:"status"`
	From             int64  `form:"from"`
	To               int64  `form:"to"`
}
type OrderDetail struct {
	PayoutNo string `form:"payoutNo"`
}
