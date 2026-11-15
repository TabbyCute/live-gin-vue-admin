package eventdelivery

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	model "tb_live_module/model/eventdelivery"
	"tb_live_module/model/system"
	"unicode"
)

type Search struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Status   string `form:"status"`
	EventID  string `form:"eventId"`
}
type Page struct {
	List     []model.Delivery `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
}
type Detail struct {
	Delivery model.Delivery     `json:"delivery"`
	Event    model.Event        `json:"event"`
	Payload  json.RawMessage    `json:"payload"`
	Attempts []model.Attempt    `json:"attempts"`
	Retries  []model.RetryAudit `json:"retries"`
}
type RetryRequest struct {
	RequestID  string `json:"requestId"`
	DeliveryID uint64 `json:"deliveryId,string"`
	Generation uint32 `json:"generation"`
	Reason     string `json:"reason"`
}

func root(tx *gorm.DB, actor uint64) error {
	if !system.IsSuperAdmin(actor) {
		return ErrForbidden
	}
	var n int64
	if err := tx.Model(&system.SysUser{}).Where("id=? AND enable=1", actor).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return ErrForbidden
	}
	return nil
}
func (s *Service) List(ctx context.Context, actor uint64, q Search) (Page, error) {
	out := Page{List: []model.Delivery{}}
	if err := s.checkDB(); err != nil {
		return out, err
	}
	db := s.DB.WithContext(ctx)
	if err := root(db, actor); err != nil {
		return out, err
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 1000 || q.PageSize < 1 || q.PageSize > 100 {
		return out, ErrInvalid
	}
	db = db.Model(&model.Delivery{})
	if q.Status != "" {
		if !validStatus(q.Status) {
			return out, ErrInvalid
		}
		db = db.Where("status=?", q.Status)
	}
	if q.EventID != "" {
		if !validNo(q.EventID, 72) {
			return out, ErrInvalid
		}
		db = db.Where("event_id=?", q.EventID)
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	if err := db.Count(&out.Total).Error; err != nil {
		return out, err
	}
	return out, db.Order("id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&out.List).Error
}
func validStatus(v string) bool {
	return v == "pending" || v == "leased" || v == "blocked" || v == "delivered" || v == "dead"
}
func (s *Service) Detail(ctx context.Context, actor, id uint64) (Detail, error) {
	out := Detail{Attempts: []model.Attempt{}, Retries: []model.RetryAudit{}}
	if err := s.checkDB(); err != nil {
		return out, err
	}
	db := s.DB.WithContext(ctx)
	if err := root(db, actor); err != nil {
		return out, err
	}
	if id == 0 {
		return out, ErrInvalid
	}
	if err := db.Where("id=?", id).Take(&out.Delivery).Error; err != nil {
		return out, err
	}
	if err := db.Where("event_id=?", out.Delivery.EventID).Take(&out.Event).Error; err != nil {
		return out, err
	}
	if validEnvelope(out.Event) {
		out.Payload = json.RawMessage(out.Event.Payload)
	}
	if err := db.Where("delivery_id=?", id).Order("number DESC").Limit(220).Find(&out.Attempts).Error; err != nil {
		return out, err
	}
	return out, db.Where("delivery_id=?", id).Order("created_at DESC").Limit(10).Find(&out.Retries).Error
}
func (s *Service) Retry(ctx context.Context, actor uint64, r RetryRequest) (model.RetryAudit, error) {
	var out model.RetryAudit
	if err := s.checkDB(); err != nil {
		return out, err
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		out = model.RetryAudit{}
		if err := root(tx, actor); err != nil {
			return err
		}
		if len(r.RequestID) < 8 || !validNo(r.RequestID, 64) || r.DeliveryID == 0 || len(r.Reason) < 8 || len(r.Reason) > 200 || strings.TrimSpace(r.Reason) != r.Reason || strings.IndexFunc(r.Reason, unicode.IsControl) >= 0 {
			return ErrInvalid
		}
		// Lock the stable delivery row first: competing retries cannot reset attempts twice.
		var d model.Delivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", r.DeliveryID).Take(&d).Error; err != nil {
			return err
		}
		if err := tx.Where("request_id=?", r.RequestID).Find(&out).Error; err != nil {
			return err
		}
		if out.RequestID != "" {
			if out.DeliveryID != r.DeliveryID || out.ActorID != actor || out.Reason != r.Reason || out.Generation != r.Generation+1 {
				return ErrConflict
			}
			return nil
		}
		if d.RetryGeneration != r.Generation || d.RetryGeneration >= 10 || (d.Status != "dead" && d.Status != "blocked") {
			return ErrConflict
		}
		if s.Adapter == nil {
			return ErrUnavailable
		}
		now := s.now()
		result := tx.Model(&model.Delivery{}).Where("id=? AND status=? AND retry_generation=?", d.ID, d.Status, r.Generation).Updates(map[string]any{"status": "pending", "retry_generation": r.Generation + 1, "cycle_attempts": 0, "last_code": "OPERATOR_RETRY", "next_attempt_at": now, "lease_token": "", "lease_until": 0, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		out = model.RetryAudit{RequestID: r.RequestID, DeliveryID: d.ID, ActorID: actor, Reason: r.Reason, Generation: r.Generation + 1, CreatedAt: now}
		return tx.Create(&out).Error
	})
	return out, err
}

type MetricSnapshot struct {
	AdapterConfigured  bool             `json:"adapterConfigured"`
	Counts             map[string]int64 `json:"counts"`
	CountsCapped       bool             `json:"countsCapped"`
	OldestPendingAgeMs int64            `json:"oldestPendingAgeMs"`
	LastTickAt         int64            `json:"lastTickAt"`
	LastSuccessAt      int64            `json:"lastSuccessAt"`
	WorkerFailures     uint64           `json:"workerFailures"`
	LastCode           string           `json:"lastCode"`
	Capture            []model.Cursor   `json:"capture"`
}

func (s *Service) Snapshot(ctx context.Context) (MetricSnapshot, error) {
	out := MetricSnapshot{Counts: map[string]int64{}, Capture: []model.Cursor{}}
	if err := s.checkDB(); err != nil {
		return out, err
	}
	out.AdapterConfigured = s.Adapter != nil
	out.LastTickAt = s.lastTick.Load()
	out.LastSuccessAt = s.lastSuccess.Load()
	out.WorkerFailures = s.failures.Load()
	if v := s.lastCode.Load(); v != nil {
		out.LastCode = v.(string)
	}
	db := s.DB.WithContext(ctx)
	for _, status := range []string{"pending", "leased", "blocked", "dead", "delivered"} {
		var n int64
		sub := db.Model(&model.Delivery{}).Select("id").Where("consumer=? AND status=?", Consumer, status).Limit(10001)
		if err := db.Table("(?) AS bounded_events", sub).Count(&n).Error; err != nil {
			return out, err
		}
		if n > 10000 {
			out.CountsCapped = true
			n = 10000
		}
		out.Counts[status] = n
	}
	var oldest int64
	for _, status := range []string{"pending", "blocked", "leased"} {
		var first model.Delivery
		if err := db.Select("created_at").Where("status=?", status).Order("created_at").Limit(1).Find(&first).Error; err != nil {
			return out, err
		}
		if first.CreatedAt > 0 && (oldest == 0 || first.CreatedAt < oldest) {
			oldest = first.CreatedAt
		}
	}
	if oldest > 0 {
		out.OldestPendingAgeMs = max(0, s.now()-oldest)
	}
	return out, db.Order("source").Find(&out.Capture).Error
}

// PublicError is deliberately coarse: never expose SQL, adapter details or credentials.
func PublicError(err error) string { return safeError(err) }
