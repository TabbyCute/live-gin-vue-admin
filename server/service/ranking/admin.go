package ranking

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"strconv"
	"tb_live_module/internal/modulekit"
	gift "tb_live_module/model/gift"
	model "tb_live_module/model/ranking"
	req "tb_live_module/model/ranking/request"
)

// ReconcileSnapshot is a trusted read-only maintenance boundary used by the CLI.
func (s *Service) ReconcileSnapshot(ctx context.Context) (any, error) {
	var out any
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var cp model.Checkpoint
		if e := tx.Where("consumer_name='gift-revenue'").Take(&cp).Error; e != nil {
			return e
		}
		var e error
		out, e = s.reconcile(tx, cp)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}

func (s *Service) authorize(tx *gorm.DB, a Actor, path, method string) error {
	return modulekit.Authorize(tx, a, "ranking_admin_permission", "/ranking/admin/"+path, method)
}
func (s *Service) Status(ctx context.Context, a Actor) (any, error) {
	db := s.db(ctx)
	if e := s.authorize(db, a, "status", "GET"); e != nil {
		return nil, e
	}
	var cp model.Checkpoint
	if e := db.Where("consumer_name='gift-revenue'").Take(&cp).Error; e != nil {
		return nil, e
	}
	n, e := lag(db, cp.Generation)
	return map[string]any{"checkpoint": cp, "lagCount": strconv.FormatInt(n, 10), "generatedAt": s.now(), "actions": map[string]bool{"rebuild": s.authorize(db, a, "rebuild", "POST") == nil, "reconcile": s.authorize(db, a, "reconcile", "POST") == nil}}, e
}
func (s *Service) Jobs(ctx context.Context, a Actor, q req.Query) (any, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 {
		return nil, ErrInvalid
	}
	db := s.db(ctx)
	if e := s.authorize(db, a, "job/list", "GET"); e != nil {
		return nil, e
	}
	var n int64
	var rows []model.RebuildJob
	if e := db.Model(&model.RebuildJob{}).Count(&n).Error; e != nil {
		return nil, e
	}
	e := db.Order("id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&rows).Error
	return map[string]any{"list": rows, "total": strconv.FormatInt(n, 10), "page": q.Page, "pageSize": q.PageSize}, e
}
func (s *Service) AdminAction(ctx context.Context, a Actor, action string, r req.Action) (string, error) {
	if !modulekit.RequestID.MatchString(r.RequestID) || (r.Scope != "all" && r.Scope != "") || (action != "rebuild" && action != "reconcile") {
		return "", ErrInvalid
	}
	raw, _ := json.Marshal(r)
	result := ""
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, action, "POST"); e != nil {
			return e
		}
		cp, e := checkpoint(tx, s.zone())
		if e != nil {
			return e
		}
		var audit model.Audit
		e = tx.Where("request_id=?", r.RequestID).Take(&audit).Error
		if e == nil {
			if audit.OperatorID != a.ID || audit.Action != action || audit.Input != string(raw) {
				return modulekit.Conflict
			}
			result = audit.Result
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var out any
		if action == "rebuild" {
			var running int64
			if e = tx.Model(&model.RebuildJob{}).Where("status='running'").Count(&running).Error; e != nil {
				return e
			}
			if running > 0 {
				return modulekit.Conflict
			}
			var highest uint64
			if e = tx.Model(&model.RebuildJob{}).Select("COALESCE(MAX(generation),1)").Scan(&highest).Error; e != nil {
				return e
			}
			job := model.RebuildJob{JobNo: modulekit.No("rj_"), Scope: "all", Status: "running", Generation: max(highest, cp.Generation) + 1, StartedAt: s.now()}
			if e = tx.Create(&job).Error; e != nil {
				return e
			}
			out = job
		} else {
			out, e = s.reconcile(tx, cp)
			if e != nil {
				return e
			}
		}
		b, e := json.Marshal(out)
		if e != nil {
			return e
		}
		result = string(b)
		return tx.Create(&model.Audit{RequestID: r.RequestID, OperatorID: a.ID, Action: action, Input: string(raw), Result: result, ActorIP: a.IP, CreatedAt: s.now()}).Error
	})
	return result, e
}

