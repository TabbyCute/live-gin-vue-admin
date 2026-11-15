package live

import (
	"context"
	"errors"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"sort"
	"strings"
	"sync"
	"tb_live_module/global"
	model "tb_live_module/model/live"
	req "tb_live_module/model/live/request"
	socialModel "tb_live_module/model/social"
	"testing"
	"time"
)

func presenceMySQL(t *testing.T, s *PresenceService, room model.LiveRoom, session model.LiveSession) {
	t.Helper()
	raw := os.Getenv("PRESENCE_TEST_MYSQL_DSN")
	if raw == "" {
		t.Skip("isolated MySQL required")
	}
	c, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(c.Addr, "127.0.0.1:"))
	require.Equal(t, "presence_test", c.DBName)
	db, e := gorm.Open(mysql.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	name := "presence_test_" + presenceRandom()[:16]
	require.NoError(t, db.Exec("CREATE DATABASE "+name+" CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error)
	pool, _ := db.DB()
	t.Cleanup(func() { _ = db.Exec("DROP DATABASE " + name).Error; pool.Close() })
	c.DBName = name
	target, e := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	p, _ := target.DB()
	p.SetMaxOpenConns(24)
	t.Cleanup(func() { p.Close() })
	models := append(model.PresenceModels(), &model.LiveAccount{}, &model.LiveAnchor{}, &model.LiveRoom{}, &model.LiveSession{})
	models = append(models, socialModel.Models()...)
	require.NoError(t, target.AutoMigrate(models...))
	var accounts []model.LiveAccount
	var anchors []model.LiveAnchor
	require.NoError(t, s.DB.Find(&accounts).Error)
	require.NoError(t, s.DB.Find(&anchors).Error)
	require.NoError(t, target.Create(&accounts).Error)
	require.NoError(t, target.Create(&anchors).Error)
	room.Visibility = model.LiveRoomVisibilityPublic
	require.NoError(t, target.Create(&room).Error)
	require.NoError(t, target.Create(&session).Error)
	oldGlobal := global.GVA_DB
	global.GVA_DB = target
	t.Cleanup(func() { global.GVA_DB = oldGlobal })
	s.DB = target
	s.Social.DB = target
}
func TestPresenceMySQLConcurrentPersistenceAndFinalization(t *testing.T) {
	s, room, session := presenceFixture(t)
	presenceMySQL(t, s, room, session)
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	tokens := []string{}
	errs := []error{}
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.Open(ctx, presenceViewer(), s.IPKey("same-ip"), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: "mysql_idempotent_open"})
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				errs = append(errs, e)
			} else {
				tokens = append(tokens, v.PresenceToken)
			}
		}()
	}
	wg.Wait()
	require.Empty(t, errs)
	require.Len(t, tokens, 20)
	for _, v := range tokens {
		require.Equal(t, tokens[0], v)
	}
	for n := 0; n < 10; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = s.FlushSession(ctx, session.SessionNo) }()
	}
	wg.Wait()
	var count int64
	require.NoError(t, s.DB.Model(&model.LiveViewerVisit{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, s.DB.Model(&session).Updates(map[string]any{"status": model.LiveSessionEnded, "ended_at": time.Now().UnixMilli()}).Error)
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	require.NoError(t, s.DB.First(&session, session.ID).Error)
	require.Positive(t, session.StatsFinalizedAt)
	require.EqualValues(t, 1, session.ViewCount)
}
func TestPresenceLoad1000(t *testing.T) {
	if os.Getenv("PRESENCE_LOAD") != "1" {
		t.Skip("opt-in isolated load run")
	}
	s, room, session := presenceFixture(t)
	presenceMySQL(t, s, room, session)
	ctx := context.Background()
	start := time.Now()
	cp, e := s.ensure(ctx, room.RoomNo, session.SessionNo)
	require.NoError(t, e)
	// Isolate real Redis state-machine throughput from authentication SQL. All identities
	// below are fixture members; end-to-end open is measured separately above.
	lat := make([]time.Duration, 1000)
	tokens := make([]presenceToken, 1000)
	var wg sync.WaitGroup
	slots := make(chan struct{}, 32)
	var failed atomicCounter
	for n := 0; n < 1000; n++ {
		slots <- struct{}{}
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			defer func() { <-slots }()
			tkn := presenceToken{Room: room.RoomNo, Session: session.SessionNo, Epoch: cp.Epoch, Actor: fmt.Sprintf("u:%d", 10000+n), Connection: "pc_" + s.mac("load", fmt.Sprint(n))}
			tokens[n] = tkn
			at := time.Now()
			_, err := s.run(ctx, tkn, map[string]any{"op": "open", "roomLimit": 10000, "ip": "fixture", "history": "", "historyVersion": 0, "historyEpoch": 0})
			lat[n] = time.Since(at)
			if err != nil {
				failed.add()
			}
		}(n)
	}
	wg.Wait()
	require.Equal(t, 0, failed.get())
	reportLatency(t, "hot-room-open-state", lat, time.Since(start))
	start = time.Now()
	for n := 0; n < 1000; n++ {
		slots <- struct{}{}
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			defer func() { <-slots }()
			at := time.Now()
			_, err := s.run(ctx, tokens[n], map[string]any{"op": "heartbeat", "sequence": "1", "playing": true, "visible": true})
			lat[n] = time.Since(at)
			if err != nil {
				failed.add()
			}
		}(n)
	}
	wg.Wait()
	require.Equal(t, 0, failed.get())
	reportLatency(t, "1000-heartbeat-state", lat, time.Since(start))
	// Real account profiles + MySQL-backed cursor pages, rather than empty mock profiles.
	accounts := make([]model.LiveAccount, 1000)
	for n := range accounts {
		accounts[n] = model.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: uint(10000 + n)}, AccountNo: fmt.Sprintf("load%d", n), Username: fmt.Sprintf("load%d", n), Status: model.LiveAccountStatusNormal, TokenVersion: 1}
	}
	require.NoError(t, s.DB.CreateInBatches(&accounts, 100).Error)
	listLatency := make([]time.Duration, 10)
	cursor := ""
	for n := range listLatency {
		at := time.Now()
		page, err := s.Viewers(ctx, presenceViewer(), req.PresenceQuery{RoomNo: room.RoomNo, Limit: 100, Cursor: cursor}, false)
		require.NoError(t, err)
		require.Len(t, page.List, 100)
		cursor = page.NextCursor
		listLatency[n] = time.Since(at)
	}
	require.Empty(t, cursor)
	reportLatency(t, "online-list-100-per-page-serial", listLatency, 0)
	// A burst of 1000 distinct requests must not exceed the account minute budget.
	first := openPresence(t, s, room.RoomNo, "load_like_actor")
	accepted := atomicCounter{}
	start = time.Now()
	for n := 0; n < 1000; n++ {
		slots <- struct{}{}
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			defer func() { <-slots }()
			at := time.Now()
			_, err := s.Like(ctx, presenceViewer(), req.PresenceLike{PresenceToken: first.PresenceToken, RequestID: fmt.Sprintf("load_like_%d", n), Sequence: fmt.Sprint(n + 1), Count: "1"})
			lat[n] = time.Since(at)
			if err == nil {
				accepted.add()
			} else if !errors.Is(err, ErrPresenceLimited) && !errors.Is(err, ErrPresenceSequence) {
				failed.add()
			}
		}(n)
	}
	wg.Wait()
	require.Zero(t, failed.get())
	require.Positive(t, accepted.get())
	require.LessOrEqual(t, accepted.get(), 120)
	reportLatency(t, "1000-like-burst-32-workers", lat, time.Since(start))
	t.Logf("like accepted=%d rejected=%d", accepted.get(), 1000-accepted.get())
	start = time.Now()
	for n := 0; n < 12; n++ {
		require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	}
	t.Logf("mysql checkpoint 1001 visits elapsed=%s", time.Since(start))
	require.Zero(t, s.Redis.ZCard(ctx, presenceKeys(session.SessionNo)[7]).Val())
	tokens = append(tokens, presenceToken{Connection: first.ConnectionNo})
	for _, tkn := range tokens {
		require.NoError(t, s.Redis.ZAdd(ctx, presenceKeys(session.SessionNo)[2], redis.Z{Score: 1, Member: tkn.Connection}).Err())
	}
	start = time.Now()
	for n := 0; n < 11; n++ {
		_, e = s.run(ctx, tokens[0], map[string]any{"op": "expire", "batch": 100})
		require.NoError(t, e)
	}
	t.Logf("expire 1001 connections in batches100 elapsed=%s", time.Since(start))
	require.Zero(t, s.Redis.ZCard(ctx, presenceKeys(session.SessionNo)[2]).Val())
	t.Log(s.Redis.Info(ctx, "memory", "cpu", "stats").Val())
}

