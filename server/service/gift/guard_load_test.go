package gift_test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"sort"
	gm "tb_live_module/model/guard"
	rq "tb_live_module/model/ranking/request"
	"testing"
	"time"
)

func TestMySQLGuardRankingLoad(t *testing.T) {
	if os.Getenv("GUARD_RUN_LOAD") != "1" {
		t.Skip("GUARD_RUN_LOAD=1 required")
	}
	s, gs, rs, _ := engagementFixture(t, true)
	latency := make([]float64, 100)
	start := time.Now()
	for _, e := range parallel(100, func(i int) error {
		at := time.Now()
		_, e := send(s, fmt.Sprintf("guard_load_%04d", i), "1")
		latency[i] = float64(time.Since(at).Microseconds()) / 1000
		return e
	}) {
		require.NoError(t, e)
	}
	sendElapsed := time.Since(start)
	balance(t, s, "0")
	start = time.Now()
	for _, e := range parallel(4, func(i int) error { _, e := s.ProcessBenefits(ctx, 100); return e }) {
		require.NoError(t, e)
	}
	_, e := s.ProcessBenefits(ctx, 100)
	require.NoError(t, e)
	count(t, s, &gm.Grant{}, 100)
	require.NoError(t, gs.VerifyAll(ctx))
	fulfillmentElapsed := time.Since(start)
	start = time.Now()
	for _, e := range parallel(4, func(i int) error { _, e := rs.Consume(ctx, 100); return e }) {
		require.NoError(t, e)
	}
	drain(t, rs)
	projectionElapsed := time.Since(start)
	board, e := rs.Board(ctx, 10, "wealth", rq.Query{Period: "all"})
	require.NoError(t, e)
	require.Equal(t, "1000", board.List[0].Score)
	require.Equal(t, "100", board.List[0].Quantity)
	var explain []map[string]any
	require.NoError(t, s.DB.Raw("EXPLAIN SELECT subject_no,score_coin FROM ranking_score_bucket WHERE generation=1 AND rank_type='wealth' AND period_type='all' AND period_key='all' AND scope_type='platform' AND scope_no='' AND gift_no='' ORDER BY score_sort DESC,subject_no ASC LIMIT 20").Scan(&explain).Error)
	sort.Float64s(latency)
	t.Logf("guard load real MySQL: orders=100 concurrency=100 send_ms=%d p50_ms=%.3f p95_ms=%.3f p99_ms=%.3f fulfill_4_workers_ms=%d ranking_4_workers_ms=%d", sendElapsed.Milliseconds(), latency[49], latency[94], latency[98], fulfillmentElapsed.Milliseconds(), projectionElapsed.Milliseconds())
	t.Logf("ranking EXPLAIN: %v", explain)
}
