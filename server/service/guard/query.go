package guard

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"tb_live_module/global"
	"tb_live_module/internal/modulekit"
	model "tb_live_module/model/guard"
	req "tb_live_module/model/guard/request"
	res "tb_live_module/model/guard/response"
	liveModel "tb_live_module/model/live"
	live "tb_live_module/service/live"
	"time"
)

func page(q *req.Query) error {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 {
		return ErrInvalid
	}
	for _, n := range []string{q.AnchorNo, q.AccountNo, q.MembershipNo} {
		if n != "" && !modulekit.Number.MatchString(n) {
			return ErrInvalid
		}
	}
	if q.Status != "" && q.Status != "active" && q.Status != "expired" && q.Status != "enabled" && q.Status != "disabled" && q.Status != "pending" && q.Status != "processing" && q.Status != "failed" && q.Status != "succeeded" {
		return ErrInvalid
	}
	return nil
}
func (s *Service) Plans(ctx context.Context, a *Actor, q req.Query) (res.Page, error) {
	out := res.Page{}
	if e := page(&q); e != nil {
		return out, e
	}
	db := s.db(ctx)
	if a != nil {
		if e := s.authorize(db, *a, "plan/list", "GET"); e != nil {
			return out, e
		}
	}
	query := db.Model(&model.Plan{})
	if a == nil {
		query = query.Where("status='enabled'")
	} else if q.Status != "" {
		query = query.Where("status=?", q.Status)
	}
	var total int64
	var rows []model.Plan
	if e := query.Count(&total).Error; e != nil {
		return out, e
	}
	e := query.Order("sort_order, plan_no").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&rows).Error
	return res.Page{List: rows, Total: strconv.FormatInt(total, 10), Page: q.Page, PageSize: q.PageSize, GuardEnabled: global.GuardReady.Load() && global.GVA_CONFIG.Gift.GuardEnabled, Actions: s.actions(db, a)}, e
}
func (s *Service) Members(ctx context.Context, user uint64, mode string, a *Actor, q req.Query) (res.Page, error) {
	out := res.Page{}
	if a == nil && (q.AccountNo != "" || q.MembershipNo != "") {
		return out, ErrInvalid
	}
	if e := page(&q); e != nil {
		return out, e
	}
	db := s.db(ctx)
	if a != nil {
		if e := s.authorize(db, *a, "member/"+mode, "GET"); e != nil {
			return out, e
		}
	} else if user == 0 {
		return out, modulekit.Forbidden
	}
	query := db.Model(&model.Membership{})
	now := s.now()
	if a == nil && (mode == "mine" || mode == "status") {
		query = query.Where("user_id=?", user)
	}
	if q.AnchorNo != "" {
		var anchor liveModel.LiveAnchor
		if e := db.Where("anchor_no=?", q.AnchorNo).Take(&anchor).Error; e != nil {
			return out, modulekit.NotFound
		}
		query = query.Where("anchor_id=?", anchor.ID)
	} else if mode == "status" || mode == "members" {
		return out, ErrInvalid
	}
	if a != nil && q.AccountNo != "" {
		var u liveModel.LiveAccount
		if e := db.Where("account_no=?", q.AccountNo).Take(&u).Error; e != nil {
			return out, modulekit.NotFound
		}
		query = query.Where("user_id=?", u.ID)
	}
	if q.MembershipNo != "" {
		query = query.Where("membership_no=?", q.MembershipNo)
	}
	if mode == "members" || q.Status == "active" {
		query = query.Where("valid_until>?", now)
	} else if q.Status == "expired" {
		query = query.Where("valid_until<=?", now)
	}
	var total int64
	if e := query.Count(&total).Error; e != nil {
		return out, e
	}
	var rows []model.Membership
	if e := query.Order("id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&rows).Error; e != nil {
		return out, e
	}
	ids := []uint64{}
	anchors := []uint64{}
	for _, m := range rows {
		ids = append(ids, m.UserID)
		anchors = append(anchors, m.AnchorID)
	}
	profiles, e := (live.SocialDirectory{}).Profiles(ctx, db, user, ids)
	if e != nil {
		return out, e
	}
	names := map[uint64]string{}
	if len(anchors) > 0 {
		var aa []liveModel.LiveAnchor
		if e = db.Select("id", "anchor_no").Where("id IN ?", anchors).Find(&aa).Error; e != nil {
			return out, e
		}
		for _, v := range aa {
			names[uint64(v.ID)] = v.AnchorNo
		}
	}
	list := []res.Membership{}
	ledgerNos := []string{}
	for _, m := range rows {
		ledgerNos = append(ledgerNos, m.LedgerNo)
	}
	sealed := map[string]string{}
	if len(ledgerNos) > 0 {
		var ledgers []model.Ledger
		if e = db.Select("ledger_no", "after_snapshot").Where("ledger_no IN ?", ledgerNos).Find(&ledgers).Error; e != nil {
			return out, e
		}
		for _, l := range ledgers {
			sealed[l.LedgerNo] = l.AfterSnapshot
		}
	}
	for _, m := range rows {
		if m.LedgerNo == "" || sealed[m.LedgerNo] != m.Projection {
			return out, ErrInvariant
		}
		var seg []Segment
		if json.Unmarshal([]byte(m.Projection), &seg) != nil {
			return out, ErrInvariant
		}
		plan, start, end, _, status := projection(seg, now)
		var snapshot *model.PlanSnapshot
		for _, v := range seg {
			if v.Plan.PlanNo == plan && v.Start <= now && v.End > now && (snapshot == nil || v.Plan.Priority > snapshot.Priority) {
				copy := v.Plan
				snapshot = &copy
			}
		}
		remaining := max(0, end-now)
		days := remaining / DayMillis
		if remaining%DayMillis != 0 {
			days++
		}
		list = append(list, res.Membership{ActivePlan: snapshot, MembershipNo: m.MembershipNo, User: profiles[m.UserID], AnchorNo: names[m.AnchorID], ActivePlanNo: plan, StartedAt: start, ExpiresAt: end, RemainingMillis: strconv.FormatInt(remaining, 10), RemainingDays: strconv.FormatInt(days, 10), Status: status, GeneratedAt: now})
	}
	return res.Page{List: list, Total: strconv.FormatInt(total, 10), Page: q.Page, PageSize: q.PageSize, Actions: s.actions(db, a)}, nil
}
func (s *Service) AdminList(ctx context.Context, a Actor, kind string, q req.Query) (res.Page, error) {
	out := res.Page{}
	if e := page(&q); e != nil {
		return out, e
	}
	db := s.db(ctx)
	if e := s.authorize(db, a, kind+"/list", "GET"); e != nil {
		return out, e
	}
	var dest any
	switch kind {
	case "grant":
		dest = &[]model.Grant{}
	case "compensation":
		dest = &[]model.Compensation{}
	case "ledger":
		dest = &[]model.Ledger{}
	case "task":
		dest = &[]model.Task{}
	default:
		return out, ErrInvalid
	}
	query := db.Model(dest)
	if kind == "task" {
		query = query.Where("kind <> 'control'")
	} else if q.MembershipNo != "" {
		query = query.Where("membership_no=?", q.MembershipNo)
	}
	var total int64
	if e := query.Count(&total).Error; e != nil {
		return out, e
	}
	e := query.Order("id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(dest).Error
	return res.Page{List: dest, Total: strconv.FormatInt(total, 10), Page: q.Page, PageSize: q.PageSize, Actions: s.actions(db, &a)}, e
}

// ExpireBatch also switches to a lower-priority unexpired source. Reads never
// trust the worker's last status: projection() evaluates current server time.
func (s *Service) ExpireBatch(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	var members []model.Membership
	if e := s.db(ctx).Where("next_transition_at>0 AND next_transition_at<=?", s.now()).Where("NOT EXISTS (SELECT 1 FROM guard_task t WHERE t.source_no=guard_membership.membership_no AND t.kind='expire' AND (t.retry_count>=20 OR t.next_retry_at>?))", s.now()).Order("next_transition_at, id").Limit(limit).Find(&members).Error; e != nil {
		return 0, e
	}
	count := 0
	var first error
	for _, candidate := range members {
		e := s.transaction(ctx, func(tx *gorm.DB) error {
			m, e := lockMember(tx, candidate.UserID, candidate.AnchorID, s.now())
			if e != nil {
				return e
			}
			if m.NextTransitionAt == 0 || m.NextTransitionAt > s.now() {
				return nil
			}
			e = s.saveProjection(tx, &m, "expire", modulekit.No("ge_"), m.ActivePlanNo, 0, s.now())
			if e == nil {
				count++
				e = tx.Model(&model.Task{}).Where("kind='expire' AND source_no=?", m.MembershipNo).Updates(map[string]any{"status": "succeeded", "completed_at": s.now(), "last_error": "", "next_retry_at": 0, "retry_count": 0, "updated_at": s.now()}).Error
			}
			return e
		})
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			if first == nil {
				first = e
			}
			finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			task := model.Task{TaskNo: modulekit.No("gt_"), Kind: "expire", SourceNo: candidate.MembershipNo, Status: "failed", RetryCount: 1, LastError: "到期核验失败，请核对来源与投影后重试", NextRetryAt: s.now() + 30000, CreatedAt: s.now(), UpdatedAt: s.now()}
			saveErr := s.db(finish).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_no"}}, DoUpdates: clause.Assignments(map[string]any{"status": "failed", "retry_count": gorm.Expr("retry_count+1"), "last_error": task.LastError, "next_retry_at": task.NextRetryAt, "updated_at": task.UpdatedAt})}).Create(&task).Error
			cancel()
			if saveErr != nil {
				return count, saveErr
			}
		}
	}
	return count, first
}