type atomicCounter struct {
	mu sync.Mutex
	n  int
}

func (c *atomicCounter) add()     { c.mu.Lock(); c.n++; c.mu.Unlock() }
func (c *atomicCounter) get() int { c.mu.Lock(); defer c.mu.Unlock(); return c.n }
func reportLatency(t *testing.T, label string, lat []time.Duration, elapsed time.Duration) {
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	t.Logf("%s count=%d elapsed=%s p95=%s p99=%s", label, len(lat), elapsed, lat[len(lat)*95/100], lat[len(lat)*99/100])
}

func TestPresenceLoadMultiRoom(t *testing.T) {
	if os.Getenv("PRESENCE_LOAD") != "1" {
		t.Skip("opt-in isolated load run")
	}
	s, _, _ := presenceFixture(t)
	ctx := context.Background()
	tokens := make([]presenceToken, 160)
	for room := 0; room < 16; room++ {
		no := "load_" + presenceRandom()
		epoch := presenceRandom()
		t.Cleanup(func() { _ = s.Redis.Del(ctx, presenceKeys(no)...).Err() })
		_, e := s.run(ctx, presenceToken{Session: no, Epoch: epoch}, map[string]any{"op": "init", "room": no, "view": "0", "viewer": "0", "peak": "0", "likes": "0", "watch": "0", "lost": "0"})
		require.NoError(t, e)
		for n := 0; n < 10; n++ {
			tokens[room*10+n] = presenceToken{Room: no, Session: no, Epoch: epoch, Actor: fmt.Sprintf("u:%d", n+1), Connection: "pc_" + s.mac("multiroom", fmt.Sprintf("%s/%d", no, n))}
		}
	}
	lat := make([]time.Duration, len(tokens))
	slots := make(chan struct{}, 32)
	var wg sync.WaitGroup
	failed := atomicCounter{}
	start := time.Now()
	for n, tkn := range tokens {
		slots <- struct{}{}
		wg.Add(1)
		go func(n int, tkn presenceToken) {
			defer wg.Done()
			defer func() { <-slots }()
			at := time.Now()
			_, e := s.run(ctx, tkn, map[string]any{"op": "open", "roomLimit": 10000, "ip": "fixture", "history": "", "historyVersion": 0, "historyEpoch": 0})
			lat[n] = time.Since(at)
			if e != nil {
				failed.add()
			}
		}(n, tkn)
	}
	wg.Wait()
	require.Zero(t, failed.get())
	reportLatency(t, "16-room-160-open-state-32-workers", lat, time.Since(start))
	for room := 0; room < 16; room++ {
		v, e := s.run(ctx, tokens[room*10], map[string]any{"op": "stats"})
		require.NoError(t, e)
		r, e := s.sessionResult(tokens[room*10], v)
		require.NoError(t, e)
		require.Equal(t, "10", r.MemberCount)
	}
}
