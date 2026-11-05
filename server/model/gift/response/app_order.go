package response

import model "tb_live_module/model/gift"

// AppOrder is an explicit public allowlist. orderNo is the customer-support reference.
// Never embed Order: adding an internal accounting field must not change the APP contract.
type AppOrder struct {
	OrderNo            string `json:"orderNo"`
	RequestID          string `json:"requestId"`
	AnchorNo           string `json:"anchorNo"`
	RoomNo             string `json:"roomNo"`
	SessionNo          string `json:"sessionNo"`
	CategoryNo         string `json:"categoryNo"`
	CategoryName       string `json:"categoryName"`
	GiftNo             string `json:"giftNo"`
	GiftName           string `json:"giftName"`
	IconURL            string `json:"iconUrl"`
	AnimationURL       string `json:"animationUrl"`
	AnimationType      string `json:"animationType"`
	UnitPrice          int64  `json:"unitPrice,string"`
	Quantity           int64  `json:"quantity,string"`
	GrossCoinAmount    int64  `json:"grossCoinAmount,string"`
	RefundedCoinAmount int64  `json:"refundedCoinAmount,string"`
	Status             string `json:"status"`
	CreatedAt          int64  `json:"createdAt"`
}

func PublicOrder(o model.Order) AppOrder {
	return AppOrder{OrderNo: o.OrderNo, RequestID: o.RequestID, AnchorNo: o.AnchorNo,
		RoomNo: o.RoomNo, SessionNo: o.SessionNo, CategoryNo: o.CategoryNo, CategoryName: o.CategoryName,
		GiftNo: o.GiftNo, GiftName: o.GiftName, IconURL: o.IconURL, AnimationURL: o.AnimationURL,
		AnimationType: o.AnimationType, UnitPrice: o.UnitPrice, Quantity: o.Quantity,
		GrossCoinAmount: o.GrossCoinAmount, RefundedCoinAmount: o.RefundedCoinAmount,
		Status: o.Status, CreatedAt: o.CreatedAt}
}
