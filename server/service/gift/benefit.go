package gift

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"strconv"
	"tb_live_module/internal/modulekit"
	model "tb_live_module/model/gift"
	guard "tb_live_module/model/guard"
)

func grantCommand(g model.BenefitGrant) (guard.GrantInput, error) {
	var snap guard.BenefitSnapshot
	if json.Unmarshal([]byte(g.BenefitSnapshot), &snap) != nil {
		return guard.GrantInput{}, ErrInvariant
	}
	days, e := strconv.ParseInt(snap.TotalDurationDays, 10, 64)
	if e != nil {
		return guard.GrantInput{}, ErrInvariant
	}
	return guard.GrantInput{GrantNo: g.GrantNo, GiftOrderNo: g.GiftOrderNo, UserID: g.ReceiverUserID, AnchorID: g.AnchorID, PlanNo: snap.GuardPlanNo, DurationDays: days, BenefitSnapshot: g.BenefitSnapshot}, nil
}

// Pending facts are scanned with task backoff. No provider invocation is made
// inside the original order/refund transaction or while wallet locks are held.
func (s *Service) ProcessBenefits(ctx context.Context, limit int) (int, error) {
	if s.Guard == nil {
		return 0, ErrGuard
	}
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	count := 0
	var grants []model.BenefitGrant
	ready := "NOT EXISTS (SELECT 1 FROM guard_task t WHERE t.source_no=gift_benefit_grant.grant_no AND (t.status='succeeded' OR t.retry_count>=20 OR t.next_retry_at>? OR (t.status='processing' AND t.lease_until>?)))"
	if e := s.db(ctx).Where("status IN ('pending','failed','compensating')").Where(ready, s.now(), s.now()).Order("id").Limit(limit).Find(&grants).Error; e != nil {
		return count, e
	}
	var first error
	for _, g := range grants {
		e := s.Guard.RunTask(ctx, "grant", g.GrantNo, func(call context.Context) error {
			in, e := grantCommand(g)
			if e != nil {
				return e
			}
			if e = s.Guard.Fulfill(call, in); e != nil {
				return e
			}
			return s.db(call).Model(&model.BenefitGrant{}).Where("grant_no=? AND status IN ('pending','failed')", g.GrantNo).Updates(map[string]any{"status": "succeeded", "last_error": "", "updated_at": s.now()}).Error
		})
		if e == nil {
			count++
		} else if !errors.Is(e, modulekit.Conflict) && first == nil {
			first = e
		}
	}
	var comps []model.BenefitCompensation
	ready = "NOT EXISTS (SELECT 1 FROM guard_task t WHERE t.source_no=gift_benefit_compensation.compensation_no AND (t.status='succeeded' OR t.retry_count>=20 OR t.next_retry_at>? OR (t.status='processing' AND t.lease_until>?)))"
	if e := s.db(ctx).Where("status <> 'succeeded'").Where(ready, s.now(), s.now()).Order("id").Limit(limit).Find(&comps).Error; e != nil {
		return count, e
	}
	for _, c := range comps {
		e := s.Guard.RunTask(ctx, "compensate", c.CompensationNo, func(call context.Context) error {
			var g model.BenefitGrant
			if e := s.db(call).Where("gift_order_no=? AND benefit_type='anchor_guard'", c.GiftOrderNo).Take(&g).Error; e != nil {
				return e
			}
			in, e := grantCommand(g)
			if e != nil {
				return e
			}
			if g.ReceiverUserID != c.ReceiverUserID || g.AnchorID != c.AnchorID {
				return ErrInvariant
			}
			if e = s.Guard.Revoke(call, guard.CompensationInput{Grant: in, CompensationNo: c.CompensationNo, RefundNo: c.RefundNo, DurationDays: c.DurationDays}); e != nil {
				return e
			}
			// The existing compensation trigger requires the claimed state before acknowledgement.
			return s.transaction(call, func(tx *gorm.DB) error {
				var current model.BenefitCompensation
				if e := tx.Where("id=?", c.ID).Take(&current).Error; e != nil {
					return e
				}
				if current.Status == "succeeded" {
					return nil
				}
				now := s.now()
				if e := tx.Model(&current).Updates(map[string]any{"status": "processing", "retry_count": current.RetryCount + 1, "lease_until": now + 60000, "updated_at": now, "completed_at": 0, "last_error": ""}).Error; e != nil {
					return e
				}
				return tx.Model(&current).Updates(map[string]any{"status": "succeeded", "lease_until": 0, "updated_at": now, "completed_at": now, "last_error": ""}).Error
			})
		})
		if e == nil {
			count++
		} else if !errors.Is(e, modulekit.Conflict) && first == nil {
			first = e
		}
	}
	return count, first
}
