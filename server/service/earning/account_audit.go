package earning

import (
	"gorm.io/gorm"
	"math/big"
	model "tb_live_module/model/earning"
	"tb_live_module/model/system"
)

// auditLedgerPage bounds database reads and retained business objects, not the
// age or total size of an account. Every historical entry is still verified.
const auditLedgerPage = 128

// verifyAccount requires the anchor financial lock or a repeatable-read snapshot.
// There is deliberately no trusted persisted checkpoint: immutable facts and
// mutable gift/refund and payout proofs are all rechecked on every deep audit.
func (s *Service) verifyAccount(tx *gorm.DB, a model.Account, deep bool) error {
	return s.verifyAccountProgress(tx, a, deep, nil)
}

type AuditProgress struct {
	LedgerVersion   int64  `json:"ledgerVersion,string"`
	IncomeFacts     int64  `json:"incomeFacts,string"`
	SettlementCount int64  `json:"settlementCount,string"`
	WithdrawalCount int64  `json:"withdrawalCount,string"`
	Credits         string `json:"credits"`
	Negative        string `json:"negative"`
}

func (s *Service) verifyAccountProgress(tx *gorm.DB, a model.Account, deep bool, progress func(AuditProgress) error) error {
	current := model.Account{AnchorID: a.AnchorID, Currency: a.Currency}
	rounding := map[string]model.RoundingBalance{}
	var settlementCount, withdrawalCount, incomeFacts int64
	credits, negative := new(big.Int), new(big.Int)
	report := func() error {
		if progress == nil {
			return nil
		}
		return progress(AuditProgress{current.Version, incomeFacts, settlementCount, withdrawalCount, credits.String(), negative.String()})
	}
	for {
		if e := tx.Statement.Context.Err(); e != nil {
			return e
		}
		var rows []model.AccountLedger
		query := tx.Where("anchor_id=? AND currency=?", a.AnchorID, a.Currency)
		if current.Version > 0 {
			query = query.Where("version>?", current.Version)
		}
		if e := query.Order("version ASC").Limit(auditLedgerPage).Find(&rows).Error; e != nil {
			return e
		}
		if len(rows) == 0 {
			break
		}
		batch, e := loadAuditBatch(tx, rows)
		if e != nil {
			return e
		}
		for start := 0; start < len(rows); {
			end, count := start, int64(0)
			for end < len(rows) {
				st, ok := batch.settlements[rows[end].BusinessNo]
				if ok && (st.ItemCount <= 0 || st.ItemCount > maxSettlementFacts) {
					return ErrInvariant
				}
				if ok && count+st.ItemCount > maxSettlementFacts {
					break
				}
				if ok {
					count += st.ItemCount
				}
				end++
			}
			proof, e := loadAuditFacts(tx, batch.settlements, rows[start:end])
			if e != nil {
				return e
			}
			if deep && len(proof.allFacts) > 0 {
				if e = s.verifySources(tx, proof.allFacts); e != nil {
					return e
				}
			}
			for _, r := range rows[start:end] {
				if e := tx.Statement.Context.Err(); e != nil {
					return e
				}
				if r.Version != current.Version+1 || r.AvailableBefore != current.Available || r.FrozenBefore != current.Frozen || r.DebtBefore != current.Debt || r.PaidBefore != current.Paid {
					return ErrInvariant
				}
				next, e := move(current, r.EntryType, r.ChangeAmount)
				if e != nil || next.Available != r.AvailableAfter || next.Frozen != r.FrozenAfter || next.Debt != r.DebtAfter || next.Paid != r.PaidAfter {
					return ErrInvariant
				}
				switch r.EntryType {
				case "settlement_credit", "negative_adjustment":
					st, ok := batch.settlements[r.BusinessNo]
					if !ok || st.AnchorID != a.AnchorID || st.Currency != a.Currency || st.Status != "approved" || st.Amount != r.ChangeAmount || (st.Amount < 0) != (r.EntryType == "negative_adjustment") || st.CreatedBy == 0 || st.ApprovedBy == 0 || (st.CreatedBy == st.ApprovedBy && !system.IsSuperAdmin(st.ApprovedBy)) {
						return ErrInvariant
					}
					c, yes := batch.audits[st.SettlementNo+":settlement.create"]
					review, reviewed := batch.audits[st.SettlementNo+":settlement.review"]
					if !yes || !reviewed || c.ActorID != st.CreatedBy || review.ActorID != st.ApprovedBy || review.Snapshot != encode(st) {
						return ErrInvariant
					}
					items, facts := proof.items[st.SettlementNo], proof.facts[st.SettlementNo]
					if e = replayRounding(tx, a, st, r.Version, items, facts, rounding); e != nil {
						return e
					}
					settlementCount++
					incomeFacts += int64(len(facts))
					if st.Amount >= 0 {
						credits.Add(credits, big.NewInt(st.Amount))
					} else {
						negative.Add(negative, big.NewInt(st.Amount))
					}
				case "withdrawal_freeze", "withdrawal_release", "withdrawal_paid":
					w, ok := batch.withdrawals[r.BusinessNo]
					if !ok || w.AnchorID != a.AnchorID || w.Currency != a.Currency || w.Amount <= 0 || w.Amount != r.ChangeAmount {
						return ErrInvariant
					}
					if r.EntryType == "withdrawal_freeze" {
						if e := verifyAuditWithdrawal(w, batch); e != nil {
							return e
						}
						withdrawalCount++
					} else if (r.EntryType == "withdrawal_paid" && w.Status != "paid") || (r.EntryType == "withdrawal_release" && w.Status != "rejected" && w.Status != "cancelled") {
						return ErrInvariant
					}
				default:
					return ErrInvariant
				}
				current = next
			}
			if e := report(); e != nil {
				return e
			}
			start = end
		}
	}
	if current.Version != a.Version || current.Available != a.Available || current.Frozen != a.Frozen || current.Debt != a.Debt || current.Paid != a.Paid {
		return ErrInvariant
	}
	// Counts detect business records with missing ledger entries. Unique business
	// and version constraints plus per-entry lookups detect duplicates and orphans.
	for _, check := range []struct {
		model any
		where string
		want  int64
	}{{&model.Settlement{}, "anchor_id=? AND currency=? AND status='approved'", settlementCount}, {&model.Withdrawal{}, "anchor_id=? AND currency=?", withdrawalCount}} {
		var count int64
		if e := tx.Model(check.model).Where(check.where, a.AnchorID, a.Currency).Count(&count).Error; e != nil {
			return e
		}
		if count != check.want {
			return ErrInvariant
		}
	}
	if e := verifyRoundingStates(tx, a, rounding); e != nil {
		return e
	}
	return report()
}

