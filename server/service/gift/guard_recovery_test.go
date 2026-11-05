package gift_test

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	gift "tb_live_module/model/gift"
	gm "tb_live_module/model/guard"
	gr "tb_live_module/model/guard/request"
	rm "tb_live_module/model/ranking"
	rq "tb_live_module/model/ranking/request"
	"tb_live_module/service/guard"
	"testing"
)

func TestGuardTaskLeaseRecoveryFencesOldWorker(t *testing.T) {
	s, gs, _, clock := engagementFixture(t, false)
	_, e := send(s, "lease_order_test", "1")
	require.NoError(t, e)
	var g gift.BenefitGrant
	require.NoError(t, s.DB.Take(&g).Error)
	in := gm.GrantInput{GrantNo: g.GrantNo, GiftOrderNo: g.GiftOrderNo, UserID: g.ReceiverUserID, AnchorID: g.AnchorID, PlanNo: "plan01", DurationDays: 1, BenefitSnapshot: g.BenefitSnapshot}
	claimed, resume, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		result <- gs.RunTask(ctx, "grant", g.GrantNo, func(c context.Context) error { e := gs.Fulfill(c, in); close(claimed); <-resume; return e })
	}()
	<-claimed
	clock.Add(61000)
	require.NoError(t, gs.RunTask(ctx, "grant", g.GrantNo, func(c context.Context) error { return gs.Fulfill(c, in) }))
	close(resume)
	require.ErrorIs(t, <-result, guard.ErrConflict)
	count(t, s, &gm.Grant{}, 1)
	count(t, s, &gm.Ledger{}, 1)
	var task gm.Task
	require.NoError(t, s.DB.Where("source_no=?", g.GrantNo).Take(&task).Error)
	require.Equal(t, "succeeded", task.Status)
	require.Equal(t, 2, task.RetryCount)
}
func TestGuardCompensationRejectsChangedHistoricalPlan(t *testing.T) {
	s, gs, _, _ := engagementFixture(t, false)
	_, e := send(s, "snapshot_order01", "1")
	require.NoError(t, e)
	_, e = s.ProcessBenefits(ctx, 20)
	require.NoError(t, e)
	var g gift.BenefitGrant
	require.NoError(t, s.DB.Take(&g).Error)
	var snapshot gm.BenefitSnapshot
	require.NoError(t, json.Unmarshal([]byte(g.BenefitSnapshot), &snapshot))
	snapshot.Plan.Priority++
	raw, e := json.Marshal(snapshot)
	require.NoError(t, e)
	in := gm.CompensationInput{CompensationNo: "comp_snapshot01", RefundNo: "refund_snapshot01", DurationDays: 1, Grant: gm.GrantInput{GrantNo: g.GrantNo, GiftOrderNo: g.GiftOrderNo, UserID: g.ReceiverUserID, AnchorID: g.AnchorID, PlanNo: "plan01", DurationDays: 1, BenefitSnapshot: string(raw)}}
	require.ErrorIs(t, gs.Revoke(ctx, in), guard.ErrInvariant)
	count(t, s, &gm.Compensation{}, 0)
	count(t, s, &gm.Ledger{}, 1)
	require.NoError(t, gs.VerifyAll(ctx))
}
func TestGuardExpiryCorruptionRecordsFailureAndRequiresRepair(t *testing.T) {
	s, gs, _, clock := engagementFixture(t, false)
	_, e := send(s, "expiry_failure01", "1")
	require.NoError(t, e)
	_, e = s.ProcessBenefits(ctx, 20)
	require.NoError(t, e)
	var m gm.Membership
	require.NoError(t, s.DB.Take(&m).Error)
	require.NoError(t, s.DB.Model(&m).Update("projection", "{damaged-json").Error)
	clock.Add(2 * guard.DayMillis)
	_, e = gs.ExpireBatch(ctx, 20)
	require.ErrorIs(t, e, guard.ErrInvariant)
	var task gm.Task
	require.NoError(t, s.DB.Where("kind='expire'").Take(&task).Error)
	require.Equal(t, "failed", task.Status)
	require.NotEmpty(t, task.LastError)
	result, e := gs.AdminAction(ctx, guard.Actor{ID: 1}, "rebuild", gr.Action{RequestID: "repair_projection1", Number: m.MembershipNo})
	require.NoError(t, e)
	require.Contains(t, result, `"consistent":true`)
	require.Contains(t, result, `"consistentBefore":false`)
	require.NoError(t, gs.VerifyAll(ctx))
	_, e = gs.AdminAction(ctx, guard.Actor{ID: 1}, "task/retry", gr.Action{RequestID: "retry_expiration1", Number: task.TaskNo})
	require.NoError(t, e)
	require.NoError(t, s.DB.Take(&m).Error)
	require.Equal(t, "expired", m.Status)
}
func TestRankingFailureDoesNotRollBackGiftAndResumes(t *testing.T) {
	s, _, rs, clock := engagementFixture(t, false)
	_, e := send(s, "rank_failure_01", "1")
	require.NoError(t, e)
	balance(t, s, "990")
	require.NoError(t, s.DB.Exec("CREATE TRIGGER ranking_test_fault BEFORE INSERT ON ranking_score_bucket BEGIN SELECT RAISE(ABORT,'controlled test failure'); END").Error)
	_, e = rs.Consume(ctx, 20)
	require.Error(t, e)
	count(t, s, &rm.AppliedEvent{}, 0)
	count(t, s, &rm.Receipt{}, 0)
	balance(t, s, "990")
	_, e = rs.Board(ctx, 10, "wealth", rq.Query{Period: "all"})
	require.Error(t, e)
	require.NoError(t, s.DB.Exec("DROP TRIGGER ranking_test_fault").Error)
	clock.Add(31000)
	drain(t, rs)
	board, e := rs.Board(ctx, 10, "wealth", rq.Query{Period: "all"})
	require.NoError(t, e)
	require.Equal(t, "10", board.List[0].Score)
	raw, e := json.Marshal(board)
	require.NoError(t, e)
	require.Contains(t, string(raw), `"score":"10"`)
}
