package gift

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"math"
	"strconv"
	earning "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	model "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	"tb_live_module/model/system"
	es "tb_live_module/service/earning"
)

func (s *Service) SaveCategory(ctx context.Context, a Actor, r req.Category) (model.Category, error) {
	var out model.Category
	if !numberPattern.MatchString(r.CategoryNo) || !textOK(r.Name, 64) || !urlOK(r.IconURL, true) || len(r.Description) > 500 || (r.Status != "enabled" && r.Status != "disabled") {
		return out, ErrInvalid
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "category/save", "POST"); e != nil {
			return e
		}
		if _, e := lockControl(tx, true); e != nil {
			return e
		}
		e := tx.Where("category_no=?", r.CategoryNo).Take(&out).Error
		if e == nil {
			if r.Version != strconv.FormatInt(out.Version, 10) || out.Version == math.MaxInt64 {
				return ErrConflict
			}
			out.Version++
		} else if errors.Is(e, gorm.ErrRecordNotFound) {
			if r.Version != "" {
				return ErrConflict
			}
			out = model.Category{CategoryNo: r.CategoryNo, CreatedAt: s.now(), Version: 1}
		} else {
			return e
		}
		out.Name = r.Name
		out.IconURL = r.IconURL
		out.Description = r.Description
		out.SortOrder = r.SortOrder
		out.Status = r.Status
		out.UpdatedAt = s.now()
		if e = tx.Save(&out).Error; e != nil {
			return e
		}
		if e = bump(tx); e != nil {
			return e
		}
		b, _ := json.Marshal(out)
		return audit(tx, a, out.CategoryNo, "category/save", string(b), s.now())
	})
	return out, e
}
func (s *Service) SaveItem(ctx context.Context, a Actor, r req.Item) (model.Item, error) {
	var out model.Item
	price, e := integer(r.CoinPrice, false)
	quantity, qe := integer(r.MaxQuantityPerOrder, false)
	if e != nil || qe != nil || quantity > 10000 || !numberPattern.MatchString(r.GiftNo) || !numberPattern.MatchString(r.CategoryNo) || !textOK(r.Name, 64) || !urlOK(r.IconURL, r.Status == "draft") || !urlOK(r.AnimationURL, true) || len(r.Description) > 500 {
		return out, ErrInvalid
	}
	if r.Status != "draft" && r.Status != "enabled" && r.Status != "disabled" {
		return out, ErrInvalid
	}
	if r.AnimationType != "none" && r.AnimationType != "svga" && r.AnimationType != "mp4" && r.AnimationType != "lottie" {
		return out, ErrInvalid
	}
	if (r.AnimationType == "none") != (r.AnimationURL == "") {
		return out, ErrInvalid
	}
	if r.BenefitType == "" {
		r.BenefitType = "none"
	}
	if r.BenefitStatus == "" {
		r.BenefitStatus = "disabled"
	}
	if r.BenefitStatus != "disabled" && (r.BenefitStatus != "enabled" || !s.GuardEnabled || s.Guard == nil) {
		return out, ErrGuard
	}
	var days int64
	if r.BenefitType == "anchor_guard" {
		days, e = integer(r.GuardDurationDays, false)
		if e != nil || days > 3650 || !r.IsPremium || !numberPattern.MatchString(r.GuardPlanNo) || r.EffectiveTo <= r.EffectiveFrom {
			return out, ErrInvalid
		}
	} else if r.BenefitType != "none" || r.GuardPlanNo != "" {
		return out, ErrInvalid
	}
	e = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "item/save", "POST"); e != nil {
			return e
		}
		if _, e := lockControl(tx, true); e != nil {
			return e
		}
		if r.BenefitType == "anchor_guard" && s.Guard != nil {
			if _, e := s.Guard.PlanSnapshotInTx(tx, r.GuardPlanNo); e != nil {
				return ErrGuard
			}
		}
		var c model.Category
		if e := tx.Where("category_no=?", r.CategoryNo).Take(&c).Error; e != nil {
			return ErrState
		}
		e := tx.Where("gift_no=?", r.GiftNo).Take(&out).Error
		if e == nil {
			if r.Version != strconv.FormatInt(out.Version, 10) || out.Version == math.MaxInt64 {
				return ErrConflict
			}
			out.Version++
		} else if errors.Is(e, gorm.ErrRecordNotFound) {
			if r.Version != "" {
				return ErrConflict
			}
			out = model.Item{GiftNo: r.GiftNo, Version: 1, CreatedAt: s.now()}
		} else {
			return e
		}
		out.CategoryNo = r.CategoryNo
		out.Name = r.Name
		out.IconURL = r.IconURL
		out.AnimationURL = r.AnimationURL
		out.AnimationType = r.AnimationType
		out.CoinPrice = price
		out.SortOrder = r.SortOrder
		out.Status = r.Status
		out.IsPremium = r.IsPremium
		out.MaxQuantityPerOrder = quantity
		out.Description = r.Description
		out.BenefitType = r.BenefitType
		out.GuardPlanNo = r.GuardPlanNo
		out.GuardDurationDays = days
		out.BenefitStatus = r.BenefitStatus
		out.EffectiveFrom = r.EffectiveFrom
		out.EffectiveTo = r.EffectiveTo
		out.UpdatedAt = s.now()
		if e = tx.Save(&out).Error; e != nil {
			return e
		}
		if e = bump(tx); e != nil {
			return e
		}
		b, _ := json.Marshal(out)
		return audit(tx, a, out.GiftNo, "item/save", string(b), s.now())
	})
	return out, e
}
func (s *Service) Sort(ctx context.Context, a Actor, r req.Sort) error {
	if len(r.Numbers) > 1000 || len(r.Numbers) == 0 {
		return ErrInvalid
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "sort", "POST"); e != nil {
			return e
		}
		c, e := lockControl(tx, true)
		if e != nil {
			return e
		}
		if strconv.FormatInt(c.Version, 10) != r.Version {
			return ErrConflict
		}
		var numbers []string
		var table, column string
		switch r.Kind {
		case "category":
			table = "gift_category"
			column = "category_no"
			e = tx.Model(&model.Category{}).Pluck(column, &numbers).Error
		case "item":
			table = "gift_item"
			column = "gift_no"
			if !numberPattern.MatchString(r.CategoryNo) {
				return ErrInvalid
			}
			e = tx.Model(&model.Item{}).Where("category_no=?", r.CategoryNo).Pluck(column, &numbers).Error
		default:
			return ErrInvalid
		}
		if e != nil {
			return e
		}
		if len(numbers) != len(r.Numbers) {
			return ErrConflict
		}
		allowed := map[string]bool{}
		for _, n := range numbers {
			allowed[n] = true
		}
		for i, n := range r.Numbers {
			if !allowed[n] {
				return ErrInvalid
			}
			delete(allowed, n)
			result := tx.Table(table).Where(column+"=? AND version < ?", n, int64(math.MaxInt64)).Updates(map[string]any{"sort_order": i, "version": gorm.Expr("version+1"), "updated_at": s.now()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrConflict
			}
		}
		if e = bump(tx); e != nil {
			return e
		}
		b, _ := json.Marshal(r)
		return audit(tx, a, r.Kind, "sort", string(b), s.now())
	})
}
func (s *Service) Delete(ctx context.Context, a Actor, r req.Delete) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "delete", "POST"); e != nil {
			return e
		}
		if _, e := lockControl(tx, true); e != nil {
			return e
		}
		var count int64
		var result *gorm.DB
		v, e := integer(r.Version, false)
		if e != nil {
			return e
		}
		scope := r.Kind
		if scope == "item" {
			scope = "gift"
		}
		// Keep every referenced number reserved, including rejected/terminated history.
		if e = tx.Model(&earning.RevenueRule{}).Where("scope_type=? AND scope_no=?", scope, r.No).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return ErrConflict
		}
		switch r.Kind {
		case "category":
			if e = tx.Model(&model.Item{}).Where("category_no=?", r.No).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 {
				return ErrConflict
			}
			if e = tx.Model(&model.Order{}).Where("category_no=?", r.No).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 {
				return ErrConflict
			}
			result = tx.Where("category_no=? AND version=?", r.No, v).Delete(&model.Category{})
		case "item":
			if e = tx.Model(&model.Order{}).Where("gift_no=?", r.No).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 {
				return ErrConflict
			}
			result = tx.Where("gift_no=? AND version=?", r.No, v).Delete(&model.Item{})
		default:
			return ErrInvalid
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		if e = bump(tx); e != nil {
			return e
		}
		return audit(tx, a, r.No, "delete", r.Kind, s.now())
	})
}
func (s *Service) CreateRule(ctx context.Context, a Actor, r er.Rule) (earning.RevenueRule, error) {
	var out earning.RevenueRule
	bps, e := integer(r.AnchorShareBPS, true)
	if e != nil || bps > 10000 || !textOK(r.Name, 64) || r.EffectiveTo <= r.EffectiveFrom || r.EffectiveFrom < 0 {
		return out, ErrInvalid
	}
	if r.ScopeType == "global" {
		if r.ScopeNo != "" {
			return out, ErrInvalid
		}
	} else if !numberPattern.MatchString(r.ScopeNo) || (r.ScopeType != "category" && r.ScopeType != "gift" && r.ScopeType != "anchor") {
		return out, ErrInvalid
	}
	e = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "rule/create", "POST"); e != nil {
			return e
		}
		if _, e := lockControl(tx, true); e != nil {
			return e
		}
		if e := validateRuleScope(tx, r.ScopeType, r.ScopeNo); e != nil {
			return e
		}
		var version int64
		if e := tx.Model(&earning.RevenueRule{}).Where("scope_type=? AND scope_no=?", r.ScopeType, r.ScopeNo).Select("COALESCE(MAX(version),0)").Scan(&version).Error; e != nil {
			return e
		}
		if version == math.MaxInt64 {
			return ErrConflict
		}
		out = earning.RevenueRule{RuleNo: no("rr_"), Name: r.Name, ScopeType: r.ScopeType, ScopeNo: r.ScopeNo, Priority: r.Priority, AnchorShareBPS: bps, EffectiveFrom: r.EffectiveFrom, EffectiveTo: r.EffectiveTo, Status: "pending", Version: version + 1, CreatedBy: a.ID, CreatedAt: s.now()}
		if e := tx.Create(&out).Error; e != nil {
			return e
		}
		b, _ := json.Marshal(out)
		return audit(tx, a, out.RuleNo, "rule/create", string(b), s.now())
	})
	return out, e
}
func (s *Service) ReviewRule(ctx context.Context, a Actor, r er.Review) (earning.RevenueRule, error) {
	var out earning.RevenueRule
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "rule/review", "POST"); e != nil {
			return e
		}
		if _, e := lockControl(tx, true); e != nil {
			return e
		}
		if e := tx.Where("rule_no=?", r.RuleNo).Take(&out).Error; e != nil {
			return ErrNotFound
		}
		desired := "rejected"
		if r.Approve {
			desired = "active"
		}
		if out.Status == desired && out.ApprovedBy == a.ID {
			return nil
		}
		if out.Status != "pending" || (out.CreatedBy == a.ID && !system.IsSuperAdmin(a.ID)) {
			return es.ErrRuleConflict
		}
		if r.Approve {
			if e := validateRuleScope(tx, out.ScopeType, out.ScopeNo); e != nil {
				return e
			}
			var count int64
			if e := tx.Model(&earning.RevenueRule{}).Where("status='active' AND scope_type=? AND scope_no=? AND effective_from < ? AND effective_to > ?", out.ScopeType, out.ScopeNo, out.EffectiveTo, out.EffectiveFrom).
				Where("NOT EXISTS (SELECT 1 FROM anchor_rule_termination t WHERE t.rule_no=anchor_revenue_rule.rule_no AND t.status='approved' AND t.reviewed_at <= ?)", out.EffectiveFrom).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 || out.EffectiveTo <= s.now() {
				return es.ErrRuleConflict
			}
		}
		out.Status = desired
		out.ApprovedBy = a.ID
		out.ApprovedAt = s.now()
		if e := tx.Model(&earning.RevenueRule{}).Where("id=?", out.ID).Updates(map[string]any{"status": out.Status, "approved_by": a.ID, "approved_at": out.ApprovedAt}).Error; e != nil {
			return e
		}
		b, _ := json.Marshal(out)
		return audit(tx, a, out.RuleNo, "rule/review", string(b), s.now())
	})
	return out, e
}
