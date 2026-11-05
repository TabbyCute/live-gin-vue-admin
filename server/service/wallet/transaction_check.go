package wallet

import (
	"gorm.io/gorm"
	"strconv"
	model "tb_live_module/model/wallet"
	res "tb_live_module/model/wallet/response"
)

// 独立扫描消费事实，发现交易丢失流水。各关联按批查询，避免每笔交易单独访问数据库。
func checkTransactions(tx *gorm.DB, w model.WalletAccount, after uint64, limit int, result *res.Check) error {
	result.NextTransactionID = strconv.FormatUint(after, 10)
	var rows []model.WalletTransaction
	if e := tx.Where("account_id = ? AND currency_code = ? AND id > ?", w.AccountID, w.CurrencyCode, after).Order("id").Limit(limit + 1).Find(&rows).Error; e != nil {
		return e
	}
	if len(rows) > limit {
		result.HasMore = true
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		return nil
	}
	nos := []string{}
	originals := []string{}
	for _, t := range rows {
		nos = append(nos, t.TransactionNo)
		if t.Kind == model.TransactionRefund {
			originals = append(originals, t.OriginalNo)
		} else {
			originals = append(originals, t.TransactionNo)
		}
	}
	var ls []model.WalletLedger
	if e := tx.Where("business_type IN ? AND business_no IN ?", []string{model.BusinessDebit, model.BusinessRefund}, nos).Limit(2*len(rows) + 1).Find(&ls).Error; e != nil {
		return e
	}
	ledgers := map[string][]model.WalletLedger{}
	for _, l := range ls {
		ledgers[l.BusinessNo] = append(ledgers[l.BusinessNo], l)
	}
	var logs []model.WalletAuditLog
	if e := tx.Where("object_no IN ?", nos).Order("id").Limit(len(rows) + 1).Find(&logs).Error; e != nil {
		return e
	}
	audits := map[string][]model.WalletAuditLog{}
	for _, l := range logs {
		audits[l.ObjectNo] = append(audits[l.ObjectNo], l)
	}
	var os []model.WalletTransaction
	if e := tx.Where("transaction_no IN ?", originals).Find(&os).Error; e != nil {
		return e
	}
	byNo := map[string]model.WalletTransaction{}
	for _, o := range os {
		byNo[o.TransactionNo] = o
	}
	var totals []struct {
		OriginalNo string
		Amount     int64
		Count      int64
	}
	if e := tx.Model(&model.WalletTransaction{}).Select("original_no, SUM(amount) AS amount, COUNT(*) AS count").Where("original_no IN ?", originals).Group("original_no").Scan(&totals).Error; e != nil {
		return e
	}
	amounts := map[string]int64{}
	counts := map[string]int64{}
	for _, v := range totals {
		amounts[v.OriginalNo] = v.Amount
		counts[v.OriginalNo] = v.Count
	}
	issue := func(code, no string) { result.Consistent = false; result.Issues = append(result.Issues, code+":"+no) }
	for _, t := range rows {
		result.CheckedTransactions++
		result.NextTransactionID = strconv.FormatUint(t.ID, 10)
		ls := ledgers[t.TransactionNo]
		as := audits[t.TransactionNo]
		if len(ls) != 1 || !validTransactionLedger(t, ls[0], w) {
			issue("transaction_ledger", t.TransactionNo)
			continue
		}
		l := ls[0]
		if len(as) != 1 || as[0].Action != t.Kind || as[0].OperatorID != l.OperatorID || as[0].OperatorIP != t.ActorIP || as[0].Reason != t.Reason || as[0].CreatedAt != t.CreatedAt {
			issue("transaction_audit", t.TransactionNo)
		}
		o := t
		if t.Kind == model.TransactionRefund {
			o = byNo[t.OriginalNo]
		}
		if o.Kind != model.TransactionDebit || o.AccountID != t.AccountID || o.CurrencyCode != t.CurrencyCode || o.BusinessType != t.BusinessType || o.Amount <= 0 || o.RefundedAmount != amounts[o.TransactionNo] || o.RefundedAmount < 0 || o.RefundedAmount > o.Amount || counts[o.TransactionNo] > maxRefundParts || (t.Kind == model.TransactionRefund && (l.OriginalLedgerNo != o.LedgerNo || o.RefundedAmount < t.Amount)) || (t.Kind == model.TransactionDebit && l.OriginalLedgerNo != "") {
			issue("transaction_refund", t.TransactionNo)
		}
	}
	return nil
}
