package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"tb_live_module/internal/eventcapture"
	"tb_live_module/internal/telemetry"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/live"
)

// Run owns bounded loops and waits for cancellation. Each session has a distributed
// lease; DB row locking, epoch and revision fencing also protect lease-loss retries.
func (s *PresenceService) Run(ctx context.Context) {
	factsDone := make(chan struct{})
	go func() {
		defer close(factsDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				finish := telemetry.Default.StartWorker("presence_projection", 30*time.Second)
				run, cancel := context.WithTimeout(ctx, 5*time.Second)
				e := s.ProjectRoomFacts(run)
				finish(e)
				if e != nil && ctx.Err() == nil {
					s.failure("source_projection_failed", "")
				}
				cancel()
			}
		}
	}()
	defer func() { <-factsDone }()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		finish := telemetry.Default.StartWorker("presence_checkpoint", 25*time.Second)
		run, cancel := context.WithTimeout(ctx, 10*time.Second)
		e := s.Tick(run)
		cancel()
		finish(e)
		if e != nil && ctx.Err() == nil {
			s.failure("worker_failed", "")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *PresenceService) Tick(ctx context.Context) error {
	if e := s.ready(); e != nil {
		return e
	}
	var empty []uint
	if e := s.DB.WithContext(ctx).Model(&model.LiveSession{}).Where("status IN ? AND stats_finalized_at=0 AND NOT EXISTS (SELECT 1 FROM live_presence_checkpoint p WHERE p.session_no=live_session.session_no)", []uint8{model.LiveSessionEnded, model.LiveSessionCancelled, model.LiveSessionFailed}).Order("id").Limit(100).Pluck("id", &empty).Error; e != nil {
		return e
	}
	if len(empty) > 0 {
		if e := s.DB.WithContext(ctx).Model(&model.LiveSession{}).Where("id IN ? AND stats_finalized_at=0", empty).Update("stats_finalized_at", time.Now().UnixMilli()).Error; e != nil {
			return e
		}
	}
	var rows []model.LivePresenceCheckpoint
	// Oldest checkpoint first provides fairness across rooms, including failed sessions.
	if e := s.DB.WithContext(ctx).Where("finalized_at=0").Order("attempt_at,session_no").Limit(32).Find(&rows).Error; e != nil {
		return e
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	var mu sync.Mutex
	var errs []error
	for _, cp := range rows {
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(cp model.LivePresenceCheckpoint) {
			defer wg.Done()
			defer func() { <-slots }()
			if e := s.FlushSession(ctx, cp.SessionNo); e != nil {
				mu.Lock()
				errs = append(errs, e)
				mu.Unlock()
				s.failure("checkpoint_failed", cp.SessionNo)
			}
		}(cp)
	}
	wg.Wait()
	return errors.Join(errs...)
}
func (s *PresenceService) FlushSession(ctx context.Context, no string) error {
	keys := presenceKeys(no)
	lease := keys[0] + ":lease"
	token := presenceRandom()
	rc, cancel := presenceRedisContext(ctx)
	ok, e := s.Redis.SetNX(rc, lease, token, 15*time.Second).Result()
	cancel()
	if e != nil || !ok {
		return e
	}
	defer func() {
		c, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = s.Redis.Eval(c, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1])end;return 0`, []string{lease}, token).Err()
	}()
	ctx, stop := context.WithTimeout(ctx, 8*time.Second)
	defer stop()
	var cp model.LivePresenceCheckpoint
	if e = s.DB.WithContext(ctx).Where("session_no=?", no).First(&cp).Error; e != nil {
		return e
	}
	if cp.FinalizedAt > 0 {
		return nil
	}
	if e = s.DB.WithContext(ctx).Model(&cp).Update("attempt_at", time.Now().UnixMilli()).Error; e != nil {
		return e
	}
	var session model.LiveSession
	if e = s.DB.WithContext(ctx).Unscoped().Where("session_no=?", no).First(&session).Error; e != nil {
		return e
	}
	t := presenceToken{Room: cp.RoomNo, Session: no, Epoch: cp.Epoch}
	rc, cancel = presenceRedisContext(ctx)
	exists, e := s.Redis.Exists(rc, keys[0]).Result()
	cancel()
	if e != nil {
		return ErrPresenceUnavailable
	}
	if exists == 0 {
		// Never silently rebuild real-time counts or erase durable visits after Redis loss.
		// The operator recovery command must advance the epoch before collecting again.
		if cp.LastError == "initializing" {
			if session.Status == model.LiveSessionLiving {
				return nil
			}
			_, e = s.run(ctx, t, map[string]any{"op": "init", "room": cp.RoomNo, "view": presenceUint(cp.ViewCount), "viewer": presenceUint(cp.ViewerCount), "peak": presenceUint(cp.PeakOnlineCount), "likes": presenceUint(cp.LikeCount), "watch": presenceUint(cp.TotalWatchDurationMs), "lost": map[bool]string{true: "1", false: "0"}[cp.DataLost]})
			if e != nil {
				return e
			}
			if e = s.DB.WithContext(ctx).Model(&cp).Update("last_error", "").Error; e != nil {
				return e
			}
		} else {
			return s.DB.WithContext(ctx).Model(&cp).Updates(map[string]any{"data_lost": true, "last_error": "redis_state_lost"}).Error
		}
	}
	ended := session.Status != model.LiveSessionLiving
	if ended {
		cutoff := session.EndedAt
		if cutoff <= 0 {
			cutoff = time.Now().UnixMilli()
		}
		if _, e = s.run(ctx, t, map[string]any{"op": "close", "cutoff": cutoff}); e != nil {
			return e
		}
	}
	// If cleanup cannot append events, drain already committed work first. Returning
	// here would deadlock a full event queue, especially after the final close event.
	if _, e = s.run(ctx, t, map[string]any{"op": "expire", "batch": 100}); e != nil && !errors.Is(e, ErrPresenceCapacity) {
		return e
	}
	rc, cancel = presenceRedisContext(ctx)
	p := s.Redis.Pipeline()
	dirty := p.ZRange(rc, keys[7], 0, 99)
	events := p.XRangeN(rc, keys[8], "-", "+", 100)
	stats := p.HGetAll(rc, keys[0])
	_, e = p.Exec(rc)
	cancel()
	if e != nil {
		return ErrPresenceUnavailable
	}
	state := stats.Val()
	if state["epoch"] != cp.Epoch {
		return ErrPresenceExpired
	}
	revision, e := strconv.ParseUint(state["revision"], 10, 64)
	if e != nil {
		return ErrPresenceCorrupt
	}
	visits := []model.LiveViewerVisit{}
	metas := map[string]presenceMeta{}
	actors := map[string]presenceActor{}
	if len(dirty.Val()) > 0 {
		rc, cancel = presenceRedisContext(ctx)
		values, err := s.Redis.HMGet(rc, keys[1], dirty.Val()...).Result()
		cancel()
		if err != nil {
			return ErrPresenceUnavailable
		}
		for idx, raw := range values {
			var m presenceMeta
			if raw == nil || json.Unmarshal([]byte(fmt.Sprint(raw)), &m) != nil || m.Revision > presenceMaxInteger || m.Duration > presenceMaxInteger {
				return ErrPresenceCorrupt
			}
			id := dirty.Val()[idx]
			if len(id) != 67 || !strings.HasPrefix(id, "pc_") {
				return ErrPresenceCorrupt
			}
			metas[id] = m
			var viewer uint64
			if strings.HasPrefix(m.Actor, "u:") {
				viewer, _ = strconv.ParseUint(m.Actor[2:], 10, 64)
				if viewer == 0 {
					return ErrPresenceCorrupt
				}
			}
			status := "online"
			if m.Closed {
				status = "closed"
			}
			visits = append(visits, model.LiveViewerVisit{VisitNo: id, SessionNo: no, RoomNo: cp.RoomNo, ViewerID: viewer, VisitorKey: m.Actor, JoinedAt: m.Joined, LastSeenAt: m.Last, LeftAt: m.Left, WatchDurationMs: m.Duration, ExitReason: m.Reason, Status: status, Revision: m.Revision, CreatedAt: m.Joined, UpdatedAt: m.Last})
			actors[m.Actor] = presenceActor{}
		}
		names := []string{}
		for a := range actors {
			names = append(names, a)
		}
		rc, cancel = presenceRedisContext(ctx)
		values, err = s.Redis.HMGet(rc, keys[5], names...).Result()
		cancel()
		if err != nil {
			return ErrPresenceUnavailable
		}
		for idx, raw := range values {
			var a presenceActor
			if raw == nil || json.Unmarshal([]byte(fmt.Sprint(raw)), &a) != nil {
				return ErrPresenceCorrupt
			}
			actors[names[idx]] = a
		}
	}
	nums := map[string]uint64{}
	for _, key := range []string{"view", "viewer", "peak", "likes", "watch"} {
		n, err := strconv.ParseUint(state[key], 10, 64)
		if err != nil || n > presenceMaxInteger {
			return ErrPresenceCorrupt
		}
		nums[key] = n
	}
	eventRows := []model.LiveRoomEvent{}
	eventIDs := []string{}
	allowed := map[string]bool{"room.viewer.entered": true, "room.viewer.left": true, "room.viewer.timeout": true, "room.viewer.liked": true, "room.session.ended": true}
	for _, event := range events.Val() {
		v := event.Values
		typ := fmt.Sprint(v["type"])
		if !allowed[typ] {
			return ErrPresenceCorrupt
		}
		at, err := strconv.ParseInt(fmt.Sprint(v["at"]), 10, 64)
		if err != nil {
			return ErrPresenceCorrupt
		}
		actor := fmt.Sprint(v["actor"])
		var id uint64
		if strings.HasPrefix(actor, "u:") {
			id, _ = strconv.ParseUint(actor[2:], 10, 64)
		}
		eventRows = append(eventRows, model.LiveRoomEvent{EventID: no + ":" + cp.Epoch + ":" + event.ID, EventType: typ, SchemaVersion: 1, RoomNo: cp.RoomNo, SessionNo: no, ActorID: id, SourceType: "presence", SourceNo: fmt.Sprint(v["connection"]), OccurredAt: at, Payload: fmt.Sprint(v["payload"])})
		eventIDs = append(eventIDs, event.ID)
	}
	now := time.Now().UnixMilli()
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.LivePresenceCheckpoint
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("session_no=?", no).First(&locked).Error; e != nil {
			return e
		}
		if locked.Epoch != cp.Epoch || locked.FinalizedAt > 0 {
			return ErrPresenceExpired
		}
		checkpointEvents, err := presenceCheckpointEvents(tx, visits, metas)
		if err != nil {
			return err
		}
		eventRows = append(eventRows, checkpointEvents...)
		if e := persistPresenceVisits(tx, visits); e != nil {
			return e
		}
		if e := persistPresenceHistory(tx, cp.RoomNo, visits, metas); e != nil {
			return e
		}
		viewerRows := make([]model.LiveViewerSession, 0, len(actors))
		for actor, a := range actors {
			var viewer uint64
			if strings.HasPrefix(actor, "u:") {
				viewer, _ = strconv.ParseUint(actor[2:], 10, 64)
			}
			viewerRows = append(viewerRows, model.LiveViewerSession{SessionNo: no, VisitorKey: actor, ViewerID: viewer, WatchDurationMs: a.Duration, LastSeenAt: a.LastActive})
		}
		if e := persistPresenceViewers(tx, viewerRows); e != nil {
			return e
		}
		if len(eventRows) > 0 {
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&eventRows).Error; e != nil {
				return e
			}
			if e := eventcapture.Room(tx, eventRows); e != nil {
				return e
			}
		}
		if revision >= locked.Revision {
			values := map[string]any{"revision": revision, "view_count": max(locked.ViewCount, nums["view"]), "viewer_count": max(locked.ViewerCount, nums["viewer"]), "peak_online_count": max(locked.PeakOnlineCount, nums["peak"]), "like_count": max(locked.LikeCount, nums["likes"]), "total_watch_duration_ms": max(locked.TotalWatchDurationMs, nums["watch"]), "updated_at": now, "last_error": ""}
			if e := tx.Model(&locked).Updates(values).Error; e != nil {
				return e
			}
			updates := map[string]any{"stats_checkpoint_at": now}
			for _, key := range []string{"view_count", "viewer_count", "peak_online_count", "like_count", "total_watch_duration_ms"} {
				updates[key] = gorm.Expr("CASE WHEN "+key+" < ? THEN ? ELSE "+key+" END", values[key], values[key])
			}
			if e := tx.Model(&model.LiveSession{}).Where("session_no=?", no).Updates(updates).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	ack := []map[string]any{}
	for c, m := range metas {
		ack = append(ack, map[string]any{"c": c, "revision": m.Revision})
	}
	if _, e = s.run(ctx, t, map[string]any{"op": "ack", "visits": ack, "events": eventIDs}); e != nil {
		return e
	}
	if session.Status == model.LiveSessionEnded || session.Status == model.LiveSessionCancelled || session.Status == model.LiveSessionFailed {
		// No new open/hb/like after close; finalization follows draining ALL dirty facts.
		if _, e = s.run(ctx, t, map[string]any{"op": "finalize"}); errors.Is(e, ErrPresenceCapacity) {
			return nil
		} else if e != nil {
			return e
		}
		return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&model.LivePresenceCheckpoint{}).Where("session_no=? AND epoch=? AND finalized_at=0", no, cp.Epoch).Updates(map[string]any{"finalized_at": now, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			var final model.LivePresenceCheckpoint
			if e := tx.Where("session_no=?", no).First(&final).Error; e != nil {
				return e
			}
			if session.StartedAt > 0 {
				changed := tx.Model(&model.LiveAnchor{}).Where("id=? AND total_view_count<=?", session.AnchorId, presenceMaxInteger-final.ViewCount).Updates(map[string]any{"total_view_count": gorm.Expr("total_view_count + ?", final.ViewCount), "max_online_count": gorm.Expr("CASE WHEN max_online_count<? THEN ? ELSE max_online_count END", final.PeakOnlineCount, final.PeakOnlineCount)})
				if changed.Error != nil {
					return changed.Error
				}
				if changed.RowsAffected == 0 && final.ViewCount > 0 {
					return ErrPresenceCorrupt
				}
			}
			return tx.Model(&model.LiveSession{}).Where("session_no=?", no).Update("stats_finalized_at", now).Error
		})
	}
	return nil
}
