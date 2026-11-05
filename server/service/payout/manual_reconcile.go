package payout

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"strings"
	em "tb_live_module/model/earning"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	"tb_live_module/model/system"
)

func (s *Service) CancelOrder(ctx context.Context, a Actor, trace string, r req.CancelOrder) (res.ActionResult, error) {
	var empty res.ActionResult
	reason, e := safeNote(r.Reason)
	if e != nil || !r.ConfirmNoTransfer || !orderNoRE.MatchString(r.PayoutNo) {
		return empty, ErrOrderInvalid
	}
	return s.manualCommand(ctx, a, "order/cancel", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, e := s.lockOrder(tx, r.PayoutNo)
		if e != nil {
			return empty, e
		}
		if o.Status != "ready" && o.Status != "failed" {
			return empty, ErrOrderState
		}
		var attempts []model.Attempt
		if e = tx.Where("payout_no=?", o.PayoutNo).Limit(101).Find(&attempts).Error; e != nil {
			return empty, e
		}
		if len(attempts) > 100 {
			return empty, ErrOrderState
		}
		for _, at := range attempts {
			if (at.Status != "failed" && at.Status != "abandoned") || (at.Status == "failed" && at.FailureCode != "confirmed_unpaid" && at.FailureCode != "not_transferred") {
				return empty, ErrOrderState
			}
		}
		now := s.now()
		h, e := transitionIntent(tx, o, a, "", r.RequestID, "cancelled", reason, now)
		if e != nil {
			return empty, e
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, o.PayoutNo, "order.cancel", map[string]any{"reason": reason, "confirmedNoTransfer": true}, now, h.EventNo); e != nil {
			return empty, e
		}
		if s.FinancialFinalize == nil {
			return empty, ErrOrderUnavailable
		}
		if _, e = s.FinancialFinalize(tx, model.FinalizeInput{Order: o, TransitionNo: h.EventNo, ActorID: a.ID, Now: now}); e != nil {
			return empty, e
		}
		if e = applyTransition(tx, &o, h); e != nil {
			return empty, e
		}
		if _, e = s.reconcileOrderInTx(tx, a, o, now, true); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, Status: o.Status}, nil
	})
}
func (s *Service) InvestigateOrder(ctx context.Context, a Actor, trace string, r req.Investigate) (res.ActionResult, error) {
	var empty res.ActionResult
	note, e := safeNote(r.Note)
	if e != nil || !orderNoRE.MatchString(r.PayoutNo) || (r.EvidenceNo != "" && !evidenceNoRE.MatchString(r.EvidenceNo)) {
		return empty, ErrOrderInvalid
	}
	return s.manualCommand(ctx, a, "order/investigate", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, e := s.lockOrder(tx, r.PayoutNo)
		if e != nil {
			return empty, e
		}
		if o.Status != "unknown" {
			return empty, ErrOrderState
		}
		var at model.Attempt
		if e = tx.Where("payout_no=? AND status='unknown'", o.PayoutNo).Take(&at).Error; e != nil {
			return empty, e
		}
		if r.EvidenceNo != "" {
			var proof model.Evidence
			if e = tx.Where("evidence_no=? AND attempt_no=? AND created_by=?", r.EvidenceNo, at.AttemptNo, a.ID).Take(&proof).Error; e != nil {
				return empty, ErrEvidence
			}
			if _, e = s.readEvidenceBytes(ctx, proof); e != nil {
				return empty, e
			}
		}
		var n int64
		if e = tx.Model(&model.Investigation{}).Where("payout_no=?", o.PayoutNo).Count(&n).Error; e != nil {
			return empty, e
		}
		if n >= 1000 {
			return empty, ErrOrderState
		}
		row := model.Investigation{InvestigationNo: number("pin_"), PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, ActorID: a.ID, Note: note, EvidenceNo: r.EvidenceNo, CreatedAt: s.now()}
		if e = tx.Create(&row).Error; e != nil {
			return empty, e
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, row.InvestigationNo, "order.investigate", row, row.CreatedAt); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, Status: o.Status, InvestigationNo: row.InvestigationNo}, nil
	})
}

