package guard

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	model "tb_live_module/model/guard"
	"testing"
)

func grantFact(no, plan string, priority int, at, duration, seq int64) model.Grant {
	raw, _ := json.Marshal(model.PlanSnapshot{PlanNo: plan, Priority: priority, Version: 1})
	return model.Grant{GrantNo: no, PlanNo: plan, PlanSnapshot: string(raw), EffectiveAt: at, GrantedDuration: duration, Sequence: seq}
}
func TestReplayRenewRefundPreservesPast(t *testing.T) {
	g := []model.Grant{grantFact("first", "p", 1, 100, 100, 1), grantFact("second", "p", 1, 120, 100, 2)}
	c := []model.Compensation{{GrantNo: "first", Duration: 100, CreatedAt: 150, Sequence: 3}}
	segments, e := Replay(g, c)
	require.NoError(t, e)
	require.Equal(t, []Segment{{GrantNo: "first", Plan: model.PlanSnapshot{PlanNo: "p", Priority: 1, Version: 1}, Start: 100, End: 150, Revoked: 50}, {GrantNo: "second", Plan: model.PlanSnapshot{PlanNo: "p", Priority: 1, Version: 1}, Start: 150, End: 250}}, segments)
	plan, start, end, _, status := projection(segments, 175)
	require.Equal(t, "p", plan)
	require.Equal(t, int64(100), start)
	require.Equal(t, int64(250), end)
	require.Equal(t, "active", status)
}
func TestReplayCompensationBeforeGrantAndLevels(t *testing.T) {
	g := []model.Grant{grantFact("a", "low", 1, 100, 100, 2), grantFact("b", "high", 10, 100, 20, 3)}
	c := []model.Compensation{{GrantNo: "a", Duration: 30, CreatedAt: 90, Sequence: 1}}
	seg, e := Replay(g, c)
	require.NoError(t, e)
	require.Equal(t, int64(170), seg[0].End)
	plan, _, _, _, _ := projection(seg, 110)
	require.Equal(t, "high", plan)
	plan, _, _, _, _ = projection(seg, 120)
	require.Equal(t, "low", plan)
	_, _, _, _, status := projection(seg, 170)
	require.Equal(t, "expired", status)
	c = append(c, model.Compensation{GrantNo: "a", Duration: 71, CreatedAt: 115, Sequence: 4})
	_, e = Replay(g, c)
	require.ErrorIs(t, e, ErrInvariant)
}
func TestReplayExpiredRefundAndInvalidSequence(t *testing.T) {
	g := []model.Grant{grantFact("a", "p", 1, 100, 20, 1)}
	c := []model.Compensation{{GrantNo: "a", Duration: 20, CreatedAt: 130, Sequence: 2}}
	seg, e := Replay(g, c)
	require.NoError(t, e)
	require.Equal(t, int64(0), seg[0].Revoked)
	require.Equal(t, int64(120), seg[0].End)
	c[0].Sequence = 1
	_, e = Replay(g, c)
	require.ErrorIs(t, e, ErrInvariant)
}
func FuzzReplay(f *testing.F) {
	f.Add(int64(10), int64(5), int64(3))
	f.Fuzz(func(t *testing.T, duration, used, refund int64) {
		duration = duration%100000 + 1
		if duration <= 0 {
			return
		}
		if used < 0 || used > duration || refund < 1 || refund > duration {
			return
		}
		g := []model.Grant{grantFact("a", "p", 1, 100, duration, 1)}
		c := []model.Compensation{{GrantNo: "a", Duration: refund, CreatedAt: 100 + used, Sequence: 2}}
		seg, e := Replay(g, c)
		require.NoError(t, e)
		require.GreaterOrEqual(t, seg[0].End, 100+used)
		require.LessOrEqual(t, seg[0].Revoked, refund)
	})
}
