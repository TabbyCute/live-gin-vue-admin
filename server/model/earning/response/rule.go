package response

// Overview 只表示COIN待结算收益，不表示法币或可提现余额。
type Overview struct {
	PendingEarningCoin string `json:"pendingEarningCoin"`
}
