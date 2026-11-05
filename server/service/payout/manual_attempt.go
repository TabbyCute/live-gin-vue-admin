package payout

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	"tb_live_module/model/system"
	"time"
)

func (s *Service) StartOrder(ctx context.Context, a Actor, trace string, r req.StartOrder, retry bool) (res.ActionResult, error) {
	var empty res.ActionResult
	reason, e := safeNote(r.Reason)
	if e != nil || !orderNoRE.MatchString(r.PayoutNo) || !companyNoRE.MatchString(r.ChannelAccountNo) {
		return empty, ErrOrderInvalid
	}
	if s.EvidenceStore == nil || s.FinancialGuard == nil {
		return empty, ErrEvidence
	}
	op, want := "start", "ready"
	if retry {
		op, want = "retry", "failed"
	}
	return s.manualCommand(ctx, a, "order/"+op, r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, e := s.lockOrder(tx, r.PayoutNo)
		if e != nil {
			return empty, e
		}
		if o.Status != want {
			return empty, ErrOrderState
		}
		if _, _, e = s.FinancialGuard(tx, o.BusinessNo); e != nil {
			return empty, e
		}
		var count, active int64
		if e = tx.Model(&model.Attempt{}).Where("payout_no=?", o.PayoutNo).Count(&count).Error; e != nil {
			return empty, e
		}
		if count >= 100 {
			return empty, ErrChannelLimit
		}
		if e = tx.Model(&model.Attempt{}).Where("payout_no=? AND status IN ('processing','submitted','unknown','succeeded')", o.PayoutNo).Count(&active).Error; e != nil {
			return empty, e
		}
		if active != 0 {
			return empty, ErrOrderState
		}
		var company model.ChannelAccount
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_account_no=?", r.ChannelAccountNo).Take(&company).Error; e != nil {
			return empty, e
		}
		if company.Status != "active" || company.ChannelCode != "manual_bank" || company.Currency != o.Currency || o.PayeeMethod != "bank_card" || o.Amount > company.SingleLimit {
			return empty, ErrChannelLimit
		}
		if _, e = s.openCompany(company); e != nil {
			return empty, e
		}
		now := s.now()
		day := time.UnixMilli(now).UTC().Format("2006-01-02")
		var usage model.DailyUsage
		e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_account_no=? AND day=?", company.ChannelAccountNo, day).Take(&usage).Error
		fresh := errors.Is(e, gorm.ErrRecordNotFound)
		if e != nil && !fresh {
			return empty, e
		}
		if usage.Version == math.MaxInt64 || usage.Amount > company.DailyLimit || o.Amount > company.DailyLimit-usage.Amount {
			return empty, ErrChannelLimit
		}
		slot := 1
		at := model.Attempt{AttemptNo: number("pat_"), PayoutNo: o.PayoutNo, Sequence: count + 1, ActiveSlot: &slot, Status: "processing", ChannelCode: company.ChannelCode, ChannelAccountNo: company.ChannelAccountNo, CompanyMasked: company.MaskedAccount, Amount: o.Amount, Currency: o.Currency, PayeeSnapshotHash: o.PayeeSnapshotHash, OperatorID: a.ID, CreatedAt: now, UpdatedAt: now, LimitDay: day, UsageVersion: usage.Version + 1}
		h, e := transitionIntent(tx, o, a, at.AttemptNo, r.RequestID, "processing", reason, now)
		if e != nil {
			return empty, e
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, at.AttemptNo, "order."+op, map[string]any{"attemptNo": at.AttemptNo, "channelAccountNo": company.ChannelAccountNo, "amount": o.Amount, "currency": o.Currency, "reason": reason}, now, h.EventNo); e != nil {
			return empty, e
		}
		if e = saveAttempt(tx, model.Attempt{}, &at, a, "order."+op, reason, h.EventNo); e != nil {
			return empty, e
		}
		if fresh {
			e = tx.Create(&model.DailyUsage{ChannelAccountNo: company.ChannelAccountNo, Day: day, Amount: o.Amount, Version: 1, LastAttemptNo: at.AttemptNo}).Error
		} else {
			result := tx.Model(&model.DailyUsage{}).Where("channel_account_no=? AND day=? AND version=?", company.ChannelAccountNo, day, usage.Version).Updates(map[string]any{"amount": usage.Amount + o.Amount, "version": usage.Version + 1, "last_attempt_no": at.AttemptNo})
			e = result.Error
			if e == nil && result.RowsAffected != 1 {
				e = ErrOrderConflict
			}
		}
		if e != nil {
			return empty, e
		}
		if e = applyTransition(tx, &o, h); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, Status: o.Status, ChannelAccountNo: company.ChannelAccountNo}, nil
	})
}

