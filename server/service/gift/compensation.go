package gift

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/gift"
	"time"
)

// ProcessBenefitCompensation is a trusted worker boundary, never an HTTP endpoint.
// Providers must permanently deduplicate by compensationNo, including ambiguous timeouts.
// No provider is installed in phase one; this cannot create a fictitious success.
func (s *Service) ProcessBenefitCompensation(ctx context.Context, number string, provider LegacyGuardCompensator) error {
	if provider == nil {
		return ErrGuard
	}
	if !numberPattern.MatchString(number) {
		return ErrInvalid
	}
	var work model.BenefitCompensation
	done := false
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("compensation_no=?", number).Take(&work).Error; e != nil {
			return e
		}
		now := s.now()
		if work.Status == "succeeded" {
			done = true
			return nil
		}
		if work.RetryCount >= 10000 || (work.Status == "processing" && work.LeaseUntil > now) {
			return ErrConflict
		}
		if work.Status != "pending" && work.Status != "failed" && work.Status != "processing" {
			return ErrState
		}
		work.RetryCount++
		return tx.Model(&model.BenefitCompensation{}).Where("id=?", work.ID).Updates(map[string]any{"status": "processing", "retry_count": work.RetryCount, "lease_until": now + 60000, "updated_at": now, "last_error": "", "completed_at": 0}).Error
	})
	if e != nil || done {
		return e
	}
	// External I/O occurs after the claim transaction releases all database locks.
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	providerErr := provider.Compensate(callCtx, work.CompensationNo, work.GiftOrderNo, work.DurationDays)
	cancel()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	e = s.transaction(finishCtx, func(tx *gorm.DB) error {
		now := s.now()
		status, lastError, completed := "succeeded", "", now
		if providerErr != nil {
			status = "failed"
			lastError = "守护补偿调用失败，请按补偿号核验后重试"
			completed = 0
		}
		r := tx.Model(&model.BenefitCompensation{}).Where("id=? AND status='processing' AND retry_count=?", work.ID, work.RetryCount).
			Updates(map[string]any{"status": status, "last_error": lastError, "completed_at": completed, "lease_until": 0, "updated_at": now})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrConflict
		}
		return nil
	})
	if e != nil {
		return e
	}
	return providerErr
}
