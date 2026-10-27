package social

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	repo "tb_live_module/repository/social"
	"time"
)

type Action string

const (
	Follow      Action = "follow"
	Unfollow    Action = "unfollow"
	Block       Action = "block"
	Unblock     Action = "unblock"
	Mute        Action = "mute"
	Unmute      Action = "unmute"
	Subscribe   Action = "subscribe"
	Unsubscribe Action = "unsubscribe"
)

func (s *Service) ChangeRelationship(ctx context.Context, actor uint64, target req.Target, action Action, scope model.MuteScope) (res.Change, error) {
	defer s.observe(string(action), time.Now())()
	id, err := s.Resolve(ctx, target)
	if err != nil {
		return res.Change{}, err
	}
	if actor == id {
		return res.Change{}, ErrSelf
	}
	switch action {
	case Follow, Unfollow, Block, Unblock, Subscribe, Unsubscribe:
	case Mute, Unmute:
		if !scope.Valid() {
			return res.Change{}, ErrInvalid
		}
	default:
		return res.Change{}, ErrInvalid
	}
	changed := false
	err = s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		changed = false // 重试不能复用失败事务的返回状态。
		if _, e := repo.LockUsers(tx, actor, id); e != nil {
			return e
		}
		if e := s.Directory.Validate(ctx, tx, []uint64{actor}); e != nil {
			return e
		}
		if action == Follow || action == Block || action == Mute || action == Subscribe {
			if e := s.Directory.Validate(ctx, tx, []uint64{id}); e != nil {
				return e
			}
		}
		switch action {
		case Follow:
			blocked, e := repo.Blocked(tx, actor, id)
			if e != nil {
				return e
			}
			if blocked {
				return ErrBlocked
			}
			changed, e = repo.InsertAbsent(tx, &model.SocialFollow{UserID: actor, TargetUserID: id}, "user_id = ? AND target_user_id = ?", actor, id)
			if e != nil {
				return e
			}
			if changed {
				if e = repo.CountDelta(tx, actor, "following_count", 1); e != nil {
					return e
				}
				if e = repo.CountDelta(tx, id, "fans_count", 1); e != nil {
					return e
				}
				if s.Config.AutoSubscribe {
					isAnchor, e := s.Directory.IsAnchor(ctx, tx, id)
					if e != nil {
						return e
					}
					if isAnchor {
						if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.SocialLiveSubscription{UserID: actor, TargetUserID: id, Enabled: true}).Error; e != nil {
							return e
						}
					}
				}
			}
		case Unfollow:
			changed, err = removeFollow(tx, actor, id)
			if err != nil {
				return err
			}
		case Block:
			var e error
			changed, e = repo.InsertAbsent(tx, &model.SocialBlock{UserID: actor, TargetUserID: id}, "user_id = ? AND target_user_id = ?", actor, id)
			if e != nil {
				return e
			}
			// 即使已有拉黑，也检查/移除历史脏关系；整个操作只有一个事务。
			ab, e := removeFollow(tx, actor, id)
			if e != nil {
				return e
			}
			ba, e := removeFollow(tx, id, actor)
			if e != nil {
				return e
			}
			changed = changed || ab || ba
		case Unblock:
			result := tx.Where("user_id = ? AND target_user_id = ?", actor, id).Delete(&model.SocialBlock{})
			if result.Error != nil {
				return result.Error
			}
			changed = result.RowsAffected > 0
		case Mute:
			var e error
			changed, e = repo.InsertAbsent(tx, &model.SocialMute{UserID: actor, TargetUserID: id, Scope: scope}, "user_id = ? AND target_user_id = ? AND scope = ?", actor, id, scope)
			if e != nil {
				return e
			}
		case Unmute:
			result := tx.Where("user_id = ? AND target_user_id = ? AND scope = ?", actor, id, scope).Delete(&model.SocialMute{})
			if result.Error != nil {
				return result.Error
			}
			changed = result.RowsAffected > 0
		case Subscribe, Unsubscribe:
			enabled := action == Subscribe
			if enabled {
				ok, e := s.Directory.IsAnchor(ctx, tx, id)
				if e != nil {
					return e
				}
				if !ok {
					return ErrUnavailableUser
				}
				blocked, e := repo.Blocked(tx, actor, id)
				if e != nil {
					return e
				}
				if blocked {
					return ErrBlocked
				}
				follows, e := repo.Following(tx, actor, id)
				if e != nil {
					return e
				}
				if !follows {
					return ErrFollowRequired
				}
			}
			// 关闭也保留显式记录，避免下一次关注重新打开。
			row := model.SocialLiveSubscription{UserID: actor, TargetUserID: id, Enabled: false}
			inserted, insertErr := repo.InsertAbsent(tx, &row, "user_id = ? AND target_user_id = ?", actor, id)
			if insertErr != nil {
				return insertErr
			}
			result := tx.Model(&model.SocialLiveSubscription{}).Where("user_id = ? AND target_user_id = ? AND enabled <> ?", actor, id, enabled).Update("enabled", enabled)
			if result.Error != nil {
				return result.Error
			}
			changed = result.RowsAffected > 0 || inserted
		}
		if !changed {
			return nil
		}
		if e := repo.BumpVersions(tx, actor, id); e != nil {
			return e
		}
		return event(tx, model.EventRelationship, actor, id, "", string(action), "")
	})
	if err != nil {
		s.Metrics.Failures.Add(1)
		if i := operationIndex(string(action)); i >= 0 {
			s.Metrics.OperationFailures[i].Add(1)
		}
	} else {
		s.Metrics.Changes.Add(1)
	}
	return res.Change{Changed: changed && err == nil}, err
}
func removeFollow(tx *gorm.DB, a, b uint64) (bool, error) {
	r := tx.Where("user_id = ? AND target_user_id = ?", a, b).Delete(&model.SocialFollow{})
	if r.Error != nil {
		return false, r.Error
	}
	if r.RowsAffected > 0 {
		if e := repo.CountDelta(tx, a, "following_count", -1); e != nil {
			return false, e
		}
		if e := repo.CountDelta(tx, b, "fans_count", -1); e != nil {
			return false, e
		}
	}
	// 取消关注幂等关闭订阅，不物理删除偏好。
	sub := tx.Model(&model.SocialLiveSubscription{}).Where("user_id = ? AND target_user_id = ? AND enabled = ?", a, b, true).Update("enabled", false)
	return r.RowsAffected > 0 || sub.RowsAffected > 0, sub.Error
}

