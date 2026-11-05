package wallet

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/wallet"
)

func validOrderAudit(o model.WalletAdjustmentOrder, logs []model.WalletAuditLog) bool {
	createAction := "create"
	if o.Kind == model.KindReverse {
		createAction = "create_reverse"
	}
	reviewAction, operator, reviewedAt := "", uint64(0), int64(0)
	switch o.Status {
	case model.StatusCredited, model.StatusReversed:
		reviewAction, operator, reviewedAt = "approve_and_credit", o.ApprovedBy, o.ApprovedAt
	case model.StatusRejected:
		reviewAction, operator, reviewedAt = "reject", o.RejectedBy, o.RejectedAt
	case model.StatusPending:
	default:
		return false
	}
	expected := 1
	if reviewAction != "" {
		expected++
	}
	if len(logs) != expected {
		return false
	}
	created, reviewed := 0, 0
	for _, log := range logs {
		if log.ObjectNo != o.OrderNo {
			return false
		}
		if log.Action == createAction && log.OperatorID == o.CreatedBy && log.OperatorIP == o.CreatedIP && log.Reason == o.Reason && log.CreatedAt == o.CreatedAt {
			created++
		}
		if reviewAction != "" && operator > 0 && reviewedAt > 0 && log.Action == reviewAction && log.OperatorID == operator && log.OperatorIP == o.ReviewedIP && log.Reason == o.ReviewReason && log.CreatedAt == reviewedAt {
			reviewed++
		}
	}
	return created == 1 && reviewed == expected-1
}

func validOrderLedger(o model.WalletAdjustmentOrder, l model.WalletLedger, walletID uint64) bool {
	delta, business := o.Amount, model.BusinessGrant
	if o.Kind == model.KindReverse {
		delta, business = -o.Amount, model.BusinessReverse
	} else if o.Kind != model.KindGrant {
		return false
	}
	if l.Reason != o.Reason || l.BalanceBefore < 0 || l.BalanceAfter < 0 ||
		(delta > 0 && (l.BalanceAfter < delta || l.BalanceAfter-delta != l.BalanceBefore)) ||
		(delta < 0 && (l.BalanceBefore < -delta || l.BalanceBefore+delta != l.BalanceAfter)) ||
		(o.Kind == model.KindGrant && l.OriginalLedgerNo != "") {
		return false
	}
	return o.Amount > 0 && walletID > 0 && l.WalletAccountID == walletID && l.LedgerNo == o.LedgerNo && l.BusinessNo == o.OrderNo && l.ChangeAmount == delta && l.BusinessType == business && l.OperatorID == o.ApprovedBy && o.ApprovedBy > 0 && (o.CreatedBy != o.ApprovedBy || o.ApprovedBy == topSuperAdminUserID) && o.ApprovedAt > 0 && o.CreditedAt == o.ApprovedAt && l.CreatedAt == o.CreditedAt
}

// 重试可以返回成功的前提是已提交事实齐全，不能仅凭可更新的订单status判定成功。
func validateOrderAudit(tx *gorm.DB, o model.WalletAdjustmentOrder) error {
	var logs []model.WalletAuditLog
	if err := tx.Where("object_no = ?", o.OrderNo).Order("id").Limit(3).Find(&logs).Error; err != nil {
		return err
	}
	if !validOrderAudit(o, logs) {
		return ErrInvariant
	}
	return nil
}

func validateReviewReplay(tx *gorm.DB, o model.WalletAdjustmentOrder) error {
	// legacy没有可重算的新摘要，仍须核实原有流水和审计；不可用于新充值。
	if o.SourceType != model.SourceLegacy && !validOrderRequest(o) {
		return ErrInvariant
	}
	if err := validateOrderAudit(tx, o); err != nil {
		return err
	}
	if o.Status == model.StatusRejected {
		if o.LedgerNo != "" || o.ApprovedBy != 0 {
			return ErrInvariant
		}
		return nil
	}
	var w model.WalletAccount
	// review处于RC事务；核对余额与尾流水期间要阻止其他消费推进版本。
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("account_id = ? AND currency_code = ?", o.AccountID, o.CurrencyCode).Take(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvariant
		}
		return err
	}
	var l model.WalletLedger
	if err := tx.Where("ledger_no = ?", o.LedgerNo).Take(&l).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvariant
		}
		return err
	}
	if !validOrderLedger(o, l, w.ID) || l.WalletVersion < 1 || l.WalletVersion > w.Version {
		return ErrInvariant
	}
	if o.Kind == model.KindReverse {
		var original model.WalletAdjustmentOrder
		if err := tx.Where("order_no = ?", o.OriginalOrderNo).Take(&original).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvariant
			}
			return err
		}
		if original.Kind != model.KindGrant || original.Status != model.StatusReversed || original.ReversalOrderNo != o.OrderNo || original.AccountID != o.AccountID || original.CurrencyCode != o.CurrencyCode || original.Amount != o.Amount || l.OriginalLedgerNo != original.LedgerNo {
			return ErrInvariant
		}
		if err := validateReviewReplay(tx, original); err != nil {
			return err
		}
	}
	return validateWalletTail(tx, w)
}
