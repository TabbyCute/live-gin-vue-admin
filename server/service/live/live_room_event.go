package live

import (
	"context"
	"encoding/json"
	"gorm.io/gorm/clause"
	"strconv"
	model "tb_live_module/model/live"
	socialModel "tb_live_module/model/social"
	"time"
)

// ProjectRoomFacts only reads COMMITTED source outboxes. It does not consume/change
// their delivery status and never copies money or relationship state. Anti-join by
// permanent event ID handles retries and transactions committing out of ID order.
func (s *PresenceService) ProjectRoomFacts(ctx context.Context) error {
	type fact struct {
		EventID, SourceNo, RoomNo, SessionNo string
		ActorID                              uint64
		OccurredAt                           int64
	}
	var gifts []fact
	giftID := "CONCAT('gift:',e.event_id)"
	if s.DB.Dialector.Name() == "sqlite" {
		giftID = "('gift:' || e.event_id)"
	}
	if e := s.DB.WithContext(ctx).Raw(`SELECT e.event_id,e.gift_order_no AS source_no,e.room_no,e.session_no,o.sender_user_id AS actor_id,e.created_at AS occurred_at
 FROM gift_event_outbox e JOIN gift_order o ON o.order_no=e.gift_order_no AND o.room_no=e.room_no AND o.session_no=e.session_no
 LEFT JOIN live_room_event p ON p.event_id=` + giftID + `
 WHERE e.event_type='gift.sent' AND p.event_id IS NULL ORDER BY e.id LIMIT 100`).Scan(&gifts).Error; e != nil {
		return e
	}
	for _, f := range gifts {
		if e := s.projectFact(ctx, f.EventID, "room.gift.sent", "gift", f.SourceNo, f.RoomNo, f.SessionNo, f.ActorID, f.OccurredAt); e != nil {
			return e
		}
	}
	return s.projectFollows(ctx)
}
func (s *PresenceService) projectFollows(ctx context.Context) error {
	type fact struct {
		EventID, RoomNo, SessionNo string
		UserID                     uint64
		CreatedAt                  socialModel.UTCTime
	}
	var rows []fact
	eventID := "CONCAT('social:',e.event_id,':',p.session_no)"
	at := "(TIMESTAMPDIFF(MICROSECOND,'1970-01-01 00:00:00',e.created_at) DIV 1000)"
	if s.DB.Dialector.Name() == "sqlite" {
		eventID = "('social:' || e.event_id || ':' || p.session_no)"
		at = "CAST((julianday(e.created_at)-2440587.5)*86400000 AS INTEGER)"
	}
	// Restrict to the supported maximum session lifetime. Source facts themselves remain intact.
	err := s.DB.WithContext(ctx).Raw(`SELECT e.event_id,e.user_id,e.created_at,p.room_no,p.session_no
 FROM social_outbox_event e JOIN live_anchor a ON a.user_id=e.target_user_id
 JOIN live_room r ON r.anchor_id=a.id JOIN live_presence_checkpoint p ON p.room_no=r.room_no
 LEFT JOIN live_room_event done ON done.event_id=`+eventID+`
 WHERE e.event_type='relationship.changed' AND e.action='follow' AND e.created_at >= ? AND done.event_id IS NULL
 AND EXISTS(SELECT 1 FROM live_viewer_visit v WHERE v.session_no=p.session_no AND v.viewer_id=e.user_id AND v.joined_at<=`+at+` AND ((v.left_at>0 AND v.left_at>=`+at+`) OR (v.left_at=0 AND v.last_seen_at+90000>=`+at+`)))
 ORDER BY e.id LIMIT 100`, socialModel.NewUTCTime(time.Now().Add(-7*24*time.Hour))).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, f := range rows {
		if e := s.projectFact(ctx, f.EventID, "room.viewer.followed", "social", f.EventID, f.RoomNo, f.SessionNo, f.UserID, f.CreatedAt.UnixMilli()); e != nil {
			return e
		}
	}
	return nil
}

func (s *PresenceService) projectFact(ctx context.Context, id, typ, source, no, room, session string, actor uint64, at int64) error {
	eventID := source + ":" + id
	if source == "social" {
		eventID += ":" + session
	}
	payload, _ := json.Marshal(map[string]string{"sourceNo": no})
	row := model.LiveRoomEvent{EventID: eventID, EventType: typ, SchemaVersion: 1, SourceType: source, SourceNo: no, RoomNo: room, SessionNo: session, ActorID: actor, OccurredAt: at, Payload: string(payload)}
	if e := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
		return e
	}
	// Best-effort live activity; durable event above remains the source of truth.
	rc, cancel := presenceRedisContext(ctx)
	defer cancel()
	keys := presenceKeys(session)
	name := "u:" + strconv.FormatUint(actor, 10)
	_ = s.Redis.Eval(rc, `local raw=redis.call('HGET',KEYS[1],ARGV[1]);if not raw then return 0 end;local u=cjson.decode(raw);if u.lastActive>=tonumber(ARGV[2]) or not redis.call('ZSCORE',KEYS[3],ARGV[1]) then return 0 end;if u.index~='' then redis.call('ZREM',KEYS[2],u.index)end;u.lastActive=tonumber(ARGV[2]);u.activity=ARGV[3];u.index=string.format('%016.0f',9007199254740991-u.lastActive)..':'..ARGV[1];redis.call('ZADD',KEYS[2],0,u.index);redis.call('HSET',KEYS[1],ARGV[1],cjson.encode(u));return 1`, []string{keys[5], keys[9], keys[3]}, name, at, map[string]string{"gift": "gift", "social": "follow"}[source]).Err()
	return nil
}