func (s *Service) UnpaidAttempt(ctx context.Context, a Actor, trace string, r req.UnpaidAction, fail bool) (res.ActionResult, error) {
	var empty res.ActionResult
	reason, e := safeNote(r.Reason)
	if e != nil || !r.ConfirmNoTransfer || !attemptNoRE.MatchString(r.AttemptNo) {
		return empty, ErrOrderInvalid
	}
	path, action, target, nextStatus := "order/release", "order.release", "ready", "abandoned"
	if fail {
		path, action, target, nextStatus = "attempt/fail-unpaid", "attempt.fail_unpaid", "failed", "failed"
	}
	return s.manualCommand(ctx, a, path, r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, at, e := s.lockAttempt(tx, r.AttemptNo)
		if e != nil {
			return empty, e
		}
		if o.Status != "processing" || at.Status != "processing" || at.OperatorID != a.ID || at.BankSubmittedAt != 0 || at.PaymentReference != "" || at.EvidenceReference != "" {
			return empty, ErrOrderState
		}
		var count int64
		if e = tx.Model(&model.Evidence{}).Where("attempt_no=?", at.AttemptNo).Count(&count).Error; e != nil {
			return empty, e
		}
		if count != 0 {
			return empty, ErrOrderState
		}
		now := s.now()
		h, e := transitionIntent(tx, o, a, at.AttemptNo, r.RequestID, target, reason, now)
		if e != nil {
			return empty, e
		}
		next := at
		next.Status = nextStatus
		next.ActiveSlot = nil
		next.UpdatedAt = now
		next.CompletedAt = now
		if fail {
			next.FailureCode = "not_transferred"
			next.FailureReason = reason
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, at.AttemptNo, action, map[string]any{"attemptNo": at.AttemptNo, "reason": reason, "confirmedNoTransfer": true}, now, h.EventNo); e != nil {
			return empty, e
		}
		if e = saveAttempt(tx, at, &next, a, action, reason, h.EventNo); e != nil {
			return empty, e
		}
		if e = applyTransition(tx, &o, h); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, Status: o.Status}, nil
	})
}

func (s *Service) MarkBankSubmitted(ctx context.Context, a Actor, trace string, r req.AttemptAction) (res.ActionResult, error) {
	var empty res.ActionResult
	reason, e := safeNote(r.Reason)
	if e != nil || !attemptNoRE.MatchString(r.AttemptNo) {
		return empty, ErrOrderInvalid
	}
	return s.manualCommand(ctx, a, "attempt/bank-submitted", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, at, e := s.lockAttempt(tx, r.AttemptNo)
		if e != nil {
			return empty, e
		}
		if o.Status != "processing" || at.Status != "processing" || at.OperatorID != a.ID || at.BankSubmittedAt != 0 {
			return empty, ErrOrderState
		}
		now := s.now()
		next := at
		next.BankSubmittedAt = now
		next.UpdatedAt = now
		if _, e = auditManual(tx, a, trace, o.PayoutNo, at.AttemptNo, "attempt.bank_submitted", map[string]any{"attemptNo": at.AttemptNo, "reason": reason}, now); e != nil {
			return empty, e
		}
		if e = saveAttempt(tx, at, &next, a, "attempt.bank_submitted", reason, ""); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, Status: o.Status}, nil
	})
}

