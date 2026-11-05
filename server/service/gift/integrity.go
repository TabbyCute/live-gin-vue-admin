package gift

import (
	"fmt"
	"gorm.io/gorm"
	model "tb_live_module/model/gift"
	res "tb_live_module/model/gift/response"
	wallet "tb_live_module/model/wallet"
	"tb_live_module/service/earning"
)

// Each order has at most 10,000 units and therefore at most 10,000 refunds.
// A fixed number of indexed batch reads avoids per-refund database round trips.
func loadOrderFacts(tx *gorm.DB, d *res.Detail) error {
	for _, dest := range []any{&d.Refunds, &d.Revenue, &d.Earnings, &d.Events} {
		if e := tx.Where("gift_order_no=?", d.Order.OrderNo).Order("id ASC").Limit(10002).Find(dest).Error; e != nil {
			return e
		}
	}
	return nil
}

func validateOrderFacts(tx *gorm.DB, o model.Order) error {
	d := res.Detail{Order: o}
	if e := loadOrderFacts(tx, &d); e != nil {
		return e
	}
	if e := checkOrderFacts(tx, &d); e != nil {
		return e
	}
	if !d.Consistent {
		return ErrInvariant
	}
	return nil
}

func checkOrderFacts(tx *gorm.DB, d *res.Detail) error {
	bundle, e := loadWalletProof(tx, []model.Order{d.Order})
	if e != nil {
		return e
	}
	return checkOrderFactsWithProof(d, bundle)
}
func checkOrderFactsWithProof(d *res.Detail, bundle walletProof) error {
	o := d.Order
	d.Consistent = true
	d.Issues = []string{}
	bad := func(no, issue string) {
		d.Consistent = false
		if len(d.Issues) < 100 {
			d.Issues = append(d.Issues, fmt.Sprintf("%s: %s", no, issue))
		}
	}
	if o.UnitPrice <= 0 || o.Quantity < 1 || o.Quantity > 10000 || o.AnchorShareBPS < 0 || o.AnchorShareBPS > 10000 || o.GrossCoinAmount <= 0 || o.RefundedCoinAmount < 0 || o.RefundedCoinAmount > o.GrossCoinAmount {
		bad(o.OrderNo, "订单金额或比例无效")
		return nil
	}
	amount, e := multiply(o.UnitPrice, o.Quantity)
	if e != nil || amount != o.GrossCoinAmount {
		bad(o.OrderNo, "订单金额与数量不符")
	}
	if len(d.Refunds) > int(o.Quantity) || len(d.Revenue) != len(d.Refunds)+1 || len(d.Earnings) != len(d.Refunds)+1 {
		bad(o.OrderNo, "退款、收入或收益笔数不一致")
	}
	if len(d.Events) != 1 || d.Events[0].EventID != o.OrderNo || d.Events[0].EventType != "gift.sent" || d.Events[0].RoomNo != o.RoomNo || d.Events[0].SessionNo != o.SessionNo {
		bad(o.OrderNo, "广播事件关联不一致")
	}
	transactions := bundle.transactions[o.OrderNo]
	account, exists := bundle.accounts[o.SenderUserID]
	if !exists {
		bad(o.OrderNo, "钱包账户缺失")
	}
	if len(transactions) != len(d.Refunds)+1 {
		bad(o.OrderNo, "钱包交易笔数不一致")
	}
	txByBusiness := map[string]wallet.WalletTransaction{}
	ledgerByNo := bundle.ledgerByNo
	for _, t := range transactions {
		if _, ok := txByBusiness[t.BusinessNo]; ok {
			bad(t.BusinessNo, "重复钱包业务关联")
		}
		txByBusiness[t.BusinessNo] = t
	}
	revenueByBusiness := map[string]model.Revenue{}
	for _, r := range d.Revenue {
		revenueByBusiness[r.BusinessNo] = r
	}
	earningByBusiness := map[string]int{}
	for i, r := range d.Earnings {
		earningByBusiness[r.BusinessNo] = i
	}
	check := func(business, transactionNo, ledgerNo, reversalNo string, amount, quantity, earned int64, refund bool) {
		kind, entry, earningEntry, ledgerType, original, originalLedger := wallet.TransactionDebit, "gift", "earning", wallet.BusinessDebit, "", ""
		delta := -amount
		if refund {
			kind = wallet.TransactionRefund
			entry = "refund"
			earningEntry = "reversal"
			ledgerType = wallet.BusinessRefund
			original = o.WalletTransactionNo
			originalLedger = o.WalletLedgerNo
			delta = amount
		}
		t, ok := txByBusiness[business]
		if !ok || t.TransactionNo != transactionNo || t.LedgerNo != ledgerNo || t.Kind != kind || t.BusinessType != "gift" || t.BusinessNo != business || t.OriginalNo != original || t.AccountID != o.SenderUserID || t.CurrencyCode != wallet.CurrencyCOIN || t.Amount != amount {
			bad(business, "钱包交易逐笔关联不一致")
		}
		if (!refund && t.RefundedAmount != o.RefundedCoinAmount) || (refund && t.RefundedAmount != 0) {
			bad(business, "钱包累计退款不一致")
		}
		l, ok := ledgerByNo[ledgerNo]
		if !ok || l.BusinessNo != transactionNo || l.BusinessType != ledgerType || l.WalletAccountID != account.ID || l.ChangeAmount != delta || l.OriginalLedgerNo != originalLedger || l.CreatedAt != t.CreatedAt {
			bad(business, "钱包流水逐笔关联不一致")
		}
		r, ok := revenueByBusiness[business]
		revenueAmount := amount
		if refund {
			revenueAmount = -amount
		}
		if !ok || r.GiftOrderNo != o.OrderNo || r.EntryType != entry || r.GrossCoinAmount != revenueAmount || r.Quantity != quantity {
			bad(business, "收入事实逐笔关联不一致")
		}
		i, ok := earningByBusiness[business]
		if !ok {
			bad(business, "缺少收益事实")
		} else {
			v := d.Earnings[i]
			if v.GiftOrderNo != o.OrderNo || v.EntryType != earningEntry || v.AnchorID != o.AnchorID || v.RevenueRuleNo != o.RevenueRuleNo || v.ShareBPS != o.AnchorShareBPS || v.EarningCoinAmount != earned || v.Status != "pending" || (refund && v.EarningNo != reversalNo) {
				bad(business, "收益冲减逐笔关联不一致")
			}
		}
	}
	initialEarning := earning.Share(o.GrossCoinAmount, o.AnchorShareBPS)
	if initialEarning != o.EstimatedAnchorEarningCoin || o.GrossCoinAmount-initialEarning != o.EstimatedPlatformIncomeCoin {
		bad(o.OrderNo, "订单分成快照不一致")
	}
	check(o.OrderNo, o.WalletTransactionNo, o.WalletLedgerNo, "", o.GrossCoinAmount, o.Quantity, initialEarning, false)
	var refunded int64
	seen := map[string]bool{}
	for _, r := range d.Refunds {
		v, e := multiply(o.UnitPrice, r.RefundQuantity)
		if e != nil || v != r.RefundCoinAmount || r.RefundCoinAmount > o.GrossCoinAmount-refunded || r.Status != "succeeded" || seen[r.RefundNo] {
			bad(r.RefundNo, "退款金额、状态或编号异常")
			continue
		}
		seen[r.RefundNo] = true
		reversal := earning.Share(o.GrossCoinAmount-refunded-r.RefundCoinAmount, o.AnchorShareBPS) - earning.Share(o.GrossCoinAmount-refunded, o.AnchorShareBPS)
		check(r.RefundNo, r.WalletRefundTransactionNo, r.WalletRefundLedgerNo, r.EarningReversalNo, r.RefundCoinAmount, -r.RefundQuantity, reversal, true)
		refunded += r.RefundCoinAmount
	}
	status := "succeeded"
	if refunded > 0 {
		status = "partial_refund"
	}
	if refunded == o.GrossCoinAmount {
		status = "refunded"
	}
	if refunded != o.RefundedCoinAmount || status != o.Status {
		bad(o.OrderNo, "订单累计退款或状态不一致")
	}
	return nil
}
