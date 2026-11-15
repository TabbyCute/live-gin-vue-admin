package eventdelivery

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sync"
	"sync/atomic"
	model "tb_live_module/model/eventdelivery"
	"time"
)

func (s *Service) Tick(ctx context.Context) error {
	if err := s.checkDB(); err != nil {
		return err
	}
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	s.lastTick.Store(s.now())
	captureErr := s.CaptureSources(ctx)
	// A bad source page must not block previously captured deliveries from other sources.
	return errors.Join(captureErr, s.Dispatch(ctx))
}

// Dispatch has a strict per-call item and concurrency budget. Claims happen just
// before each external call (not a preclaimed batch that can expire while queued).
func (s *Service) Dispatch(ctx context.Context) error {
	if err := s.checkDB(); err != nil {
		return err
	}
	opts := s.Options.normalized()
	var remaining atomic.Int64
	remaining.Store(int64(opts.Batch))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var result error
	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil && remaining.Add(-1) >= 0 {
				d, ok, err := s.claim(ctx, opts)
				if err == nil && ok {
					err = s.deliver(ctx, d, opts)
				}
				if err != nil {
					mu.Lock()
					result = errors.Join(result, err)
					mu.Unlock()
					return
				}
				if !ok {
					return
				}
			}
		}()
	}
	wg.Wait()
	return result
}
func (s *Service) claim(ctx context.Context, o Options) (model.Delivery, bool, error) {
	var row model.Delivery
	found := false
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		row = model.Delivery{}
		found = false
		now := s.now()
		// Exact status prefixes keep MySQL's SKIP LOCKED range scan bounded;
		// an IN + filesort can lock far more rows than the LIMIT suggests.
		for _, status := range []string{"leased", "pending"} {
			q := tx.Where("consumer=? AND status=? AND next_attempt_at<=?", Consumer, status, now).Order("next_attempt_at,id").Limit(1)
			if tx.Dialector.Name() == "mysql" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
			}
			if err := q.Find(&row).Error; err != nil {
				return err
			}
			if row.ID != 0 {
				break
			}
		}
		if row.ID == 0 {
			return nil
		}
		if row.Status == "leased" {
			if err := tx.Model(&model.Attempt{}).Where("delivery_id=? AND number=? AND outcome='STARTED'", row.ID, row.Attempts).Updates(map[string]any{"outcome": "LEASE_EXPIRED_UNKNOWN", "finished_at": now}).Error; err != nil {
				return err
			}
		}
		base := tx.Model(&model.Delivery{}).Where("id=? AND status=? AND lease_token=? AND next_attempt_at<=?", row.ID, row.Status, row.LeaseToken, now)
		if s.Adapter == nil {
			return base.Updates(map[string]any{"status": "blocked", "last_code": "CHANNEL_UNCONFIGURED", "lease_token": "", "lease_until": 0, "updated_at": now}).Error
		}
		if row.CycleAttempts >= o.MaxAttempts {
			return base.Updates(map[string]any{"status": "dead", "last_code": "ATTEMPTS_EXHAUSTED", "lease_token": "", "lease_until": 0, "updated_at": now}).Error
		}
		row.LeaseToken = uuid.NewString()
		row.LeaseUntil = now + o.Lease.Milliseconds()
		row.Attempts++
		row.CycleAttempts++
		result := base.Updates(map[string]any{"status": "leased", "lease_token": row.LeaseToken, "lease_until": row.LeaseUntil, "next_attempt_at": row.LeaseUntil, "attempts": row.Attempts, "cycle_attempts": row.CycleAttempts, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		attempt := model.Attempt{DeliveryID: row.ID, Number: row.Attempts, LeaseToken: row.LeaseToken, Outcome: "STARTED", StartedAt: now}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		found = true
		return nil
	})
	return row, found, err
}
func (s *Service) deliver(ctx context.Context, d model.Delivery, o Options) error {
	var event model.Event
	if err := s.DB.WithContext(ctx).Where("event_id=?", d.EventID).Take(&event).Error; err != nil {
		return err
	}
	code := "PAYLOAD_INVALID"
	success := false
	permanent := true
	if validEnvelope(event) {
		key := idempotencyKey(event.EventID)
		envelope := Envelope{EventID: event.EventID, IdempotencyKey: key, EventType: event.EventType, SchemaVersion: event.SchemaVersion, Audience: event.Audience, Destination: event.Destination, Payload: event.Payload, OccurredAt: event.OccurredAt}
		call, cancel := context.WithTimeout(ctx, o.Timeout)
		ack, err := callAdapter(call, s.Adapter, envelope)
		cancel()
		permanent = false
		code = "DELIVERY_UNKNOWN"
		if err == nil && ack.Durable && ack.EventID == event.EventID && ack.IdempotencyKey == key {
			success = true
			code = "ACKNOWLEDGED"
		}
		var rejection PermanentError
		if errors.As(err, &rejection) {
			permanent = true
			code = "PERMANENT_REJECTION"
		}
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return s.finish(finish, d, o, code, success, permanent)
}
func (s *Service) finish(ctx context.Context, d model.Delivery, o Options, code string, success, permanent bool) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		now := s.now()
		status := "pending"
		next := now + s.backoff(d, o).Milliseconds()
		if success {
			status = "delivered"
			next = 0
		} else if permanent || d.CycleAttempts >= o.MaxAttempts {
			status = "dead"
			next = 0
		}
		changes := map[string]any{"status": status, "last_code": code, "lease_token": "", "lease_until": 0, "next_attempt_at": next, "updated_at": now}
		if success {
			changes["delivered_at"] = now
		}
		r := tx.Model(&model.Delivery{}).Where("id=? AND status='leased' AND lease_token=? AND lease_until>?", d.ID, d.LeaseToken, now).Updates(changes)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrLeaseLost
		}
		return tx.Model(&model.Attempt{}).Where("delivery_id=? AND number=? AND lease_token=? AND outcome='STARTED'", d.ID, d.Attempts, d.LeaseToken).Updates(map[string]any{"outcome": code, "finished_at": now}).Error
	})
}
func (s *Service) backoff(d model.Delivery, o Options) time.Duration {
	delay := o.BaseBackoff
	for i := uint32(1); i < d.CycleAttempts && delay < o.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > o.MaxBackoff {
		delay = o.MaxBackoff
	}
	// Stable bounded jitter avoids synchronized retry storms and is reproducible in audits.
	h := hashPayload(d.EventID + string(rune(d.Attempts)))
	jitter := time.Duration(h[0]%26) * delay / 100
	if delay+jitter > o.MaxBackoff {
		return o.MaxBackoff
	}
	return delay + jitter
}

// Panic isolation prevents a faulty adapter from crashing unrelated finance workers.
// The result remains unknown because a side effect may already have occurred.
func callAdapter(ctx context.Context, adapter Adapter, e Envelope) (ack Ack, err error) {
	defer func() {
		if recover() != nil {
			ack = Ack{}
			err = ErrUnavailable
		}
	}()
	return adapter.Deliver(ctx, e)
}
