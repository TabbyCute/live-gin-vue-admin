package guard

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"tb_live_module/internal/modulekit"
	model "tb_live_module/model/guard"
)

func (s *Service) PlanSnapshotInTx(tx *gorm.DB, number string) (model.PlanSnapshot, error) {
	var p model.Plan
	e := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("plan_no=? AND status='enabled'", number).Take(&p).Error
	if e != nil {
		return model.PlanSnapshot{}, e
	}
	return model.PlanSnapshot{PlanNo: p.PlanNo, Name: p.Name, Level: p.Level, Priority: p.Priority, BenefitConfig: p.BenefitConfig, Version: p.Version}, nil
}
func validateInput(in model.GrantInput) (model.BenefitSnapshot, error) {
	var snapshot model.BenefitSnapshot
	if !modulekit.Number.MatchString(in.GrantNo) || !modulekit.Number.MatchString(in.GiftOrderNo) || in.UserID == 0 || in.AnchorID == 0 || in.DurationDays < 1 || in.DurationDays > MaxDays || len(in.BenefitSnapshot) > 8192 {
		return snapshot, ErrInvalid
	}
	if json.Unmarshal([]byte(in.BenefitSnapshot), &snapshot) != nil || snapshot.BenefitType != "anchor_guard" || snapshot.Status != "enabled" || snapshot.GuardPlanNo != in.PlanNo || snapshot.Plan.PlanNo != in.PlanNo || snapshot.Plan.Priority < 0 || snapshot.Plan.Version < 1 || snapshot.TotalDurationDays != strconv.FormatInt(in.DurationDays, 10) {
		return snapshot, ErrInvalid
	}
	return snapshot, nil
}
func lockMember(tx *gorm.DB, user, anchor uint64, now int64) (model.Membership, error) {
	m := model.Membership{MembershipNo: modulekit.No("gm_"), UserID: user, AnchorID: anchor, Status: "expired", Projection: "[]", UpdatedAt: now}
	if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error; e != nil {
		return m, e
	}
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=? AND anchor_id=?", user, anchor).Take(&m).Error
	if e == nil && now < m.UpdatedAt {
		return m, ErrConflict
	}
	return m, e
}
func loadFacts(tx *gorm.DB, m model.Membership) ([]model.Grant, []model.Compensation, error) {
	var g []model.Grant
	var c []model.Compensation
	if e := tx.Where("membership_no=?", m.MembershipNo).Order("sequence").Limit(maxFacts + 1).Find(&g).Error; e != nil {
		return nil, nil, e
	}
	if e := tx.Where("membership_no=?", m.MembershipNo).Order("sequence").Limit(maxFacts + 1).Find(&c).Error; e != nil {
		return nil, nil, e
	}
	if len(g)+len(c) > maxFacts {
		return nil, nil, ErrInvariant
	}
	return g, c, nil
}
func (s *Service) saveProjection(tx *gorm.DB, m *model.Membership, kind, source, plan string, delta int64, now int64) error {
	if kind != "rebuild" && m.Version > 0 {
		var sealed model.Ledger
		if e := tx.Where("ledger_no=? AND membership_no=?", m.LedgerNo, m.MembershipNo).Take(&sealed).Error; e != nil {
			return e
		}
		if sealed.AfterSnapshot != m.Projection {
			return ErrInvariant
		}
	}
	g, c, e := loadFacts(tx, *m)
	if e != nil {
		return e
	}
	segments, e := Replay(g, c)
	if e != nil {
		return e
	}
	if kind == "rebuild" {
		m.Sequence = 0
		for _, v := range g {
			m.Sequence = max(m.Sequence, v.Sequence)
		}
		for _, v := range c {
			m.Sequence = max(m.Sequence, v.Sequence)
		}
	}
	raw, e := json.Marshal(segments)
	if e != nil {
		return e
	}
	before := m.Projection
	var oldSegments []Segment
	if json.Unmarshal([]byte(before), &oldSegments) != nil && kind != "rebuild" {
		return ErrInvariant
	}
	if kind == "compensate" {
		var oldRevoked, newRevoked int64
		for _, v := range oldSegments {
			oldRevoked += v.Revoked
		}
		for _, v := range segments {
			newRevoked += v.Revoked
		}
		delta = oldRevoked - newRevoked
	}
	m.ValidUntil = 0
	priorGrant := map[string]model.Grant{}
	for _, source := range g {
		priorGrant[source.GrantNo] = source
	}
	for _, segment := range segments {
		if segment.End > segment.Start {
			m.ValidUntil = max(m.ValidUntil, segment.End)
		}
		state := "active"
		if segment.Start > now {
			state = "scheduled"
		}
		if segment.End <= now {
			state = "expired"
		}
		if segment.Revoked > 0 && segment.End == segment.Start {
			state = "compensated"
		}
		previous := priorGrant[segment.GrantNo]
		if previous.CompensatedDuration == segment.Revoked && previous.ExpiresAt == segment.End && previous.Status == state {
			continue
		}
		if e = tx.Model(&model.Grant{}).Where("grant_no=?", segment.GrantNo).Updates(map[string]any{"compensated_duration": segment.Revoked, "expires_at": segment.End, "status": state, "updated_at": now}).Error; e != nil {
			return e
		}
	}
	m.ActivePlanNo, m.StartedAt, m.ExpiresAt, m.NextTransitionAt, m.Status = projection(segments, now)
	m.Projection = string(raw)
	m.Version++
	m.UpdatedAt = now
	m.LedgerNo = modulekit.No("gl_")
	if e = tx.Save(m).Error; e != nil {
		return e
	}
	return tx.Create(&model.Ledger{LedgerNo: m.LedgerNo, MembershipNo: m.MembershipNo, EntryType: kind, SourceNo: source, PlanNo: plan, DurationDelta: delta, BeforeSnapshot: before, AfterSnapshot: string(raw), CreatedAt: now}).Error
}
func (s *Service) Fulfill(ctx context.Context, in model.GrantInput) error {
	snap, e := validateInput(in)
	if e != nil {
		return e
	}
	planRaw, _ := json.Marshal(snap.Plan)
	return s.transaction(ctx, func(tx *gorm.DB) error {
		now := s.now()
		m, e := lockMember(tx, in.UserID, in.AnchorID, now)
		if e != nil {
			return e
		}
		var existing model.Grant
		e = tx.Where("grant_no=?", in.GrantNo).Take(&existing).Error
		if e == nil {
			if existing.SourceNo != in.GiftOrderNo || existing.UserID != in.UserID || existing.AnchorID != in.AnchorID || existing.DurationDays != in.DurationDays || existing.PlanSnapshot != string(planRaw) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		m.Sequence++
		commandRaw, _ := json.Marshal(in)
		var incompatible int64
		if e = tx.Model(&model.Compensation{}).Where("grant_no=? AND (membership_no<>? OR source_snapshot<>?)", in.GrantNo, m.MembershipNo, string(commandRaw)).Count(&incompatible).Error; e != nil {
			return e
		}
		if incompatible > 0 {
			return ErrInvariant
		}
		g := model.Grant{GrantNo: in.GrantNo, MembershipNo: m.MembershipNo, SourceType: "gift_benefit", SourceNo: in.GiftOrderNo, SourceEntryNo: in.GrantNo, UserID: in.UserID, AnchorID: in.AnchorID, PlanNo: in.PlanNo, PlanSnapshot: string(planRaw), DurationDays: in.DurationDays, GrantedDuration: in.DurationDays * DayMillis, Status: "granted", Sequence: m.Sequence, EffectiveAt: now, ExpiresAt: now + in.DurationDays*DayMillis, CreatedAt: now, UpdatedAt: now}
		if e = tx.Create(&g).Error; e != nil {
			return e
		}
		kind := "grant"
		if m.Version > 0 {
			kind = "renew"
		}
		return s.saveProjection(tx, &m, kind, in.GrantNo, in.PlanNo, g.GrantedDuration, now)
	})
}
func (s *Service) Revoke(ctx context.Context, in model.CompensationInput) error {
	commandRaw, _ := json.Marshal(in.Grant)
	snapshot, e := validateInput(in.Grant)
	if e != nil {
		return e
	}
	planRaw, _ := json.Marshal(snapshot.Plan)
	if !modulekit.Number.MatchString(in.CompensationNo) || !modulekit.Number.MatchString(in.RefundNo) || in.DurationDays < 1 || in.DurationDays > in.Grant.DurationDays {
		return ErrInvalid
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		now := s.now()
		m, e := lockMember(tx, in.Grant.UserID, in.Grant.AnchorID, now)
		if e != nil {
			return e
		}
		var existing model.Compensation
		e = tx.Where("compensation_no=?", in.CompensationNo).Take(&existing).Error
		if e == nil {
			if existing.GrantNo != in.Grant.GrantNo || existing.MembershipNo != m.MembershipNo || existing.SourceNo != in.RefundNo || existing.Duration != in.DurationDays*DayMillis || existing.SourceSnapshot != string(commandRaw) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var grants []model.Grant
		if e = tx.Where("grant_no=?", in.Grant.GrantNo).Find(&grants).Error; e != nil {
			return e
		}
		if len(grants) > 0 && (grants[0].MembershipNo != m.MembershipNo || grants[0].DurationDays != in.Grant.DurationDays || grants[0].SourceNo != in.Grant.GiftOrderNo || grants[0].PlanSnapshot != string(planRaw)) {
			return ErrInvariant
		}
		var total int64
		var incompatible int64
		if e = tx.Model(&model.Compensation{}).Where("grant_no=? AND source_snapshot<>?", in.Grant.GrantNo, string(commandRaw)).Count(&incompatible).Error; e != nil {
			return e
		}
		if incompatible > 0 {
			return ErrInvariant
		}
		if e = tx.Model(&model.Compensation{}).Where("grant_no=?", in.Grant.GrantNo).Select("COALESCE(SUM(duration),0)").Scan(&total).Error; e != nil {
			return e
		}
		if total > (in.Grant.DurationDays-in.DurationDays)*DayMillis {
			return ErrInvalid
		}
		m.Sequence++
		c := model.Compensation{SourceSnapshot: string(commandRaw), CompensationNo: in.CompensationNo, GrantNo: in.Grant.GrantNo, MembershipNo: m.MembershipNo, SourceType: "gift_refund", SourceNo: in.RefundNo, SourceEntryNo: in.CompensationNo, Duration: in.DurationDays * DayMillis, Reason: "礼物退款撤回未使用守护", Status: "succeeded", Sequence: m.Sequence, CreatedAt: now, CompletedAt: now}
		if e = tx.Create(&c).Error; e != nil {
			return e
		}
		return s.saveProjection(tx, &m, "compensate", in.CompensationNo, in.Grant.PlanNo, -c.Duration, now)
	})
}
