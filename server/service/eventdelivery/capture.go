package eventdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"regexp"
	"strconv"
	model "tb_live_module/model/eventdelivery"
	gift "tb_live_module/model/gift"
	live "tb_live_module/model/live"
	payout "tb_live_module/model/payout"
)

var publicNo = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)

func validNo(s string, n int) bool { return len(s) <= n && publicNo.MatchString(s) }

func (s *Service) enqueue(tx *gorm.DB, source, sourceID, typ, audience, destination string, payload any, occurred int64) error {
	if !validNo(sourceID, 128) || !validNo(destination, 64) || occurred <= 0 {
		return ErrInvalid
	}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > 2048 {
		return ErrInvalid
	}
	now := s.now()
	event := model.Event{EventID: stableID(source, sourceID), Source: source, SourceID: sourceID, EventType: typ, SchemaVersion: 1, Audience: audience, Destination: destination, Payload: string(raw), PayloadHash: hashPayload(string(raw)), OccurredAt: occurred, CreatedAt: now}
	if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error; err != nil {
		return err
	}
	var existing model.Event
	if err = tx.Where("event_id=?", event.EventID).Take(&existing).Error; err != nil {
		return err
	}
	if existing.PayloadHash != event.PayloadHash || existing.EventType != typ || existing.Audience != audience || existing.Destination != destination || existing.Source != source || existing.SourceID != sourceID || existing.OccurredAt != occurred {
		return ErrConflict
	}
	delivery := model.Delivery{EventID: event.EventID, Consumer: Consumer, Status: "pending", NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}
	if s.Adapter == nil {
		delivery.Status = "blocked"
		delivery.LastCode = "CHANNEL_UNCONFIGURED"
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&delivery).Error
}

// Gift reconstructs a deliberately small display payload. Raw source JSON, sender
// identity, URLs, money, IP and trace information never cross this boundary.
func (s *Service) Gift(tx *gorm.DB, e gift.Event) error {
	if e.EventType != "gift.sent" {
		return nil
	}
	if !validNo(e.GiftOrderNo, 40) || !validNo(e.RoomNo, 64) || !validNo(e.SessionNo, 64) {
		return ErrInvalid
	}
	payload := struct {
		OrderNo   string `json:"orderNo"`
		RoomNo    string `json:"roomNo"`
		SessionNo string `json:"sessionNo"`
	}{e.GiftOrderNo, e.RoomNo, e.SessionNo}
	return s.enqueue(tx, "gift", e.EventID, e.EventType, "room", e.SessionNo, payload, e.CreatedAt)
}

// Payout is a private status-change hint. The authenticated client fetches finance
// details separately; no balance, payee, account, evidence or amount is in the event.
func (s *Service) Payout(tx *gorm.DB, e payout.TerminalEvent, recipient uint64) error {
	if e.Kind != "payout.succeeded" && e.Kind != "payout.cancelled" {
		return ErrInvalid
	}
	if recipient == 0 || !validNo(e.PayoutNo, 40) {
		return ErrInvalid
	}
	payload := struct {
		PayoutNo string `json:"payoutNo"`
		Status   string `json:"status"`
	}{e.PayoutNo, e.Kind}
	return s.enqueue(tx, "payout", e.EventNo, e.Kind, "user", strconv.FormatUint(recipient, 10), payload, e.CreatedAt)
}

var roomTypes = map[string]bool{"room.viewer.entered": true, "room.viewer.left": true, "room.viewer.timeout": true, "room.viewer.liked": true, "room.session.ended": true, "room.presence.recovered": true}

// Room never forwards raw Redis payload, visitor keys, actor IDs or source connection IDs.
// Gift and relationship projections are deliberately excluded (duplicate/privacy).
func (s *Service) Room(tx *gorm.DB, rows []live.LiveRoomEvent) error {
	for _, e := range rows {
		if !roomTypes[e.EventType] {
			continue
		}
		if !validNo(e.RoomNo, 64) || !validNo(e.SessionNo, 64) {
			return ErrInvalid
		}
		payload := struct {
			RoomNo    string `json:"roomNo"`
			SessionNo string `json:"sessionNo"`
		}{e.RoomNo, e.SessionNo}
		if err := s.enqueue(tx, "presence", e.EventID, e.EventType, "room", e.SessionNo, payload, e.OccurredAt); err != nil {
			return err
		}
	}
	return nil
}

