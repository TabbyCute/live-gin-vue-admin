package social

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	repo "tb_live_module/repository/social"
)

const historyMaxRecords = 1000
const historyBufferSeconds = 60
const historyShards = 16

type bufferedHistory struct {
	Actor      uint64            `json:"actor"`
	Report     req.HistoryReport `json:"report"`
	ReceivedAt int64             `json:"receivedAt"`
}

func historyKeys(actor uint64, r req.Resource) (string, string) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", actor, r.ResourceType, r.ResourceID)))
	tag := fmt.Sprintf("{history-%02d}", int(sum[0])%historyShards)
	return redisPrefix + tag + ":item:" + hex.EncodeToString(sum[:]), redisPrefix + tag + ":pending"
}

// 有界每分片最多1000条，首次入队5秒后可消费，后续心跳合并而不延长写入期限。
var bufferHistoryScript = redis.NewScript(`
local old=redis.call('GET',KEYS[1]);local incoming=cjson.decode(ARGV[1]);
if old then local prev=cjson.decode(old);if prev.report.epoch>incoming.report.epoch or (prev.report.epoch==incoming.report.epoch and (prev.report.sessionVersion>incoming.report.sessionVersion or (prev.report.sessionVersion==incoming.report.sessionVersion and prev.report.sequence>=incoming.report.sequence))) then return 0 end end
if not redis.call('ZSCORE',KEYS[2],KEYS[1]) and redis.call('ZCARD',KEYS[2])>=1000 then return -1 end
local now=redis.call('TIME');local ms=now[1]*1000+math.floor(now[2]/1000)
redis.call('SET',KEYS[1],ARGV[1],'EX',60);redis.call('ZADD',KEYS[2],'NX',ms+5000,KEYS[1]);return 1`)
var ackHistoryScript = redis.NewScript(`if redis.call('GET',KEYS[1])==ARGV[1] then redis.call('DEL',KEYS[1]);redis.call('ZREM',KEYS[2],KEYS[1]);return 1 end;if not redis.call('EXISTS',KEYS[1]) or redis.call('EXISTS',KEYS[1])==0 then redis.call('ZREM',KEYS[2],KEYS[1]) end;return 0`)