// CheckRelationship / CheckBlockRelationship 只读事实库，用于授权，绝不读取展示缓存。
func (s *Service) CheckRelationship(ctx context.Context, db *gorm.DB, a, b uint64) (following, blocked bool, err error) {
	if db == nil {
		return false, false, ErrUnavailable
	}
	db = db.WithContext(ctx)
	blocked, err = repo.Blocked(db, a, b)
	if err != nil || blocked {
		return
	}
	following, err = repo.Following(db, a, b)
	return
}
func (s *Service) CheckBlockRelationship(ctx context.Context, db *gorm.DB, a, b uint64) (bool, error) {
	if db == nil {
		return false, ErrUnavailable
	}
	return repo.Blocked(db.WithContext(ctx), a, b)
}
func (s *Service) BatchStatus(ctx context.Context, actor uint64, targets []req.Target) ([]res.Relationship, error) {
	defer s.observe("batch_status", time.Now())()
	ids, err := s.resolveMany(ctx, targets)
	if err != nil {
		return nil, err
	}
	ids = uniqueIDs(ids)
	// 两个端点版本组成键，旧读回填不会覆盖关系变更后的新版本。
	key := ""
	if s.Redis != nil {
		key, err = s.relationshipCacheKey(ctx, actor, ids)
		if err != nil {
			return nil, err
		}
	}
	db := s.db(ctx)
	profiles, err := s.Directory.Profiles(ctx, db, actor, ids)
	if err != nil {
		return nil, err
	}
	var cached []res.Relationship
	if s.cacheGet(ctx, key, &cached) {
		current := map[string]res.User{}
		for _, u := range profiles {
			current[u.AccountNo] = u
		}
		visible := make([]res.Relationship, 0, len(cached))
		for _, r := range cached {
			if u, ok := current[r.AccountNo]; ok {
				r.User = u
				visible = append(visible, r)
			}
		}
		return visible, nil
	}
	rows := map[uint64]*res.Relationship{}
	for _, id := range ids {
		if u, ok := profiles[id]; ok {
			rows[id] = &res.Relationship{User: u, MutedScopes: []model.MuteScope{}}
		}
	}
	// UNION ALL 在单条语句的快照内读取所有方向，避免关注/拉黑跨事务拼接。
	var facts []struct {
		OtherID uint64
		Kind    string
		Scope   model.MuteScope
	}
	err = db.Raw(`
		SELECT target_user_id AS other_id, 'following' AS kind, '' AS scope FROM social_follow WHERE user_id = ? AND target_user_id IN ?
		UNION ALL SELECT user_id, 'follows_you', '' FROM social_follow WHERE target_user_id = ? AND user_id IN ?
		UNION ALL SELECT target_user_id, 'blocked_by_me', '' FROM social_block WHERE user_id = ? AND target_user_id IN ?
		UNION ALL SELECT user_id, 'blocked_by_other', '' FROM social_block WHERE target_user_id = ? AND user_id IN ?
		UNION ALL SELECT target_user_id, 'mute', scope FROM social_mute WHERE user_id = ? AND target_user_id IN ?
		UNION ALL SELECT target_user_id, 'subscription', '' FROM social_live_subscription WHERE user_id = ? AND target_user_id IN ? AND enabled = ?`,
		actor, ids, actor, ids, actor, ids, actor, ids, actor, ids, actor, ids, true).Scan(&facts).Error
	if err != nil {
		return nil, err
	}
	for _, f := range facts {
		r := rows[f.OtherID]
		if r == nil {
			continue
		}
		switch f.Kind {
		case "following":
			r.Following = true
		case "follows_you":
			r.FollowsYou = true
		case "blocked_by_me":
			r.Blocked, r.BlockedByMe = true, true
		case "blocked_by_other":
			r.Blocked = true
		case "mute":
			r.MutedScopes = append(r.MutedScopes, f.Scope)
		case "subscription":
			r.Subscribed = true
		}
	}
	out := make([]res.Relationship, 0, len(rows))
	for _, id := range ids {
		if r := rows[id]; r != nil {
			if r.Blocked {
				r.Following, r.FollowsYou = false, false
			}
			r.Mutual = r.Following && r.FollowsYou
			r.Subscribed = r.Subscribed && r.Following && !r.Blocked
			out = append(out, *r)
		}
	}
	// Profile/账号状态不缓存，缓存载荷只在同请求现查 profile 后使用，见 cacheGet。
	s.cacheSet(ctx, key, out)
	return out, nil
}
func (s *Service) Counters(ctx context.Context, t req.Target) (res.Counters, error) {
	id, err := s.Resolve(ctx, t)
	if err != nil {
		return res.Counters{}, err
	}
	profiles, err := s.Directory.Profiles(ctx, s.db(ctx), 0, []uint64{id})
	if err != nil {
		return res.Counters{}, err
	}
	u, ok := profiles[id]
	if !ok {
		return res.Counters{}, ErrUnavailableUser
	}
	var c model.SocialUserCounter
	result := s.db(ctx).Where("user_id = ?", id).Limit(1).Find(&c)
	if result.Error != nil {
		return res.Counters{}, result.Error
	}
	return res.Counters{AccountNo: u.AccountNo, FollowingCount: c.FollowingCount, FansCount: c.FansCount}, nil
}

