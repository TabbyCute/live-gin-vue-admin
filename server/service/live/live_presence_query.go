package live

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"strconv"
	"strings"
	model "tb_live_module/model/live"
	req "tb_live_module/model/live/request"
	res "tb_live_module/model/live/response"
	socialService "tb_live_module/service/social"
	"time"
)

func presenceSnapshot(cp model.LivePresenceCheckpoint) res.PresenceStats {
	return res.PresenceStats{RoomNo: cp.RoomNo, SessionNo: cp.SessionNo, ConnectionCount: "", OnlineCount: "", MemberCount: "", GuestCount: "", TotalOnlineCount: "", ViewCount: presenceUint(cp.ViewCount), ViewerCount: presenceUint(cp.ViewerCount), PeakOnlineCount: presenceUint(cp.PeakOnlineCount), LikeCount: presenceUint(cp.LikeCount), TotalWatchDurationMs: presenceUint(cp.TotalWatchDurationMs), CheckpointAt: cp.UpdatedAt, UpdatedAt: cp.UpdatedAt, FinalizedAt: cp.FinalizedAt, Stale: true, DataLost: cp.DataLost}
}
func (s *PresenceService) readStats(ctx context.Context, cp model.LivePresenceCheckpoint) (res.PresenceStats, error) {
	if cp.FinalizedAt > 0 {
		r := presenceSnapshot(cp)
		r.Stale = false
		r.ConnectionCount = "0"
		r.OnlineCount = "0"
		r.MemberCount = "0"
		r.GuestCount = "0"
		r.TotalOnlineCount = "0"
		return r, nil
	}
	t := presenceToken{Room: cp.RoomNo, Session: cp.SessionNo, Epoch: cp.Epoch}
	v, e := s.run(ctx, t, map[string]any{"op": "stats"})
	if e != nil {
		return presenceSnapshot(cp), e
	}
	out, e := s.sessionResult(t, v)
	if e != nil {
		return presenceSnapshot(cp), e
	}
	out.DataLost = cp.DataLost
	return out.PresenceStats, nil
}
func (s *PresenceService) Stats(ctx context.Context, i PresenceIdentity, room string) (res.PresenceStats, error) {
	if !presenceNumber.MatchString(room) {
		return res.PresenceStats{}, ErrPresenceInvalid
	}
	if e := s.ready(); e != nil {
		return res.PresenceStats{}, e
	}
	if e := s.access(ctx, i, room, ""); e != nil {
		return res.PresenceStats{}, e
	}
	var cp model.LivePresenceCheckpoint
	e := s.DB.WithContext(ctx).Table("live_presence_checkpoint p").Select("p.*").Joins("JOIN live_session s ON s.session_no=p.session_no JOIN live_room r ON r.current_session_id=s.id").Where("r.room_no=?", room).Take(&cp).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return res.PresenceStats{RoomNo: room, ConnectionCount: "0", OnlineCount: "0", MemberCount: "0", GuestCount: "0", TotalOnlineCount: "0", ViewCount: "0", ViewerCount: "0", PeakOnlineCount: "0", LikeCount: "0", TotalWatchDurationMs: "0", UpdatedAt: time.Now().UnixMilli()}, nil
	}
	if e != nil {
		return res.PresenceStats{}, ErrPresenceUnavailable
	}
	return s.readStats(ctx, cp)
}

