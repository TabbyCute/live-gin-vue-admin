package gift

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	model "tb_live_module/model/gift"
	live "tb_live_module/model/live"
	"tb_live_module/model/system"
)

// Caller holds gift_control UPDATE. Object locks also serialize anchor removal.
func validateRuleScope(tx *gorm.DB, kind, no string) error {
	var db *gorm.DB
	switch kind {
	case "global":
		if no != "" {
			return ErrInvalid
		}
		return nil
	case "category":
		db = tx.Model(&model.Category{}).Where("category_no=?", no)
	case "gift":
		db = tx.Model(&model.Item{}).Where("gift_no=?", no)
	case "anchor":
		db = tx.Model(&live.LiveAnchor{}).Where("anchor_no=?", no)
	default:
		return ErrInvalid
	}
	var row struct{ ID uint64 }
	e := db.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return e
}

func (s *Service) TerminateRule(ctx context.Context, a Actor, r er.TerminateRule) (em.RuleTermination, error) {
	var out em.RuleTermination
	if !numberPattern.MatchString(r.RuleNo) || !keyPattern.MatchString(r.RequestID) || !textOK(r.Reason, 500) {
		return out, ErrInvalid
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "rule/terminate", "POST"); e != nil {
			return e
		}
		if _, e := lockControl(tx, true); e != nil {
			return e
		}
		e := tx.Where("request_id=?", r.RequestID).Take(&out).Error
		if e == nil {
			if out.RuleNo != r.RuleNo || out.Reason != r.Reason || out.CreatedBy != a.ID {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var rule em.RevenueRule
		if e := tx.Where("rule_no=?", r.RuleNo).Take(&rule).Error; e != nil {
			return ErrNotFound
		}
		if rule.Status != "active" || rule.EffectiveTo <= s.now() {
			return ErrState
		}
		var count int64
		if e := tx.Model(&em.RuleTermination{}).Where("rule_no=? AND status IN ('pending','approved')", r.RuleNo).Count(&count).Error; e != nil {
			return e
		}
		if count != 0 {
			return ErrConflict
		}
		out = em.RuleTermination{TerminationNo: no("rt_"), RequestID: r.RequestID, RuleNo: r.RuleNo, Reason: r.Reason, Status: "pending", CreatedBy: a.ID, CreatedAt: s.now()}
		if e := tx.Create(&out).Error; e != nil {
			return e
		}
		b, _ := json.Marshal(out)
		return audit(tx, a, out.TerminationNo, "rule/terminate", string(b), s.now())
	})
	return out, e
}

func (s *Service) ReviewTermination(ctx context.Context, a Actor, r er.ReviewTermination) (em.RuleTermination, error) {
	var out em.RuleTermination
	if !numberPattern.MatchString(r.TerminationNo) {
		return out, ErrInvalid
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "rule/terminate/review", "POST"); e != nil {
			return e
		}
		if _, e := lockControl(tx, true); e != nil {
			return e
		}
		if e := tx.Where("termination_no=?", r.TerminationNo).Take(&out).Error; e != nil {
			return ErrNotFound
		}
		desired := "rejected"
		if r.Approve {
			desired = "approved"
		}
		if out.Status == desired && out.ReviewedBy == a.ID {
			return nil
		}
		if out.Status != "pending" || (out.CreatedBy == a.ID && !system.IsSuperAdmin(a.ID)) {
			return ErrConflict
		}
		// Invalid scopes may be terminated too: this is the escape path for historical bad rules.
		out.Status = desired
		out.ReviewedBy = a.ID
		out.ReviewedAt = s.now()
		if e := tx.Model(&em.RuleTermination{}).Where("id=? AND status='pending'", out.ID).Updates(map[string]any{"status": desired, "reviewed_by": a.ID, "reviewed_at": out.ReviewedAt}).Error; e != nil {
			return e
		}
		b, _ := json.Marshal(out)
		return audit(tx, a, out.TerminationNo, "rule/terminate/review", string(b), s.now())
	})
	return out, e
}
