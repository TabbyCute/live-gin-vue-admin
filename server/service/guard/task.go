package guard

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"tb_live_module/internal/modulekit"
	model "tb_live_module/model/guard"
	"time"
)

func (s *Service) RunTask(ctx context.Context, kind, source string, fn func(context.Context) error) error {
	if (kind != "grant" && kind != "compensate") || !modulekit.Number.MatchString(source) {
		return ErrInvalid
	}
	token := modulekit.No("lease_")
	attempt := 0
	skip := false
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		t := model.Task{TaskNo: modulekit.No("gt_"), Kind: kind, SourceNo: source, Status: "pending", CreatedAt: s.now(), UpdatedAt: s.now()}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&t).Error; e != nil {
			return e
		}
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_no=?", source).Take(&t).Error; e != nil {
			return e
		}
		if t.Kind != kind {
			return ErrInvariant
		}
		if t.Status == "succeeded" {
			skip = true
			return nil
		}
		if t.RetryCount >= 20 || t.NextRetryAt > s.now() || t.Status == "processing" && t.LeaseUntil > s.now() {
			return ErrConflict
		}
		attempt = t.RetryCount + 1
		return tx.Model(&t).Updates(map[string]any{"status": "processing", "lease_token": token, "lease_until": s.now() + 60000, "retry_count": t.RetryCount + 1, "updated_at": s.now()}).Error
	})
	if e != nil || skip {
		return e
	}
	call, cancel := context.WithTimeout(ctx, 25*time.Second)
	workErr := fn(call)
	cancel()
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	changes := map[string]any{"status": "succeeded", "lease_token": "", "lease_until": 0, "completed_at": s.now(), "updated_at": s.now(), "last_error": "", "next_retry_at": 0}
	if workErr != nil {
		changes["status"] = "failed"
		changes["completed_at"] = 0
		changes["next_retry_at"] = s.now() + min(int64(1000)<<min(attempt, 10), int64(300000))
		changes["last_error"] = "权益任务失败，请核验来源后重试"
	}
	result := s.db(finish).Model(&model.Task{}).Where("source_no=? AND lease_token=? AND status='processing'", source, token).Updates(changes)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrConflict
	}
	return workErr
}
