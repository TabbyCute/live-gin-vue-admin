package earning

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"tb_live_module/internal/telemetry"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	res "tb_live_module/model/earning/response"
	"time"
)

const auditRunBudget = 30 * time.Second

var errAuditCancelled = errors.New("earning audit cancelled")

func (s *Service) StartAudit(ctx context.Context, actor Actor, r req.StartAudit) (model.AuditJob, error) {
	var out model.AuditJob
	if !keyRE.MatchString(r.RequestID) || !noRE.MatchString(r.AnchorNo) || !currencyRE.MatchString(r.Currency) || (r.RetryOf != "" && !noRE.MatchString(r.RetryOf)) {
		return out, ErrInvalid
	}
	digest := hash(struct {
		Request req.StartAudit
		Actor   uint64
	}{r, actor.ID})
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, actor, "audit/start", "POST"); e != nil {
			return e
		}
		anchor, e := anchorByNo(tx, r.AnchorNo)
		if e != nil {
			return e
		}
		if e = lockAnchor(tx, uint64(anchor.ID)); e != nil {
			return e
		}
		e = tx.Where("request_id=?", r.RequestID).Take(&out).Error
		if e == nil {
			if out.RequestHash != digest {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if _, e = loadAccount(tx, uint64(anchor.ID), r.AnchorNo, r.Currency, s.now(), false); e != nil {
			return ErrNotFound
		}
		if r.RetryOf != "" {
			var old model.AuditJob
			if e = tx.Where("job_no=? AND anchor_id=? AND currency=?", r.RetryOf, anchor.ID, r.Currency).Take(&old).Error; e != nil {
				return ErrNotFound
			}
			if old.Status != "failed" && old.Status != "cancelled" {
				return ErrConflict
			}
		}
		var active int64
		if e = tx.Model(&model.AuditJob{}).Where("anchor_id=? AND status IN ?", anchor.ID, []string{"queued", "running", "cancel_requested"}).Count(&active).Error; e != nil {
			return e
		}
		if active != 0 {
			return ErrConflict
		}
		out = model.AuditJob{JobNo: number("aj_"), RequestID: r.RequestID, RequestHash: digest, AnchorID: uint64(anchor.ID), AnchorNo: r.AnchorNo, Currency: r.Currency, RetryOf: r.RetryOf, Status: "queued", Progress: "{}", CreatedBy: actor.ID, CreatedAt: s.now(), UpdatedAt: s.now()}
		return tx.Create(&out).Error
	})
	return out, e
}

func (s *Service) ListAudits(ctx context.Context, actor Actor, q req.FinanceQuery) (res.Page, error) {
	out := res.Page{}
	if e := pagination(&q); e != nil {
		return out, e
	}
	db := s.db(ctx)
	if e := s.authorize(db, actor, "audit/list", "GET"); e != nil {
		return out, e
	}
	db = db.Model(&model.AuditJob{})
	if q.AnchorNo != "" {
		db = db.Where("anchor_no=?", q.AnchorNo)
	}
	if q.No != "" {
		db = db.Where("job_no=?", q.No)
	}
	if q.Status != "" {
		db = db.Where("status=?", q.Status)
	}
	out.Page, out.PageSize = q.Page, q.PageSize
	if e := db.Count(&out.Total).Error; e != nil {
		return out, e
	}
	var rows []model.AuditJob
	e := db.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error
	out.List = rows
	return out, e
}

func (s *Service) CancelAudit(ctx context.Context, actor Actor, r req.CancelAudit) (model.AuditJob, error) {
	var out model.AuditJob
	if !noRE.MatchString(r.JobNo) {
		return out, ErrInvalid
	}
	db := s.db(ctx)
	if e := s.authorize(db, actor, "audit/cancel", "POST"); e != nil {
		return out, e
	}
	// Single conditional statement makes cancellation win over a racing worker's
	// final status update; terminal reports are never overwritten.
	e := db.Model(&model.AuditJob{}).Where("job_no=? AND status IN ?", r.JobNo, []string{"queued", "running", "cancel_requested"}).Updates(map[string]any{"status": "cancelled", "cancelled_by": actor.ID, "finished_at": s.now(), "updated_at": s.now()}).Error
	if e != nil {
		return out, e
	}
	e = db.Where("job_no=?", r.JobNo).Take(&out).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrNotFound
	}
	if e == nil && out.Status != "cancelled" {
		return out, ErrConflict
	}
	return out, e
}