func (s *Service) RunOrderReconciliation(ctx context.Context, a Actor, trace string, r req.OrderAction) (res.ActionResult, error) {
	var empty res.ActionResult
	reason, e := safeNote(r.Reason)
	if e != nil || !orderNoRE.MatchString(r.PayoutNo) {
		return empty, ErrOrderInvalid
	}
	return s.manualCommand(ctx, a, "order/reconcile", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, e := s.lockOrder(tx, r.PayoutNo)
		if e != nil {
			return empty, e
		}
		row, e := s.reconcileOrderInTx(tx, a, o, s.now(), false)
		if e != nil {
			return empty, e
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, row.ReconciliationNo, "order.reconcile", map[string]any{"reconciliationNo": row.ReconciliationNo, "status": row.Status, "reason": reason}, row.CreatedAt); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, Status: o.Status, ReconciliationNo: row.ReconciliationNo}, nil
	})
}

func (s *Service) reconcileOrderInTx(tx *gorm.DB, a Actor, o model.Order, now int64, requirePassed bool) (model.OrderReconciliation, error) {
	check, e := s.checkOrderInTx(tx, o, now, requirePassed)
	if e != nil {
		return model.OrderReconciliation{}, e
	}
	if requirePassed && !check.Consistent {
		return model.OrderReconciliation{}, ErrOrderState
	}
	b, e := json.Marshal(check)
	if e != nil {
		return model.OrderReconciliation{}, e
	}
	row := model.OrderReconciliation{ReconciliationNo: number("por_"), PayoutNo: o.PayoutNo, Status: "abnormal", Snapshot: string(b), SnapshotHash: snapshotHash(string(b)), CreatedBy: a.ID, CreatedAt: now}
	if check.Consistent {
		row.Status = "passed"
	}
	return row, tx.Create(&row).Error
}

