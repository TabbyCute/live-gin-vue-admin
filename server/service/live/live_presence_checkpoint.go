package live

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	model "tb_live_module/model/live"
	socialModel "tb_live_module/model/social"
)

// Batch UPSERTs keep a remote MySQL checkpoint to a fixed number of round trips.
// Revision is assigned LAST: MySQL evaluates assignments left-to-right.
func persistPresenceVisits(tx *gorm.DB, visits []model.LiveViewerVisit) error {
	if len(visits) == 0 {
		return nil
	}
	incoming := func(column string) string {
		if tx.Dialector.Name() == "mysql" {
			return "VALUES(" + column + ")"
		}
		return "excluded." + column
	}
	updates := clause.Set{}
	for _, column := range []string{"last_seen_at", "left_at", "watch_duration_ms", "like_count", "event_checkpoint_at", "exit_reason", "status", "updated_at", "revision"} {
		updates = append(updates, clause.Assignment{Column: clause.Column{Name: column}, Value: gorm.Expr("CASE WHEN revision < " + incoming("revision") + " THEN " + incoming(column) + " ELSE " + column + " END")})
	}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "visit_no"}}, DoUpdates: updates}).Create(&visits).Error
}
func persistPresenceViewers(tx *gorm.DB, rows []model.LiveViewerSession) error {
	if len(rows) == 0 {
		return nil
	}
	incoming := func(column string) string {
		if tx.Dialector.Name() == "mysql" {
			return "VALUES(" + column + ")"
		}
		return "excluded." + column
	}
	updates := clause.Set{}
	for _, c := range []string{"watch_duration_ms", "last_seen_at"} {
		updates = append(updates, clause.Assignment{Column: clause.Column{Name: c}, Value: gorm.Expr("CASE WHEN " + c + " < " + incoming(c) + " THEN " + incoming(c) + " ELSE " + c + " END")})
	}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "session_no"}, {Name: "visitor_key"}}, DoUpdates: updates}).Create(&rows).Error
}
func persistPresenceHistory(tx *gorm.DB, room string, visits []model.LiveViewerVisit, metas map[string]presenceMeta) error {
	when := []string{}
	where := []string{}
	args := []any{}
	whereArgs := []any{}
	for _, v := range visits {
		m := metas[v.VisitNo]
		if v.ViewerID == 0 || m.History == "" {
			continue
		}
		condition := "(user_id=? AND resource_type=? AND resource_id=? AND session_token=? AND session_version=? AND epoch=? AND hidden=? AND last_watched_at<?)"
		values := []any{v.ViewerID, socialModel.ResourceLiveRoom, room, m.History, m.HistoryVersion, m.HistoryEpoch, false, m.Last}
		when = append(when, "WHEN "+condition+" THEN ?")
		args = append(args, values...)
		args = append(args, m.Last)
		where = append(where, condition)
		whereArgs = append(whereArgs, values...)
	}
	if len(when) == 0 {
		return nil
	}
	args = append(args, whereArgs...)
	return tx.Exec("UPDATE social_watch_history SET last_watched_at=CASE "+strings.Join(when, " ")+" ELSE last_watched_at END WHERE "+strings.Join(where, " OR "), args...).Error
}

// Snapshot events aggregate accepted likes and emit at most one heartbeat event per
// connection per minute. The visit's event watermark commits in the same transaction.
func presenceCheckpointEvents(tx *gorm.DB, visits []model.LiveViewerVisit, metas map[string]presenceMeta) ([]model.LiveRoomEvent, error) {
	ids := []string{}
	for _, v := range visits {
		ids = append(ids, v.VisitNo)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var old []model.LiveViewerVisit
	if e := tx.Select("visit_no,like_count,event_checkpoint_at,revision").Where("visit_no IN ?", ids).Find(&old).Error; e != nil {
		return nil, e
	}
	byID := map[string]model.LiveViewerVisit{}
	for _, v := range old {
		byID[v.VisitNo] = v
	}
	events := []model.LiveRoomEvent{}
	for n := range visits {
		v := &visits[n]
		before := byID[v.VisitNo]
		m := metas[v.VisitNo]
		v.LikeCount = m.Likes
		v.EventCheckpointAt = before.EventCheckpointAt
		if v.Revision <= before.Revision {
			continue
		}
		emit := func(typ string, payload any) {
			at := v.LastSeenAt
			if typ == "room.viewer.liked" && m.LikeAt > 0 {
				at = m.LikeAt
			}
			raw, _ := json.Marshal(payload)
			digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", typ, v.VisitNo, v.Revision)))
			events = append(events, model.LiveRoomEvent{EventID: "checkpoint:" + hex.EncodeToString(digest[:]), EventType: typ, SchemaVersion: 1, RoomNo: v.RoomNo, SessionNo: v.SessionNo, ActorID: v.ViewerID, SourceType: "presence", SourceNo: v.VisitNo, OccurredAt: at, Payload: string(raw)})
		}
		if m.Likes > before.LikeCount {
			emit("room.viewer.liked", map[string]string{"acceptedCount": presenceUint(m.Likes - before.LikeCount), "connectionLikeCount": presenceUint(m.Likes)})
		}
		if m.Seq != "0" && v.LastSeenAt-before.EventCheckpointAt >= 60000 {
			v.EventCheckpointAt = v.LastSeenAt
			emit("room.viewer.heartbeat_checkpoint", map[string]string{"connectionDurationMs": presenceUint(v.WatchDurationMs)})
		}
	}
	return events, nil
}
