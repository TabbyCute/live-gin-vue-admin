package live

import (
	"fmt"
	"gorm.io/gorm"
	"tb_live_module/global"
	model "tb_live_module/model/gift"
	res "tb_live_module/model/live/response"
)

func giftSummary(db *gorm.DB, column string, id uint) (*res.GiftSummary, error) {
	rows, e := giftSummaries(db, column, []uint{id})
	if e != nil {
		return nil, e
	}
	return rows[id], nil
}

// Aggregate once per page; the old SRS fields are never used as a fallback.
func giftSummaries(db *gorm.DB, column string, ids []uint) (map[uint]*res.GiftSummary, error) {
	result := map[uint]*res.GiftSummary{}
	if !global.GiftFactsReady.Load() || len(ids) == 0 {
		return result, nil
	}
	if column != "anchor_id" && column != "room_id" && column != "session_id" {
		return nil, fmt.Errorf("invalid gift summary scope")
	}
	div := "/"
	if db.Dialector.Name() == "mysql" {
		div = " DIV "
	}
	net := "(gross_coin_amount-refunded_coin_amount)"
	earn := "((" + net + div + "10000)*anchor_share_bps+(" + net + "%10000)*anchor_share_bps" + div + "10000)"
	var rows []struct {
		ScopeID uint
		res.GiftSummary
	}
	e := db.Model(&model.Order{}).Where(column+" IN ?", ids).Select(column + " AS scope_id, CAST(SUM(gross_coin_amount) AS CHAR) AS gross_coin, CAST(SUM(refunded_coin_amount) AS CHAR) AS refund_coin, CAST(SUM(" + net + ") AS CHAR) AS net_coin, CAST(SUM(" + earn + ") AS CHAR) AS pending_earning_coin, CAST(SUM(quantity) AS CHAR) AS gross_quantity, CAST(SUM(refunded_coin_amount" + div + "unit_price) AS CHAR) AS refund_quantity, CAST(SUM(quantity-refunded_coin_amount" + div + "unit_price) AS CHAR) AS quantity, CAST(COUNT(*) AS CHAR) AS orders, CAST(COUNT(DISTINCT CASE WHEN gross_coin_amount>refunded_coin_amount THEN sender_user_id END) AS CHAR) AS users").Group(column).Scan(&rows).Error
	if e != nil {
		return nil, e
	}
	for _, id := range ids {
		result[id] = &res.GiftSummary{GrossCoin: "0", RefundCoin: "0", NetCoin: "0", PendingEarningCoin: "0", Quantity: "0", GrossQuantity: "0", RefundQuantity: "0", Orders: "0", Users: "0"}
	}
	for _, row := range rows {
		r := row.GiftSummary
		result[row.ScopeID] = &r
	}
	return result, nil
}

func authorizedGiftSummary(db *gorm.DB, column string, id uint, allowed []bool) (*res.GiftSummary, error) {
	if len(allowed) != 1 || !allowed[0] {
		return nil, nil
	}
	return giftSummary(db, column, id)
}
func authorizedGiftSummaries(db *gorm.DB, column string, ids []uint, allowed []bool) (map[uint]*res.GiftSummary, error) {
	if len(allowed) != 1 || !allowed[0] {
		return map[uint]*res.GiftSummary{}, nil
	}
	return giftSummaries(db, column, ids)
}