// RunNextAudit claims one job. Financial reads use one repeatable-read, read-only
// snapshot, while progress uses separate short writes. A failed/cancelled retry
// starts a fresh snapshot at version zero, never treating progress as evidence.
func (s *Service) RunNextAudit(ctx context.Context) (bool, error) {
	db := s.db(ctx)
	// A process crash cannot strand a running job. No old run may finish after
	// recovery because completion and progress require the running status.
	if e := db.Model(&model.AuditJob{}).Where("status='running' AND started_at<?", s.now()-int64((auditRunBudget+15*time.Second)/time.Millisecond)).Updates(map[string]any{"status": "failed", "issue_code": "worker_interrupted", "finished_at": s.now(), "updated_at": s.now()}).Error; e != nil {
		return false, e
	}
	var job model.AuditJob
	e := db.Where("status='queued'").Order("created_at,id").Take(&job).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	claim := db.Model(&model.AuditJob{}).Where("id=? AND status='queued'", job.ID).Updates(map[string]any{"status": "running", "started_at": s.now(), "updated_at": s.now()})
	if claim.Error != nil {
		return false, claim.Error
	}
	if claim.RowsAffected != 1 {
		return false, nil
	}
	runCtx, cancel := context.WithTimeout(ctx, auditRunBudget)
	defer cancel()
	var snapshot model.Account
	progress := AuditProgress{}
	writeProgress := func(p AuditProgress) error {
		progress = p
		result := s.db(runCtx).Model(&model.AuditJob{}).Where("id=? AND status='running'", job.ID).Updates(map[string]any{"progress": encode(p), "account_snapshot": encode(snapshot), "snapshot_hash": hash(snapshot), "updated_at": s.now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			// MySQL may report zero for a same-value update in the same millisecond.
			var state model.AuditJob
			if e := s.db(runCtx).Where("id=?", job.ID).Take(&state).Error; e != nil {
				return e
			}
			if state.Status != "running" {
				return errAuditCancelled
			}
		}
		return nil
	}
	e = s.db(runCtx).Transaction(func(tx *gorm.DB) error {
		if s.SourceAuditScope != nil {
			tx = s.SourceAuditScope(tx)
		}
		var e error
		snapshot, e = loadAccount(tx, job.AnchorID, job.AnchorNo, job.Currency, s.now(), false)
		if e != nil {
			return e
		}
		if e = writeProgress(progress); e != nil {
			return e
		}
		return s.verifyAccountProgress(tx, snapshot, true, writeProgress)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	status, issue := "passed", ""
	if e != nil {
		status, issue = "failed", "audit_unavailable"
		switch {
		case errors.Is(e, errAuditCancelled):
			status, issue = "cancelled", "cancelled"
		case errors.Is(e, context.DeadlineExceeded):
			issue = "audit_timeout"
		case errors.Is(e, context.Canceled):
			issue = "worker_interrupted"
		case errors.Is(e, ErrInvariant):
			issue = "invariant_violation"
		case errors.Is(e, ErrSource):
			issue = "untrusted_source"
		case errors.Is(e, ErrLimit):
			issue = "proof_batch_limit"
		}
	}
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer finishCancel()
	result := s.db(finishCtx).Model(&model.AuditJob{}).Where("id=? AND status='running'", job.ID).Updates(map[string]any{"status": status, "issue_code": issue, "progress": encode(progress), "account_snapshot": encode(snapshot), "snapshot_hash": hash(snapshot), "finished_at": s.now(), "updated_at": s.now()})
	if result.Error != nil {
		return true, result.Error
	}
	return true, e
}

// RunAuditWorker is owned by the server lifecycle; cancellation stops polling
// and cancels the active read snapshot. It never starts an unbounded goroutine.
func (s *Service) RunAuditWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		finish := telemetry.Default.StartWorker("earning_audit", 45*time.Second)
		_, e := s.RunNextAudit(ctx)
		finish(e)
	}
}
