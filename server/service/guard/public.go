package guard

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"tb_live_module/internal/modulekit"
	model "tb_live_module/model/guard"
	res "tb_live_module/model/guard/response"
	liveModel "tb_live_module/model/live"
	"tb_live_module/service/live"
	"tb_live_module/service/social"
	"unicode"
	"unicode/utf8"
)

// Only explicitly public descriptive strings are exposed. Arbitrary entitlement
// JSON, internal keys, expiration dates and purchase/ledger numbers stay private.
func publicPlan(p model.PlanSnapshot) res.PublicPlan {
	out := res.PublicPlan{PlanNo: p.PlanNo, Name: p.Name, Level: p.Level, Priority: p.Priority, Benefits: []string{}}
	var config struct {
		Benefits []string `json:"publicBenefits"`
	}
	if len(p.BenefitConfig) > 4096 || json.Unmarshal([]byte(p.BenefitConfig), &config) != nil {
		return out
	}
	for _, b := range config.Benefits {
		b = strings.TrimSpace(b)
		if b == "" || !utf8.ValidString(b) || utf8.RuneCountInString(b) > 80 || strings.IndexFunc(b, unicode.IsControl) >= 0 {
			continue
		}
		out.Benefits = append(out.Benefits, b)
		if len(out.Benefits) == 8 {
			break
		}
	}
	return out
}

// AnchorMembersSnapshot is called only after home visibility is authorized in
// the same read-only Service.DB transaction. Entitlements are evaluated at the
// current server time, not the expiry worker's last cached status.
func (s *Service) AnchorMembersSnapshot(ctx context.Context, viewer, anchorID uint64, page, size int, enabled bool) (res.PublicMembers, error) {
	now := s.now()
	out := res.PublicMembers{List: []res.PublicMember{}, Plans: []res.PublicPlan{}, Page: page, PageSize: size, Enabled: enabled, GeneratedAt: now}
	if anchorID == 0 || page < 1 || page > 100 || size < 1 || size > 50 {
		return out, modulekit.Invalid
	}
	db := s.db(ctx)
	// Keep the JSON_TABLE input in its own query block. A direct outer-column
	// JSON_TABLE correlation fails on MySQL 8.0.46 during EXISTS optimization.
	// The inner membership lookup uses its primary key, not another anchor scan.
	activeSegment := `EXISTS (SELECT 1 FROM guard_membership AS am JOIN JSON_TABLE(am.projection, '$[*]' COLUMNS (seg_start BIGINT PATH '$.start', seg_end BIGINT PATH '$.end')) AS seg ON seg.seg_start<=? AND seg.seg_end>? WHERE am.id=m.id)`
	if db.Dialector.Name() == "sqlite" {
		activeSegment = `EXISTS (SELECT 1 FROM json_each(m.projection) AS seg WHERE json_extract(seg.value,'$.start')<=? AND json_extract(seg.value,'$.end')>?)`
	}
	query := func() *gorm.DB {
		return db.Table("guard_membership AS m").Where("m.anchor_id=? AND m.valid_until>?", anchorID, now).
			Where(activeSegment, now, now).
			Where("EXISTS (?)", db.Table("live_account AS pu").Select("1").Where("pu.id=m.user_id AND pu.deleted_at IS NULL AND pu.status=?", liveModel.LiveAccountStatusNormal).
				Scopes(social.VisibleOwnerScope(viewer, "pu.id", "1", 1, 2, 3)))
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return out, err
	}
	var rows []model.Membership
	if err := query().Select("m.*").Order("m.id DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error; err != nil {
		return out, err
	}
	ids := make([]uint64, 0, len(rows))
	ledgerNos := make([]string, 0, len(rows))
	for _, m := range rows {
		ids = append(ids, m.UserID)
		ledgerNos = append(ledgerNos, m.LedgerNo)
	}
	profiles, err := (live.SocialDirectory{}).Profiles(ctx, db, viewer, ids)
	if err != nil {
		return out, err
	}
	sealed := make(map[string]string, len(rows))
	if len(ledgerNos) > 0 {
		var ll []model.Ledger
		if err = db.Select("ledger_no", "after_snapshot").Where("ledger_no IN ?", ledgerNos).Find(&ll).Error; err != nil {
			return out, err
		}
		for _, l := range ll {
			sealed[l.LedgerNo] = l.AfterSnapshot
		}
	}
	for _, m := range rows {
		if m.LedgerNo == "" || sealed[m.LedgerNo] != m.Projection {
			return out, ErrInvariant
		}
		var segments []Segment
		if json.Unmarshal([]byte(m.Projection), &segments) != nil {
			return out, ErrInvariant
		}
		plan, _, _, _, status := projection(segments, now)
		if status != "active" {
			return out, ErrInvariant
		}
		var snapshot *model.PlanSnapshot
		for _, seg := range segments {
			if seg.Start <= now && seg.End > now && seg.Plan.PlanNo == plan && (snapshot == nil || seg.Plan.Priority > snapshot.Priority) {
				p := seg.Plan
				snapshot = &p
			}
		}
		p, ok := profiles[m.UserID]
		if snapshot == nil || !ok {
			return out, ErrInvariant
		}
		out.List = append(out.List, res.PublicMember{User: p, Plan: publicPlan(*snapshot)})
	}
	var plans []model.Plan
	if err = db.Where("status=?", "enabled").Order("sort_order, plan_no").Limit(50).Find(&plans).Error; err != nil {
		return out, err
	}
	for _, p := range plans {
		out.Plans = append(out.Plans, publicPlan(model.PlanSnapshot{PlanNo: p.PlanNo, Name: p.Name, Level: p.Level, Priority: p.Priority, BenefitConfig: p.BenefitConfig}))
	}
	out.Total = strconv.FormatInt(total, 10)
	return out, nil
}
