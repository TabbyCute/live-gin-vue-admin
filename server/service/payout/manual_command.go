package payout

import (
	"context"
	"encoding/json"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"regexp"
	"strconv"
	"strings"
	model "tb_live_module/model/payout"
	res "tb_live_module/model/payout/response"
	"unicode"
)

var (
	ErrSelfReview    = &Error{49207, "操作人与复核人必须不同"}
	ErrReveal        = &Error{49208, "查看凭证无效、已使用或已过期，请重新申请"}
	ErrEvidence      = &Error{49209, "私有凭证不可用、格式不符或无权访问"}
	ErrChannelLimit  = &Error{49210, "公司账户不可用或超过平台风控限额"}
	ErrInvestigation = &Error{49211, "必须提供独立的调查记录和新凭证才能解决未知结果"}
)
var companyNoRE = regexp.MustCompile(`^pca_[a-f0-9]{32}$`)
var attemptNoRE = regexp.MustCompile(`^pat_[a-f0-9]{32}$`)
var evidenceNoRE = regexp.MustCompile(`^pev_[a-f0-9]{32}$`)
var uuidReferenceRE = regexp.MustCompile(`(?i)^[a-f0-9]{8}-?[a-f0-9]{4}-?[a-f0-9]{4}-?[a-f0-9]{4}-?[a-f0-9]{12}$`)
var referenceRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9./_-]{5,127}$`)
var noteDigits = regexp.MustCompile(`[0-9]{6,}`)
var noteContact = regexp.MustCompile(`[A-Za-z0-9_.+%-]+@[A-Za-z0-9.-]+|https?://\S+`)

