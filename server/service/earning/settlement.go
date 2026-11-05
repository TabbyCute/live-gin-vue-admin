package earning

import (
	"context"
	"errors"
	"gorm.io/gorm"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	"tb_live_module/model/system"
)

const maxSettlementFacts = 1000
const maxAuditFacts = 20000

func incomeHash(f model.Income) string {
	return hash(struct {
		Fact       model.Income
		Anchor     uint64
		SourceHash string
	}{f, f.AnchorID, f.SourceHash})
}
func unallocated(tx *gorm.DB, anchor uint64) *gorm.DB {
	return tx.Model(&model.Income{}).Where("anchor_id=? AND NOT EXISTS (SELECT 1 FROM anchor_settlement_allocation a WHERE a.income_no=anchor_income_fact.income_no)", anchor)
}

// buildItems preserves fractions across batches; reversals retain their original exchange rule.
func buildItems(tx *gorm.DB, facts []model.Income, rule model.SettlementRule) ([]model.SettlementItem, int64, error) {
	if len(facts) == 0 || len(facts) > maxSettlementFacts {
		return nil, 0, ErrLimit
	}
	names := []string{}
	for _, f := range facts {
		names = append(names, f.IncomeNo)
		if f.OriginalIncomeNo != "" {
			names = append(names, f.OriginalIncomeNo)
		}
	}
	var allocated []model.SettlementItem
	if e := tx.Table("anchor_settlement_item i").Select("i.*").Joins("JOIN anchor_settlement_allocation a ON a.income_no=i.income_no AND a.settlement_no=i.settlement_no").Where("i.income_no IN ?", names).Find(&allocated).Error; e != nil {
		return nil, 0, e
	}
	byIncome := map[string]model.SettlementItem{}
	for _, item := range allocated {
		byIncome[item.IncomeNo] = item
	}
	var balances []model.RoundingBalance
	if e := tx.Where("anchor_id=? AND currency=?", facts[0].AnchorID, rule.Currency).Limit(maxSettlementFacts + 1).Find(&balances).Error; e != nil {
		return nil, 0, e
	}
	if len(balances) > maxSettlementFacts {
		return nil, 0, ErrLimit
	}
	states := map[string]model.RoundingBalance{}
	for _, r := range balances {
		states[r.RuleNo] = r
	}
	items := []model.SettlementItem{}
	var total int64
	for index, f := range facts {
		if _, ok := byIncome[f.IncomeNo]; ok {
			return nil, 0, ErrConflict
		}
		item := model.SettlementItem{CoinUnits: rule.CoinUnits, MinorUnits: rule.MinorUnits, RuleNo: rule.RuleNo, Currency: rule.Currency}
		if f.EntryType == "reversal" {
			original, ok := byIncome[f.OriginalIncomeNo]
			if !ok {
				return nil, 0, ErrConflict
			}
			item.CoinUnits = original.CoinUnits
			item.MinorUnits = original.MinorUnits
			item.RuleNo = original.RuleNo
			item.Currency = original.Currency
		}
		if item.Currency != rule.Currency || f.AnchorID != facts[0].AnchorID {
			return nil, 0, ErrExchange
		}
		state, exists := states[item.RuleNo]
		if exists && (state.CoinUnits != item.CoinUnits || state.MinorUnits != item.MinorUnits || state.Currency != item.Currency) {
			return nil, 0, ErrInvariant
		}
		item.IncomeNo = f.IncomeNo
		item.IncomeHash = incomeHash(f)
		item.Sequence = int64(index + 1)
		item.RoundingVersion = state.Version
		item.RemainderBefore = state.Remainder
		var e error
		item.Amount, item.RemainderAfter, e = ConvertWithRemainder(f.Amount, state.Remainder, item.CoinUnits, item.MinorUnits)
		if e != nil {
			return nil, 0, e
		}
		state.RuleNo = item.RuleNo
		state.Currency = item.Currency
		state.CoinUnits = item.CoinUnits
		state.MinorUnits = item.MinorUnits
		state.Remainder = item.RemainderAfter
		states[item.RuleNo] = state
		total, e = add(total, item.Amount)
		if e != nil {
			return nil, 0, e
		}
		items = append(items, item)
		byIncome[f.IncomeNo] = item
	}
	sortItems(items)
	return items, total, nil
}
func (s *Service) CreateSettlement(ctx context.Context, a Actor, r req.CreateSettlement) (model.Settlement, error) {
	var out model.Settlement
	if !keyRE.MatchString(r.RequestID) || !noRE.MatchString(r.AnchorNo) || !noRE.MatchString(r.RuleNo) || r.PeriodStart < 0 || r.PeriodEnd <= r.PeriodStart || r.PeriodEnd > s.now() {
		return out, ErrInvalid
	}
	digest := hash(struct {
		R     req.CreateSettlement
		Actor uint64
	}{r, a.ID})
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "settlement/create", "POST"); e != nil {
			return e
		}
		if e := lockRules(tx, false); e != nil {
			return e
		}
		anchor, e := anchorByNo(tx, r.AnchorNo)
		if e != nil {
			return e
		}
		if e = lockAnchor(tx, uint64(anchor.ID)); e != nil {
			return e
		}
		e = tx.Where("request_id=?", r.RequestID).Take(&out).Error
		if e == nil {
			if out.RequestHash != digest {
				return ErrConflict
			}
			return s.verifySettlement(tx, out, false)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		rule, e := activeRule(tx, r.RuleNo, s.now())
		if e != nil {
			return e
		}
		var facts []model.Income
		if e = unallocated(tx, uint64(anchor.ID)).Where("unit=? AND created_at>=? AND created_at<?", rule.SourceUnit, r.PeriodStart, r.PeriodEnd).Where("entry_type='income' OR NOT EXISTS (SELECT 1 FROM anchor_settlement_allocation a WHERE a.income_no=anchor_income_fact.original_income_no) OR EXISTS (SELECT 1 FROM anchor_settlement_allocation a JOIN anchor_settlement_item i ON i.income_no=a.income_no AND i.settlement_no=a.settlement_no WHERE a.income_no=anchor_income_fact.original_income_no AND i.currency=?)", rule.Currency).Order("id ASC").Limit(maxSettlementFacts + 1).Find(&facts).Error; e != nil {
			return e
		}
		if len(facts) == 0 {
			return ErrNotFound
		}
		if len(facts) > maxSettlementFacts {
			return ErrLimit
		}
		if e = s.verifySources(tx, facts); e != nil {
			return e
		}
		items, amount, e := buildItems(tx, facts, rule)
		if e != nil {
			return e
		}
		out = model.Settlement{SettlementNo: number("st_"), RequestID: r.RequestID, RequestHash: digest, AnchorID: uint64(anchor.ID), AnchorNo: anchor.AnchorNo, PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd, Currency: rule.Currency, RuleNo: rule.RuleNo, RuleSnapshot: encode(rule), Amount: amount, ItemCount: int64(len(items)), ItemsHash: hash(items), Status: "pending", CreatedBy: a.ID, CreatedAt: s.now()}
		for _, f := range facts {
			out.EarningCoin, e = add(out.EarningCoin, f.Amount)
			if e != nil {
				return e
			}
			if f.GrossAmount >= 0 {
				out.GrossCoin, e = add(out.GrossCoin, f.GrossAmount)
			} else {
				out.RefundCoin, e = add(out.RefundCoin, -f.GrossAmount)
			}
			if e != nil {
				return e
			}
		}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		for i := range items {
			items[i].SettlementNo = out.SettlementNo
		}
		if e = tx.CreateInBatches(&items, 100).Error; e != nil {
			return e
		}
		return audit(tx, a, out.SettlementNo, "settlement.create", encode(out), s.now())
	})
	return out, e
}
func settlementFacts(tx *gorm.DB, out model.Settlement) ([]model.SettlementItem, []model.Income, error) {
	var items []model.SettlementItem
	if e := tx.Where("settlement_no=?", out.SettlementNo).Order("income_no ASC").Limit(maxSettlementFacts + 1).Find(&items).Error; e != nil {
		return nil, nil, e
	}
	if int64(len(items)) != out.ItemCount || len(items) > maxSettlementFacts {
		return nil, nil, ErrInvariant
	}
	var sum int64
	ids := []string{}
	for i := range items {
		items[i].ID = 0
		items[i].SettlementNo = ""
		ids = append(ids, items[i].IncomeNo)
		var e error
		sum, e = add(sum, items[i].Amount)
		if e != nil {
			return nil, nil, ErrInvariant
		}
	}
	if sum != out.Amount || hash(items) != out.ItemsHash {
		return nil, nil, ErrInvariant
	}
	var facts []model.Income
	if e := tx.Where("income_no IN ?", ids).Order("id ASC").Find(&facts).Error; e != nil {
		return nil, nil, e
	}
	if len(facts) != len(items) {
		return nil, nil, ErrInvariant
	}
	byNo := map[string]model.Income{}
	for _, f := range facts {
		byNo[f.IncomeNo] = f
		if f.AnchorID != out.AnchorID || f.CreatedAt < out.PeriodStart || f.CreatedAt >= out.PeriodEnd {
			return nil, nil, ErrInvariant
		}
	}
	for _, item := range items {
		if item.IncomeHash != incomeHash(byNo[item.IncomeNo]) || item.Currency != out.Currency {
			return nil, nil, ErrInvariant
		}
	}
	return items, facts, nil
}
func (s *Service) verifySettlement(tx *gorm.DB, out model.Settlement, deep bool) error {
	items, facts, e := settlementFacts(tx, out)
	if e != nil {
		return e
	}
	if deep {
		if e = s.verifySources(tx, facts); e != nil {
			return e
		}
	}
	if out.Status == "approved" {
		var allocations []model.Allocation
		if e = tx.Where("settlement_no=?", out.SettlementNo).Find(&allocations).Error; e != nil {
			return e
		}
		if int64(len(allocations)) != out.ItemCount {
			return ErrInvariant
		}
		mapped := map[string]bool{}
		for _, v := range allocations {
			mapped[v.IncomeNo] = true
		}
		for _, v := range items {
			if !mapped[v.IncomeNo] {
				return ErrInvariant
			}
		}
		kind := "settlement_credit"
		if out.Amount < 0 {
			kind = "negative_adjustment"
		}
		var ledger model.AccountLedger
		if e = tx.Where("business_no=? AND entry_type=? AND anchor_id=? AND currency=?", out.SettlementNo, kind, out.AnchorID, out.Currency).Take(&ledger).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrInvariant
			}
			return e
		}
		if ledger.ChangeAmount != out.Amount {
			return ErrInvariant
		}
		var reviewed model.FinanceAudit
		if e = tx.Where("object_no=? AND action='settlement.review'", out.SettlementNo).Take(&reviewed).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrInvariant
			}
			return e
		}
		if reviewed.Snapshot != encode(out) || reviewed.ActorID != out.ApprovedBy {
			return ErrInvariant
		}
	}

	return nil
}
func (s *Service) ReviewSettlement(ctx context.Context, a Actor, r req.FinanceReview) (model.Settlement, error) {
	var out model.Settlement
	if !noRE.MatchString(r.No) || !clean(r.Reason, 500) {
		return out, ErrInvalid
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "settlement/review", "POST"); e != nil {
			return e
		}
		if e := lockRules(tx, false); e != nil {
			return e
		}
		if e := tx.Where("settlement_no=?", r.No).Take(&out).Error; e != nil {
			return ErrNotFound
		}
		if e := lockAnchor(tx, out.AnchorID); e != nil {
			return e
		}
		if e := tx.Where("settlement_no=?", r.No).Take(&out).Error; e != nil {
			return e
		}
		want := "rejected"
		if r.Approve {
			want = "approved"
		}
		if out.Status != "pending" {
			if out.Status == want && out.ApprovedBy == a.ID && out.ReviewReason == r.Reason {
				return s.verifySettlement(tx, out, true)
			}
			return ErrConflict
		}
		if out.CreatedBy == a.ID && !system.IsSuperAdmin(a.ID) {
			return ErrForbidden
		}
		if r.Approve {
			rule, e := activeRule(tx, out.RuleNo, s.now())
			if e != nil {
				return e
			}
			if encode(rule) != out.RuleSnapshot {
				return ErrInvariant
			}
			items, facts, e := settlementFacts(tx, out)
			if e != nil {
				return e
			}
			if e = s.verifySources(tx, facts); e != nil {
				return e
			}
			rebuilt, total, e := buildItems(tx, facts, rule)
			if e != nil {
				return e
			}
			if total != out.Amount || hash(rebuilt) != hash(items) {
				return ErrConflict
			}
			account, e := loadAccount(tx, out.AnchorID, out.AnchorNo, out.Currency, s.now(), true)
			if e != nil {
				return e
			}
			if e = s.verifyAccount(tx, account, false); e != nil {
				return e
			}
			allocations := []model.Allocation{}
			for _, f := range facts {
				allocations = append(allocations, model.Allocation{IncomeNo: f.IncomeNo, SettlementNo: out.SettlementNo, CreatedAt: s.now()})
			}
			if e = tx.CreateInBatches(&allocations, 100).Error; e != nil {
				return e
			}
			kind := "settlement_credit"
			if out.Amount < 0 {
				kind = "negative_adjustment"
			}
			if e = post(tx, &account, kind, out.SettlementNo, out.Amount, s.now()); e != nil {
				return e
			}
		}
		approvedAt := s.now()
		change := tx.Model(&model.Settlement{}).Where("id=? AND status='pending'", out.ID).Updates(map[string]any{"status": want, "approved_by": a.ID, "approved_at": approvedAt, "review_reason": r.Reason})
		if change.Error != nil {
			return change.Error
		}
		if change.RowsAffected != 1 {
			return ErrConflict
		}
		out.Status = want
		out.ApprovedBy = a.ID
		out.ApprovedAt = approvedAt
		out.ReviewReason = r.Reason
		if r.Approve {
			if e := applyRounding(tx, out); e != nil {
				return e
			}
		}
		return audit(tx, a, out.SettlementNo, "settlement.review", encode(out), s.now())
	})
	return out, e
}