func (s *Service) OpenHistory(ctx context.Context, actor uint64, input req.HistoryOpen) (res.WatchSession, error) {
	out := res.WatchSession{}
	if !validResource(input.Resource) || len(input.RequestID) < 8 || len(input.RequestID) > 64 {
		return out, ErrInvalid
	}
	err := s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		counters, err := repo.LockUsers(tx, actor)
		if err != nil {
			return err
		}
		if err = s.Directory.Validate(ctx, tx, []uint64{actor}); err != nil {
			return err
		}
		resources, err := s.resources(ctx, tx, actor, []req.Resource{input.Resource})
		if err != nil {
			return err
		}
		if _, ok := resources[input.Resource]; !ok {
			return ErrResource
		}
		var row model.SocialWatchHistory
		query := tx.Where("user_id = ? AND resource_type = ? AND resource_id = ?", actor, input.ResourceType, input.ResourceID)
		if err = query.Limit(1).Find(&row).Error; err != nil {
			return err
		}
		epoch := counters[actor].HistoryEpoch
		if row.ID > 0 && row.OpenRequestID == input.RequestID && row.Epoch == epoch && !row.Hidden {
			out = res.WatchSession{SessionToken: row.SessionToken, SessionVersion: row.SessionVersion, Epoch: row.Epoch, PositionMs: row.PositionMs, Completed: row.Completed}
			return nil
		}
		// 从毫秒时间起步以兼容旧数据，之后在用户锁下单调递增。
		version := max(counters[actor].HistoryVersion, row.SessionVersion, uint64(time.Now().UnixMilli()))
		if version >= repo.MaxSafeInteger {
			return ErrConflict
		}
		isNew := row.ID == 0
		if isNew {
			row = model.SocialWatchHistory{UserID: actor, ResourceType: input.ResourceType, ResourceID: input.ResourceID}
		}
		row.SessionVersion = version + 1
		if err = tx.Model(&model.SocialUserCounter{}).Where("user_id = ?", actor).Update("history_version", row.SessionVersion).Error; err != nil {
			return err
		}
		row.SessionToken = uuid.NewString()
		row.Sequence = 0
		row.Epoch = epoch
		row.Hidden = false
		row.OpenRequestID = input.RequestID
		row.LastWatchedAt = time.Now().UnixMilli()
		if row.ResourceType == model.ResourceLiveRoom {
			row.PositionMs = 0
			row.Completed = false
		}
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
		// 一个用户最多1000条。仅在新建时清理超额，清除的token不能被旧缓冲重建。
		if isNew {
			var count int64
			if err = tx.Model(&model.SocialWatchHistory{}).Where("user_id = ?", actor).Count(&count).Error; err != nil {
				return err
			}
			if count > historyMaxRecords {
				var ids []uint64
				if err = tx.Model(&model.SocialWatchHistory{}).Where("user_id = ? AND id <> ?", actor, row.ID).Order("last_watched_at,id").Limit(int(count-historyMaxRecords)).Pluck("id", &ids).Error; err != nil {
					return err
				}
				if err = tx.Where("id IN ?", ids).Delete(&model.SocialWatchHistory{}).Error; err != nil {
					return err
				}
			}
		}
		out = res.WatchSession{SessionToken: row.SessionToken, SessionVersion: row.SessionVersion, Epoch: row.Epoch, PositionMs: row.PositionMs, Completed: row.Completed}
		return nil
	})
	return out, err
}
func validReport(r req.HistoryReport) bool {
	_, e := uuid.Parse(r.SessionToken)
	return e == nil && validResource(r.Resource) && r.SessionVersion > 0 && r.SessionVersion <= repo.MaxSafeInteger && r.Sequence > 0 && r.Sequence <= repo.MaxSafeInteger && r.Epoch <= repo.MaxSafeInteger && r.PositionMs >= 0 && r.PositionMs <= 604800000
}
func (s *Service) ReportHistory(ctx context.Context, actor uint64, r req.HistoryReport) (res.WatchAccepted, error) {
	if !validReport(r) {
		return res.WatchAccepted{}, ErrInvalid
	}
	if r.Checkpoint {
		accepted, err := s.persistHistory(ctx, bufferedHistory{Actor: actor, Report: r, ReceivedAt: time.Now().UnixMilli()})
		return res.WatchAccepted{Persisted: err == nil, Accepted: accepted}, err
	}
	if s.Redis == nil {
		return res.WatchAccepted{}, ErrUnavailable
	}
	if s.Resources[r.ResourceType] == nil {
		return res.WatchAccepted{}, ErrUnsupported
	}
	key, queue := historyKeys(actor, r.Resource)
	raw, _ := json.Marshal(bufferedHistory{Actor: actor, Report: r, ReceivedAt: time.Now().UnixMilli()})
	ctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	n, err := bufferHistoryScript.Run(ctx, s.Redis, []string{key, queue}, string(raw)).Int()
	if err != nil {
		return res.WatchAccepted{}, ErrUnavailable
	}
	if n < 0 {
		return res.WatchAccepted{}, ErrRateLimited
	}
	s.Metrics.HistoryBuffered.Add(1)
	return res.WatchAccepted{Accepted: n == 1, Persisted: false, MaxBufferAgeMs: historyBufferSeconds * 1000}, nil
}
func (s *Service) persistHistory(ctx context.Context, p bufferedHistory) (bool, error) {
	accepted := false
	r := p.Report
	err := s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		accepted = false
		counters, err := repo.LockUsers(tx, p.Actor)
		if err != nil {
			return err
		}
		if counters[p.Actor].HistoryEpoch != r.Epoch {
			return ErrConflict
		}
		if err = s.Directory.Validate(ctx, tx, []uint64{p.Actor}); err != nil {
			return err
		}
		var row model.SocialWatchHistory
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND resource_type = ? AND resource_id = ?", p.Actor, r.ResourceType, r.ResourceID).Limit(1).Find(&row).Error; err != nil {
			return err
		}
		if row.ID == 0 || row.Hidden || row.SessionToken != r.SessionToken || row.SessionVersion != r.SessionVersion || row.Epoch != r.Epoch {
			return ErrConflict
		}
		if row.Sequence >= r.Sequence {
			return nil
		}
		resources, err := s.resources(ctx, tx, p.Actor, []req.Resource{r.Resource})
		if err != nil {
			return err
		}
		resource, ok := resources[r.Resource]
		if !ok {
			return ErrResource
		}
		position := r.PositionMs
		completed := r.Completed
		if r.ResourceType == model.ResourceLiveRoom {
			position = 0
			completed = false
		} else if resource.DurationMs > 0 {
			if position > resource.DurationMs {
				position = resource.DurationMs
			}
			completed = completed && position >= resource.DurationMs-1000
		}
		// 服务端接收顺序由sessionVersion+seq决定；客户端时间不参与。
		now := time.Now().UnixMilli()
		at := p.ReceivedAt
		if at > now {
			at = now
		}
		if at < row.LastWatchedAt {
			at = row.LastWatchedAt
		}
		err = tx.Model(&row).Updates(map[string]interface{}{"sequence": r.Sequence, "position_ms": position, "completed": completed, "last_watched_at": at}).Error
		accepted = err == nil
		return err
	})
	if accepted && err == nil {
		s.Metrics.HistoryPersisted.Add(1)
	}
	return accepted, err
}