// AdminStats batches MySQL and Redis reads; one request covers the visible page.
func (s *PresenceService) AdminStats(ctx context.Context, q req.PresenceAdminBatch) ([]res.PresenceStats, error) {
	if len(q.RoomNos)+len(q.SessionNos) < 1 || len(q.RoomNos)+len(q.SessionNos) > 100 {
		return nil, ErrPresenceInvalid
	}
	for _, v := range append(append([]string{}, q.RoomNos...), q.SessionNos...) {
		if !presenceNumber.MatchString(v) {
			return nil, ErrPresenceInvalid
		}
	}
	var sessions []struct {
		model.LiveSession
		RoomNo string
	}
	db := s.DB.WithContext(ctx).Table("live_session s").Select("s.*,r.room_no").Joins("JOIN live_room r ON r.id=s.room_id")
	if len(q.RoomNos) > 0 && len(q.SessionNos) > 0 {
		db = db.Where("(s.session_no IN ? OR (r.room_no IN ? AND r.current_session_id=s.id))", q.SessionNos, q.RoomNos)
	} else if len(q.SessionNos) > 0 {
		db = db.Where("s.session_no IN ?", q.SessionNos)
	} else {
		db = db.Where("r.room_no IN ? AND r.current_session_id=s.id", q.RoomNos)
	}
	if e := db.Limit(100).Find(&sessions).Error; e != nil {
		return nil, ErrPresenceUnavailable
	}
	nos := []string{}
	for _, v := range sessions {
		nos = append(nos, v.SessionNo)
	}
	var cps []model.LivePresenceCheckpoint
	if len(nos) > 0 {
		if e := s.DB.WithContext(ctx).Where("session_no IN ?", nos).Find(&cps).Error; e != nil {
			return nil, ErrPresenceUnavailable
		}
	}
	cpmap := map[string]model.LivePresenceCheckpoint{}
	for _, v := range cps {
		cpmap[v.SessionNo] = v
	}
	// EVALs are pipelined to avoid N network round trips for a 100-room admin page.
	type pending struct {
		cp  model.LivePresenceCheckpoint
		cmd *redis.Cmd
	}
	wait := []pending{}
	out := []res.PresenceStats{}
	rc, cancel := presenceRedisContext(ctx)
	defer cancel()
	var pipe redis.Pipeliner
	if s.Redis != nil {
		pipe = s.Redis.Pipeline()
	}
	for _, ls := range sessions {
		cp, ok := cpmap[ls.SessionNo]
		if !ok {
			cp = model.LivePresenceCheckpoint{RoomNo: ls.RoomNo, SessionNo: ls.SessionNo, ViewCount: ls.ViewCount, ViewerCount: ls.ViewerCount, PeakOnlineCount: uint64(ls.PeakOnlineCount), LikeCount: ls.LikeCount, TotalWatchDurationMs: ls.TotalWatchDurationMs, FinalizedAt: ls.StatsFinalizedAt}
		}
		r := presenceSnapshot(cp)
		if !ok || cp.FinalizedAt > 0 {
			r.Stale = false
			r.ConnectionCount = "0"
			r.OnlineCount = "0"
			r.MemberCount = "0"
			r.GuestCount = "0"
			r.TotalOnlineCount = "0"
			out = append(out, r)
			continue
		}
		if pipe == nil {
			out = append(out, r)
			continue
		}
		b, _ := json.Marshal(map[string]any{"op": "stats", "epoch": cp.Epoch})
		wait = append(wait, pending{cp, pipe.Eval(rc, presenceStateLua, presenceKeys(cp.SessionNo), string(b))})
	}
	if pipe != nil && len(wait) > 0 {
		_, _ = pipe.Exec(rc)
	}
	for _, p := range wait {
		r := presenceSnapshot(p.cp)
		if v, e := p.cmd.Slice(); e == nil && len(v) > 1 && v[0] == "ok" {
			if got, e := s.sessionResult(presenceToken{Room: p.cp.RoomNo, Session: p.cp.SessionNo}, v); e == nil {
				r = got.PresenceStats
				r.DataLost = p.cp.DataLost
			}
		}
		out = append(out, r)
	}
	// Rooms without a current session have no live connections. Resolve only existing
	// requested room numbers; an arbitrary caller cannot manufacture a room row.
	if len(q.RoomNos) > 0 {
		found := map[string]bool{}
		for _, v := range out {
			found[v.RoomNo] = true
		}
		var rooms []model.LiveRoom
		if e := s.DB.WithContext(ctx).Select("room_no").Where("room_no IN ? AND current_session_id=0", q.RoomNos).Find(&rooms).Error; e != nil {
			return nil, ErrPresenceUnavailable
		}
		for _, room := range rooms {
			if !found[room.RoomNo] {
				r := presenceSnapshot(model.LivePresenceCheckpoint{RoomNo: room.RoomNo})
				r.Stale = false
				r.ConnectionCount = "0"
				r.MemberCount = "0"
				r.OnlineCount = "0"
				r.GuestCount = "0"
				r.TotalOnlineCount = "0"
				out = append(out, r)
			}
		}
	}
	return out, nil
}
func (s *PresenceService) Viewers(ctx context.Context, i PresenceIdentity, q req.PresenceQuery, active bool) (res.PresenceViewers, error) {
	out := res.PresenceViewers{List: []res.PresenceViewer{}}
	if !presenceNumber.MatchString(q.RoomNo) || len(q.Cursor) > 256 || q.Limit < 0 || q.Limit > 100 {
		return out, ErrPresenceInvalid
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if e := s.access(ctx, i, q.RoomNo, ""); e != nil {
		return out, e
	}
	var cp model.LivePresenceCheckpoint
	e := s.DB.WithContext(ctx).Table("live_presence_checkpoint p").Select("p.*").Joins("JOIN live_session s ON s.session_no=p.session_no JOIN live_room r ON r.current_session_id=s.id").Where("r.room_no=? AND p.finalized_at=0", q.RoomNo).Take(&cp).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if e != nil {
		return out, ErrPresenceUnavailable
	}
	lower := "-"
	if q.Cursor != "" {
		b, err := base64.RawURLEncoding.Strict().DecodeString(q.Cursor)
		parts := strings.SplitN(string(b), "|", 2)
		if err != nil || len(parts) != 2 || parts[0] != cp.Epoch || len(parts[1]) > 96 || len(parts[1]) < 20 {
			return out, ErrPresenceInvalid
		}
		lower = "(" + parts[1]
	}
	keys := presenceKeys(cp.SessionNo)
	rc, cancel := presenceRedisContext(ctx)
	indices, e := s.Redis.ZRangeByLex(rc, keys[9], &redis.ZRangeBy{Min: lower, Max: "+", Offset: 0, Count: int64(q.Limit + 1)}).Result()
	cancel()
	if e != nil {
		return out, ErrPresenceUnavailable
	}
	out.HasMore = len(indices) > q.Limit
	if out.HasMore {
		indices = indices[:q.Limit]
	}
	if len(indices) == 0 {
		return out, nil
	}
	names := []string{}
	ids := []uint64{}
	for _, v := range indices {
		parts := strings.SplitN(v, ":u:", 2)
		if len(parts) != 2 {
			return out, ErrPresenceCorrupt
		}
		id, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil || id == 0 {
			return out, ErrPresenceCorrupt
		}
		ids = append(ids, id)
		names = append(names, "u:"+parts[1])
	}
	rc, cancel = presenceRedisContext(ctx)
	p := s.Redis.Pipeline()
	actors := p.HMGet(rc, keys[5], names...)
	scores := []*redis.FloatCmd{}
	for _, name := range names {
		scores = append(scores, p.ZScore(rc, keys[3], name))
	}
	_, e = p.Exec(rc)
	cancel()
	if e != nil && !errors.Is(e, redis.Nil) {
		return out, ErrPresenceUnavailable
	}
	profiles, e := (SocialDirectory{}).Profiles(ctx, s.DB, i.Actor, ids)
	if e != nil {
		return out, ErrPresenceUnavailable
	}
	rel, e := (&socialService.Service{}).BatchAccessRelationships(ctx, s.DB, i.Actor, ids)
	if e != nil {
		return out, ErrPresenceUnavailable
	}
	var owner uint64
	if e := s.DB.WithContext(ctx).Table("live_room r").Select("a.user_id").Joins("JOIN live_anchor a ON a.id=r.anchor_id").Where("r.room_no=?", q.RoomNo).Scan(&owner).Error; e != nil {
		return out, ErrPresenceUnavailable
	}
	ownerRelations, e := (&socialService.Service{}).BatchAccessRelationships(ctx, s.DB, owner, ids)
	if e != nil {
		return out, ErrPresenceUnavailable
	}
	now := time.Now().UnixMilli()
	for idx, raw := range actors.Val() {
		if raw == nil || scores[idx].Val() <= float64(now) {
			continue
		}
		var a presenceActor
		if json.Unmarshal([]byte(fmt.Sprint(raw)), &a) != nil {
			return out, ErrPresenceCorrupt
		}
		profile, ok := profiles[ids[idx]]
		if !ok || rel[ids[idx]].Blocked || ownerRelations[ids[idx]].Blocked {
			continue
		}
		typ := a.Activity
		if !active {
			typ = "heartbeat"
		}
		out.List = append(out.List, res.PresenceViewer{User: profile, LastActiveAt: a.LastActive, LastActivityType: typ})
	}
	if out.HasMore {
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(cp.Epoch + "|" + indices[len(indices)-1]))
	}
	return out, nil
}
