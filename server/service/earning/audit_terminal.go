package earning

import (
	"gorm.io/gorm"
	model "tb_live_module/model/earning"
	pm "tb_live_module/model/payout"
)

type auditTerminal struct {
	events   map[string]pm.TerminalEvent
	orders   map[string]pm.Order
	attempts map[string][]pm.Attempt
}

func loadAuditTerminal(tx *gorm.DB, withdrawals []model.Withdrawal) (auditTerminal, error) {
	out := auditTerminal{map[string]pm.TerminalEvent{}, map[string]pm.Order{}, map[string][]pm.Attempt{}}
	nos := []string{}
	for _, w := range withdrawals {
		if w.Status == "paid" || w.Status == "cancelled" {
			nos = append(nos, w.WithdrawalNo)
		}
	}
	if len(nos) == 0 {
		return out, nil
	}
	var events []pm.TerminalEvent
	if e := tx.Where("withdrawal_no IN ?", nos).Limit(auditLedgerPage + 1).Find(&events).Error; e != nil {
		return out, e
	}
	if len(events) != len(nos) {
		return out, ErrInvariant
	}
	orders := []string{}
	for _, event := range events {
		if _, exists := out.events[event.WithdrawalNo]; exists {
			return out, ErrInvariant
		}
		out.events[event.WithdrawalNo] = event
		orders = append(orders, event.PayoutNo)
	}
	var os []pm.Order
	if e := tx.Where("payout_no IN ?", orders).Find(&os).Error; e != nil {
		return out, e
	}
	for _, o := range os {
		out.orders[o.PayoutNo] = o
	}
	var attempts []pm.Attempt
	if e := tx.Where("payout_no IN ? AND status='succeeded'", orders).Limit(auditLedgerPage + 1).Find(&attempts).Error; e != nil {
		return out, e
	}
	if len(attempts) > auditLedgerPage {
		return out, ErrInvariant
	}
	for _, a := range attempts {
		out.attempts[a.PayoutNo] = append(out.attempts[a.PayoutNo], a)
	}
	return out, nil
}

func verifyAuditTerminal(w model.Withdrawal, b auditBatch) error {
	event, ok := b.terminal.events[w.WithdrawalNo]
	if !ok {
		return ErrInvariant
	}
	order, ok := b.terminal.orders[event.PayoutNo]
	if !ok {
		return ErrInvariant
	}
	entry, status, kind := "withdrawal_release", "cancelled", "payout.cancelled"
	if w.Status == "paid" {
		entry, status, kind = "withdrawal_paid", "succeeded", "payout.succeeded"
	}
	l, ok := b.ledgers[entry+":"+w.WithdrawalNo]
	if !ok || event.Kind != kind || event.Amount != w.Amount || event.Currency != w.Currency || event.LedgerNo != l.LedgerNo || event.AccountVersion != l.Version || event.ActorID == 0 || order.BusinessNo != w.WithdrawalNo || order.Amount != w.Amount || order.Currency != w.Currency || order.AnchorID != w.AnchorID || order.Status != status {
		return ErrInvariant
	}
	attempts := b.terminal.attempts[order.PayoutNo]
	if w.Status == "cancelled" {
		if len(attempts) != 0 {
			return ErrInvariant
		}
		return nil
	}
	if len(attempts) != 1 {
		return ErrInvariant
	}
	a := attempts[0]
	if a.AttemptNo != event.AttemptNo || a.Amount != w.Amount || a.Currency != w.Currency || a.ReviewerID != event.ActorID || a.PaymentReferenceFingerprint == nil || a.EvidenceReference == "" {
		return ErrInvariant
	}
	return nil
}