func (s *Service) SubmitAttempt(ctx context.Context, a Actor, trace string, r req.SubmitAttempt) (res.ActionResult, error) {
	var empty res.ActionResult
	note, e := safeNote(r.OperatorNote)
	if e != nil || !r.ConfirmRealTransfer || !attemptNoRE.MatchString(r.AttemptNo) || !evidenceNoRE.MatchString(r.EvidenceNo) {
		return empty, ErrOrderInvalid
	}
	reference, e := normalizeReference(r.PaymentReference)
	if e != nil {
		return empty, e
	}
	return s.manualCommand(ctx, a, "attempt/submit", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, at, e := s.lockAttempt(tx, r.AttemptNo)
		if e != nil {
			return empty, e
		}
		now := s.now()
		if o.Status != "processing" || at.Status != "processing" || at.OperatorID != a.ID || r.PaymentTime < at.CreatedAt || r.PaymentTime > now+60000 {
			return empty, ErrOrderState
		}
		var proof model.Evidence
		if e = tx.Where("evidence_no=? AND attempt_no=? AND payout_no=? AND created_by=?", r.EvidenceNo, at.AttemptNo, o.PayoutNo, a.ID).Take(&proof).Error; e != nil {
			return empty, ErrEvidence
		}
		if _, e = s.readEvidenceBytes(ctx, proof); e != nil {
			return empty, e
		}
		fp := s.Crypto.digest("payout.payment.reference.v1", []string{at.ChannelAccountNo, reference})
		var used int64
		if e = tx.Model(&model.Attempt{}).Where("channel_account_no=? AND payment_reference_fingerprint=?", at.ChannelAccountNo, fp).Count(&used).Error; e != nil {
			return empty, e
		}
		if used != 0 {
			return empty, ErrOrderConflict
		}
		h, e := transitionIntent(tx, o, a, at.AttemptNo, r.RequestID, "pending_confirmation", note, now)
		if e != nil {
			return empty, e
		}
		next := at
		next.Status = "submitted"
		next.PaymentReference = reference
		next.PaymentReferenceFingerprint = &fp
		next.PaymentTime = r.PaymentTime
		next.EvidenceReference = proof.EvidenceNo
		next.EvidenceHash = proof.SHA256
		next.OperatorNote = note
		next.SubmittedAt = now
		next.UpdatedAt = now
		if next.BankSubmittedAt == 0 {
			next.BankSubmittedAt = now
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, at.AttemptNo, "attempt.submit", map[string]any{"attemptNo": at.AttemptNo, "evidenceNo": proof.EvidenceNo, "evidenceHash": proof.SHA256, "paymentTime": r.PaymentTime, "operatorNote": note}, now, h.EventNo); e != nil {
			return empty, e
		}
		if e = saveAttempt(tx, at, &next, a, "attempt.submit", note, h.EventNo); e != nil {
			return empty, e
		}
		if e = applyTransition(tx, &o, h); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, Status: o.Status}, nil
	})
}