// CaptureSources performs at most 100 source reads per source and tick, under a
// durable cursor lock. Each empty pass wraps: lower IDs committed after a pass are
// not lost. This is repair/backfill; current producers enqueue in the same transaction.
func (s *Service) CaptureSources(ctx context.Context) error {
	var failures error
	for _, source := range []string{"gift", "payout", "presence"} {
		failures = errors.Join(failures, s.capturePage(ctx, source))
	}
	return failures
}

// existingDeliveries avoids rewriting/reading every already captured payload on
// historical repair passes. The bounded IN lookup uses uk_delivery_consumer.
func existingDeliveries(tx *gorm.DB, source string, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = stableID(source, id)
	}
	var present []string
	if err := tx.Model(&model.Delivery{}).Where("event_id IN ? AND consumer=?", keys, Consumer).Pluck("event_id", &present).Error; err != nil {
		return nil, err
	}
	for _, id := range present {
		out[id] = true
	}
	return out, nil
}
func (s *Service) capturePage(ctx context.Context, source string) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Cursor{Source: source, LastKey: ""}).Error; err != nil {
			return err
		}
		var cursor model.Cursor
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source=?", source).Take(&cursor).Error; err != nil {
			return err
		}
		last, n := cursor.LastKey, 0
		switch source {
		case "gift":
			var rows []gift.Event
			q := tx.Select("id,event_id,event_type,gift_order_no,room_no,session_no,created_at").Order("id").Limit(100)
			if last != "" {
				q = q.Where("id>?", last)
			}
			if err := q.Find(&rows).Error; err != nil {
				return err
			}
			n = len(rows)
			ids := make([]string, len(rows))
			for i, e := range rows {
				ids[i] = e.EventID
			}
			existing, err := existingDeliveries(tx, source, ids)
			if err != nil {
				return err
			}
			for _, e := range rows {
				if !existing[stableID(source, e.EventID)] {
					if err := s.Gift(tx, e); err != nil {
						return err
					}
				}
				last = strconv.FormatUint(e.ID, 10)
			}
		case "payout":
			var rows []payout.TerminalEvent
			if err := tx.Select("event_no,payout_no,kind,created_at").Where("event_no>?", last).Order("event_no").Limit(100).Find(&rows).Error; err != nil {
				return err
			}
			n = len(rows)
			ids := make([]string, len(rows))
			for i, e := range rows {
				ids[i] = e.EventNo
			}
			existing, err := existingDeliveries(tx, source, ids)
			if err != nil {
				return err
			}
			for _, e := range rows {
				last = e.EventNo
				if existing[stableID(source, e.EventNo)] {
					continue
				}
				var o struct{ ApplicantUserID uint64 }
				if err := tx.Model(&payout.Order{}).Select("applicant_user_id").Where("payout_no=?", e.PayoutNo).Take(&o).Error; err != nil {
					return err
				}
				if err := s.Payout(tx, e, o.ApplicantUserID); err != nil {
					return err
				}
				last = e.EventNo
			}
		case "presence":
			var rows []live.LiveRoomEvent
			if err := tx.Select("event_id,event_type,room_no,session_no,occurred_at").Where("event_id>?", last).Order("event_id").Limit(100).Find(&rows).Error; err != nil {
				return err
			}
			n = len(rows)
			ids := make([]string, len(rows))
			for i, e := range rows {
				ids[i] = e.EventID
			}
			existing, err := existingDeliveries(tx, source, ids)
			if err != nil {
				return err
			}
			missing := make([]live.LiveRoomEvent, 0, len(rows))
			for _, e := range rows {
				if !existing[stableID(source, e.EventID)] {
					missing = append(missing, e)
				}
			}
			if err := s.Room(tx, missing); err != nil {
				return err
			}
			if n > 0 {
				last = rows[n-1].EventID
			}
		}
		changes := map[string]any{"last_key": last, "scanned": gorm.Expr("scanned+?", n), "updated_at": s.now()}
		if n == 0 {
			changes["last_key"] = ""
			changes["cycles"] = gorm.Expr("cycles+1")
		}
		return tx.Model(&model.Cursor{}).Where("source=?", source).Updates(changes).Error
	})
}
