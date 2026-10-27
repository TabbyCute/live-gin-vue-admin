package social

import (
	"context"
	"gorm.io/gorm"
	"strconv"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	repo "tb_live_module/repository/social"
	"time"
)

func formatID(id uint64) string { return strconv.FormatUint(id, 10) }
func (s *Service) AuditCounters(ctx context.Context, t req.Target, repair bool) (res.CounterAudit, error) {
	out := res.CounterAudit{}
	id, err := s.Resolve(ctx, t)
	if err != nil {
		return out, err
	}
	err = s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		rows, err := repo.LockUsers(tx, id)
		if err != nil {
			return err
		}
		c := rows[id]
		var following, fans int64
		if err = tx.Model(&model.SocialFollow{}).Where("user_id = ?", id).Count(&following).Error; err != nil {
			return err
		}
		if err = tx.Model(&model.SocialFollow{}).Where("target_user_id = ?", id).Count(&fans).Error; err != nil {
			return err
		}
		out = res.CounterAudit{Counters: res.Counters{AccountNo: t.AccountNo, FollowingCount: c.FollowingCount, FansCount: c.FansCount}, ActualFollowing: uint64(following), ActualFans: uint64(fans), Mismatch: c.FollowingCount != uint64(following) || c.FansCount != uint64(fans)}
		if out.Mismatch {
			s.Metrics.CounterDifferences.Add(1)
		}
		if !repair || !out.Mismatch {
			return nil
		}
		if err = tx.Model(&model.SocialUserCounter{}).Where("user_id = ?", id).Updates(map[string]interface{}{"following_count": following, "fans_count": fans}).Error; err != nil {
			return err
		}
		if err = repo.BumpVersions(tx, id); err != nil {
			return err
		}
		return event(tx, model.EventRelationship, id, 0, "", "counter_repaired", "")
	})
	return out, err
}
func (s *Service) EventList(ctx context.Context, q req.CursorSearch) (res.EventList, error) {
	out := res.EventList{List: []res.Event{}}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	db := s.db(ctx)
	if c.ID > 0 {
		db = db.Where("id < ?", c.ID)
	}
	var rows []model.SocialOutboxEvent
	if err = db.Order("id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return out, err
	}
	out.HasMore = len(rows) > limit
	if out.HasMore {
		rows = rows[:limit]
	}
	for _, r := range rows {
		out.List = append(out.List, res.Event{EventID: r.EventID, EventType: r.EventType, Status: r.Status, CreatedAt: r.CreatedAt.UnixMilli(), Attempts: r.Attempts, NextAttemptAt: r.NextAttemptAt, LastError: r.LastError})
	}
	if out.HasMore {
		out.NextCursor = encodeCursor(0, rows[len(rows)-1].ID)
	}
	return out, nil
}
func (s *Service) ReplayEvent(ctx context.Context, id string) (res.Change, error) {
	r := s.db(ctx).Model(&model.SocialOutboxEvent{}).Where("event_id = ? AND status = ?", id, model.EventFailed).Updates(map[string]interface{}{"status": model.EventPending, "attempts": 0, "last_error": "", "lease_token": "", "next_attempt_at": time.Now().UnixMilli()})
	return res.Change{Changed: r.RowsAffected > 0}, r.Error
}

// Health 对每个状态最多读取1001个索引候选，避免健康轮询全表COUNT。
func (s *Service) Health(ctx context.Context) (map[string]interface{}, error) {
	const sampleLimit = 1000
	events := make([]map[string]interface{}, 0, 4)
	for _, status := range []model.EventStatus{model.EventPending, model.EventFailed, model.EventDone, model.EventSkipped} {
		var rows []model.SocialOutboxEvent
		if err := s.db(ctx).Select("id", "created_at").Where("status = ?", status).Limit(sampleLimit + 1).Find(&rows).Error; err != nil {
			return nil, err
		}
		lowerBound := len(rows) > sampleLimit
		if lowerBound {
			rows = rows[:sampleLimit]
		}
		var oldest int64
		for _, row := range rows {
			at := row.CreatedAt.UnixMilli()
			if oldest == 0 || at < oldest {
				oldest = at
			}
		}
		events = append(events, map[string]interface{}{"status": status, "count": len(rows), "countIsLowerBound": lowerBound, "sampleOldestCreatedAt": oldest})
	}
	return map[string]interface{}{"events": events, "metrics": s.MetricSnapshot(), "deliveryChannelConfigured": s.Sink != nil}, nil
}

// Anomalies 有界检查拉黑后仍有关注的脏关系，游标使用关注事实ID。
func (s *Service) Anomalies(ctx context.Context, q req.CursorSearch) (res.UserList, error) {
	out := res.UserList{List: []res.UserItem{}}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	db := s.db(ctx).Table("social_follow f")
	if c.ID > 0 {
		db = db.Where("f.id > ?", c.ID)
	}
	var rows []model.SocialFollow
	if err = db.Select("f.*").Order("f.id").Limit(limit + 1).Find(&rows).Error; err != nil {
		return out, err
	}
	out.HasMore = len(rows) > limit
	if out.HasMore {
		rows = rows[:limit]
	}
	pairs := make([][]interface{}, 0, len(rows)*2)
	for _, r := range rows {
		pairs = append(pairs, []interface{}{r.UserID, r.TargetUserID}, []interface{}{r.TargetUserID, r.UserID})
	}
	var blocks []model.SocialBlock
	if len(pairs) > 0 {
		if err = s.db(ctx).Select("user_id", "target_user_id").Where("(user_id,target_user_id) IN ?", pairs).Find(&blocks).Error; err != nil {
			return out, err
		}
	}
	blocked := make(map[[2]uint64]bool, len(blocks))
	for _, b := range blocks {
		blocked[[2]uint64{b.UserID, b.TargetUserID}] = true
	}
	ids := []uint64{}
	for _, r := range rows {
		if blocked[[2]uint64{r.UserID, r.TargetUserID}] || blocked[[2]uint64{r.TargetUserID, r.UserID}] {
			ids = append(ids, r.UserID)
		}
	}
	profiles, err := s.Directory.Profiles(ctx, s.db(ctx), 0, uniqueIDs(ids))
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		if !blocked[[2]uint64{r.UserID, r.TargetUserID}] && !blocked[[2]uint64{r.TargetUserID, r.UserID}] {
			continue
		}
		if p, ok := profiles[r.UserID]; ok {
			out.List = append(out.List, res.UserItem{User: p, CreatedAt: r.CreatedAt.UnixMilli()})
		}
	}
	if out.HasMore {
		out.NextCursor = encodeCursor(0, rows[len(rows)-1].ID)
	}
	return out, nil
}