var releaseHistoryLease = redis.NewScript(`if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end;return 0`)

func (s *Service) FlushHistory(ctx context.Context) error {
	if s.Redis == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for shard := 0; shard < historyShards; shard++ {
		if err := s.flushHistoryShard(ctx, shard); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) flushHistoryShard(ctx context.Context, shard int) error {
	queue := fmt.Sprintf("%s{history-%02d}:pending", redisPrefix, shard)
	lease := fmt.Sprintf("%s{history-%02d}:lease", redisPrefix, shard)
	token := uuid.NewString()
	claimed, err := s.Redis.SetNX(ctx, lease, token, 15*time.Second).Result()
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 150*time.Millisecond)
		defer cancel()
		_ = releaseHistoryLease.Run(cleanup, s.Redis, []string{lease}, token).Err()
	}()
	keys, err := s.Redis.ZRangeByScore(ctx, queue, &redis.ZRangeBy{Min: "-inf", Max: fmt.Sprint(time.Now().UnixMilli()), Count: 32}).Result()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	values, err := s.Redis.MGet(ctx, keys...).Result()
	if err != nil {
		return err
	}
	for i, key := range keys {
		raw, _ := values[i].(string)
		var p bufferedHistory
		if raw != "" && json.Unmarshal([]byte(raw), &p) == nil && validReport(p.Report) {
			_, err = s.persistHistory(ctx, p)
			if err != nil && !isBusiness(err) {
				return err
			}
			if err != nil {
				s.Metrics.HistoryDropped.Add(1)
			}
		}
		if err = ackHistoryScript.Run(ctx, s.Redis, []string{key, queue}, raw).Err(); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) DeleteHistory(ctx context.Context, actor uint64, r *req.Resource) error {
	if r != nil && !validResource(*r) {
		return ErrInvalid
	}
	return s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		counters, err := repo.LockUsers(tx, actor)
		if err != nil {
			return err
		}
		q := tx.Where("user_id = ?", actor)
		if r != nil {
			q = q.Where("resource_type = ? AND resource_id = ?", r.ResourceType, r.ResourceID)
		} else {
			if counters[actor].HistoryEpoch >= repo.MaxSafeInteger {
				return ErrConflict
			}
			if err = tx.Model(&model.SocialUserCounter{}).Where("user_id = ?", actor).Update("history_epoch", gorm.Expr("history_epoch + 1")).Error; err != nil {
				return err
			}
		}
		return q.Delete(&model.SocialWatchHistory{}).Error
	})
}