func (s *Service) ReviewAttempt(ctx context.Context, a Actor, trace string, r req.ReviewAttempt, decision string) (res.ActionResult, error) {
	path := "attempt/review-" + decision
	if decision != "success" && decision != "failure" && decision != "unknown" {
		return res.ActionResult{}, ErrOrderInvalid
	}
	return s.reviewAttempt(ctx, a, trace, path, r.RequestID, r, r.AttemptNo, r.Reason, r.ConfirmExternalResult, decision, "")
}
func (s *Service) ResolveAttempt(ctx context.Context, a Actor, trace string, r req.ResolveAttempt) (res.ActionResult, error) {
	if r.Decision != "success" && r.Decision != "failure" {
		return res.ActionResult{}, ErrOrderInvalid
	}
	return s.reviewAttempt(ctx, a, trace, "attempt/resolve-unknown", r.RequestID, r, r.AttemptNo, r.Reason, r.ConfirmExternalResult, r.Decision, r.InvestigationNo)
}
func (s *Service) reviewAttempt(ctx context.Context, a Actor, trace, path, key string, request any, no, rawReason string, confirmed bool, decision, investigation string) (res.ActionResult, error) {
	var empty res.ActionResult
	reason, e := safeNote(rawReason)
	if e != nil || !confirmed || !attemptNoRE.MatchString(no) {
		return empty, ErrOrderInvalid
	}
	resolving := path == "attempt/resolve-unknown"
	return s.manualCommand(ctx, a, path, key, request, func(tx *gorm.DB) (res.ActionResult, error) {
		o, at, e := s.lockAttempt(tx, no)
		if e != nil {
			return empty, e
		}
		if at.OperatorID == a.ID && !system.IsSuperAdmin(a.ID) {
			return empty, ErrSelfReview
		}
		if (!resolving && (o.Status != "pending_confirmation" || at.Status != "submitted")) || (resolving && (o.Status != "unknown" || at.Status != "unknown")) {
			return empty, ErrOrderState
		}
		if at.PaymentReference == "" || at.PaymentReferenceFingerprint == nil || at.EvidenceReference == "" || at.SubmittedAt == 0 {
			return empty, ErrOrderState
		}
		var evidence model.Evidence
		if e = tx.Where("evidence_no=? AND attempt_no=?", at.EvidenceReference, at.AttemptNo).Take(&evidence).Error; e != nil {
			return empty, ErrEvidence
		}
		if evidence.SHA256 != at.EvidenceHash {
			return empty, ErrEvidence
		}
		if _, e = s.readEvidenceBytes(ctx, evidence); e != nil {
			return empty, e
		}
		if resolving {
			var report model.Investigation
			if investigation == "" {
				return empty, ErrInvestigation
			}
			if e = tx.Where("investigation_no=? AND attempt_no=? AND payout_no=? AND actor_id=?", investigation, at.AttemptNo, o.PayoutNo, a.ID).Take(&report).Error; e != nil {
				return empty, ErrInvestigation
			}
			if report.CreatedAt < at.ReviewedAt || report.EvidenceNo == "" || report.EvidenceNo == at.EvidenceReference {
				return empty, ErrInvestigation
			}
			var proof model.Evidence
			if e = tx.Where("evidence_no=? AND attempt_no=? AND created_by=?", report.EvidenceNo, at.AttemptNo, a.ID).Take(&proof).Error; e != nil {
				return empty, ErrEvidence
			}
			if _, e = s.readEvidenceBytes(ctx, proof); e != nil {
				return empty, e
			}
		}
		target, attemptStatus := "unknown", "unknown"
		if decision == "success" {
			target, attemptStatus = "succeeded", "succeeded"
		}
		if decision == "failure" {
			target, attemptStatus = "failed", "failed"
		}
		now := s.now()
		h, e := transitionIntent(tx, o, a, at.AttemptNo, key, target, reason, now)
		if e != nil {
			return empty, e
		}
		action := "attempt.review_" + decision
		if resolving {
			action = "attempt.resolve_unknown"
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, at.AttemptNo, action, map[string]any{"attemptNo": at.AttemptNo, "decision": decision, "reason": reason, "investigationNo": investigation, "superRootException": a.ID == at.OperatorID && system.IsSuperAdmin(a.ID)}, now, h.EventNo); e != nil {
			return empty, e
		}
		next := at
		next.Status = attemptStatus
		next.ReviewerID = a.ID
		next.ReviewReason = reason
		next.ReviewedAt = now
		next.UpdatedAt = now
		if decision != "unknown" {
			next.ActiveSlot = nil
			next.CompletedAt = now
		}
		if decision == "success" {
			slot := 1
			next.SuccessSlot = &slot
		}
		if decision == "failure" {
			next.FailureCode = "confirmed_unpaid"
			next.FailureReason = reason
		}
		if e = saveAttempt(tx, at, &next, a, action, reason, h.EventNo); e != nil {
			return empty, e
		}
		if decision == "success" {
			if s.FinancialFinalize == nil {
				return empty, ErrOrderUnavailable
			}
			if _, e = s.FinancialFinalize(tx, model.FinalizeInput{Order: o, AttemptNo: at.AttemptNo, TransitionNo: h.EventNo, ActorID: a.ID, Paid: true, Now: now}); e != nil {
				return empty, e
			}
		}
		if e = applyTransition(tx, &o, h); e != nil {
			return empty, e
		}
		if decision == "success" {
			if _, e = s.reconcileOrderInTx(tx, a, o, now, true); e != nil {
				return empty, e
			}
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, Status: o.Status}, nil
	})
}
