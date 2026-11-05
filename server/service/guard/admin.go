package guard

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"tb_live_module/internal/modulekit"
	model "tb_live_module/model/guard"
	req "tb_live_module/model/guard/request"
)

func (s *Service) authorize(tx *gorm.DB, a Actor, path, method string) error {
	return modulekit.Authorize(tx, a, "guard_admin_permission", "/guard/"+path, method)
}
func (s *Service) actions(tx *gorm.DB, a *Actor) map[string]bool {
	if a == nil {
		return nil
	}
	out := map[string]bool{}
	for name, path := range map[string]string{"save": "plan/save", "retry": "task/retry", "reconcile": "reconcile", "rebuild": "rebuild"} {
		out[name] = s.authorize(tx, *a, path, "POST") == nil
	}
	return out
}
func (s *Service) adminWrite(ctx context.Context, a Actor, path, key string, input any, fn func(*gorm.DB) (any, error)) (string, error) {
	if !modulekit.RequestID.MatchString(key) {
		return "", ErrInvalid
	}
	raw, _ := json.Marshal(input)
	result := ""
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, path, "POST"); e != nil {
			return e
		}
		// Serialize catalog/admin idempotency without holding any wallet/entitlement locks.
		var lock model.Task
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_no='guard-admin-control'").Take(&lock).Error; e != nil {
			return e
		}
		var audit model.Audit
		e := tx.Where("request_id=?", key).Take(&audit).Error
		if e == nil {
			if audit.OperatorID != a.ID || audit.Action != path || audit.Input != string(raw) {
				return ErrConflict
			}
			result = audit.Result
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		out, e := fn(tx)
		if e != nil {
			return e
		}
		b, e := json.Marshal(out)
		if e != nil {
			return e
		}
		result = string(b)
		return tx.Create(&model.Audit{RequestID: key, OperatorID: a.ID, Action: path, Input: string(raw), Result: result, ActorIP: a.IP, CreatedAt: s.now()}).Error
	})
	return result, e
}
func (s *Service) SavePlan(ctx context.Context, a Actor, r req.Plan) (string, error) {
	days, e := strconv.ParseInt(r.DefaultDurationDays, 10, 64)
	var config map[string]any
	if e != nil || days < 1 || days > 3650 || strconv.FormatInt(days, 10) != r.DefaultDurationDays || !modulekit.Number.MatchString(r.PlanNo) || len(r.Name) < 1 || len(r.Name) > 64 || strings.TrimSpace(r.Name) != r.Name || r.Level < 1 || r.Level > 1000 || r.Priority < 0 || r.Priority > 100000 || r.SortOrder < 0 || r.SortOrder > 100000 || len(r.BenefitConfig) > 4096 || json.Unmarshal([]byte(r.BenefitConfig), &config) != nil || config == nil || (r.Status != "enabled" && r.Status != "disabled") {
		return "", ErrInvalid
	}
	return s.adminWrite(ctx, a, "plan/save", r.RequestID, r, func(tx *gorm.DB) (any, error) {
		var p model.Plan
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("plan_no=?", r.PlanNo).Take(&p).Error
		if e == nil {
			if r.Version != strconv.FormatInt(p.Version, 10) {
				return nil, ErrConflict
			}
		} else if errors.Is(e, gorm.ErrRecordNotFound) {
			if r.Version != "" {
				return nil, ErrConflict
			}
			p = model.Plan{PlanNo: r.PlanNo, CreatedBy: a.ID, CreatedAt: s.now()}
		} else {
			return nil, e
		}
		p.Name = r.Name
		p.Level = r.Level
		p.Priority = r.Priority
		p.DefaultDurationDays = days
		p.BenefitConfig = r.BenefitConfig
		p.Status = r.Status
		p.SortOrder = r.SortOrder
		p.Version++
		p.UpdatedBy = a.ID
		p.UpdatedAt = s.now()
		return p, tx.Save(&p).Error
	})
}
func (s *Service) AdminAction(ctx context.Context, a Actor, path string, r req.Action) (string, error) {
	if !modulekit.Number.MatchString(r.Number) {
		return "", ErrInvalid
	}
	if path != "task/retry" && path != "reconcile" && path != "rebuild" {
		return "", ErrInvalid
	}
	return s.adminWrite(ctx, a, path, r.RequestID, r, func(tx *gorm.DB) (any, error) {
		if path == "task/retry" {
			var t model.Task
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_no=? AND kind IN ('grant','compensate','expire')", r.Number).Take(&t).Error; e != nil {
				return nil, e
			}
			if t.Status != "failed" || t.LeaseUntil > s.now() {
				return nil, ErrConflict
			}
			return map[string]any{"queued": true}, tx.Model(&t).Updates(map[string]any{"status": "pending", "retry_count": 0, "next_retry_at": s.now(), "last_error": "", "updated_at": s.now()}).Error
		}
		var m model.Membership
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("membership_no=?", r.Number).Take(&m).Error; e != nil {
			return nil, e
		}
		g, c, e := loadFacts(tx, m)
		if e != nil {
			return nil, e
		}
		segments, e := Replay(g, c)
		if e != nil {
			return nil, e
		}
		b, _ := json.Marshal(segments)
		consistent := m.Projection == string(b)
		if path == "rebuild" {
			e = s.saveProjection(tx, &m, "rebuild", r.RequestID, "", 0, s.now())
		}
		return map[string]any{"membershipNo": m.MembershipNo, "consistentBefore": consistent, "consistent": consistent || (path == "rebuild" && e == nil), "rebuilt": path == "rebuild"}, e
	})
}
