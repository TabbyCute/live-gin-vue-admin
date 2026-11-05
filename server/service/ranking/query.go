package ranking

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"strconv"
	"tb_live_module/internal/modulekit"
	liveModel "tb_live_module/model/live"
	model "tb_live_module/model/ranking"
	req "tb_live_module/model/ranking/request"
	res "tb_live_module/model/ranking/response"
	live "tb_live_module/service/live"
	"time"
)

func (s *Service) validateQuery(kind string, q *req.Query) error {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Period == "" {
		q.Period = "day"
	}
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 {
		return ErrInvalid
	}
	for _, n := range []string{q.ScopeNo, q.GiftNo} {
		if n != "" && !modulekit.Number.MatchString(n) {
			return ErrInvalid
		}
	}
	if q.PeriodKey == "" {
		key, e := PeriodKey(q.Period, s.now(), s.zone())
		if e != nil {
			return e
		}
		q.PeriodKey = key
	}
	if q.Period == "all" {
		if q.PeriodKey != "all" {
			return ErrInvalid
		}
	} else {
		layout := "2006-01-02"
		if q.Period == "month" {
			layout = "2006-01"
		}
		t, e := time.Parse(layout, q.PeriodKey)
		if e != nil || t.Format(layout) != q.PeriodKey {
			return ErrInvalid
		}
		if q.Period == "week" && t.Weekday() != time.Monday {
			return ErrInvalid
		}
		if q.Period != "day" && q.Period != "week" && q.Period != "month" {
			return ErrInvalid
		}
	}
	switch kind {
	case "gift":
		if (q.ScopeType != "anchor" && q.ScopeType != "room" && q.ScopeType != "session") || q.ScopeNo == "" {
			return ErrInvalid
		}
	case "wealth":
		if q.ScopeType == "" {
			q.ScopeType = "platform"
		}
		if q.ScopeType != "platform" || q.ScopeNo != "" || q.GiftNo != "" {
			return ErrInvalid
		}
	case "charm":
		if q.ScopeType == "" {
			q.ScopeType = "platform"
		}
		if q.GiftNo != "" || !(q.ScopeType == "platform" && q.ScopeNo == "" || q.ScopeType == "category" && q.ScopeNo != "") {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func boardQuery(tx *gorm.DB, generation uint64, kind string, q req.Query) *gorm.DB {
	return tx.Model(&model.ScoreBucket{}).Where("generation=? AND rank_type=? AND period_type=? AND period_key=? AND scope_type=? AND scope_no=? AND gift_no=? AND score_sort>?", generation, kind, q.Period, q.PeriodKey, q.ScopeType, q.ScopeNo, q.GiftNo, sortKey("0"))
}
func lag(tx *gorm.DB, generation uint64) (int64, error) {
	var count int64
	e := tx.Table("gift_revenue_ledger AS r").Where("NOT EXISTS (SELECT 1 FROM ranking_generation_receipt p WHERE p.generation=? AND p.source_id=r.id)", generation).Count(&count).Error
	return count, e
}
func (s *Service) Board(ctx context.Context, user uint64, kind string, q req.Query) (res.Board, error) {
	if q.RankType != "" {
		return res.Board{}, ErrInvalid
	}
	if user == 0 {
		return res.Board{}, modulekit.Forbidden
	}
	return s.board(ctx, user, kind, q)
}
func (s *Service) AdminBoard(ctx context.Context, a Actor, q req.Query) (res.Board, error) {
	if e := s.authorize(s.db(ctx), a, "board", "GET"); e != nil {
		return res.Board{}, e
	}
	return s.board(ctx, 0, q.RankType, q)
}
func (s *Service) board(ctx context.Context, user uint64, kind string, q req.Query) (res.Board, error) {
	out := res.Board{List: []res.Entry{}}
	if e := s.validateQuery(kind, &q); e != nil {
		return out, e
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if user > 0 && kind == "gift" && (q.ScopeType == "room" || q.ScopeType == "session") {
			roomNo := q.ScopeNo
			if q.ScopeType == "session" {
				var session liveModel.LiveSession
				if e := tx.Where("session_no=?", q.ScopeNo).Take(&session).Error; e != nil {
					return modulekit.NotFound
				}
				var room liveModel.LiveRoom
				if e := tx.First(&room, session.RoomId).Error; e != nil {
					return modulekit.NotFound
				}
				roomNo = room.RoomNo
			}
			access, e := (&live.LiveAccessService{}).BatchCheckLiveAccess(ctx, tx, user, []string{roomNo}, false)
			if e != nil {
				return e
			}
			if len(access) != 1 {
				return modulekit.Forbidden
			}
		}
		var cp model.Checkpoint
		if e := tx.Where("consumer_name='gift-revenue'").Take(&cp).Error; e != nil {
			return e
		}
		if cp.Timezone != s.zone() || cp.LastError != "" {
			return ErrInvariant
		}
		var total int64
		if e := boardQuery(tx, cp.Generation, kind, q).Count(&total).Error; e != nil {
			return e
		}
		var rows []model.ScoreBucket
		if e := boardQuery(tx, cp.Generation, kind, q).Order("score_sort DESC, subject_no ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&rows).Error; e != nil {
			return e
		}
		var own string
		if user == 0 {
			own = ""
		} else if kind == "charm" {
			var a liveModel.LiveAnchor
			e := tx.Select("anchor_no").Where("user_id=?", user).Take(&a).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			own = a.AnchorNo
		} else {
			var u liveModel.LiveAccount
			if e := tx.Select("account_no").Where("id=?", user).Take(&u).Error; e != nil {
				return e
			}
			own = u.AccountNo
		}
		pageLength := len(rows)
		var mine model.ScoreBucket
		myIndex := -1
		myRank := int64(0)
		if own != "" {
			e := boardQuery(tx, cp.Generation, kind, q).Where("subject_no=?", own).Take(&mine).Error
			if e == nil {
				if e = boardQuery(tx, cp.Generation, kind, q).Where("score_sort>? OR (score_sort=? AND subject_no<?)", mine.ScoreSort, mine.ScoreSort, own).Count(&myRank).Error; e != nil {
					return e
				}
				myRank++
				for i, b := range rows {
					if b.SubjectNo == own {
						myIndex = i
					}
				}
				if myIndex < 0 {
					rows = append(rows, mine)
					myIndex = len(rows) - 1
				}
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		// One batch lookup for all subjects, then the shared public profile provider.
		numbers := []string{}
		for _, b := range rows {
			numbers = append(numbers, b.SubjectNo)
		}
		userIDs := map[string]uint64{}
		ids := []uint64{}
		if len(numbers) > 0 {
			if kind == "charm" {
				var aa []liveModel.LiveAnchor
				if e := tx.Select("anchor_no", "user_id").Where("anchor_no IN ?", numbers).Find(&aa).Error; e != nil {
					return e
				}
				for _, a := range aa {
					userIDs[a.AnchorNo] = a.UserId
					ids = append(ids, a.UserId)
				}
			} else {
				var uu []liveModel.LiveAccount
				if e := tx.Select("id", "account_no").Where("account_no IN ?", numbers).Find(&uu).Error; e != nil {
					return e
				}
				for _, u := range uu {
					userIDs[u.AccountNo] = uint64(u.ID)
					ids = append(ids, uint64(u.ID))
				}
			}
		}
		// Assemble profiles in two bounded batches without losing missing/deleted users.
		entries := make([]res.Entry, len(rows))
		for i, b := range rows {
			entries[i] = res.Entry{Rank: strconv.Itoa((q.Page-1)*q.PageSize + i + 1), Score: b.ScoreCoin, Quantity: b.Quantity, SubjectNo: b.SubjectNo, SubjectType: b.SubjectType}
		}
		for start := 0; start < len(ids); start += 100 {
			profiles, e := (live.SocialDirectory{}).Profiles(ctx, tx, user, ids[start:min(start+100, len(ids))])
			if e != nil {
				return e
			}
			for i, b := range rows {
				if p, ok := profiles[userIDs[b.SubjectNo]]; ok {
					entries[i].User = p
				}
			}
		}
		if myIndex >= 0 {
			my := entries[myIndex]
			my.Rank = strconv.FormatInt(myRank, 10)
			out.MyRank = &my
		}
		// Keep only the actual page (the appended own row is never an extra page item).
		out.List = entries[:pageLength]
		lagCount, e := lag(tx, cp.Generation)
		if e != nil {
			return e
		}
		out.Total = strconv.FormatInt(total, 10)
		out.Page = q.Page
		out.PageSize = q.PageSize
		out.RankType = kind
		out.Period = q.Period
		out.PeriodKey = q.PeriodKey
		out.Timezone = s.zone()
		out.ScopeType = q.ScopeType
		out.ScopeNo = q.ScopeNo
		out.GiftNo = q.GiftNo
		out.GeneratedAt = s.now()
		out.DataAsOf = cp.DataAsOf
		out.LagCount = strconv.FormatInt(lagCount, 10)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
