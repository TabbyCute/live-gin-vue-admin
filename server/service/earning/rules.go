package earning

import (
	"context"
	"errors"
	"gorm.io/gorm"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	"tb_live_module/model/system"
)

func (s *Service) CreateExchangeRule(ctx context.Context, a Actor, r req.ExchangeRule) (model.SettlementRule, error) {
	var out model.SettlementRule
	coin, e := integer(r.CoinUnits, false)
	if e != nil {
		return out, e
	}
	minor, e := integer(r.MinorUnits, false)
	if e != nil {
		return out, e
	}
	if !keyRE.MatchString(r.RequestID) || !clean(r.Name, 80) || r.SourceUnit != "COIN" || !currencyRE.MatchString(r.Currency) || r.Currency == "COIN" || r.RoundingMode != "floor" || r.EffectiveFrom < 0 || r.EffectiveTo <= r.EffectiveFrom {
		return out, ErrInvalid
	}
	digest := hash(struct {
		R     req.ExchangeRule
		Actor uint64
	}{r, a.ID})
	e = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "exchange/create", "POST"); e != nil {
			return e
		}
		if e := lockRules(tx, true); e != nil {
			return e
		}
		e := tx.Where("request_id=?", r.RequestID).Take(&out).Error
		if e == nil {
			if out.RequestHash != digest {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var version int64
		if e = tx.Model(&model.SettlementRule{}).Where("source_unit=? AND currency=?", r.SourceUnit, r.Currency).Select("COALESCE(MAX(version),0)").Scan(&version).Error; e != nil {
			return e
		}
		version, e = add(version, 1)
		if e != nil {
			return e
		}
		out = model.SettlementRule{RuleNo: number("er_"), RequestID: r.RequestID, RequestHash: digest, Name: r.Name, SourceUnit: r.SourceUnit, Currency: r.Currency, CoinUnits: coin, MinorUnits: minor, RoundingMode: r.RoundingMode, EffectiveFrom: r.EffectiveFrom, EffectiveTo: r.EffectiveTo, Status: "pending", Version: version, CreatedBy: a.ID, CreatedAt: s.now()}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		return audit(tx, a, out.RuleNo, "exchange.create", encode(out), s.now())
	})
	return out, e
}
func (s *Service) ReviewExchangeRule(ctx context.Context, a Actor, r req.FinanceReview) (model.SettlementRule, error) {
	var out model.SettlementRule
	if !noRE.MatchString(r.No) || !clean(r.Reason, 500) {
		return out, ErrInvalid
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "exchange/review", "POST"); e != nil {
			return e
		}
		if e := lockRules(tx, true); e != nil {
			return e
		}
		if e := tx.Where("rule_no=?", r.No).Take(&out).Error; e != nil {
			return ErrNotFound
		}
		want := "rejected"
		if r.Approve {
			want = "active"
		}
		if out.Status != "pending" {
			if out.Status == want && out.ApprovedBy == a.ID && out.ReviewReason == r.Reason {
				return nil
			}
			return ErrConflict
		}
		if out.CreatedBy == a.ID && !system.IsSuperAdmin(a.ID) {
			return ErrForbidden
		}
		if r.Approve {
			var n int64
			if out.EffectiveTo <= s.now() {
				return ErrExchange
			}
			if e := tx.Model(&model.SettlementRule{}).Where("status='active' AND source_unit=? AND currency=? AND effective_from<? AND effective_to>?", out.SourceUnit, out.Currency, out.EffectiveTo, out.EffectiveFrom).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return ErrExchange
			}
		}
		approvedAt := s.now()
		result := tx.Model(&model.SettlementRule{}).Where("id=? AND status='pending'", out.ID).Updates(map[string]any{"status": want, "approved_by": a.ID, "approved_at": approvedAt, "review_reason": r.Reason})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		out.Status = want
		out.ApprovedBy = a.ID
		out.ApprovedAt = approvedAt
		out.ReviewReason = r.Reason
		return audit(tx, a, out.RuleNo, "exchange.review", encode(out), s.now())
	})
	return out, e
}
func activeRule(tx *gorm.DB, no string, now int64) (model.SettlementRule, error) {
	var r model.SettlementRule
	e := tx.Where("rule_no=? AND status='active' AND effective_from<=? AND effective_to>?", no, now, now).Take(&r).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return r, ErrExchange
	}
	return r, e
}
