package earning

import (
	"errors"
	"gorm.io/gorm"
	"math"
	"sort"
	model "tb_live_module/model/earning"
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

func sortItems(items []model.SettlementItem) {
	sort.Slice(items, func(i, j int) bool { return items[i].IncomeNo < items[j].IncomeNo })
}
