package earning

import (
	"errors"
	"gorm.io/gorm"
	"math"
	"sort"
	model "tb_live_module/model/earning"
	"tb_live_module/model/system"
)

func loadAccount(tx *gorm.DB, anchor uint64, no, currency string, now int64, create bool) (model.Account, error) {
	var a model.Account
	e := tx.Where("anchor_id=? AND currency=?", anchor, currency).Take(&a).Error
	if errors.Is(e, gorm.ErrRecordNotFound) && create {
		a = model.Account{AnchorID: anchor, AnchorNo: no, Currency: currency, CreatedAt: now, UpdatedAt: now}
		e = tx.Create(&a).Error
	}
	return a, e
}
func move(a model.Account, kind string, amount int64) (model.Account, error) {
	if a.Available < 0 || a.Frozen < 0 || a.Debt < 0 || a.Paid < 0 || (a.Debt > 0 && a.Available > 0) || a.Version < 0 || a.Version == math.MaxInt64 {
		return a, ErrInvariant
	}
	var e error
	switch kind {
	case "settlement_credit":
		if amount < 0 {
			return a, ErrInvalid
		}
		v := amount
		if v <= a.Debt {
			a.Debt -= v
		} else {
			v -= a.Debt
			a.Debt = 0
			a.Available, e = add(a.Available, v)
		}
	case "negative_adjustment":
		if amount >= 0 || amount == math.MinInt64 {
			return a, ErrInvalid
		}
		v := -amount
		if v <= a.Available {
			a.Available -= v
		} else {
			v -= a.Available
			a.Available = 0
			a.Debt, e = add(a.Debt, v)
		}
	case "withdrawal_freeze":
		if amount <= 0 || a.Debt != 0 || amount > a.Available {
			return a, ErrInsufficient
		}
		a.Available -= amount
		a.Frozen, e = add(a.Frozen, amount)
	case "withdrawal_release":
		if amount <= 0 || amount > a.Frozen {
			return a, ErrInvariant
		}
		a.Frozen -= amount
		v := amount
		if v <= a.Debt {
			a.Debt -= v
		} else {
			v -= a.Debt
			a.Debt = 0
			a.Available, e = add(a.Available, v)
		}
	case "withdrawal_paid":
		if amount <= 0 || amount > a.Frozen {
			return a, ErrInvariant
		}
		a.Frozen -= amount
		a.Paid, e = add(a.Paid, amount)
	default:
		return a, ErrInvalid
	}
	if e != nil {
		return a, e
	}
	a.Version++
	return a, nil
}

