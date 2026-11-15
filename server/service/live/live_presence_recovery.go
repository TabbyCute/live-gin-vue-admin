package live

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"tb_live_module/internal/eventcapture"
	model "tb_live_module/model/live"
	"time"
)

// RecoverLostSession is an operator-only CLI primitive. It refuses recovery if ANY
// original session key remains, never deletes a real key, and preserves all SQL facts.
// The dataLost marker is permanent; the uncheckpointed window cannot be reconstructed.
func (s *PresenceService) RecoverLostSession(ctx context.Context, no, reason string) error {
	reason = strings.TrimSpace(reason)
	if !presenceNumber.MatchString(no) || len(reason) < 8 || len(reason) > 200 {
		return ErrPresenceInvalid
	}
	if e := s.ready(); e != nil {
		return e
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cp model.LivePresenceCheckpoint
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("session_no=?", no).First(&cp).Error; e != nil {
			return e
		}
		if cp.FinalizedAt > 0 {
			return ErrPresenceAccess
		}
		rc, cancel := presenceRedisContext(ctx)
		n, e := s.Redis.Exists(rc, presenceKeys(no)...).Result()
		cancel()
		if e != nil {
			return ErrPresenceUnavailable
		}
		if n != 0 {
			return fmt.Errorf("recovery refused: original session keys still exist")
		}
		// A stable recovery epoch is committed BEFORE any new Redis state is initialized.
		previousEpoch := cp.Epoch
		nextEpoch := presenceRandom()
		if e = tx.Model(&cp).Updates(map[string]any{"epoch": nextEpoch, "revision": 0, "data_lost": true, "last_error": "initializing"}).Error; e != nil {
			return e
		}
		if e = tx.Model(&model.LiveViewerVisit{}).Where("session_no=? AND status='online'", no).Updates(map[string]any{"status": "closed", "exit_reason": "redis_lost", "left_at": gorm.Expr("last_seen_at"), "updated_at": time.Now().UnixMilli()}).Error; e != nil {
			return e
		}
		payload, _ := json.Marshal(map[string]string{"reason": reason, "previousEpoch": previousEpoch, "newEpoch": nextEpoch})
		event := model.LiveRoomEvent{EventID: "recovery:" + nextEpoch, EventType: "room.presence.recovered", SchemaVersion: 1, RoomNo: cp.RoomNo, SessionNo: no, SourceType: "operator", SourceNo: no, OccurredAt: time.Now().UnixMilli(), Payload: string(payload)}
		if e := tx.Create(&event).Error; e != nil {
			return e
		}
		return eventcapture.Room(tx, []model.LiveRoomEvent{event})
	})
}