// HistoryList 只展示已经接入的直播间所属用户；记录键仍可用于单条删除。
func (s *Service) HistoryList(ctx context.Context, actor uint64, q req.CursorSearch) (res.HistoryUserList, error) {
	out := res.HistoryUserList{List: []res.HistoryUserItem{}}
	if q.ResourceType != "" && q.ResourceType != model.ResourceLiveRoom {
		return out, ErrUnsupported
	}
	q.ResourceType = model.ResourceLiveRoom
	resources, err := s.historyRead(ctx, actor, q, nil)
	if err != nil {
		return out, err
	}
	out.HasMore, out.NextCursor = resources.HasMore, resources.NextCursor
	ids := make([]uint64, 0, len(resources.List))
	for _, r := range resources.List {
		ids = append(ids, r.OwnerUserID)
	}
	profiles, err := s.Directory.Profiles(ctx, s.db(ctx), actor, uniqueIDs(ids))
	if err != nil {
		return out, err
	}
	for _, r := range resources.List {
		if u, ok := profiles[r.OwnerUserID]; ok {
			out.List = append(out.List, res.HistoryUserItem{User: u, ResourceType: r.ResourceType, ResourceID: r.ResourceID, CreatedAt: r.CreatedAt, LastWatchedAt: r.LastWatchedAt})
		}
	}
	return out, nil
}
func (s *Service) HistoryProgress(ctx context.Context, actor uint64, refs []req.Resource) (res.ResourceList, error) {
	if len(refs) == 0 || len(refs) > 100 {
		return res.ResourceList{}, ErrInvalid
	}
	for _, r := range refs {
		if !validResource(r) {
			return res.ResourceList{}, ErrInvalid
		}
	}
	return s.historyRead(ctx, actor, req.CursorSearch{Limit: 100}, refs)
}
func (s *Service) historyRead(ctx context.Context, actor uint64, q req.CursorSearch, refs []req.Resource) (res.ResourceList, error) {
	out := res.ResourceList{List: []res.ResourceItem{}}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	db := s.db(ctx).Where("user_id = ? AND hidden = ? AND last_watched_at >= ?", actor, false, time.Now().AddDate(0, 0, -s.retentionDays()).UnixMilli())
	if refs != nil {
		pairs := [][]interface{}{}
		for _, r := range refs {
			pairs = append(pairs, []interface{}{r.ResourceType, r.ResourceID})
		}
		db = db.Where("(resource_type,resource_id) IN ?", pairs)
	}
	if q.ResourceType != "" {
		if !q.ResourceType.Valid() {
			return out, ErrInvalid
		}
		db = db.Where("resource_type = ?", q.ResourceType)
	}
	if c.ID > 0 {
		db = db.Where("last_watched_at < ? OR (last_watched_at = ? AND id < ?)", c.At, c.At, c.ID)
	}
	var rows []model.SocialWatchHistory
	if err = db.Select("id", "resource_type", "resource_id", "created_at", "last_watched_at", "position_ms", "completed").Order("last_watched_at DESC,id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return out, err
	}
	out.HasMore = len(rows) > limit
	if out.HasMore {
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		return out, nil
	}
	refs = []req.Resource{}
	for _, row := range rows {
		refs = append(refs, req.Resource{ResourceType: row.ResourceType, ResourceID: row.ResourceID})
	}
	visible, err := s.resources(ctx, s.db(ctx), actor, refs)
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		if resource, ok := visible[req.Resource{ResourceType: row.ResourceType, ResourceID: row.ResourceID}]; ok {
			out.List = append(out.List, res.ResourceItem{Resource: resource, CreatedAt: row.CreatedAt.UnixMilli(), LastWatchedAt: row.LastWatchedAt, PositionMs: row.PositionMs, Completed: row.Completed})
		}
	}
	if out.HasMore {
		last := rows[len(rows)-1]
		out.NextCursor = encodeCursor(last.LastWatchedAt, last.ID)
	}
	return out, nil
}
func (s *Service) retentionDays() int {
	days := s.Config.HistoryRetentionDays
	if days < 1 || days > 365 {
		return 90
	}
	return days
}
