package guard

import (
	"encoding/json"
	"math"
	"sort"
	model "tb_live_module/model/guard"
)

// Segment is a projection of one immutable source, never a standalone right to grant time.
type Segment struct {
	GrantNo string             `json:"grantNo"`
	Plan    model.PlanSnapshot `json:"plan"`
	Start   int64              `json:"start"`
	End     int64              `json:"end"`
	Revoked int64              `json:"revoked"`
}
type fact struct {
	seq   int64
	grant *model.Grant
	comp  *model.Compensation
}

// Replay preserves past use. Compensation only shortens a source's unused suffix;
// later renewals move forward no earlier than the compensation instant.
func Replay(grants []model.Grant, comps []model.Compensation) ([]Segment, error) {
	events := make([]fact, 0, len(grants)+len(comps))
	for i := range grants {
		events = append(events, fact{seq: grants[i].Sequence, grant: &grants[i]})
	}
	for i := range comps {
		events = append(events, fact{seq: comps[i].Sequence, comp: &comps[i]})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].seq < events[j].seq })
	segments := []Segment{}
	pending := map[string]int64{}
	requested := map[string]int64{}
	durations := map[string]int64{}
	previous := int64(0)
	for _, e := range events {
		if e.seq <= previous {
			return nil, ErrInvariant
		}
		previous = e.seq
		if g := e.grant; g != nil {
			var p model.PlanSnapshot
			if json.Unmarshal([]byte(g.PlanSnapshot), &p) != nil || p.PlanNo != g.PlanNo || g.GrantedDuration <= 0 || durations[g.GrantNo] != 0 {
				return nil, ErrInvariant
			}
			durations[g.GrantNo] = g.GrantedDuration
			if requested[g.GrantNo] > g.GrantedDuration {
				return nil, ErrInvariant
			}
			start := g.EffectiveAt
			for _, s := range segments {
				if s.Plan.PlanNo == g.PlanNo && s.End > start {
					start = s.End
				}
			}
			duration := g.GrantedDuration - pending[g.GrantNo]
			if start > math.MaxInt64-duration {
				return nil, ErrInvariant
			}
			segments = append(segments, Segment{GrantNo: g.GrantNo, Plan: p, Start: start, End: start + duration, Revoked: pending[g.GrantNo]})
			continue
		}
		c := e.comp
		if c.Duration <= 0 || requested[c.GrantNo] > math.MaxInt64-c.Duration {
			return nil, ErrInvariant
		}
		requested[c.GrantNo] += c.Duration
		if durations[c.GrantNo] > 0 && requested[c.GrantNo] > durations[c.GrantNo] {
			return nil, ErrInvariant
		}
		found := -1
		for i := range segments {
			if segments[i].GrantNo == c.GrantNo {
				found = i
				break
			}
		}
		if found < 0 {
			pending[c.GrantNo] += c.Duration
			continue
		}
		target := &segments[found]
		cut := c.Duration
		available := target.End - max(target.Start, c.CreatedAt)
		if available < 0 {
			available = 0
		}
		if cut > available {
			cut = available
		}
		oldEnd := target.End
		target.End -= cut
		target.Revoked += cut
		for i := found + 1; i < len(segments); i++ {
			next := &segments[i]
			if next.Plan.PlanNo != target.Plan.PlanNo || next.Start < oldEnd {
				continue
			}
			next.Start -= cut
			next.End -= cut
		}
	}
	return segments, nil
}
func projection(segments []Segment, now int64) (plan string, start, end, next int64, status string) {
	status = "expired"
	priority := -1
	for _, seg := range segments {
		if seg.End <= now || seg.End <= seg.Start {
			continue
		}
		transition := seg.End
		if seg.Start > now {
			transition = seg.Start
		}
		if next == 0 || transition < next {
			next = transition
		}
		if seg.Start <= now && (seg.Plan.Priority > priority || seg.Plan.Priority == priority && seg.Plan.PlanNo < plan) {
			plan = seg.Plan.PlanNo
			priority = seg.Plan.Priority
			start = seg.Start
			end = seg.End
			status = "active"
		}
	}
	// Same-plan renewals form one continuous visible membership.
	if plan != "" {
		for _, seg := range segments {
			if seg.Plan.PlanNo == plan && seg.Start <= end && seg.End > end {
				end = seg.End
			}
		}
		for i := len(segments) - 1; i >= 0; i-- {
			seg := segments[i]
			if seg.Plan.PlanNo == plan && seg.End >= start && seg.Start < start && seg.End > seg.Start {
				start = seg.Start
			}
		}
	}
	return
}
