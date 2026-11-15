package gift

import (
	"context"
	"gorm.io/gorm"
	em "tb_live_module/model/earning"
	gm "tb_live_module/model/gift"
	res "tb_live_module/model/gift/response"
	wm "tb_live_module/model/wallet"
	es "tb_live_module/service/earning"
)

const maxProofRows = 20000

type incomeProofCacheKey struct{}
type incomeProofCache struct{ expected map[string]em.Income }

// WithIncomeProofCache MUST only wrap one repeatable-read read-only transaction.
// It cannot be reused by another transaction or any mutating financial path.
// Cache misses still perform the complete gift, wallet, refund and event proof.
func WithIncomeProofCache(tx *gorm.DB) *gorm.DB {
	return tx.WithContext(context.WithValue(tx.Statement.Context, incomeProofCacheKey{}, &incomeProofCache{expected: map[string]em.Income{}}))
}

func sameIncomeSource(f, want em.Income) bool {
	return f.SourceType == "gift" && f.SourceEntryNo == want.SourceEntryNo && f.OriginalIncomeNo == want.OriginalIncomeNo && f.CreatedAt == want.CreatedAt && f.AnchorID == want.AnchorID && f.SourceNo == want.SourceNo && f.Amount == want.Amount && f.GrossAmount == want.GrossAmount && f.RuleNo == want.RuleNo && f.ShareBPS == want.ShareBPS && f.Unit == want.Unit && f.SourceHash == want.SourceHash && f.EntryType == want.EntryType
}

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
	cache, _ := tx.Statement.Context.Value(incomeProofCacheKey{}).(*incomeProofCache)
	if cache != nil {
		missing := make([]em.Income, 0, len(incomes))
		for _, f := range incomes {
			want, ok := cache.expected[f.SourceEntryNo]
			if !ok {
				missing = append(missing, f)
				continue
			}
			if !sameIncomeSource(f, want) {
				return es.ErrInvariant
			}
		}
		incomes = missing
		if len(incomes) == 0 {
			return nil
		}
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
	orderSet := map[string]bool{}
	for _, l := range earnings {
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
	expected := map[string]em.Income{}
	for _, l := range allEarnings {
		d := details[l.GiftOrderNo]
		gross, kind, originalNo := d.Order.GrossCoinAmount, "income", ""
		if l.EntryType == "reversal" {
			gross, kind = -refundByNo[l.BusinessNo].RefundCoinAmount, "reversal"
			original, ok := originalByOrder[l.GiftOrderNo]
			if !ok || original.AnchorID != l.AnchorID {
				return es.ErrInvariant
			}
			originalNo = original.IncomeNo
		}
		expected[l.EarningNo] = em.Income{SourceType: "gift", SourceEntryNo: l.EarningNo, SourceNo: l.GiftOrderNo, AnchorID: l.AnchorID, OriginalIncomeNo: originalNo, CreatedAt: l.CreatedAt, Amount: l.EarningCoinAmount, GrossAmount: gross, RuleNo: l.RevenueRuleNo, ShareBPS: l.ShareBPS, Unit: "COIN", SourceHash: es.GiftSourceHash(l), EntryType: kind}
	}
	for _, f := range incomes {
		if !sameIncomeSource(f, expected[f.SourceEntryNo]) {
			return es.ErrInvariant
		}
	}
	if cache != nil {
		if len(cache.expected)+len(expected) > maxProofRows {
			cache.expected = map[string]em.Income{}
		}
		for no, f := range expected {
			cache.expected[no] = f
		}
	}
	return nil
}