// post is the sole balance mutation method. Caller holds the per-anchor financial mutex.
func post(tx *gorm.DB, a *model.Account, kind, business string, amount, now int64) error {
	next, e := move(*a, kind, amount)
	if e != nil {
		return e
	}
	row := model.AccountLedger{LedgerNo: number("al_"), AnchorID: a.AnchorID, Currency: a.Currency, Version: next.Version, EntryType: kind, BusinessNo: business, ChangeAmount: amount, AvailableBefore: a.Available, AvailableAfter: next.Available, FrozenBefore: a.Frozen, FrozenAfter: next.Frozen, DebtBefore: a.Debt, DebtAfter: next.Debt, PaidBefore: a.Paid, PaidAfter: next.Paid, CreatedAt: now}
	if e = tx.Create(&row).Error; e != nil {
		return e
	}
	result := tx.Model(&model.Account{}).Where("id=? AND version=?", a.ID, a.Version).Updates(map[string]any{"available": next.Available, "frozen": next.Frozen, "debt": next.Debt, "paid": next.Paid, "version": next.Version, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrConflict
	}
	next.UpdatedAt = now
	*a = next
	return nil
}
func (s *Service) verifyAccount(tx *gorm.DB, a model.Account, deep bool) error {
	var rows []model.AccountLedger
	if e := tx.Where("anchor_id=? AND currency=?", a.AnchorID, a.Currency).Order("version ASC").Limit(maxAuditFacts + 1).Find(&rows).Error; e != nil {
		return e
	}
	if len(rows) > maxAuditFacts {
		return ErrLimit
	}
	current := model.Account{AnchorID: a.AnchorID, Currency: a.Currency}
	byBusiness := map[string]model.AccountLedger{}
	for _, r := range rows {
		if r.Version != current.Version+1 || r.AvailableBefore != current.Available || r.FrozenBefore != current.Frozen || r.DebtBefore != current.Debt || r.PaidBefore != current.Paid {
			return ErrInvariant
		}
		next, e := move(current, r.EntryType, r.ChangeAmount)
		if e != nil || next.Available != r.AvailableAfter || next.Frozen != r.FrozenAfter || next.Debt != r.DebtAfter || next.Paid != r.PaidAfter {
			return ErrInvariant
		}
		current = next
		key := r.EntryType + ":" + r.BusinessNo
		if _, ok := byBusiness[key]; ok {
			return ErrInvariant
		}
		byBusiness[key] = r
	}
	if current.Version != a.Version || current.Available != a.Available || current.Frozen != a.Frozen || current.Debt != a.Debt || current.Paid != a.Paid {
		return ErrInvariant
	}
	var settlements []model.Settlement
	var withdrawals []model.Withdrawal
	if e := tx.Where("anchor_id=? AND currency=? AND status='approved'", a.AnchorID, a.Currency).Limit(maxAuditFacts + 1).Find(&settlements).Error; e != nil {
		return e
	}
	if e := tx.Where("anchor_id=? AND currency=?", a.AnchorID, a.Currency).Limit(maxAuditFacts + 1).Find(&withdrawals).Error; e != nil {
		return e
	}
	if len(settlements) > maxAuditFacts || len(withdrawals) > maxAuditFacts {
		return ErrLimit
	}
	expected := 0
	settlementNos := []string{}
	objectNos := []string{}
	reconNos := []string{}
	for _, st := range settlements {
		kind := "settlement_credit"
		if st.Amount < 0 {
			kind = "negative_adjustment"
		}
		l, ok := byBusiness[kind+":"+st.SettlementNo]
		if !ok || l.ChangeAmount != st.Amount || st.CreatedBy == 0 || st.ApprovedBy == 0 || (st.CreatedBy == st.ApprovedBy && !system.IsSuperAdmin(st.ApprovedBy)) {
			return ErrInvariant
		}
		expected++
		settlementNos = append(settlementNos, st.SettlementNo)
		objectNos = append(objectNos, st.SettlementNo)
	}
	for _, w := range withdrawals {
		l, ok := byBusiness["withdrawal_freeze:"+w.WithdrawalNo]
		if !ok || l.ChangeAmount != w.Amount || w.Amount <= 0 {
			return ErrInvariant
		}
		expected++
		if w.Status == "rejected" || w.Status == "cancelled" {
			l, ok = byBusiness["withdrawal_release:"+w.WithdrawalNo]
			if !ok || l.ChangeAmount != w.Amount {
				return ErrInvariant
			}
			expected++
		} else if w.Status == "paid" {
			l, ok = byBusiness["withdrawal_paid:"+w.WithdrawalNo]
			if !ok || l.ChangeAmount != w.Amount {
				return ErrInvariant
			}
			expected++
		} else if w.Status != "pending_review" && w.Status != "approved" {
			return ErrInvariant
		}
		if w.Status != "pending_review" && (w.ReviewedBy == 0 || w.ReviewedAt < w.CreatedAt) {
			return ErrInvariant
		}
		if w.Status == "approved" || w.Status == "paid" || w.Status == "cancelled" {
			if w.ReconciliationStatus != "passed" || w.ReconciliationNo == "" {
				return ErrInvariant
			}
			reconNos = append(reconNos, w.ReconciliationNo)
		}
		objectNos = append(objectNos, w.WithdrawalNo)
	}
	if expected != len(rows) {
		return ErrInvariant
	}
	if len(objectNos) > 0 {
		var audits []model.FinanceAudit
		if e := tx.Where("object_no IN ?", objectNos).Limit(3*maxAuditFacts + 1).Find(&audits).Error; e != nil {
			return e
		}
		if len(audits) > 3*maxAuditFacts {
			return ErrLimit
		}
		seen := map[string]model.FinanceAudit{}
		for _, v := range audits {
			key := v.ObjectNo + ":" + v.Action
			if _, ok := seen[key]; ok {
				return ErrInvariant
			}
			seen[key] = v
		}
		for _, st := range settlements {
			c, ok := seen[st.SettlementNo+":settlement.create"]
			r, yes := seen[st.SettlementNo+":settlement.review"]
			if !ok || !yes || c.ActorID != st.CreatedBy || r.ActorID != st.ApprovedBy || r.Snapshot != encode(st) {
				return ErrInvariant
			}
		}
		for _, w := range withdrawals {
			c, ok := seen[w.WithdrawalNo+":withdrawal.create"]
			if !ok || c.ActorType != "app" || c.ActorID != w.ApplicantUserID || w.ApplicantUserID == 0 {
				return ErrInvariant
			}
			if w.Status != "pending_review" {
				r, ok := seen[w.WithdrawalNo+":withdrawal.review"]
				reviewed := w
				if w.Status == "paid" || w.Status == "cancelled" {
					reviewed.Status = "approved"
				}
				if !ok || r.ActorID != w.ReviewedBy || r.Snapshot != encode(reviewed) {
					return ErrInvariant
				}
			}
			if w.Status == "paid" || w.Status == "cancelled" {
				terminal, ok := seen[w.WithdrawalNo+":withdrawal."+w.Status]
				if !ok || terminal.ActorID == 0 || terminal.Snapshot != encode(w) {
					return ErrInvariant
				}
				if e := verifyWithdrawalTerminal(tx, w, byBusiness); e != nil {
					return e
				}
			}
		}
	}
	if len(reconNos) > 0 {
		var proofs []model.Reconciliation
		if e := tx.Where("reconciliation_no IN ?", reconNos).Find(&proofs).Error; e != nil {
			return e
		}
		byNo := map[string]model.Reconciliation{}
		for _, p := range proofs {
			byNo[p.ReconciliationNo] = p
		}
		for _, w := range withdrawals {
			if w.Status != "approved" && w.Status != "paid" && w.Status != "cancelled" {
				continue
			}
			p, ok := byNo[w.ReconciliationNo]
			if !ok || p.WithdrawalNo != w.WithdrawalNo || p.AnchorID != w.AnchorID || p.Currency != w.Currency || p.Status != "passed" || p.SnapshotHash != hash(p.Snapshot) {
				return ErrInvariant
			}
		}
	}
	if len(settlementNos) == 0 {
		if e := verifyRounding(tx, a, nil, nil, nil, byBusiness); e != nil {
			return e
		}
	}
	if len(settlementNos) > 0 {
		var items []model.SettlementItem
		var allocations []model.Allocation
		if e := tx.Where("settlement_no IN ?", settlementNos).Limit(maxAuditFacts + 1).Find(&items).Error; e != nil {
			return e
		}
		if e := tx.Where("settlement_no IN ?", settlementNos).Limit(maxAuditFacts + 1).Find(&allocations).Error; e != nil {
			return e
		}
		if len(items) > maxAuditFacts || len(allocations) > maxAuditFacts {
			return ErrLimit
		}
		allocated := map[string]string{}
		for _, v := range allocations {
			allocated[v.IncomeNo] = v.SettlementNo
		}
		grouped := map[string][]model.SettlementItem{}
		incomeNos := []string{}
		for _, item := range items {
			if allocated[item.IncomeNo] != item.SettlementNo {
				return ErrInvariant
			}
			grouped[item.SettlementNo] = append(grouped[item.SettlementNo], item)
			incomeNos = append(incomeNos, item.IncomeNo)
		}
		if len(items) != len(allocations) {
			return ErrInvariant
		}
		var facts []model.Income
		if e := tx.Where("income_no IN ?", incomeNos).Find(&facts).Error; e != nil {
			return e
		}
		if len(facts) != len(items) {
			return ErrInvariant
		}
		byNo := map[string]model.Income{}
		for _, f := range facts {
			byNo[f.IncomeNo] = f
		}
		for _, st := range settlements {
			group := grouped[st.SettlementNo]
			if int64(len(group)) != st.ItemCount {
				return ErrInvariant
			}
			sortItems(group)
			var total int64
			for i := range group {
				v := &group[i]
				f := byNo[v.IncomeNo]
				if f.AnchorID != a.AnchorID || incomeHash(f) != v.IncomeHash || v.Currency != a.Currency {
					return ErrInvariant
				}
				var e error
				total, e = add(total, v.Amount)
				if e != nil {
					return ErrInvariant
				}
				v.ID = 0
				v.SettlementNo = ""
			}
			if total != st.Amount || hash(group) != st.ItemsHash {
				return ErrInvariant
			}
		}
		if e := verifyRounding(tx, a, settlements, items, facts, byBusiness); e != nil {
			return e
		}
		if deep {
			if e := s.verifySources(tx, facts); e != nil {
				return e
			}
		}
	}
	return nil
}

func sortItems(items []model.SettlementItem) {
	sort.Slice(items, func(i, j int) bool { return items[i].IncomeNo < items[j].IncomeNo })
}
