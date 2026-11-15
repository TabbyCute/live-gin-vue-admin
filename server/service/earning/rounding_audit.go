package earning

import (
	"encoding/json"
	"gorm.io/gorm"
	"sort"
	model "tb_live_module/model/earning"
)

// replayRounding carries only rule remainders across pages. Refund references are
// resolved by the permanent allocation key, never an unbounded historical map.
func replayRounding(tx *gorm.DB, account model.Account, st model.Settlement, version int64, items []model.SettlementItem, facts []model.Income, states map[string]model.RoundingBalance) error {
	var rule model.SettlementRule
	if json.Unmarshal([]byte(st.RuleSnapshot), &rule) != nil || rule.RuleNo != st.RuleNo || rule.Currency != st.Currency {
		return ErrInvariant
	}
	byFact := map[string]model.Income{}
	byIncome := map[string]model.SettlementItem{}
	originalNos := []string{}
	for _, f := range facts {
		byFact[f.IncomeNo] = f
		if f.EntryType == "reversal" {
			originalNos = append(originalNos, f.OriginalIncomeNo)
		}
	}
	for _, item := range items {
		byIncome[item.IncomeNo] = item
	}
	if len(originalNos) > 0 {
		var originals []model.SettlementItem
		// Only earlier approved account movements may supply a historical rate.
		if e := tx.Table("anchor_settlement_item i").Select("i.*").
			Joins("JOIN anchor_settlement_allocation a ON a.income_no=i.income_no AND a.settlement_no=i.settlement_no").
			Joins("JOIN anchor_settlement s ON s.settlement_no=i.settlement_no AND s.status='approved'").
			Joins("JOIN anchor_withdrawable_ledger l ON l.business_no=s.settlement_no AND l.entry_type IN ('settlement_credit','negative_adjustment') AND l.anchor_id=s.anchor_id AND l.currency=s.currency").
			Where("i.income_no IN ? AND s.anchor_id=? AND s.currency=? AND l.version<?", originalNos, account.AnchorID, account.Currency, version).Find(&originals).Error; e != nil {
			return e
		}
		for _, item := range originals {
			byIncome[item.IncomeNo] = item
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	used := map[string]bool{}
	for i, item := range items {
		if item.Sequence != int64(i+1) {
			return ErrInvariant
		}
		f := byFact[item.IncomeNo]
		expected := model.SettlementItem{RuleNo: st.RuleNo, CoinUnits: rule.CoinUnits, MinorUnits: rule.MinorUnits, Currency: rule.Currency}
		if f.EntryType == "reversal" {
			var ok bool
			expected, ok = byIncome[f.OriginalIncomeNo]
			// settlementFacts clears SettlementNo for canonical hashing. Such
			// references belong to this batch and must precede the reversal.
			if !ok || (expected.SettlementNo == "" && expected.Sequence >= item.Sequence) {
				return ErrInvariant
			}
		}
		if expected.RuleNo != item.RuleNo || expected.CoinUnits != item.CoinUnits || expected.MinorUnits != item.MinorUnits || expected.Currency != item.Currency {
			return ErrInvariant
		}
		state, exists := states[item.RuleNo]
		if !exists && len(states) >= maxSettlementFacts {
			return ErrLimit
		}
		if state.Version != item.RoundingVersion || state.Remainder != item.RemainderBefore {
			return ErrInvariant
		}
		amount, remainder, e := ConvertWithRemainder(f.Amount, state.Remainder, item.CoinUnits, item.MinorUnits)
		if e != nil || amount != item.Amount || remainder != item.RemainderAfter {
			return ErrInvariant
		}
		state.RuleNo, state.Currency = item.RuleNo, item.Currency
		state.CoinUnits, state.MinorUnits, state.Remainder = item.CoinUnits, item.MinorUnits, remainder
		state.LastSettlementNo, state.UpdatedAt = st.SettlementNo, st.ApprovedAt
		states[item.RuleNo] = state
		used[item.RuleNo] = true
	}
	for no := range used {
		state := states[no]
		state.Version++
		states[no] = state
	}
	return nil
}

func verifyRoundingStates(tx *gorm.DB, account model.Account, states map[string]model.RoundingBalance) error {
	// The live settlement builder permits at most 1000 rule balances per currency.
	var rows []model.RoundingBalance
	if e := tx.Where("anchor_id=? AND currency=?", account.AnchorID, account.Currency).Limit(maxSettlementFacts + 1).Find(&rows).Error; e != nil {
		return e
	}
	if len(rows) > maxSettlementFacts {
		return ErrLimit
	}
	if len(rows) != len(states) {
		return ErrInvariant
	}
	for _, row := range rows {
		want, ok := states[row.RuleNo]
		if !ok || row.Currency != want.Currency || row.CoinUnits != want.CoinUnits || row.MinorUnits != want.MinorUnits || row.Remainder != want.Remainder || row.Version != want.Version || row.LastSettlementNo != want.LastSettlementNo || row.UpdatedAt != want.UpdatedAt {
			return ErrInvariant
		}
	}
	return nil
}
