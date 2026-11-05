package earning

import (
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"math"
	model "tb_live_module/model/earning"
)

// IncomeInput is a trusted in-process boundary, never an HTTP request model.
// The source service owns pricing, authorization and payment; it calls this inside its business transaction.
type IncomeInput struct {
	AnchorID                                                            uint64
	SourceType, SourceNo, SourceEntryNo, OriginalIncomeNo, Unit, RuleNo string
	Amount, GrossAmount, ShareBPS, CreatedAt                            int64
	SourceHash                                                          string
}

func AccrueInTx(tx *gorm.DB, in IncomeInput) (model.Income, error) {
	var out model.Income
	if !sourceRE.MatchString(in.SourceType) || !clean(in.SourceNo, 128) || !clean(in.SourceEntryNo, 128) || in.Unit != "COIN" || in.Amount == math.MinInt64 || in.GrossAmount == math.MinInt64 || in.ShareBPS < 0 || in.ShareBPS > 10000 || !clean(in.RuleNo, 64) || len(in.SourceHash) != 64 || in.CreatedAt <= 0 {
		return out, ErrInvalid
	}
	if decoded, e := hex.DecodeString(in.SourceHash); e != nil || len(decoded) != 32 {
		return out, ErrInvalid
	}
	if e := lockAnchor(tx, in.AnchorID); e != nil {
		return out, e
	}
	e := tx.Where("source_type=? AND source_entry_no=?", in.SourceType, in.SourceEntryNo).Take(&out).Error
	if e == nil {
		if out.AnchorID != in.AnchorID || out.SourceNo != in.SourceNo || out.OriginalIncomeNo != in.OriginalIncomeNo || out.Unit != in.Unit || out.Amount != in.Amount || out.GrossAmount != in.GrossAmount || out.RuleNo != in.RuleNo || out.ShareBPS != in.ShareBPS || out.SourceHash != in.SourceHash || out.CreatedAt != in.CreatedAt {
			return out, ErrConflict
		}
		return out, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	kind := "income"
	if in.OriginalIncomeNo != "" {
		kind = "reversal"
		var original model.Income
		if in.Amount > 0 || in.GrossAmount > 0 {
			return out, ErrInvalid
		}
		if e = tx.Where("income_no=? AND entry_type='income'", in.OriginalIncomeNo).Take(&original).Error; e != nil {
			return out, ErrNotFound
		}
		if original.AnchorID != in.AnchorID || original.SourceType != in.SourceType || original.SourceNo != in.SourceNo || original.Unit != in.Unit || original.RuleNo != in.RuleNo || original.ShareBPS != in.ShareBPS {
			return out, ErrConflict
		}
		var rows []model.Income
		if e = tx.Where("original_income_no=?", original.IncomeNo).Limit(10001).Find(&rows).Error; e != nil {
			return out, e
		}
		if len(rows) > 10000 {
			return out, ErrLimit
		}
		remain, gross := original.Amount, original.GrossAmount
		for _, r := range rows {
			var err error
			remain, err = add(remain, r.Amount)
			if err != nil {
				return out, ErrInvariant
			}
			gross, err = add(gross, r.GrossAmount)
			if err != nil {
				return out, ErrInvariant
			}
		}
		if remain < 0 || gross < 0 || -in.Amount > remain || -in.GrossAmount > gross {
			return out, ErrConflict
		}
	} else if in.Amount < 0 || in.GrossAmount < 0 {
		return out, ErrInvalid
	}
	out = model.Income{IncomeNo: number("in_"), AnchorID: in.AnchorID, SourceType: in.SourceType, SourceNo: in.SourceNo, SourceEntryNo: in.SourceEntryNo, OriginalIncomeNo: in.OriginalIncomeNo, EntryType: kind, Unit: in.Unit, Amount: in.Amount, GrossAmount: in.GrossAmount, RuleNo: in.RuleNo, ShareBPS: in.ShareBPS, SourceHash: in.SourceHash, CreatedAt: in.CreatedAt}
	return out, tx.Create(&out).Error
}
func GiftSourceHash(row model.Ledger) string {
	row.ID = 0
	return hash(struct {
		Row    model.Ledger
		Anchor uint64
	}{row, row.AnchorID})
}
func RecordGiftIncomeInTx(tx *gorm.DB, row model.Ledger, gross int64) (model.Income, error) {
	original := ""
	if row.EntryType == "reversal" {
		var first model.Income
		if e := tx.Where("source_type='gift' AND source_no=? AND entry_type='income'", row.GiftOrderNo).Take(&first).Error; e != nil {
			return model.Income{}, e
		}
		original = first.IncomeNo
	}
	return AccrueInTx(tx, IncomeInput{AnchorID: row.AnchorID, SourceType: "gift", SourceNo: row.GiftOrderNo, SourceEntryNo: row.EarningNo, OriginalIncomeNo: original, Unit: "COIN", Amount: row.EarningCoinAmount, GrossAmount: gross, RuleNo: row.RevenueRuleNo, ShareBPS: row.ShareBPS, CreatedAt: row.CreatedAt, SourceHash: GiftSourceHash(row)})
}
