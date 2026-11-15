package earning

import (
	"errors"
	"gorm.io/gorm"
	"math"
	"math/big"
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
