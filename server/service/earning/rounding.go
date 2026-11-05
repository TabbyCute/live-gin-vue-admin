package earning

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"math"
	"math/big"
	"sort"
	model "tb_live_module/model/earning"
)

// The remainder is a numerator modulo coinUnits, not a rounded monetary balance.
// Euclidean division (including negative adjustments) makes splitting batches value-neutral.
func ConvertWithRemainder(amount, remainder, coin, minor int64) (int64, int64, error) {
	if coin <= 0 || minor <= 0 || remainder < 0 || remainder >= coin {
		return 0, 0, ErrInvalid
	}
	numerator := new(big.Int).Mul(big.NewInt(amount), big.NewInt(minor))
	numerator.Add(numerator, big.NewInt(remainder))
	quotient, rest := new(big.Int), new(big.Int)
	quotient.DivMod(numerator, big.NewInt(coin), rest)
	if !quotient.IsInt64() {
		return 0, 0, ErrInvalid
	}
	return quotient.Int64(), rest.Int64(), nil
}
func applyRounding(tx *gorm.DB, st model.Settlement) error {
	var items []model.SettlementItem
	if e := tx.Where("settlement_no=?", st.SettlementNo).Order("sequence ASC").Find(&items).Error; e != nil {
		return e
	}
	first, last := map[string]model.SettlementItem{}, map[string]model.SettlementItem{}
	for _, item := range items {
		if _, ok := first[item.RuleNo]; !ok {
			first[item.RuleNo] = item
		}
		last[item.RuleNo] = item
	}
	for no, begin := range first {
		if begin.RoundingVersion == math.MaxInt64 {
			return ErrInvalid
		}
		end := last[no]
		var old model.RoundingBalance
		e := tx.Where("anchor_id=? AND rule_no=?", st.AnchorID, no).Take(&old).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if begin.RoundingVersion != 0 || begin.RemainderBefore != 0 {
				return ErrConflict
			}
			next := model.RoundingBalance{AnchorID: st.AnchorID, RuleNo: no, Currency: st.Currency, CoinUnits: begin.CoinUnits, MinorUnits: begin.MinorUnits, Remainder: end.RemainderAfter, Version: 1, LastSettlementNo: st.SettlementNo, UpdatedAt: st.ApprovedAt}
			if e = tx.Create(&next).Error; e != nil {
				return e
			}
			continue
		}
		if e != nil {
			return e
		}
		if old.Version != begin.RoundingVersion || old.Remainder != begin.RemainderBefore {
			return ErrConflict
		}
		result := tx.Model(&model.RoundingBalance{}).Where("id=? AND version=? AND remainder=?", old.ID, old.Version, old.Remainder).Updates(map[string]any{"remainder": end.RemainderAfter, "version": old.Version + 1, "last_settlement_no": st.SettlementNo, "updated_at": st.ApprovedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
	}
	return nil
}
func verifyRounding(tx *gorm.DB, account model.Account, settlements []model.Settlement, items []model.SettlementItem, facts []model.Income, byBusiness map[string]model.AccountLedger) error {
	version := func(st model.Settlement) int64 {
		kind := "settlement_credit"
		if st.Amount < 0 {
			kind = "negative_adjustment"
		}
		return byBusiness[kind+":"+st.SettlementNo].Version
	}
	sort.Slice(settlements, func(i, j int) bool { return version(settlements[i]) < version(settlements[j]) })
	byFact := map[string]model.Income{}
	for _, f := range facts {
		byFact[f.IncomeNo] = f
	}
	byIncome := map[string]model.SettlementItem{}
	bySettlement := map[string][]model.SettlementItem{}
	for _, item := range items {
		byIncome[item.IncomeNo] = item
		bySettlement[item.SettlementNo] = append(bySettlement[item.SettlementNo], item)
	}
	states := map[string]model.RoundingBalance{}
	for _, st := range settlements {
		list := bySettlement[st.SettlementNo]
		sort.Slice(list, func(i, j int) bool { return list[i].Sequence < list[j].Sequence })
		used := map[string]bool{}
		var rule model.SettlementRule
		if json.Unmarshal([]byte(st.RuleSnapshot), &rule) != nil || rule.RuleNo != st.RuleNo || rule.Currency != st.Currency {
			return ErrInvariant
		}
		for i, item := range list {
			if item.Sequence != int64(i+1) {
				return ErrInvariant
			}
			f := byFact[item.IncomeNo]
			expected := model.SettlementItem{RuleNo: st.RuleNo, CoinUnits: rule.CoinUnits, MinorUnits: rule.MinorUnits, Currency: rule.Currency}
			if f.EntryType == "reversal" {
				var ok bool
				expected, ok = byIncome[f.OriginalIncomeNo]
				if !ok {
					return ErrInvariant
				}
			}
			if expected.RuleNo != item.RuleNo || expected.CoinUnits != item.CoinUnits || expected.MinorUnits != item.MinorUnits || expected.Currency != item.Currency {
				return ErrInvariant
			}
			state := states[item.RuleNo]
			if state.Version != item.RoundingVersion || state.Remainder != item.RemainderBefore {
				return ErrInvariant
			}
			amount, remainder, e := ConvertWithRemainder(f.Amount, state.Remainder, item.CoinUnits, item.MinorUnits)
			if e != nil || amount != item.Amount || remainder != item.RemainderAfter {
				return ErrInvariant
			}
			state.RuleNo = item.RuleNo
			state.Currency = item.Currency
			state.CoinUnits = item.CoinUnits
			state.MinorUnits = item.MinorUnits
			state.Remainder = remainder
			state.LastSettlementNo = st.SettlementNo
			state.UpdatedAt = st.ApprovedAt
			states[item.RuleNo] = state
			used[item.RuleNo] = true
		}
		for no := range used {
			state := states[no]
			state.Version++
			states[no] = state
		}
	}
	var rows []model.RoundingBalance
	if e := tx.Where("anchor_id=? AND currency=?", account.AnchorID, account.Currency).Find(&rows).Error; e != nil {
		return e
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
