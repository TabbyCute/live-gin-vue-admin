package earning

import (
	"errors"
	"gorm.io/gorm"
	model "tb_live_module/model/earning"
	ps "tb_live_module/service/payout"
)

// ValidatePayoutInTx is a trusted internal adapter, never an HTTP action. It uses
// the same locks and full verification as finance and does not mutate facts.
func (s *Service) ValidatePayoutInTx(tx *gorm.DB, no string) (model.Withdrawal, string, error) {
	w, proof, e := s.validatePayoutInTx(tx, no)
	if e == nil {
		return w, proof, nil
	}
	if errors.Is(e, ps.ErrCrypto) {
		return w, proof, e
	}
	var policy *Error
	if errors.As(e, &policy) && !errors.Is(e, ErrUnavailable) && !errors.Is(e, ErrDisabled) {
		return w, proof, errors.Join(ps.ErrOrderState, e)
	}
	return w, proof, errors.Join(ps.ErrOrderUnavailable, e)
}
func (s *Service) validatePayoutInTx(tx *gorm.DB, no string) (model.Withdrawal, string, error) {
	var w model.Withdrawal
	if e := lockRules(tx, false); e != nil {
		return w, "", e
	}
	if e := tx.Where("withdrawal_no=?", no).Take(&w).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return w, "", ErrNotFound
		}
		return w, "", e
	}
	anchorID := w.AnchorID
	if e := lockAnchor(tx, anchorID); e != nil {
		return w, "", e
	}
	if e := tx.Where("withdrawal_no=?", no).Take(&w).Error; e != nil {
		return w, "", e
	}
	if w.AnchorID != anchorID || w.Status != "approved" || w.ReconciliationStatus != "passed" || w.ReconciliationNo == "" {
		return w, "", ErrState
	}
	p, e := s.payee(tx, w.AnchorNo, w.PayeeRef, w.Currency)
	if e != nil {
		return w, "", e
	}
	a, e := anchorByNo(tx, w.AnchorNo)
	if e != nil {
		return w, "", e
	}
	if uint64(a.ID) != w.AnchorID || a.UserId != w.ApplicantUserID {
		return w, "", ErrInvariant
	}
	account, e := loadAccount(tx, w.AnchorID, w.AnchorNo, w.Currency, s.now(), false)
	if e != nil {
		return w, "", e
	}
	if account.Frozen < w.Amount || account.Paid < 0 {
		return w, "", ErrInvariant
	}
	var freeze model.AccountLedger
	if e = tx.Where("business_no=? AND entry_type='withdrawal_freeze'", no).Take(&freeze).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return w, "", ErrInvariant
		}
		return w, "", e
	}
	if freeze.AnchorID != w.AnchorID || freeze.Currency != w.Currency || freeze.ChangeAmount != w.Amount || freeze.FrozenAfter-freeze.FrozenBefore != w.Amount {
		return w, "", ErrInvariant
	}
	var previous model.Reconciliation
	if e = tx.Where("reconciliation_no=? AND withdrawal_no=? AND status='passed'", w.ReconciliationNo, w.WithdrawalNo).Take(&previous).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return w, "", ErrInvariant
		}
		return w, "", e
	}
	if previous.SnapshotHash != hash(previous.Snapshot) {
		return w, "", ErrInvariant
	}
	proof, e := s.buildReconciliation(tx, Actor{}, w, &p)
	if e != nil {
		return w, "", e
	}
	if proof.Status != "passed" {
		return w, "", ErrInvariant
	}
	return w, proof.Snapshot, nil
}
