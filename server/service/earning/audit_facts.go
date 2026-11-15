package earning

import (
	"gorm.io/gorm"
	"math/big"
	model "tb_live_module/model/earning"
)

// Failed reconciliation reports still show all recorded settlement totals;
// partial verified progress must never be mislabeled as the full account total.
func recordedSettlementTotals(tx *gorm.DB, account model.Account) (string, string, error) {
	credits, negative := new(big.Int), new(big.Int)
	var after uint64
	for {
		var rows []model.Settlement
		if e := tx.Select("id,amount").Where("anchor_id=? AND currency=? AND status='approved' AND id>?", account.AnchorID, account.Currency, after).Order("id").Limit(auditLedgerPage).Find(&rows).Error; e != nil {
			return "", "", e
		}
		if len(rows) == 0 {
			return credits.String(), negative.String(), nil
		}
		for _, row := range rows {
			if row.Amount >= 0 {
				credits.Add(credits, big.NewInt(row.Amount))
			} else {
				negative.Add(negative, big.NewInt(row.Amount))
			}
			after = row.ID
		}
	}
}

type auditFacts struct {
	items    map[string][]model.SettlementItem
	facts    map[string][]model.Income
	allFacts []model.Income
}

// Three bulk reads cover up to 1000 facts across multiple settlements. We retain
// independent canonical hashes and one-to-one allocation checks for every batch.
func loadAuditFacts(tx *gorm.DB, settlements map[string]model.Settlement, rows []model.AccountLedger) (auditFacts, error) {
	out := auditFacts{items: map[string][]model.SettlementItem{}, facts: map[string][]model.Income{}}
	nos := []string{}
	var expected int64
	for _, r := range rows {
		if st, ok := settlements[r.BusinessNo]; ok {
			nos = append(nos, st.SettlementNo)
			expected += st.ItemCount
		}
	}
	if len(nos) == 0 {
		return out, nil
	}
	if expected <= 0 || expected > maxSettlementFacts {
		return out, ErrInvariant
	}
	var items []model.SettlementItem
	if e := tx.Where("settlement_no IN ?", nos).Limit(maxSettlementFacts + 1).Find(&items).Error; e != nil {
		return out, e
	}
	if int64(len(items)) != expected {
		return out, ErrInvariant
	}
	var allocations []model.Allocation
	if e := tx.Where("settlement_no IN ?", nos).Limit(maxSettlementFacts + 1).Find(&allocations).Error; e != nil {
		return out, e
	}
	if len(allocations) != len(items) {
		return out, ErrInvariant
	}
	allocated := map[string]string{}
	for _, v := range allocations {
		allocated[v.IncomeNo] = v.SettlementNo
	}
	incomes := make([]string, 0, len(items))
	for _, item := range items {
		if allocated[item.IncomeNo] != item.SettlementNo {
			return out, ErrInvariant
		}
		incomes = append(incomes, item.IncomeNo)
		out.items[item.SettlementNo] = append(out.items[item.SettlementNo], item)
	}
	if e := tx.Where("income_no IN ?", incomes).Find(&out.allFacts).Error; e != nil {
		return out, e
	}
	if len(out.allFacts) != len(items) {
		return out, ErrInvariant
	}
	byNo := map[string]model.Income{}
	for _, f := range out.allFacts {
		byNo[f.IncomeNo] = f
	}
	for _, no := range nos {
		st := settlements[no]
		group := out.items[no]
		if int64(len(group)) != st.ItemCount {
			return out, ErrInvariant
		}
		sortItems(group)
		var total int64
		for i := range group {
			item := &group[i]
			f := byNo[item.IncomeNo]
			if f.AnchorID != st.AnchorID || f.CreatedAt < st.PeriodStart || f.CreatedAt >= st.PeriodEnd || item.IncomeHash != incomeHash(f) || item.Currency != st.Currency {
				return out, ErrInvariant
			}
			var e error
			total, e = add(total, item.Amount)
			if e != nil {
				return out, ErrInvariant
			}
			item.ID = 0
			item.SettlementNo = ""
			out.facts[no] = append(out.facts[no], f)
		}
		if total != st.Amount || hash(group) != st.ItemsHash {
			return out, ErrInvariant
		}
	}
	return out, nil
}