// Bounded verification of the internal accounting and immutable evidence chain.
// A passed result is not an automated bank statement reconciliation.
func (s *Service) checkOrderInTx(tx *gorm.DB, o model.Order, now int64, pendingLatestRequest bool) (res.Reconciliation, error) {
	out := res.Reconciliation{PayoutNo: o.PayoutNo, Status: o.Status, Issues: []string{}, CheckedAt: now}
	issue := func(v string) {
		for _, old := range out.Issues {
			if old == v {
				return
			}
		}
		out.Issues = append(out.Issues, v)
	}
	if e := s.VerifyOrder(o); e != nil {
		issue("order_snapshot_invalid")
	}
	if s.FinancialCheck == nil {
		return out, ErrOrderUnavailable
	}
	if e := s.FinancialCheck(tx, o.BusinessNo); e != nil {
		if errors.Is(e, ErrOrderState) {
			issue("financial_chain_invalid")
		} else {
			return out, e
		}
	}
	var w em.Withdrawal
	if e := tx.Where("withdrawal_no=?", o.BusinessNo).Take(&w).Error; e != nil {
		return out, e
	}
	if w.Amount != o.Amount || w.Currency != o.Currency || w.AnchorID != o.AnchorID || w.PayeeRef != o.PayeeAccountNo || w.PayeeVersion != o.PayeeVersion {
		issue("withdrawal_source_mismatch")
	}
	want := "approved"
	if o.Status == "succeeded" {
		want = "paid"
	}
	if o.Status == "cancelled" {
		want = "cancelled"
	}
	if w.Status != want {
		issue("withdrawal_state_mismatch")
	}
	var attempts []model.Attempt
	if e := tx.Where("payout_no=?", o.PayoutNo).Order("sequence ASC").Limit(101).Find(&attempts).Error; e != nil {
		return out, e
	}
	if len(attempts) > 100 {
		return out, ErrOrderState
	}
	active, success := 0, 0
	var allEvents []model.AttemptEvent
	if e := tx.Where("payout_no=?", o.PayoutNo).Order("attempt_no,version ASC").Limit(601).Find(&allEvents).Error; e != nil {
		return out, e
	}
	eventsByAttempt := map[string][]model.AttemptEvent{}
	for _, ev := range allEvents {
		eventsByAttempt[ev.AttemptNo] = append(eventsByAttempt[ev.AttemptNo], ev)
	}
	if len(allEvents) > 600 {
		issue("attempt_event_limit_exceeded")
	}
	var auditRows []model.OrderAudit
	if e := tx.Where("payout_no=? AND (transition_no<>'' OR action='order.create')", o.PayoutNo).Limit(513).Find(&auditRows).Error; e != nil {
		return out, e
	}
	auditsByTransition := map[string][]model.OrderAudit{}
	created := 0
	for _, au := range auditRows {
		auditsByTransition[au.TransitionNo] = append(auditsByTransition[au.TransitionNo], au)
		if au.Action == "order.create" && au.ActorID == o.CreatedBy && au.CreatedAt == o.CreatedAt {
			created++
		}
	}
	if created != 1 {
		issue("creation_audit_missing")
	}
	companyNos := []string{}
	for _, at := range attempts {
		companyNos = append(companyNos, at.ChannelAccountNo)
	}
	companies := map[string]model.ChannelAccount{}
	if len(companyNos) > 0 {
		var rows []model.ChannelAccount
		if e := tx.Where("channel_account_no IN ?", companyNos).Find(&rows).Error; e != nil {
			return out, e
		}
		for _, c := range rows {
			companies[c.ChannelAccountNo] = c
			if _, e := s.openCompany(c); e != nil {
				issue("company_snapshot_invalid")
			}
		}
	}
	byNo := map[string]model.Attempt{}
	for i, at := range attempts {
		byNo[at.AttemptNo] = at
		company, ok := companies[at.ChannelAccountNo]
		if !ok || company.Currency != at.Currency || company.ChannelCode != at.ChannelCode || company.MaskedAccount != at.CompanyMasked || at.Amount > company.SingleLimit {
			issue("company_source_mismatch")
		}
		if at.Sequence != int64(i+1) || at.Amount != o.Amount || at.Currency != o.Currency || at.PayeeSnapshotHash != o.PayeeSnapshotHash || at.OperatorID == 0 {
			issue("attempt_source_mismatch")
		}
		if at.Status == "processing" || at.Status == "submitted" || at.Status == "unknown" {
			active++
		}
		if at.Status == "succeeded" {
			success++
		}
		if at.Status == "submitted" || at.Status == "unknown" || at.Status == "succeeded" || (at.Status == "failed" && at.FailureCode == "confirmed_unpaid") {
			ref, e := normalizeReference(at.PaymentReference)
			if e != nil || ref != at.PaymentReference || at.PaymentReferenceFingerprint == nil || *at.PaymentReferenceFingerprint != s.Crypto.digest("payout.payment.reference.v1", []string{at.ChannelAccountNo, ref}) || at.SubmittedAt == 0 || at.BankSubmittedAt == 0 {
				issue("payment_reference_invalid")
			}
			var proof model.Evidence
			if e = tx.Where("evidence_no=? AND attempt_no=? AND payout_no=?", at.EvidenceReference, at.AttemptNo, o.PayoutNo).Take(&proof).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					issue("evidence_missing")
				} else {
					return out, e
				}
			} else {
				if proof.SHA256 != at.EvidenceHash || proof.CreatedBy != at.OperatorID {
					issue("evidence_mismatch")
				}
				if _, e = s.readEvidenceBytes(tx.Statement.Context, proof); e != nil {
					issue("evidence_integrity_invalid")
				}
			}
		}
		if at.Status == "unknown" || at.Status == "succeeded" || (at.Status == "failed" && at.FailureCode == "confirmed_unpaid") {
			if at.ReviewerID == 0 || (at.ReviewerID == at.OperatorID && !system.IsSuperAdmin(at.ReviewerID)) || at.ReviewedAt < at.SubmittedAt || at.ReviewReason == "" {
				issue("review_separation_invalid")
			}
		}
		if at.Status == "abandoned" || (at.Status == "failed" && at.FailureCode == "not_transferred") {
			if at.BankSubmittedAt != 0 || at.PaymentReference != "" || at.EvidenceReference != "" {
				issue("unpaid_claim_has_transfer_evidence")
			}
		}
		events := eventsByAttempt[at.AttemptNo]
		last := ""
		for n, event := range events {
			if event.Version != int64(n+1) || event.FromStatus != last || event.SnapshotHash != snapshotHash(event.Snapshot) || event.PayoutNo != o.PayoutNo {
				issue("attempt_event_chain_invalid")
			}
			last = event.ToStatus
		}
		if int64(len(events)) != at.Version || len(events) == 0 || last != at.Status {
			issue("attempt_event_chain_invalid")
		} else {
			b, _ := json.Marshal(attemptValues(at))
			if events[len(events)-1].EventNo != at.LastEventNo || events[len(events)-1].Snapshot != string(b) {
				issue("attempt_event_snapshot_mismatch")
			}
		}
	}
	if active > 1 || success > 1 {
		issue("duplicate_active_or_successful_attempt")
	}
	if o.Status == "succeeded" && success != 1 {
		issue("successful_attempt_missing")
	}
	if o.Status != "succeeded" && success != 0 {
		issue("unexpected_successful_attempt")
	}
	if o.Status == "processing" || o.Status == "pending_confirmation" || o.Status == "unknown" {
		if active != 1 {
			issue("active_attempt_missing")
		}
		want := map[string]string{"processing": "processing", "pending_confirmation": "submitted", "unknown": "unknown"}[o.Status]
		if len(attempts) == 0 || attempts[len(attempts)-1].Status != want {
			issue("active_attempt_state_mismatch")
		}
	} else if active != 0 {
		issue("unexpected_active_attempt")
	}
	var history []model.StatusHistory
	if e := tx.Where("payout_no=?", o.PayoutNo).Order("sequence ASC").Limit(512).Find(&history).Error; e != nil {
		return out, e
	}
	last := ""
	requestIDs := []string{}
	for i, h := range history {
		valid := i == 0 && h.FromStatus == "" && h.ToStatus == "ready"
		if i > 0 {
			valid = orderTransitions[last][h.ToStatus]
		}
		if !valid || h.Sequence != int64(i+1) || h.FromStatus != last || h.ActorID == 0 {
			issue("order_history_invalid")
		}
		last = h.ToStatus
		if i > 0 {
			if h.RequestID == "" {
				issue("history_request_missing")
			} else if !pendingLatestRequest || i != len(history)-1 {
				requestIDs = append(requestIDs, h.RequestID)
			}
			if h.AttemptNo != "" {
				if _, ok := byNo[h.AttemptNo]; !ok {
					issue("history_attempt_missing")
				}
			}
			audits := auditsByTransition[h.EventNo]
			if len(audits) != 1 || audits[0].ActorID != h.ActorID || audits[0].CreatedAt != h.CreatedAt {
				issue("transition_audit_missing")
			}
		}
	}
	if int64(len(history)) != o.StateVersion || len(history) == 0 || last != o.Status {
		issue("order_history_invalid")
	}
	if len(history) > 1 && history[len(history)-1].EventNo != o.LastTransitionNo {
		issue("order_transition_link_invalid")
	}
	if len(requestIDs) > 0 {
		var receipts []model.ManualRequest
		if e := tx.Where("request_id IN ?", requestIDs).Find(&receipts).Error; e != nil {
			return out, e
		}
		if len(receipts) != len(requestIDs) {
			issue("permanent_request_missing")
		}
		byKey := map[string]model.ManualRequest{}
		for _, receipt := range receipts {
			byKey[receipt.RequestID] = receipt
		}
		for i, h := range history {
			if i == 0 || (pendingLatestRequest && i == len(history)-1) {
				continue
			}
			receipt := byKey[h.RequestID]
			au := auditsByTransition[h.EventNo]
			var result res.ActionResult
			if receipt.ActorID != h.ActorID || len(au) != 1 || receipt.Action != strings.ReplaceAll(strings.Replace(au[0].Action, ".", "/", 1), "_", "-") || json.Unmarshal([]byte(receipt.Response), &result) != nil || result.PayoutNo != o.PayoutNo || result.Status != h.ToStatus {
				issue("permanent_request_mismatch")
			}
		}
	}
	if o.Status == "succeeded" || o.Status == "cancelled" {
		var event model.TerminalEvent
		if e := tx.Where("payout_no=?", o.PayoutNo).Take(&event).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				issue("terminal_event_missing")
			} else {
				return out, e
			}
		} else if event.Amount != o.Amount || event.Currency != o.Currency || event.WithdrawalNo != o.BusinessNo || event.TransitionNo != o.LastTransitionNo {
			issue("terminal_event_mismatch")
		}
	}
	out.Consistent = len(out.Issues) == 0
	return out, nil
}
