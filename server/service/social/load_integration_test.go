package social

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
)

// TestMySQLLoadProfile 是可复现的小容量测量，不代表生产容量承诺。
// 与其他MySQL测试顺序执行，仅允许social_test_*库；会清空该库社交测试表。
func TestMySQLLoadProfile(t *testing.T) {
	dsn := os.Getenv("SOCIAL_TEST_MYSQL_DSN")
	if dsn == "" || os.Getenv("SOCIAL_RUN_LOAD") != "1" {
		t.Skip("opt-in: SOCIAL_RUN_LOAD=1 and disposable MySQL DSN")
	}
	cfg, err := mysqlDriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "social_test_"))
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, _ := db.DB()
	defer pool.Close()
	pool.SetMaxOpenConns(32)
	pool.SetMaxIdleConns(32)
	require.NoError(t, db.Migrator().DropTable(append(model.Models(), &testUser{})...))
	s := fixture(t, db)
	ctx := context.Background()
	users := make([]testUser, 1000)
	for i := range users {
		users[i] = testUser{ID: uint64(1000 + i), AccountNo: fmt.Sprint(1000 + i), Normal: true}
	}
	require.NoError(t, db.CreateInBatches(users, 100).Error)
	reports := map[string]interface{}{}
	measure := func(name string, n, concurrency int, fn func(int) error) {
		durations := make([]time.Duration, n)
		errs := make([]error, n)
		jobs := make(chan int)
		var wg sync.WaitGroup
		started := time.Now()
		for worker := 0; worker < concurrency; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					at := time.Now()
					errs[i] = fn(i)
					durations[i] = time.Since(at)
				}
			}()
		}
		for i := 0; i < n; i++ {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		elapsed := time.Since(started)
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		failures := 0
		for _, e := range errs {
			if e != nil {
				failures++
			}
		}
		report := map[string]interface{}{"requests": n, "concurrency": concurrency, "elapsedSeconds": elapsed.Seconds(), "qps": float64(n) / elapsed.Seconds(), "p95Ms": float64(durations[(n-1)*95/100].Microseconds()) / 1000, "p99Ms": float64(durations[(n-1)*99/100].Microseconds()) / 1000, "errors": failures, "errorRate": float64(failures) / float64(n)}
		reports[name] = report
		raw, _ := json.Marshal(report)
		t.Log(name + ": " + string(raw))
		require.Zero(t, failures)
	}
	measure("hot_anchor_follow", 1000, 32, func(i int) error {
		_, err := s.ChangeRelationship(ctx, uint64(1000+i), target(2), Follow, "")
		return err
	})
	assertCounters(t, s, 2, 0, 1000)
	targets := make([]req.Target, 100)
	for i := range targets {
		targets[i] = target(uint64(1000 + i))
	}
	measure("batch_status_100_no_redis", 400, 16, func(int) error { _, err := s.BatchStatus(ctx, 1, targets); return err })
	measure("fans_cursor_page_100", 300, 16, func(int) error {
		_, err := s.ListUsers(ctx, 2, "followers", req.CursorSearch{Limit: 100}, 0)
		return err
	})
	r := req.Resource{ResourceType: model.ResourceShortVideo, ResourceID: "visible"}
	sessions := make([]req.HistoryReport, 100)
	for i := range sessions {
		session, err := s.OpenHistory(ctx, uint64(1000+i), req.HistoryOpen{Resource: r, RequestID: "load-00001"})
		require.NoError(t, err)
		sessions[i] = req.HistoryReport{Resource: r, SessionToken: session.SessionToken, SessionVersion: session.SessionVersion, Epoch: session.Epoch, Sequence: 1, PositionMs: 1000, Checkpoint: true}
	}
	measure("history_strict_checkpoint", 100, 16, func(i int) error { _, err := s.ReportHistory(ctx, uint64(1000+i), sessions[i]); return err })
	// 对同一热门计数行串行竞争的取消和重新关注。
	measure("hot_anchor_unfollow_refollow", 1000, 32, func(i int) error {
		if _, err := s.ChangeRelationship(ctx, uint64(1000+i), target(2), Unfollow, ""); err != nil {
			return err
		}
		_, err := s.ChangeRelationship(ctx, uint64(1000+i), target(2), Follow, "")
		return err
	})
	assertCounters(t, s, 2, 0, 1000)
	for i := 0; i < 100; i++ {
		changeForTest(t, s, uint64(1000+i), 2, Subscribe)
	}
	require.NoError(t, db.Model(&model.SocialOutboxEvent{}).Where("event_type = ?", model.EventRelationship).Update("status", model.EventDone).Error)
	sink := &testSink{}
	s.Sink = sink
	require.NoError(t, s.repository().Transaction(ctx, func(tx *gorm.DB) error { return s.RecordLiveStarted(tx, 2, "load-session") }))
	measure("notification_fanout_page_20_fake_sink", 5, 1, func(int) error { return s.TickOutbox(ctx) })
	require.Equal(t, int32(100), sink.Calls.Load())
	stats := pool.Stats()
	reports["pool"] = map[string]interface{}{"maxOpen": stats.MaxOpenConnections, "open": stats.OpenConnections, "waitCount": stats.WaitCount, "waitDurationMs": stats.WaitDuration.Milliseconds()}
	var plans []map[string]interface{}
	queries := []string{
		"EXPLAIN SELECT id,user_id FROM social_follow WHERE target_user_id=2 AND id<100000 ORDER BY id DESC LIMIT 101",
		"EXPLAIN SELECT id,target_user_id FROM social_follow WHERE user_id=1000 AND id<100000 ORDER BY id DESC LIMIT 101",
		"EXPLAIN SELECT * FROM social_live_subscription WHERE target_user_id=2 AND enabled=1 AND id>0 ORDER BY id LIMIT 20",
		"EXPLAIN SELECT * FROM social_outbox_event WHERE status='pending' AND next_attempt_at<=9999999999999 ORDER BY next_attempt_at,id LIMIT 1",
		"EXPLAIN SELECT * FROM social_watch_history WHERE user_id=1000 ORDER BY last_watched_at DESC,id DESC LIMIT 101",
	}
	for _, q := range queries {
		var rows []map[string]interface{}
		require.NoError(t, db.Raw(q).Scan(&rows).Error)
		plans = append(plans, map[string]interface{}{"query": q, "plan": rows})
	}
	reports["explain"] = plans
	if path := os.Getenv("SOCIAL_LOAD_REPORT"); path != "" {
		raw, err := json.MarshalIndent(reports, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, raw, 0600))
	}
}