func money(v string) (int64, error) {
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != v {
		return 0, ErrOrderInvalid
	}
	return n, nil
}
func safeNote(v string) (string, error) {
	if !clean(v, 3, 500) {
		return "", ErrOrderInvalid
	}
	return noteDigits.ReplaceAllString(noteContact.ReplaceAllString(v, "[已脱敏]"), "[已脱敏]"), nil
}
func normalizeReference(v string) (string, error) {
	if len(v) > 256 || strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return "", ErrOrderInvalid
	}
	var b strings.Builder
	for _, r := range v {
		if !unicode.IsSpace(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	x := b.String()
	if !referenceRE.MatchString(x) || uuidReferenceRE.MatchString(x) {
		return "", ErrOrderInvalid
	}
	return x, nil
}

// Response caches never contain plaintext account data, bearer capabilities,
// evidence bytes or storage paths. Every replay repeats current authorization.
func (s *Service) manualCommand(ctx context.Context, a Actor, path, requestID string, request any, fn func(*gorm.DB) (res.ActionResult, error)) (res.ActionResult, error) {
	var out res.ActionResult
	if !requestRE.MatchString(requestID) {
		return out, ErrOrderInvalid
	}
	if s.Crypto == nil {
		return out, ErrOrderUnavailable
	}
	digest := s.Crypto.digest("payout.manual.request.v1", []any{a.ID, path, request})
	for attempt := 0; attempt < 3; attempt++ {
		out = res.ActionResult{}
		e := s.transaction(ctx, func(tx *gorm.DB) error {
			if e := s.authorizePayout(tx, a, path, "POST"); e != nil {
				return e
			}
			var old model.ManualRequest
			e := tx.Where("request_id=?", requestID).Take(&old).Error
			if e == nil {
				if old.ActorID != a.ID || old.Action != path || old.RequestHash != digest {
					return ErrOrderConflict
				}
				if json.Unmarshal([]byte(old.Response), &out) != nil {
					return ErrOrderUnavailable
				}
				return nil
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			out, e = fn(tx)
			if e != nil {
				return e
			}
			b, e := json.Marshal(out)
			if e != nil {
				return e
			}
			return tx.Create(&model.ManualRequest{RequestID: requestID, RequestHash: digest, ActorID: a.ID, Action: path, Response: string(b), CreatedAt: s.now()}).Error
		})
		var me *driver.MySQLError
		// A concurrent identical request may finish while this transaction waits
		// for the financial lock. Resolve its committed receipt after rollback;
		// never commit any partial work merely because a receipt appeared.
		if e != nil && ctx.Err() == nil {
			var replay res.ActionResult
			lookup := s.transaction(ctx, func(tx *gorm.DB) error {
				if err := s.authorizePayout(tx, a, path, "POST"); err != nil {
					return err
				}
				var receipt model.ManualRequest
				if err := tx.Where("request_id=?", requestID).Take(&receipt).Error; err != nil {
					return err
				}
				if receipt.ActorID != a.ID || receipt.Action != path || receipt.RequestHash != digest {
					return ErrOrderConflict
				}
				if json.Unmarshal([]byte(receipt.Response), &replay) != nil {
					return ErrOrderUnavailable
				}
				return nil
			})
			if lookup == nil {
				return replay, nil
			}
			if errors.Is(lookup, ErrOrderConflict) || errors.Is(lookup, ErrOrderForbidden) {
				return res.ActionResult{}, lookup
			}
		}
		if e == nil || !errors.As(e, &me) || me.Number != 1062 {
			return out, e
		}
	}
	return out, ErrOrderConflict
}
func auditManual(tx *gorm.DB, a Actor, trace, no, object, action string, value any, now int64, transition ...string) (string, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	row := model.OrderAudit{EventNo: number("poa_"), PayoutNo: no, ObjectNo: object, Action: action, ActorID: a.ID, TraceID: trace, Snapshot: string(b), CreatedAt: now}
	if len(transition) > 0 {
		row.TransitionNo = transition[0]
	}
	return row.EventNo, tx.Create(&row).Error
}
func (s *Service) lockOrder(tx *gorm.DB, no string) (model.Order, error) {
	var o model.Order
	if !orderNoRE.MatchString(no) || s.FinancialLock == nil {
		return o, ErrOrderInvalid
	}
	if e := tx.Where("payout_no=?", no).Take(&o).Error; e != nil {
		return o, e
	}
	w, e := s.FinancialLock(tx, o.BusinessNo)
	if e != nil {
		return o, e
	}
	if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("payout_no=?", no).Take(&o).Error; e != nil {
		return o, e
	}
	if o.Amount != w.Amount || o.Currency != w.Currency || o.AnchorID != w.AnchorID || o.AnchorNo != w.AnchorNo || o.PayeeAccountNo != w.PayeeRef || o.PayeeVersion != w.PayeeVersion || o.AccountMasked != w.AccountMasked {
		return o, ErrOrderState
	}
	return o, s.VerifyOrder(o)
}
func (s *Service) lockAttempt(tx *gorm.DB, no string) (model.Order, model.Attempt, error) {
	var at model.Attempt
	var o model.Order
	if !attemptNoRE.MatchString(no) {
		return o, at, ErrOrderInvalid
	}
	if e := tx.Where("attempt_no=?", no).Take(&at).Error; e != nil {
		return o, at, e
	}
	o, e := s.lockOrder(tx, at.PayoutNo)
	if e != nil {
		return o, at, e
	}
	if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("attempt_no=?", no).Take(&at).Error; e != nil {
		return o, at, e
	}
	if at.PayoutNo != o.PayoutNo || at.Amount != o.Amount || at.Currency != o.Currency || at.PayeeSnapshotHash != o.PayeeSnapshotHash {
		return o, at, ErrOrderState
	}
	return o, at, nil
}
func transitionIntent(tx *gorm.DB, o model.Order, a Actor, attempt, requestID, target, reason string, now int64) (model.StatusHistory, error) {
	var h model.StatusHistory
	if !orderTransitions[o.Status][target] || o.StateVersion == math.MaxInt64 || now < o.UpdatedAt {
		return h, ErrOrderState
	}
	h = model.StatusHistory{EventNo: number("psh_"), PayoutNo: o.PayoutNo, Sequence: o.StateVersion + 1, AttemptNo: attempt, RequestID: requestID, FromStatus: o.Status, ToStatus: target, Reason: reason, ActorID: a.ID, CreatedAt: now}
	return h, tx.Create(&h).Error
}
func applyTransition(tx *gorm.DB, o *model.Order, h model.StatusHistory) error {
	r := tx.Model(&model.Order{}).Where("id=? AND status=? AND state_version=?", o.ID, o.Status, o.StateVersion).Updates(map[string]any{"status": h.ToStatus, "state_version": h.Sequence, "last_transition_no": h.EventNo, "updated_at": h.CreatedAt})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrOrderConflict
	}
	o.Status = h.ToStatus
	o.StateVersion = h.Sequence
	o.LastTransitionNo = h.EventNo
	o.UpdatedAt = h.CreatedAt
	return nil
}

// The event records all mutable target values. Database triggers compare the
// update to this append-only evidence; integrity checks also verify its hash.
func attemptValues(at model.Attempt) map[string]any {
	fp := ""
	if at.PaymentReferenceFingerprint != nil {
		fp = *at.PaymentReferenceFingerprint
	}
	return map[string]any{"version": strconv.FormatInt(at.Version, 10), "status": at.Status, "reviewerId": strconv.FormatUint(at.ReviewerID, 10), "paymentReference": at.PaymentReference, "paymentFingerprint": fp, "paymentTime": strconv.FormatInt(at.PaymentTime, 10), "evidenceReference": at.EvidenceReference, "evidenceHash": at.EvidenceHash, "operatorNote": at.OperatorNote, "reviewReason": at.ReviewReason, "bankSubmittedAt": strconv.FormatInt(at.BankSubmittedAt, 10), "submittedAt": strconv.FormatInt(at.SubmittedAt, 10), "reviewedAt": strconv.FormatInt(at.ReviewedAt, 10), "completedAt": strconv.FormatInt(at.CompletedAt, 10), "failureCode": at.FailureCode, "failureReason": at.FailureReason, "updatedAt": strconv.FormatInt(at.UpdatedAt, 10)}
}
func saveAttempt(tx *gorm.DB, old model.Attempt, next *model.Attempt, a Actor, action, reason, transition string) error {
	if old.Version == math.MaxInt64 {
		return ErrOrderState
	}
	next.Version = old.Version + 1
	b, e := json.Marshal(attemptValues(*next))
	if e != nil {
		return e
	}
	event := model.AttemptEvent{EventNo: number("pae_"), AttemptNo: next.AttemptNo, PayoutNo: next.PayoutNo, Version: next.Version, Action: action, FromStatus: old.Status, ToStatus: next.Status, ActorID: a.ID, Reason: reason, TransitionNo: transition, Snapshot: string(b), SnapshotHash: snapshotHash(string(b)), CreatedAt: next.UpdatedAt}
	if e = tx.Create(&event).Error; e != nil {
		return e
	}
	next.LastEventNo = event.EventNo
	if old.ID == 0 {
		return tx.Create(next).Error
	}
	updates := map[string]any{"version": next.Version, "status": next.Status, "active_slot": next.ActiveSlot, "success_slot": next.SuccessSlot, "reviewer_id": next.ReviewerID, "payment_reference": next.PaymentReference, "payment_reference_fingerprint": next.PaymentReferenceFingerprint, "payment_time": next.PaymentTime, "evidence_reference": next.EvidenceReference, "evidence_hash": next.EvidenceHash, "operator_note": next.OperatorNote, "review_reason": next.ReviewReason, "bank_submitted_at": next.BankSubmittedAt, "submitted_at": next.SubmittedAt, "reviewed_at": next.ReviewedAt, "completed_at": next.CompletedAt, "failure_code": next.FailureCode, "failure_reason": next.FailureReason, "updated_at": next.UpdatedAt, "last_event_no": next.LastEventNo}
	r := tx.Model(&model.Attempt{}).Where("id=? AND version=? AND status=?", old.ID, old.Version, old.Status).Updates(updates)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrOrderConflict
	}
	return nil
}