type auditBatch struct {
	settlements map[string]model.Settlement
	withdrawals map[string]model.Withdrawal
	audits      map[string]model.FinanceAudit
	ledgers     map[string]model.AccountLedger
	proofs      map[string]model.Reconciliation
	terminal    auditTerminal
}

func loadAuditBatch(tx *gorm.DB, rows []model.AccountLedger) (auditBatch, error) {
	b := auditBatch{settlements: map[string]model.Settlement{}, withdrawals: map[string]model.Withdrawal{}, audits: map[string]model.FinanceAudit{}, ledgers: map[string]model.AccountLedger{}, proofs: map[string]model.Reconciliation{}}
	nos, settlementNos, withdrawalNos := []string{}, []string{}, []string{}
	for _, r := range rows {
		nos = append(nos, r.BusinessNo)
		if r.EntryType == "settlement_credit" || r.EntryType == "negative_adjustment" {
			settlementNos = append(settlementNos, r.BusinessNo)
		} else {
			withdrawalNos = append(withdrawalNos, r.BusinessNo)
		}
	}
	if len(settlementNos) > 0 {
		var st []model.Settlement
		if e := tx.Where("settlement_no IN ?", settlementNos).Find(&st).Error; e != nil {
			return b, e
		}
		for _, v := range st {
			b.settlements[v.SettlementNo] = v
		}
	}
	if len(withdrawalNos) > 0 {
		var ws []model.Withdrawal
		if e := tx.Where("withdrawal_no IN ?", withdrawalNos).Find(&ws).Error; e != nil {
			return b, e
		}
		proofNos := []string{}
		for _, v := range ws {
			b.withdrawals[v.WithdrawalNo] = v
			if v.Status == "approved" || v.Status == "paid" || v.Status == "cancelled" {
				proofNos = append(proofNos, v.ReconciliationNo)
			}
		}
		if len(proofNos) > 0 {
			var proofs []model.Reconciliation
			if e := tx.Where("reconciliation_no IN ?", proofNos).Find(&proofs).Error; e != nil {
				return b, e
			}
			for _, p := range proofs {
				b.proofs[p.ReconciliationNo] = p
			}
		}
		var e error
		b.terminal, e = loadAuditTerminal(tx, ws)
		if e != nil {
			return b, e
		}
		var ls []model.AccountLedger
		if e := tx.Where("entry_type IN ? AND business_no IN ?", []string{"withdrawal_freeze", "withdrawal_release", "withdrawal_paid"}, withdrawalNos).Limit(3*auditLedgerPage + 1).Find(&ls).Error; e != nil {
			return b, e
		}
		if len(ls) > 3*auditLedgerPage {
			return b, ErrInvariant
		}
		for _, v := range ls {
			key := v.EntryType + ":" + v.BusinessNo
			if _, ok := b.ledgers[key]; ok {
				return b, ErrInvariant
			}
			b.ledgers[key] = v
		}
	}
	var audits []model.FinanceAudit
	if e := tx.Where("object_no IN ?", nos).Limit(4*auditLedgerPage + 1).Find(&audits).Error; e != nil {
		return b, e
	}
	if len(audits) > 4*auditLedgerPage {
		return b, ErrLimit
	}
	for _, v := range audits {
		key := v.ObjectNo + ":" + v.Action
		if _, ok := b.audits[key]; ok {
			return b, ErrInvariant
		}
		b.audits[key] = v
	}
	return b, nil
}

