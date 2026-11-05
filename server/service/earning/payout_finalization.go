package earning

import (
	"errors"
	"gorm.io/gorm"
	model "tb_live_module/model/earning"
	pm "tb_live_module/model/payout"
	ps "tb_live_module/service/payout"
)

// LockPayoutFinanceInTx preserves lock ordering without imposing live payee or
// risk gates after an external transfer has already happened.
func (s *Service) LockPayoutFinanceInTx(tx *gorm.DB, no string) (model.Withdrawal, error) {
	var w model.Withdrawal
	if e := lockRules(tx, false); e != nil {
		return w, e
	}
	if e := tx.Where("withdrawal_no=?", no).Take(&w).Error; e != nil {
		return w, e
	}
	anchor := w.AnchorID
	if e := lockAnchor(tx, anchor); e != nil {
		return w, e
	}
	if e := tx.Where("withdrawal_no=?", no).Take(&w).Error; e != nil {
		return w, e
	}
	if w.AnchorID != anchor {
		return w, ErrInvariant
	}
	return w, nil
}

func (s *Service) CheckPayoutFinanceInTx(tx *gorm.DB, no string) error {
	w, e := s.LockPayoutFinanceInTx(tx, no)
	if e != nil {
		return e
	}
	a, e := loadAccount(tx, w.AnchorID, w.AnchorNo, w.Currency, s.now(), false)
	if e != nil {
		return e
	}
	e = s.verifyAccount(tx, a, true)
	var business *Error
	if errors.As(e, &business) || errors.Is(e, gorm.ErrRecordNotFound) {
		return errors.Join(ps.ErrOrderState, e)
	}
	return e
}

func (s *Service) FinalizePayoutInTx(tx *gorm.DB, in pm.FinalizeInput) (pm.TerminalEvent, error) {
	var out pm.TerminalEvent
	w, e := s.LockPayoutFinanceInTx(tx, in.Order.BusinessNo)
	if e != nil {
		return out, e
	}
	o := in.Order
	if in.ActorID == 0 || in.Now < w.CreatedAt || w.Status != "approved" || w.ReconciliationStatus != "passed" || w.AnchorID != o.AnchorID || w.ApplicantUserID != o.ApplicantUserID || w.Amount != o.Amount || w.Currency != o.Currency || w.PayeeRef != o.PayeeAccountNo || w.PayeeVersion != o.PayeeVersion {
		return out, ErrInvariant
	}
	target, kind, entry := "cancelled", "payout.cancelled", "withdrawal_release"
	if in.Paid {
		target, kind, entry = "succeeded", "payout.succeeded", "withdrawal_paid"
	}
	var history pm.StatusHistory
	if e = tx.Where("event_no=? AND payout_no=? AND from_status=? AND to_status=? AND sequence=? AND actor_id=?", in.TransitionNo, o.PayoutNo, o.Status, target, o.StateVersion+1, in.ActorID).Take(&history).Error; e != nil {
		return out, e
	}
	if in.Paid {
		var attempt pm.Attempt
		if e = tx.Where("attempt_no=? AND payout_no=? AND status='succeeded'", in.AttemptNo, o.PayoutNo).Take(&attempt).Error; e != nil {
			return out, e
		}
		if attempt.Amount != o.Amount || attempt.Currency != o.Currency || attempt.PayeeSnapshotHash != o.PayeeSnapshotHash || attempt.ReviewerID != in.ActorID || attempt.PaymentReferenceFingerprint == nil || attempt.EvidenceReference == "" || history.AttemptNo != attempt.AttemptNo {
			return out, ErrInvariant
		}
	} else {
		var unsafe int64
		if e = tx.Model(&pm.Attempt{}).Where("payout_no=? AND status NOT IN ('failed','abandoned')", o.PayoutNo).Count(&unsafe).Error; e != nil {
			return out, e
		}
		if unsafe != 0 || (o.Status != "ready" && o.Status != "failed") {
			return out, ErrInvariant
		}
	}
	a, e := loadAccount(tx, w.AnchorID, w.AnchorNo, w.Currency, in.Now, false)
	if e != nil {
		return out, e
	}
	if e = s.verifyAccount(tx, a, true); e != nil {
		return out, e
	}
	if e = post(tx, &a, entry, w.WithdrawalNo, w.Amount, in.Now); e != nil {
		return out, e
	}
	var ledger model.AccountLedger
	if e = tx.Where("entry_type=? AND business_no=?", entry, w.WithdrawalNo).Take(&ledger).Error; e != nil {
		return out, e
	}
	out = pm.TerminalEvent{EventNo: number("pte_"), PayoutNo: o.PayoutNo, WithdrawalNo: w.WithdrawalNo, AttemptNo: in.AttemptNo, TransitionNo: in.TransitionNo, Kind: kind, Amount: w.Amount, Currency: w.Currency, LedgerNo: ledger.LedgerNo, AccountVersion: a.Version, ActorID: in.ActorID, CreatedAt: in.Now}
	if e = tx.Create(&out).Error; e != nil {
		return out, e
	}
	status := "cancelled"
	if in.Paid {
		status = "paid"
	}
	result := tx.Model(&model.Withdrawal{}).Where("id=? AND status='approved'", w.ID).Update("status", status)
	if result.Error != nil {
		return out, result.Error
	}
	if result.RowsAffected != 1 {
		return out, ErrConflict
	}
	w.Status = status
	if e = audit(tx, Actor{ID: in.ActorID}, w.WithdrawalNo, "withdrawal."+status, encode(w), in.Now); e != nil {
		return out, e
	}
	return out, nil
}

func verifyWithdrawalTerminal(tx *gorm.DB, w model.Withdrawal, ledgers map[string]model.AccountLedger) error {
	var event pm.TerminalEvent
	if e := tx.Where("withdrawal_no=?", w.WithdrawalNo).Take(&event).Error; e != nil {
		return e
	}
	var order pm.Order
	if e := tx.Where("payout_no=?", event.PayoutNo).Take(&order).Error; e != nil {
		return e
	}
	entry, status, kind := "withdrawal_release", "cancelled", "payout.cancelled"
	if w.Status == "paid" {
		entry, status, kind = "withdrawal_paid", "succeeded", "payout.succeeded"
	}
	l, ok := ledgers[entry+":"+w.WithdrawalNo]
	if !ok || event.Kind != kind || event.Amount != w.Amount || event.Currency != w.Currency || event.LedgerNo != l.LedgerNo || event.AccountVersion != l.Version || event.ActorID == 0 || order.BusinessNo != w.WithdrawalNo || order.Amount != w.Amount || order.Currency != w.Currency || order.AnchorID != w.AnchorID || order.Status != status {
		return ErrInvariant
	}
	var count int64
	if e := tx.Model(&pm.Attempt{}).Where("payout_no=? AND status='succeeded'", order.PayoutNo).Count(&count).Error; e != nil {
		return e
	}
	if (w.Status == "paid" && count != 1) || (w.Status == "cancelled" && count != 0) {
		return ErrInvariant
	}
	if w.Status == "paid" {
		var attempt pm.Attempt
		if e := tx.Where("attempt_no=? AND payout_no=? AND status='succeeded'", event.AttemptNo, order.PayoutNo).Take(&attempt).Error; e != nil {
			return e
		}
		if attempt.Amount != w.Amount || attempt.Currency != w.Currency || attempt.ReviewerID != event.ActorID || attempt.PaymentReferenceFingerprint == nil || attempt.EvidenceReference == "" {
			return ErrInvariant
		}
	}
	return nil
}
