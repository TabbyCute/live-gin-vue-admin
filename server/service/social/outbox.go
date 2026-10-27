package social

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/rand/v2"
	"sync"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	"time"
)

const maxEventAttempts = 8
const fanoutPageSize = 20

var errChannelUnavailable = errors.New("CHANNEL_UNAVAILABLE")
var errLeaseLost = errors.New("LEASE_LOST")

// RecordLiveStarted 只能在真正首次开播的业务事务内调用，不能在on_publish同步扇出。
func (s *Service) RecordLiveStarted(tx *gorm.DB, userID uint64, sessionNo string) error {
	// 当前没有真实通知渠道时不创建无法消费的开播事件，避免无意义失败和背压。
	if s.Sink == nil {
		return nil
	}
	e := model.SocialOutboxEvent{EventID: uuid.NewString(), BusinessKey: "live.started:" + sessionNo, EventType: model.EventLiveStarted, Version: 1, UserID: userID, ObjectNo: sessionNo, Status: model.EventPending}
	result := tx.Session(&gorm.Session{NowFunc: func() time.Time { return time.Now().UTC() }}).Clauses(clause.OnConflict{DoNothing: true}).Create(&e)
	if result.Error == nil && result.RowsAffected == 0 {
		s.Metrics.Deduplicated.Add(1)
	}
	return result.Error
}
func (s *Service) claimEvent(ctx context.Context) (model.SocialOutboxEvent, bool, error) {
	var e model.SocialOutboxEvent
	found := false
	err := s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		e = model.SocialOutboxEvent{}
		found = false
		query := tx.Where("status = ? AND next_attempt_at <= ?", model.EventPending, time.Now().UnixMilli()).Order("next_attempt_at,id").Limit(1)
		if tx.Dialector.Name() == "mysql" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := query.Find(&e).Error; err != nil {
			return err
		}
		if e.ID == 0 {
			return nil
		}
		found = true
		e.LeaseToken = uuid.NewString()
		return tx.Model(&e).Updates(map[string]interface{}{"lease_token": e.LeaseToken, "next_attempt_at": time.Now().Add(30 * time.Second).UnixMilli()}).Error
	})
	return e, found, err
}
func (s *Service) TickOutbox(ctx context.Context) error {
	e, ok, err := s.claimEvent(ctx)
	if err != nil || !ok {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	next := e.Cursor
	done, skip := false, false
	switch e.EventType {
	case model.EventRelationship:
		done = true // 内置审计消费者，无外部推送副作用。
	case model.EventLiveStarted:
		next, done, skip, err = s.deliverLivePage(ctx, e)
	default:
		err = errors.New("UNKNOWN_EVENT")
	}
	updates := map[string]interface{}{"cursor": next, "lease_token": "", "next_attempt_at": time.Now().UnixMilli(), "attempts": uint32(0), "last_error": ""}
	if err != nil {
		s.Metrics.WorkerFailures.Add(1)
		attempt := e.Attempts + 1
		updates["attempts"] = attempt
		updates["last_error"] = "DELIVERY_FAILED"
		if errors.Is(err, errChannelUnavailable) {
			updates["last_error"] = "CHANNEL_UNAVAILABLE"
		}
		if attempt >= maxEventAttempts {
			updates["status"] = model.EventFailed
		} else {
			updates["next_attempt_at"] = time.Now().Add(time.Duration((1<<attempt)+rand.IntN(5)) * time.Second).UnixMilli()
		}
	} else if skip {
		updates["status"] = model.EventSkipped
	} else if done {
		updates["status"] = model.EventDone
	}
	// 当前处理超时仍可用调用者的独立有界清理context；进程崩溃则租约到期重领。
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer finishCancel()
	result := s.db(finishCtx).Model(&model.SocialOutboxEvent{}).Where("id = ? AND lease_token = ? AND status = ?", e.ID, e.LeaseToken, model.EventPending).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errLeaseLost
	}
	return err
}
func (s *Service) deliverLivePage(ctx context.Context, e model.SocialOutboxEvent) (uint64, bool, bool, error) {
	if time.Since(e.CreatedAt.Time) > 10*time.Minute {
		return e.Cursor, true, true, nil
	} // 过期提醒不继续扇出
	valid, err := s.Directory.LiveEventValid(ctx, s.db(ctx), e.UserID, e.ObjectNo)
	if err != nil {
		return e.Cursor, false, false, err
	}
	if !valid {
		return e.Cursor, true, true, nil
	}
	if s.Sink == nil {
		return e.Cursor, false, false, errChannelUnavailable
	}
	name := s.Sink.Name()
	if !publicNumber.MatchString(name) || len(name) > 32 {
		return e.Cursor, false, false, errChannelUnavailable
	}
	var subscriptions []model.SocialLiveSubscription
	err = s.db(ctx).Where("target_user_id = ? AND enabled = ? AND id > ? AND created_at <= ?", e.UserID, true, e.Cursor, e.CreatedAt).Order("id").Limit(fanoutPageSize).Find(&subscriptions).Error
	if err != nil {
		return e.Cursor, false, false, err
	}
	next := e.Cursor
	for _, sub := range subscriptions {
		var receipt model.SocialEventConsumer
		if err = s.db(ctx).Where("event_id = ? AND consumer = ? AND user_id = ?", e.EventID, name, sub.UserID).Limit(1).Find(&receipt).Error; err != nil {
			return next, false, false, err
		}
		if receipt.ID > 0 {
			s.Metrics.Deduplicated.Add(1)
			next = sub.ID
			continue
		}
		allowed, err := s.notificationAllowed(ctx, e, sub)
		if err != nil {
			return next, false, false, err
		}
		status := model.EventSkipped
		if allowed {
			sendCtx, cancel := context.WithTimeout(ctx, time.Second)
			err = s.Sink.Deliver(sendCtx, Notification{EventID: e.EventID, UserID: sub.UserID, AnchorUserID: e.UserID, SessionNo: e.ObjectNo, IdempotencyKey: e.EventID + ":" + name + ":" + formatID(sub.UserID)})
			cancel()
			if err != nil {
				return next, false, false, err
			}
			status = model.EventDone
			s.Metrics.Delivered.Add(1)
		}
		receipt = model.SocialEventConsumer{EventID: e.EventID, Consumer: name, UserID: sub.UserID, Status: status}
		if err = s.db(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt).Error; err != nil {
			return next, false, false, err
		}
		next = sub.ID
	}
	return next, len(subscriptions) < fanoutPageSize, false, nil
}
func (s *Service) notificationAllowed(ctx context.Context, e model.SocialOutboxEvent, sub model.SocialLiveSubscription) (bool, error) {
	db := s.db(ctx)
	valid, err := s.Directory.LiveEventValid(ctx, db, e.UserID, e.ObjectNo)
	if err != nil || !valid {
		return false, err
	}
	if err = s.Directory.Validate(ctx, db, []uint64{sub.UserID}); err != nil {
		if isBusiness(err) {
			return false, nil
		}
		return false, err
	}
	follows, blocked, err := s.CheckRelationship(ctx, db, sub.UserID, e.UserID)
	if err != nil || !follows || blocked {
		return false, err
	}
	var n int64
	if err = db.Model(&model.SocialLiveSubscription{}).Where("id = ? AND enabled = ?", sub.ID, true).Count(&n).Error; err != nil || n == 0 {
		return false, err
	}
	if err = db.Model(&model.SocialMute{}).Where("user_id = ? AND target_user_id = ? AND scope = ?", sub.UserID, e.UserID, model.MuteLiveNotification).Count(&n).Error; err != nil {
		return false, err
	}
	return n == 0, nil
}

