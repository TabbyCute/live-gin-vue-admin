package gift

import (
	"gorm.io/gorm"
	em "tb_live_module/model/earning"
	gm "tb_live_module/model/gift"
	res "tb_live_module/model/gift/response"
	wm "tb_live_module/model/wallet"
	es "tb_live_module/service/earning"
)

const maxProofRows = 20000

type walletProof struct {
	transactions map[string][]wm.WalletTransaction
	ledgers      []wm.WalletLedger
	ledgerByNo   map[string]wm.WalletLedger
	accounts     map[uint64]wm.WalletAccount
}

func loadWalletProof(tx *gorm.DB, orders []gm.Order) (walletProof, error) {
	b := walletProof{ledgerByNo: map[string]wm.WalletLedger{}, transactions: map[string][]wm.WalletTransaction{}, accounts: map[uint64]wm.WalletAccount{}}
	nos := []string{}
	txnNos := []string{}
	users := []uint64{}
	byWallet := map[string][]string{}
	for _, o := range orders {
		nos = append(nos, o.OrderNo)
		txnNos = append(txnNos, o.WalletTransactionNo)
		users = append(users, o.SenderUserID)
		byWallet[o.WalletTransactionNo] = append(byWallet[o.WalletTransactionNo], o.OrderNo)
	}
	var rows []wm.WalletTransaction
	if e := tx.Where("(kind='debit' AND business_type='gift' AND business_no IN ?) OR transaction_no IN ? OR original_no IN ?", nos, txnNos, txnNos).Limit(maxProofRows + 1).Find(&rows).Error; e != nil {
		return b, e
	}
	if len(rows) > maxProofRows {
		return b, es.ErrLimit
	}
	ledgers := []string{}
	for _, r := range rows {
		ledgers = append(ledgers, r.LedgerNo)
		targets := map[string]bool{}
		if r.Kind == "debit" && r.BusinessType == "gift" {
			targets[r.BusinessNo] = true
		}
		for _, no := range byWallet[r.TransactionNo] {
			targets[no] = true
		}
		for _, no := range byWallet[r.OriginalNo] {
			targets[no] = true
		}
		for no := range targets {
			b.transactions[no] = append(b.transactions[no], r)
		}
	}
	if len(ledgers) > 0 {
		if e := tx.Where("ledger_no IN ?", ledgers).Find(&b.ledgers).Error; e != nil {
			return b, e
		}
	}
	for _, l := range b.ledgers {
		b.ledgerByNo[l.LedgerNo] = l
	}
	var accounts []wm.WalletAccount
	if e := tx.Where("account_id IN ? AND currency_code='COIN'", users).Find(&accounts).Error; e != nil {
		return b, e
	}
	for _, a := range accounts {
		b.accounts[a.AccountID] = a
	}
	return b, nil
}

// VerifyIncomeSources performs bounded batch reads; it is injected into earning to avoid a service cycle.
func (s *Service) VerifyIncomeSources(tx *gorm.DB, incomes []em.Income) error {
	if len(incomes) > maxProofRows {
		return es.ErrLimit
	}
	numbers := []string{}
	for _, f := range incomes {
		if f.SourceType != "gift" {
			return es.ErrSource
		}
		numbers = append(numbers, f.SourceEntryNo)
	}
	var earnings []em.Ledger
	if e := tx.Where("earning_no IN ?", numbers).Find(&earnings).Error; e != nil {
		return e
	}
	if len(earnings) != len(incomes) {
		return es.ErrInvariant
	}
	byEarning := map[string]em.Ledger{}
	orderSet := map[string]bool{}
	for _, l := range earnings {
		byEarning[l.EarningNo] = l
		orderSet[l.GiftOrderNo] = true
	}
	orders := []string{}
	for n := range orderSet {
		orders = append(orders, n)
	}
	var os []gm.Order
	if e := tx.Where("order_no IN ?", orders).Find(&os).Error; e != nil {
		return e
	}
	if len(os) != len(orders) {
		return es.ErrInvariant
	}
	var refunds []gm.Refund
	var revenues []gm.Revenue
	var allEarnings []em.Ledger
	var events []gm.Event
	for _, dest := range []any{&refunds, &revenues, &allEarnings, &events} {
		if e := tx.Where("gift_order_no IN ?", orders).Order("id ASC").Limit(maxProofRows + 1).Find(dest).Error; e != nil {
			return e
		}
	}
	if len(refunds) > maxProofRows || len(revenues) > maxProofRows || len(allEarnings) > maxProofRows || len(events) > maxProofRows {
		return es.ErrLimit
	}
	details := map[string]*res.Detail{}
	for _, o := range os {
		details[o.OrderNo] = &res.Detail{Order: o}
	}
	refundByNo := map[string]gm.Refund{}
	for _, r := range refunds {
		d := details[r.GiftOrderNo]
		d.Refunds = append(d.Refunds, r)
		refundByNo[r.RefundNo] = r
	}
	for _, r := range revenues {
		d := details[r.GiftOrderNo]
		d.Revenue = append(d.Revenue, r)
	}
	for _, r := range allEarnings {
		d := details[r.GiftOrderNo]
		d.Earnings = append(d.Earnings, r)
	}
	for _, r := range events {
		d := details[r.GiftOrderNo]
		d.Events = append(d.Events, r)
	}
	b, e := loadWalletProof(tx, os)
	if e != nil {
		return e
	}
	for _, d := range details {
		if e = checkOrderFactsWithProof(d, b); e != nil {
			return e
		}
		if !d.Consistent {
			return es.ErrInvariant
		}
	}
	var originals []em.Income
	if e := tx.Where("source_type='gift' AND source_no IN ? AND entry_type='income'", orders).Find(&originals).Error; e != nil {
		return e
	}
	originalByOrder := map[string]em.Income{}
	for _, f := range originals {
		if _, exists := originalByOrder[f.SourceNo]; exists {
			return es.ErrInvariant
		}
		originalByOrder[f.SourceNo] = f
	}
	for _, f := range incomes {
		l := byEarning[f.SourceEntryNo]
		d := details[l.GiftOrderNo]
		gross := d.Order.GrossCoinAmount
		kind := "income"
		originalNo := ""
		if l.EntryType == "reversal" {
			gross = -refundByNo[l.BusinessNo].RefundCoinAmount
			kind = "reversal"
			original, ok := originalByOrder[l.GiftOrderNo]
			if !ok || original.AnchorID != f.AnchorID {
				return es.ErrInvariant
			}
			originalNo = original.IncomeNo
		}
		if f.OriginalIncomeNo != originalNo || f.CreatedAt != l.CreatedAt || f.AnchorID != l.AnchorID || f.SourceNo != l.GiftOrderNo || f.Amount != l.EarningCoinAmount || f.GrossAmount != gross || f.RuleNo != l.RevenueRuleNo || f.ShareBPS != l.ShareBPS || f.Unit != "COIN" || f.SourceHash != es.GiftSourceHash(l) || f.EntryType != kind {
			return es.ErrInvariant
		}
	}
	return nil
}
