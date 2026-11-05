package response

// GiftSummary amounts remain decimal strings, including aggregate values larger than int64.
type GiftSummary struct {
	GrossCoin          string `json:"grossCoin"`
	RefundCoin         string `json:"refundCoin"`
	NetCoin            string `json:"netCoin"`
	PendingEarningCoin string `json:"pendingEarningCoin"`
	Quantity           string `json:"quantity"`
	GrossQuantity      string `json:"grossQuantity"`
	RefundQuantity     string `json:"refundQuantity"`
	Orders             string `json:"orders"`
	Users              string `json:"users"`
}
