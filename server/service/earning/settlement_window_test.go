package earning_test

import (
	"github.com/stretchr/testify/require"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	es "tb_live_module/service/earning"
	"testing"
	"time"
)

func TestSettlementSelectedWindow(t *testing.T)      { settlementSelectedWindow(t, false) }
func TestMySQLSettlementSelectedWindow(t *testing.T) { settlementSelectedWindow(t, true) }

func settlementSelectedWindow(t *testing.T, mysql bool) {
	s, g, _ := fixture(t, mysql)
	rule := exchange(t, s, "1", "1")
	now := time.Now().Truncate(time.Second)
	s.Now = func() time.Time { return now }
	start := now.Add(-7 * 24 * time.Hour)
	for i, timestamp := range []time.Time{start.Add(-time.Millisecond), start, now.Add(-time.Millisecond), now} {
		g.Now = func() time.Time { return timestamp }
		send(t, g, []string{"before_window", "at_window_start", "before_window_end", "at_window_end"}[i], "1")
	}
	r := er.CreateSettlement{RequestID: "selected_window_001", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: start.UnixMilli(), PeriodEnd: now.UnixMilli()}
	for _, invalid := range []er.CreateSettlement{
		{RequestID: "future_window", AnchorNo: r.AnchorNo, RuleNo: r.RuleNo, PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd + 1},
		{RequestID: "empty_window", AnchorNo: r.AnchorNo, RuleNo: r.RuleNo, PeriodStart: r.PeriodEnd, PeriodEnd: r.PeriodEnd},
	} {
		_, e := s.CreateSettlement(ctx, financeAdmin, invalid)
		require.ErrorIs(t, e, es.ErrInvalid)
	}
	_, e := s.CreateSettlement(ctx, es.Actor{}, r)
	require.ErrorIs(t, e, es.ErrForbidden)
	unknown := r
	unknown.RuleNo = "er_nonexistent"
	_, e = s.CreateSettlement(ctx, financeAdmin, unknown)
	require.ErrorIs(t, e, es.ErrExchange)
	st, e := s.CreateSettlement(ctx, financeAdmin, r)
	require.NoError(t, e)
	require.Equal(t, int64(2), st.ItemCount)
	require.Equal(t, int64(8), st.Amount)
	var amounts []int64
	require.NoError(t, s.DB.Model(&em.Account{}).Pluck("available", &amounts).Error)
	require.Empty(t, amounts, "preview cannot credit a withdrawable account")
	replayed, e := s.CreateSettlement(ctx, financeAdmin, r)
	require.NoError(t, e)
	require.Equal(t, st.SettlementNo, replayed.SettlementNo)
	changed := r
	changed.PeriodStart--
	_, e = s.CreateSettlement(ctx, financeAdmin, changed)
	require.ErrorIs(t, e, es.ErrConflict)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: st.SettlementNo, Approve: true, Reason: "核验七天边界"})
	require.NoError(t, e)
	account(t, s, 8, 0, 0)
	r.RequestID = "selected_window_002"
	_, e = s.CreateSettlement(ctx, financeAdmin, r)
	require.Error(t, e, "approved facts cannot be settled a second time")
	account(t, s, 8, 0, 0)
}