func verifyAuditWithdrawal(w model.Withdrawal, b auditBatch) error {
	freeze, ok := b.ledgers["withdrawal_freeze:"+w.WithdrawalNo]
	if !ok || freeze.AnchorID != w.AnchorID || freeze.Currency != w.Currency || freeze.ChangeAmount != w.Amount {
		return ErrInvariant
	}
	terminal := ""
	switch w.Status {
	case "rejected", "cancelled":
		terminal = "withdrawal_release"
	case "paid":
		terminal = "withdrawal_paid"
	case "pending_review", "approved":
	default:
		return ErrInvariant
	}
	for _, kind := range []string{"withdrawal_release", "withdrawal_paid"} {
		l, exists := b.ledgers[kind+":"+w.WithdrawalNo]
		if exists != (terminal == kind) {
			return ErrInvariant
		}
		if exists && (l.ChangeAmount != w.Amount || l.AnchorID != w.AnchorID || l.Currency != w.Currency || l.Version <= freeze.Version) {
			return ErrInvariant
		}
	}
	if w.Status != "pending_review" && (w.ReviewedBy == 0 || w.ReviewedAt < w.CreatedAt) {
		return ErrInvariant
	}
	c, ok := b.audits[w.WithdrawalNo+":withdrawal.create"]
	if !ok || c.ActorType != "app" || c.ActorID != w.ApplicantUserID || w.ApplicantUserID == 0 {
		return ErrInvariant
	}
	if w.Status != "pending_review" {
		r, ok := b.audits[w.WithdrawalNo+":withdrawal.review"]
		reviewed := w
		if w.Status == "paid" || w.Status == "cancelled" {
			reviewed.Status = "approved"
		}
		if !ok || r.ActorID != w.ReviewedBy || r.Snapshot != encode(reviewed) {
			return ErrInvariant
		}
	}
	if w.Status == "approved" || w.Status == "paid" || w.Status == "cancelled" {
		if w.ReconciliationStatus != "passed" || w.ReconciliationNo == "" {
			return ErrInvariant
		}
		p, ok := b.proofs[w.ReconciliationNo]
		if !ok {
			return ErrInvariant
		}
		if p.WithdrawalNo != w.WithdrawalNo || p.AnchorID != w.AnchorID || p.Currency != w.Currency || p.Status != "passed" || p.SnapshotHash != hash(p.Snapshot) {
			return ErrInvariant
		}
	}
	if w.Status == "paid" || w.Status == "cancelled" {
		v, ok := b.audits[w.WithdrawalNo+":withdrawal."+w.Status]
		if !ok || v.ActorID == 0 || v.Snapshot != encode(w) {
			return ErrInvariant
		}
		return verifyAuditTerminal(w, b)
	}
	return nil
}