// ListUsers 只列本人关系；common 列本人和目标用户共同关注，不暴露目标完整私有列表。
func (s *Service) ListUsers(ctx context.Context, actor uint64, kind string, q req.CursorSearch, common uint64) (res.UserList, error) {
	out := res.UserList{List: []res.UserItem{}}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	db := s.db(ctx)
	var table, other string
	switch kind {
	case "following", "mutual", "common":
		table = "social_follow"
		other = "target_user_id"
	case "followers":
		table = "social_follow"
		other = "user_id"
	case "block":
		table = "social_block"
		other = "target_user_id"
	case "mute":
		table = "social_mute"
		other = "target_user_id"
	case "subscription":
		table = "social_live_subscription"
		other = "target_user_id"
	default:
		return out, ErrInvalid
	}
	query := db.Table(table + " AS r")
	if kind == "followers" {
		query = query.Where("r.target_user_id = ?", actor)
	} else {
		query = query.Where("r.user_id = ?", actor)
	}
	if kind == "common" {
		if common == 0 {
			return out, ErrInvalid
		}
		if err := s.Directory.Validate(ctx, db, []uint64{common}); err != nil {
			return out, err
		}
		blocked, err := repo.Blocked(db, actor, common)
		if err != nil {
			return out, err
		}
		if blocked {
			return out, ErrBlocked
		}
	}
	if c.ID > 0 {
		query = query.Where("r.id < ?", c.ID)
	}
	columns := "r.id, r.created_at, r." + other + " AS other_id"
	if kind == "mute" {
		columns += ", r.scope"
	}
	if kind == "subscription" {
		columns += ", r.enabled"
	}
	var records []struct {
		ID        uint64
		CreatedAt model.UTCTime
		OtherID   uint64
		Scope     model.MuteScope
		Enabled   bool
	}
	if err = query.Select(columns).Order("r.id DESC").Limit(limit + 1).Scan(&records).Error; err != nil {
		return out, err
	}
	out.HasMore = len(records) > limit
	if out.HasMore {
		records = records[:limit]
	}
	ids := []uint64{}
	for _, row := range records {
		ids = append(ids, row.OtherID)
	}
	// 先按本人索引取有界候选，再批量求交集；交集为空也不会扫描百万关注。
	var included map[uint64]bool
	if len(ids) > 0 && (kind == "mutual" || kind == "common") {
		var matches []uint64
		q := db.Model(&model.SocialFollow{})
		if kind == "mutual" {
			q = q.Where("target_user_id = ? AND user_id IN ?", actor, ids).Select("user_id")
		} else {
			q = q.Where("user_id = ? AND target_user_id IN ?", common, ids).Select("target_user_id")
		}
		if err = q.Scan(&matches).Error; err != nil {
			return out, err
		}
		included = make(map[uint64]bool, len(matches))
		for _, id := range matches {
			included[id] = true
		}
		ids = matches
	}
	profiles, err := s.Directory.Profiles(ctx, db, actor, uniqueIDs(ids))
	if err != nil {
		return out, err
	}
	for _, row := range records {
		if included != nil && !included[row.OtherID] {
			continue
		}
		if u, ok := profiles[row.OtherID]; ok {
			item := res.UserItem{User: u, CreatedAt: row.CreatedAt.UnixMilli(), Scope: row.Scope}
			if kind == "subscription" {
				v := row.Enabled
				item.Enabled = &v
			}
			out.List = append(out.List, item)
		}
	}
	// 根据扫描到的事实记录推进，过滤导致空页也必须继续使用 nextCursor。
	if len(records) > 0 && out.HasMore {
		last := records[len(records)-1]
		out.NextCursor = encodeCursor(0, last.ID)
	}
	return out, nil
}