// Run 固定四个有界循环：通知延迟不会阻塞健康心跳、关系审计或历史落库。
func (s *Service) Run(ctx context.Context) {
	var wg sync.WaitGroup
	launch := func(interval, timeout time.Duration, code string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runLoop(ctx, interval, timeout, code, fn)
		}()
	}
	launch(time.Second, 5*time.Second, "AUDIT_HEARTBEAT_FAILED", func(ctx context.Context) error {
		if err := s.consumeRelationshipEvents(ctx); err != nil {
			return err
		}
		return s.refreshWorkerPressure(ctx)
	})
	launch(time.Second, 10*time.Second, "HISTORY_FLUSH_FAILED", s.FlushHistory)
	launch(time.Minute, 10*time.Second, "PRUNE_FAILED", s.Prune)
	s.runLoop(ctx, time.Second, 20*time.Second, "OUTBOX_FAILED", s.TickOutbox)
	wg.Wait()
}
func (s *Service) runLoop(ctx context.Context, interval, timeout time.Duration, code string, fn func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		run, cancel := context.WithTimeout(ctx, timeout)
		err := fn(run)
		cancel()
		if err != nil && ctx.Err() == nil && s.Log != nil {
			s.Log.Warn("social worker iteration failed", zap.String("code", code))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Prune 每次最多500条，保留失败事件供人工重放；审计导出/归档应在删除前由运维执行。
func (s *Service) Prune(ctx context.Context) error {
	db := s.db(ctx)
	var ids []uint64
	if err := db.Model(&model.SocialWatchHistory{}).Where("last_watched_at < ?", time.Now().AddDate(0, 0, -s.retentionDays()).UnixMilli()).Order("last_watched_at,id").Limit(500).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) > 0 {
		if err := db.Where("id IN ? AND last_watched_at < ?", ids, time.Now().AddDate(0, 0, -s.retentionDays()).UnixMilli()).Delete(&model.SocialWatchHistory{}).Error; err != nil {
			return err
		}
	}
	// 分状态沿(status,updated_at,id)索引扫描，避免按id排序扫描全量已完成事件。
	for _, status := range []model.EventStatus{model.EventDone, model.EventSkipped} {
		err := s.repository().Transaction(ctx, func(tx *gorm.DB) error {
			var events []model.SocialOutboxEvent
			query := tx.Select("id", "event_id").Where("status = ? AND updated_at < ?", status, model.NewUTCTime(time.Now().UTC().AddDate(0, 0, -30))).Order("updated_at,id").Limit(250)
			if tx.Dialector.Name() == "mysql" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
			}
			if err := query.Find(&events).Error; err != nil {
				return err
			}
			if len(events) == 0 {
				return nil
			}
			ids := make([]uint64, 0, len(events))
			eventIDs := make([]string, 0, len(events))
			for _, e := range events {
				ids = append(ids, e.ID)
				eventIDs = append(eventIDs, e.EventID)
			}
			if err := tx.Where("event_id IN ?", eventIDs).Delete(&model.SocialEventConsumer{}).Error; err != nil {
				return err
			}
			return tx.Where("id IN ?", ids).Delete(&model.SocialOutboxEvent{}).Error
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) FollowingOnline(ctx context.Context, actor uint64, q req.CursorSearch) (res.OnlineList, error) {
	out := res.OnlineList{List: []res.OnlineRoom{}}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	db := s.db(ctx).Where("user_id = ?", actor)
	if c.ID > 0 {
		db = db.Where("id < ?", c.ID)
	}
	var follows []model.SocialFollow
	if err = db.Order("id DESC").Limit(limit + 1).Find(&follows).Error; err != nil {
		return out, err
	}
	out.HasMore = len(follows) > limit
	if out.HasMore {
		follows = follows[:limit]
	}
	ids := []uint64{}
	for _, f := range follows {
		ids = append(ids, f.TargetUserID)
	}
	out.List, err = s.Directory.Online(ctx, s.db(ctx), actor, ids)
	if err != nil {
		return out, err
	}
	if out.HasMore {
		out.NextCursor = encodeCursor(0, follows[len(follows)-1].ID)
	}
	return out, nil
}

// consumeRelationshipEvents 批量确认内置无外部副作用的关系审计消费，避免每秒只处理一条关系事件。
func (s *Service) consumeRelationshipEvents(ctx context.Context) error {
	return s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		var ids []uint64
		q := tx.Model(&model.SocialOutboxEvent{}).Where("event_type = ? AND status = ? AND next_attempt_at <= ?", model.EventRelationship, model.EventPending, time.Now().UnixMilli()).Order("id").Limit(500)
		if tx.Dialector.Name() == "mysql" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		return tx.Model(&model.SocialOutboxEvent{}).Where("id IN ?", ids).Updates(map[string]interface{}{"status": model.EventDone, "lease_token": "", "last_error": ""}).Error
	})
}

// refreshWorkerPressure 只计数有界的待处理前缀。Worker停机10秒后敏感写入失败关闭，
// pending+failed达到50000时向新增写入施加背压；隐私撤销仍执行Redis写限流。
func (s *Service) refreshWorkerPressure(ctx context.Context) error {
	if s.Redis == nil {
		return ErrUnavailable
	}
	var n int64
	sub := s.db(ctx).Model(&model.SocialOutboxEvent{}).Select("id").Where("status IN ?", []model.EventStatus{model.EventPending, model.EventFailed}).Limit(50000)
	if err := s.db(ctx).Table("(?) AS backlog", sub).Count(&n).Error; err != nil {
		return err
	}
	pressure := 0
	if n >= 50000 {
		pressure = 1
	}
	return s.Redis.Set(ctx, redisPrefix+"worker:pressure", pressure, 10*time.Second).Err()
}
