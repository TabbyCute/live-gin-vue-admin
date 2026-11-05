package gift_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"tb_live_module/initialize"
	gm "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	guardModel "tb_live_module/model/guard"
	guardReq "tb_live_module/model/guard/request"
	guardRes "tb_live_module/model/guard/response"
	rankModel "tb_live_module/model/ranking"
	rankReq "tb_live_module/model/ranking/request"
	svc "tb_live_module/service/gift"
	guard "tb_live_module/service/guard"
	ranking "tb_live_module/service/ranking"
	"testing"
	"time"
)

type ambiguousGuard struct {
	*guard.Service
	fail atomic.Bool
}

func (p *ambiguousGuard) Fulfill(c context.Context, in guardModel.GrantInput) error {
	e := p.Service.Fulfill(c, in)
	if e == nil && p.fail.CompareAndSwap(true, false) {
		return errors.New("simulated lost response after commit")
	}
	return e
}
func engagementFixture(t *testing.T, mysql bool) (*svc.Service, *guard.Service, *ranking.Service, *atomic.Int64) {
	s := fixture(t, mysql)
	require.NoError(t, initialize.MigrateGuard(s.DB))
	require.NoError(t, initialize.MigrateRanking(s.DB, "Asia/Kuala_Lumpur"))
	require.NoError(t, initialize.MigrateGuard(s.DB))
	require.NoError(t, initialize.MigrateRanking(s.DB, "Asia/Kuala_Lumpur"))
	require.NoError(t, initialize.SyncEngagementMetadata(s.DB))
	clock := &atomic.Int64{}
	clock.Store(time.Now().UnixMilli())
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	s.Now = now
	gs := &guard.Service{DB: s.DB, Now: now}
	rs := &ranking.Service{DB: s.DB, Now: now, Timezone: "Asia/Kuala_Lumpur"}
	s.Guard = gs
	s.GuardEnabled = true
	_, e := gs.SavePlan(ctx, guard.Actor{ID: 1}, guardReq.Plan{RequestID: "create_plan_01", PlanNo: "plan01", Name: "月守护", Level: 1, Priority: 10, DefaultDurationDays: "30", BenefitConfig: `{"badge":"守护"}`, Status: "enabled"})
	require.NoError(t, e)
	r := item()
	r.Version = "1"
	r.IsPremium = true
	r.BenefitType = "anchor_guard"
	r.BenefitStatus = "enabled"
	r.GuardPlanNo = "plan01"
	r.GuardDurationDays = "1"
	r.EffectiveFrom = 1
	r.EffectiveTo = clock.Load() + 86400000
	_, e = s.SaveItem(ctx, admin, r)
	require.NoError(t, e)
	return s, gs, rs, clock
}
func drain(t *testing.T, rs *ranking.Service) {
	t.Helper()
	for i := 0; i < 100; i++ {
		n, e := rs.Consume(ctx, 20)
		require.NoError(t, e)
		if n == 0 {
			return
		}
	}
	t.Fatal("ranking failed to drain")
}
func fullGuardFlow(t *testing.T, mysql bool) {
	s, gs, rs, clock := engagementFixture(t, mysql)
	initial := clock.Load()
	// fixture created and independently approved real wallet recharge: 1000 COIN.
	balance(t, s, "1000")
	for _, e := range parallel(16, func(i int) error { _, e := send(s, "guard_flow_order", "3"); return e }) {
		require.NoError(t, e)
	}
	balance(t, s, "970")
	count(t, s, &gm.Order{}, 1)
	count(t, s, &gm.BenefitGrant{}, 1)
	count(t, s, &guardModel.Grant{}, 0) // delivery never happens under wallet locks.
	provider := &ambiguousGuard{Service: gs}
	provider.fail.Store(true)
	s.Guard = provider
	_, e := s.ProcessBenefits(ctx, 20)
	require.Error(t, e)
	count(t, s, &guardModel.Grant{}, 1)
	clock.Add(61000)
	_, e = s.ProcessBenefits(ctx, 20)
	require.NoError(t, e)
	count(t, s, &guardModel.Grant{}, 1)
	var original gm.Order
	require.NoError(t, s.DB.Take(&original).Error)
	_, e = send(s, "guard_flow_renew", "1")
	require.NoError(t, e)
	_, e = s.ProcessBenefits(ctx, 20)
	require.NoError(t, e)
	balance(t, s, "960")
	clock.Store(initial + guard.DayMillis/2)
	refund, e := s.Refund(ctx, admin, req.Refund{OrderNo: original.OrderNo, RequestID: "guard_refund_two", Quantity: "2", Reason: "流程测试部分退款"})
	require.NoError(t, e)
	balance(t, s, "980")
	for _, e := range parallel(12, func(i int) error { _, e := s.ProcessBenefits(ctx, 20); return e }) {
		require.NoError(t, e)
	}
	count(t, s, &guardModel.Compensation{}, 1)
	var comp gm.BenefitCompensation
	require.NoError(t, s.DB.Where("refund_no=?", refund.RefundNo).Take(&comp).Error)
	require.Equal(t, "succeeded", comp.Status)
	page, e := gs.Members(ctx, 10, "status", nil, guardReq.Query{AnchorNo: "anchor01"})
	require.NoError(t, e)
	members := page.List.([]guardRes.Membership)
	require.Len(t, members, 1)
	require.Equal(t, "active", members[0].Status)
	require.Equal(t, "2", members[0].RemainingDays)
	require.Equal(t, initial+2*guard.DayMillis, members[0].ExpiresAt)
	other, e := gs.Members(ctx, 11, "mine", nil, guardReq.Query{})
	require.NoError(t, e)
	require.Equal(t, "0", other.Total)
	drain(t, rs)
	board, e := rs.Board(ctx, 10, "wealth", rankReq.Query{Period: "all"})
	require.NoError(t, e)
	require.Equal(t, "20", board.List[0].Score)
	require.Equal(t, "2", board.List[0].Quantity)
	require.Equal(t, "1", board.MyRank.Rank)
	require.Equal(t, "0", board.LagCount)
	var checkpoint rankModel.Checkpoint
	require.NoError(t, s.DB.Take(&checkpoint).Error)
	require.Equal(t, clock.Load(), checkpoint.UpdatedAt)
	var membership guardModel.Membership
	require.NoError(t, s.DB.Take(&membership).Error)
	require.Equal(t, clock.Load(), membership.UpdatedAt)
	charm, e := rs.Board(ctx, 11, "charm", rankReq.Query{Period: "all"})
	require.NoError(t, e)
	require.Equal(t, "20", charm.List[0].Score)
	key, e := ranking.PeriodKey("day", initial, "Asia/Kuala_Lumpur")
	require.NoError(t, e)
	originalDay, e := rs.Board(ctx, 10, "wealth", rankReq.Query{Period: "day", PeriodKey: key})
	require.NoError(t, e)
	require.Equal(t, "20", originalDay.List[0].Score)
	result, e := rs.AdminAction(ctx, ranking.Actor{ID: 1}, "reconcile", rankReq.Action{RequestID: "rank_reconcile_1", Scope: "all"})
	require.NoError(t, e)
	require.Contains(t, result, `"consistent":true`)
	_, e = rs.AdminAction(ctx, ranking.Actor{ID: 1}, "rebuild", rankReq.Action{RequestID: "rank_rebuild_01", Scope: "all"})
	require.NoError(t, e)
	drain(t, rs)
	result, e = rs.AdminAction(ctx, ranking.Actor{ID: 1}, "reconcile", rankReq.Action{RequestID: "rank_reconcile_2", Scope: "all"})
	require.NoError(t, e)
	require.Contains(t, result, `"consistent":true`)
	clock.Store(initial + 4*guard.DayMillis)
	for _, e := range parallel(8, func(i int) error { _, e := gs.ExpireBatch(ctx, 20); return e }) {
		require.NoError(t, e)
	}
	page, e = gs.Members(ctx, 10, "status", nil, guardReq.Query{AnchorNo: "anchor01"})
	require.NoError(t, e)
	require.Equal(t, "expired", page.List.([]guardRes.Membership)[0].Status)
	var expirations int64
	require.NoError(t, s.DB.Model(&guardModel.Ledger{}).Where("entry_type='expire'").Count(&expirations).Error)
	require.Equal(t, int64(1), expirations)
	detail, e := s.Detail(ctx, admin, req.Query{No: original.OrderNo})
	require.NoError(t, e)
	require.True(t, detail.Consistent, detail.Issues)
	var receipts int64
	require.NoError(t, s.DB.Model(&rankModel.AppliedEvent{}).Count(&receipts).Error)
	require.Equal(t, int64(3), receipts)
	require.Error(t, s.DB.Model(&guardModel.Grant{}).Where("1=1").Update("duration_days", 999).Error)
	require.Error(t, s.DB.Where("1=1").Delete(&guardModel.Ledger{}).Error)
	t.Log("recharge 1000 -> gift debit 40 -> refund 20 -> wallet 980; guard grant/renew/compensation/expiry; wealth/charm net20; rebuild/reconcile consistent")
}
func TestGuardRechargeThroughExpiry(t *testing.T)      { fullGuardFlow(t, false) }
func TestMySQLGuardRechargeThroughExpiry(t *testing.T) { fullGuardFlow(t, true) }
func TestMySQLGuardRepeatedPartialRefundsAndScopes(t *testing.T) {
	s, gs, rs, clock := engagementFixture(t, true)
	o, e := send(s, "partial_flow_01", "3")
	require.NoError(t, e)
	_, e = s.ProcessBenefits(ctx, 20)
	require.NoError(t, e)
	clock.Add(guard.DayMillis / 2)
	for i := 0; i < 3; i++ {
		in := req.Refund{OrderNo: o.OrderNo, RequestID: fmt.Sprintf("refund_each_%02d", i), Quantity: "1", Reason: "多次部分退款至全退"}
		_, e = s.Refund(ctx, admin, in)
		require.NoError(t, e)
		_, e = s.Refund(ctx, admin, in)
		require.NoError(t, e)
		for _, e := range parallel(4, func(int) error { _, e := s.ProcessBenefits(ctx, 20); return e }) {
			require.NoError(t, e)
		}
		drain(t, rs)
		for _, period := range []string{"day", "week", "month", "all"} {
			periodKey, e := ranking.PeriodKey(period, o.CreatedAt, "Asia/Kuala_Lumpur")
			require.NoError(t, e)
			for _, scope := range []struct{ kind, no string }{{"anchor", o.AnchorNo}, {"room", o.RoomNo}, {"session", o.SessionNo}} {
				for _, giftNo := range []string{"", o.GiftNo} {
					board, e := rs.Board(ctx, 10, "gift", rankReq.Query{Period: period, PeriodKey: periodKey, ScopeType: scope.kind, ScopeNo: scope.no, GiftNo: giftNo})
					require.NoError(t, e)
					if i == 2 {
						require.Empty(t, board.List)
					} else {
						require.Len(t, board.List, 1)
						require.Equal(t, fmt.Sprint((2-i)*10), board.List[0].Score)
					}
				}
			}
			board, e := rs.Board(ctx, 10, "charm", rankReq.Query{Period: period, ScopeType: "category", ScopeNo: "different_category"})
			require.NoError(t, e)
			require.Empty(t, board.List)
		}
	}
	balance(t, s, "1000")
	count(t, s, &guardModel.Compensation{}, 3)
	var grant guardModel.Grant
	require.NoError(t, s.DB.Take(&grant).Error)
	require.Equal(t, 3*guard.DayMillis-guard.DayMillis/2, grant.CompensatedDuration)
	require.NoError(t, gs.VerifyAll(ctx))
	proof, e := rs.ReconcileSnapshot(ctx)
	require.NoError(t, e)
	require.Equal(t, true, proof.(map[string]any)["consistent"])
}
func TestGuardRefundBeforeDelivery(t *testing.T) {
	s, gs, _, clock := engagementFixture(t, false)
	o, e := send(s, "before_delivery1", "1")
	require.NoError(t, e)
	_, e = s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, RequestID: "before_refund_01", Quantity: "1", Reason: "未履约退款"})
	require.NoError(t, e)
	var g gm.BenefitGrant
	var c gm.BenefitCompensation
	require.NoError(t, s.DB.Take(&g).Error)
	require.NoError(t, s.DB.Take(&c).Error)
	var snap guardModel.BenefitSnapshot
	require.NoError(t, json.Unmarshal([]byte(g.BenefitSnapshot), &snap))
	command := guardModel.GrantInput{GrantNo: g.GrantNo, GiftOrderNo: g.GiftOrderNo, UserID: 10, AnchorID: 1, PlanNo: "plan01", DurationDays: 1, BenefitSnapshot: g.BenefitSnapshot}
	require.NoError(t, gs.Revoke(ctx, guardModel.CompensationInput{Grant: command, CompensationNo: c.CompensationNo, RefundNo: c.RefundNo, DurationDays: 1}))
	clock.Add(1000)
	require.NoError(t, gs.Fulfill(ctx, command))
	require.NoError(t, gs.Fulfill(ctx, command))
	_, e = s.ProcessBenefits(ctx, 20)
	require.NoError(t, e)
	balance(t, s, "1000")
	var m guardModel.Membership
	require.NoError(t, s.DB.Take(&m).Error)
	require.Equal(t, "expired", m.Status)
	changed := command
	changed.DurationDays = 2
	require.Error(t, gs.Fulfill(ctx, changed))
	_, e = gs.AdminAction(ctx, guard.Actor{ID: 1}, "reconcile", guardReq.Action{RequestID: "guard_reconcile_1", Number: m.MembershipNo})
	require.NoError(t, e)
	require.NoError(t, s.DB.Model(&m).Update("projection", `[]`).Error)
	_, e = gs.Members(ctx, 10, "mine", nil, guardReq.Query{})
	require.Error(t, e)
	_, e = gs.AdminAction(ctx, guard.Actor{ID: 1}, "rebuild", guardReq.Action{RequestID: "guard_rebuild_001", Number: m.MembershipNo})
	require.NoError(t, e)
	_, e = gs.Members(ctx, 10, "mine", nil, guardReq.Query{})
	require.NoError(t, e)
}
func TestGuardConcurrentGrantsAndCompensations(t *testing.T) {
	s, gs, _, _ := engagementFixture(t, false)
	for _, e := range parallel(12, func(i int) error { _, e := send(s, fmt.Sprintf("guard_parallel_%02d", i), "1"); return e }) {
		require.NoError(t, e)
	}
	for _, e := range parallel(8, func(i int) error { _, e := s.ProcessBenefits(ctx, 100); return e }) {
		require.NoError(t, e)
	}
	count(t, s, &guardModel.Grant{}, 12)
	var m guardModel.Membership
	require.NoError(t, s.DB.Take(&m).Error)
	require.Equal(t, int64(12*guard.DayMillis), m.ExpiresAt-m.StartedAt)
	_, e := gs.AdminAction(ctx, guard.Actor{ID: 999}, "rebuild", guardReq.Action{RequestID: "forbidden_001", Number: m.MembershipNo})
	require.Error(t, e)
}