// Reconcile is deliberately bounded for an interactive operation. Larger sites
// use the resumable shadow-generation rebuild then verify a dedicated snapshot.
func (s *Service) reconcile(tx *gorm.DB, cp model.Checkpoint) (any, error) {
	var events []model.AppliedEvent
	if e := tx.Order("source_id").Limit(10001).Find(&events).Error; e != nil {
		return nil, e
	}
	if len(events) > 10000 {
		return nil, modulekit.Unavailable
	}
	expected := map[string]model.ScoreBucket{}
	identity := func(b model.ScoreBucket) string {
		return b.RankType + "|" + b.PeriodType + "|" + b.PeriodKey + "|" + b.ScopeType + "|" + b.ScopeNo + "|" + b.GiftNo + "|" + b.SubjectNo
	}
	for start := 0; start < len(events); start += 100 {
		part := events[start:min(start+100, len(events))]
		ids := []uint64{}
		for _, ev := range part {
			ids = append(ids, ev.SourceID)
		}
		var sources []gift.Revenue
		if e := tx.Where("id IN ?", ids).Find(&sources).Error; e != nil {
			return nil, e
		}
		sourceMap := map[uint64]gift.Revenue{}
		numbers := []string{}
		for _, source := range sources {
			sourceMap[source.ID] = source
			numbers = append(numbers, source.GiftOrderNo)
		}
		var orders []gift.Order
		if e := tx.Where("order_no IN ?", numbers).Find(&orders).Error; e != nil {
			return nil, e
		}
		orderMap := map[string]gift.Order{}
		for _, order := range orders {
			orderMap[order.OrderNo] = order
		}
		for _, ev := range part {
			source, ok := sourceMap[ev.SourceID]
			order := orderMap[source.GiftOrderNo]
			d := dimensions{order.SenderAccountNo, order.AnchorNo, order.RoomNo, order.SessionNo, order.CategoryNo, order.GiftNo}
			raw, _ := json.Marshal(d)
			if !ok || source.RevenueNo != ev.EventNo || source.GrossCoinAmount != ev.DeltaCoin || source.Quantity != ev.DeltaQuantity || order.CreatedAt != ev.OriginalEventAt || ev.Dimensions != string(raw) {
				return nil, ErrInvariant
			}
			buckets, e := keys(d, ev.OriginalEventAt, s.zone())
			if e != nil {
				return nil, e
			}
			for _, b := range buckets {
				k := identity(b)
				current, ok := expected[k]
				if !ok {
					current = b
					current.ScoreCoin = "0"
					current.Quantity = "0"
				}
				current.ScoreCoin, e = addDecimal(current.ScoreCoin, ev.DeltaCoin)
				if e != nil {
					return nil, e
				}
				current.Quantity, e = addDecimal(current.Quantity, ev.DeltaQuantity)
				if e != nil {
					return nil, e
				}
				expected[k] = current
			}
		}
	}
	var buckets []model.ScoreBucket
	if e := tx.Where("generation=?", cp.Generation).Limit(360001).Find(&buckets).Error; e != nil {
		return nil, e
	}
	consistent := len(buckets) == len(expected)
	for _, b := range buckets {
		want, ok := expected[identity(b)]
		if !ok || want.ScoreCoin != b.ScoreCoin || want.Quantity != b.Quantity || sortKey(b.ScoreCoin) != b.ScoreSort {
			consistent = false
		}
		delete(expected, identity(b))
	}
	n, e := lag(tx, cp.Generation)
	if e != nil {
		return nil, e
	}
	if n != 0 {
		consistent = false
	}
	return map[string]any{"consistent": consistent, "lagCount": strconv.FormatInt(n, 10), "checkedEvents": strconv.Itoa(len(events)), "checkedBuckets": strconv.Itoa(len(buckets)), "generation": strconv.FormatUint(cp.Generation, 10)}, nil
}
