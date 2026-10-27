package social

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/config"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	repo "tb_live_module/repository/social"
)

type testUser struct {
	ID        uint64 `gorm:"primaryKey"`
	AccountNo string
	Normal    bool
	Anchor    bool
}

func (testUser) TableName() string { return "social_test_user" }

type testDirectory struct{}

func (testDirectory) Resolve(ctx context.Context, db *gorm.DB, ts []req.Target) ([]uint64, error) {
	var rows []testUser
	nos := []string{}
	for _, t := range ts {
		nos = append(nos, t.AccountNo+t.AnchorNo)
	}
	if err := db.WithContext(ctx).Where("account_no IN ?", nos).Find(&rows).Error; err != nil {
		return nil, err
	}
	m := map[string]uint64{}
	for _, r := range rows {
		m[r.AccountNo] = r.ID
	}
	out := []uint64{}
	for _, n := range nos {
		if m[n] == 0 {
			return nil, ErrUnavailableUser
		}
		out = append(out, m[n])
	}
	return out, nil
}
func (testDirectory) Validate(ctx context.Context, db *gorm.DB, ids []uint64) error {
	var n int64
	if err := db.WithContext(ctx).Model(&testUser{}).Where("id IN ? AND normal = ?", uniqueIDs(ids), true).Count(&n).Error; err != nil {
		return err
	}
	if n != int64(len(uniqueIDs(ids))) {
		return ErrUnavailableUser
	}
	return nil
}
func (testDirectory) Profiles(ctx context.Context, db *gorm.DB, viewer uint64, ids []uint64) (map[uint64]res.User, error) {
	var rows []testUser
	if len(ids) > 0 {
		if err := db.WithContext(ctx).Where("id IN ? AND normal = ?", ids, true).Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	out := map[uint64]res.User{}
	for _, r := range rows {
		out[r.ID] = res.User{AccountNo: r.AccountNo, Nickname: r.AccountNo}
	}
	return out, nil
}
func (testDirectory) IsAnchor(ctx context.Context, db *gorm.DB, id uint64) (bool, error) {
	var u testUser
	err := db.WithContext(ctx).First(&u, id).Error
	return u.Anchor, err
}
func (testDirectory) Online(context.Context, *gorm.DB, uint64, []uint64) ([]res.OnlineRoom, error) {
	return []res.OnlineRoom{}, nil
}
func (testDirectory) LiveEventValid(context.Context, *gorm.DB, uint64, string) (bool, error) {
	return true, nil
}

type testResource struct{}

func (testResource) Resolve(_ context.Context, _ *gorm.DB, _ uint64, rs []req.Resource) (map[req.Resource]res.Resource, error) {
	out := map[req.Resource]res.Resource{}
	for _, r := range rs {
		if r.ResourceID == "visible" {
			out[r] = res.Resource{OwnerUserID: 2, ResourceType: r.ResourceType, ResourceID: r.ResourceID, Title: "Test", DurationMs: 100000}
		}
	}
	return out, nil
}
func testService(t testing.TB) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return fixture(t, db)
}
func fixture(t testing.TB, db *gorm.DB) *Service {
	t.Helper()
	require.NoError(t, db.AutoMigrate(append(model.Models(), &testUser{})...))
	for i := 1; i <= 4; i++ {
		require.NoError(t, db.Create(&testUser{ID: uint64(i), AccountNo: fmt.Sprint(i), Normal: true, Anchor: i == 2}).Error)
	}
	return &Service{DB: db, Directory: testDirectory{}, Resources: map[model.ResourceType]ResourceProvider{model.ResourceLiveRoom: testResource{}, model.ResourceShortVideo: testResource{}}, Config: config.Social{}}
}
func target(id uint64) req.Target { return req.Target{AccountNo: fmt.Sprint(id)} }

func TestHealthUsesBoundedEventSamples(t *testing.T) {
	s := testService(t)
	rows := make([]model.SocialOutboxEvent, 1001)
	for i := range rows {
		rows[i] = model.SocialOutboxEvent{EventID: fmt.Sprintf("health-%d", i), BusinessKey: fmt.Sprintf("health-%d", i), EventType: model.EventRelationship, Status: model.EventPending}
	}
	require.NoError(t, s.DB.CreateInBatches(rows, 50).Error)
	health, err := s.Health(context.Background())
	require.NoError(t, err)
	events := health["events"].([]map[string]interface{})
	require.Len(t, events, 4)
	require.Equal(t, 1000, events[0]["count"])
	require.Equal(t, true, events[0]["countIsLowerBound"])
	require.NotZero(t, events[0]["sampleOldestCreatedAt"])
	require.Equal(t, 0, events[1]["count"])
	require.Equal(t, false, events[1]["countIsLowerBound"])
}
func changeForTest(t testing.TB, s *Service, a, b uint64, action Action) res.Change {
	t.Helper()
	r, e := s.ChangeRelationship(context.Background(), a, target(b), action, "")
	require.NoError(t, e)
	return r
}
func assertCounters(t testing.TB, s *Service, id uint64, following, fans uint64) {
	t.Helper()
	audit, err := s.AuditCounters(context.Background(), target(id), false)
	require.NoError(t, err)
	require.False(t, audit.Mismatch)
	require.Equal(t, following, audit.FollowingCount)
	require.Equal(t, fans, audit.FansCount)
}
func TestRelationshipStateMachine(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	require.True(t, changeForTest(t, s, 1, 2, Follow).Changed)
	require.False(t, changeForTest(t, s, 1, 2, Follow).Changed)
	require.True(t, changeForTest(t, s, 2, 1, Follow).Changed)
	changeForTest(t, s, 1, 2, Subscribe)
	status, e := s.BatchStatus(ctx, 1, []req.Target{target(2), target(2)})
	require.NoError(t, e)
	require.Len(t, status, 1)
	require.True(t, status[0].Mutual)
	require.True(t, status[0].Subscribed)
	require.True(t, changeForTest(t, s, 2, 1, Block).Changed)
	assertCounters(t, s, 1, 0, 0)
	assertCounters(t, s, 2, 0, 0)
	_, e = s.ChangeRelationship(ctx, 1, target(2), Follow, "")
	require.ErrorIs(t, e, ErrBlocked)
	require.False(t, changeForTest(t, s, 2, 1, Block).Changed)
	changeForTest(t, s, 1, 2, Block)
	changeForTest(t, s, 2, 1, Unblock)
	_, e = s.ChangeRelationship(ctx, 2, target(1), Follow, "")
	require.ErrorIs(t, e, ErrBlocked)
	changeForTest(t, s, 1, 2, Unblock)
	require.False(t, changeForTest(t, s, 1, 2, Unfollow).Changed)
	changeForTest(t, s, 1, 2, Follow)
	status, e = s.BatchStatus(ctx, 1, []req.Target{target(2)})
	require.NoError(t, e)
	require.False(t, status[0].Subscribed)
}
func TestValidationMuteAndAutoSubscription(t *testing.T) {
	s := testService(t)
	s.Config.AutoSubscribe = true
	ctx := context.Background()
	_, e := s.ChangeRelationship(ctx, 1, target(1), Follow, "")
	require.ErrorIs(t, e, ErrSelf)
	_, e = s.ChangeRelationship(ctx, 1, target(99), Follow, "")
	require.ErrorIs(t, e, ErrUnavailableUser)
	require.NoError(t, s.DB.Model(&testUser{}).Where("id=4").Update("normal", false).Error)
	_, e = s.ChangeRelationship(ctx, 1, target(4), Follow, "")
	require.ErrorIs(t, e, ErrUnavailableUser)
	changeForTest(t, s, 1, 2, Follow)
	changeForTest(t, s, 1, 2, Unsubscribe)
	changeForTest(t, s, 1, 2, Unfollow)
	changeForTest(t, s, 1, 2, Follow)
	_, e = s.ChangeRelationship(ctx, 1, target(2), Mute, model.MuteContent)
	require.NoError(t, e)
	status, e := s.BatchStatus(ctx, 1, []req.Target{target(2)})
	require.NoError(t, e)
	require.True(t, status[0].Following)
	require.False(t, status[0].Subscribed)
	require.Equal(t, []model.MuteScope{model.MuteContent}, status[0].MutedScopes)
	_, e = s.ChangeRelationship(ctx, 1, target(2), Mute, "unbounded")
	require.ErrorIs(t, e, ErrInvalid)
	_, e = s.BatchStatus(ctx, 1, make([]req.Target, 101))
	require.ErrorIs(t, e, ErrInvalid)
}
func TestRollbackCountInvariantAndRepair(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	require.NoError(t, s.DB.Callback().Create().After("gorm:create").Register("test:outbox_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "social_outbox_event" {
			tx.AddError(errors.New("injected disk full"))
		}
	}))
	_, e := s.ChangeRelationship(ctx, 1, target(2), Follow, "")
	require.Error(t, e)
	var n int64
	require.NoError(t, s.DB.Model(&model.SocialFollow{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.DB.Callback().Create().Remove("test:outbox_failure"))
	changeForTest(t, s, 1, 2, Follow)
	require.NoError(t, s.DB.Model(&model.SocialUserCounter{}).Where("user_id=2").Update("fans_count", 0).Error)
	_, e = s.ChangeRelationship(ctx, 1, target(2), Unfollow, "")
	require.ErrorIs(t, e, repo.ErrCounterInvariant)
	following, e := repo.Following(s.DB, 1, 2)
	require.NoError(t, e)
	require.True(t, following)
	audit, e := s.AuditCounters(ctx, target(2), true)
	require.NoError(t, e)
	require.True(t, audit.Mismatch)
	assertCounters(t, s, 2, 0, 1)
	changeForTest(t, s, 1, 2, Unfollow)
	assertCounters(t, s, 1, 0, 0)
	assertCounters(t, s, 2, 0, 0)
}
func TestTransactionRetryClassificationAndCancellation(t *testing.T) {
	s := testService(t)
	for _, number := range []uint16{1213, 1205, 1062} {
		calls := 0
		err := s.repository().Transaction(context.Background(), func(*gorm.DB) error {
			calls++
			if calls == 1 {
				return &mysqlDriver.MySQLError{Number: number}
			}
			return nil
		})
		if number == 1062 {
			require.Error(t, err)
			require.Equal(t, 1, calls)
		} else {
			require.NoError(t, err)
			require.Equal(t, 2, calls)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, s.repository().Transaction(ctx, func(*gorm.DB) error { return nil }))
}
func TestCursorStableHiddenAndVersionKeys(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	key1, e := s.relationshipCacheKey(ctx, 1, []uint64{2})
	require.NoError(t, e)
	changeForTest(t, s, 1, 2, Follow)
	key2, e := s.relationshipCacheKey(ctx, 1, []uint64{2})
	require.NoError(t, e)
	require.NotEqual(t, key1, key2)
	changeForTest(t, s, 1, 3, Follow)
	changeForTest(t, s, 1, 4, Follow)
	require.NoError(t, s.DB.Model(&testUser{}).Where("id=4").Update("normal", false).Error)
	first, e := s.ListUsers(ctx, 1, "following", req.CursorSearch{Limit: 1}, 0)
	require.NoError(t, e)
	require.Empty(t, first.List)
	require.True(t, first.HasMore)
	require.NotEmpty(t, first.NextCursor)
	second, e := s.ListUsers(ctx, 1, "following", req.CursorSearch{Limit: 1, Cursor: first.NextCursor}, 0)
	require.NoError(t, e)
	require.Equal(t, "3", second.List[0].AccountNo)
	changeForTest(t, s, 1, 3, Unfollow)
	third, e := s.ListUsers(ctx, 1, "following", req.CursorSearch{Limit: 1, Cursor: second.NextCursor}, 0)
	require.NoError(t, e)
	require.Equal(t, "2", third.List[0].AccountNo)
	require.False(t, third.HasMore)
	_, e = decodeCursor("illegal!")
	require.Error(t, e)
	_, e = decodeCursor(encodeCursor(0, ^uint64(0)))
	require.Error(t, e)
}
func TestFavoriteAndResourceFiltering(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: "visible"}
	a, e := s.Favorite(ctx, 1, r, true)
	require.NoError(t, e)
	require.True(t, a.Changed)
	a, e = s.Favorite(ctx, 1, r, true)
	require.NoError(t, e)
	require.False(t, a.Changed)
	_, e = s.Favorite(ctx, 1, req.Resource{ResourceType: model.ResourceLiveRoom, ResourceID: "private"}, true)
	require.ErrorIs(t, e, ErrResource)
	_, e = s.Favorite(ctx, 1, req.Resource{ResourceType: model.ResourceLiveReplay, ResourceID: "visible"}, true)
	require.ErrorIs(t, e, ErrUnsupported)
	status, e := s.FavoriteStatus(ctx, 1, []req.Resource{r, r})
	require.NoError(t, e)
	require.Len(t, status, 1)
	require.True(t, status[0].Favorite)
	list, e := s.FavoriteStatus(ctx, 2, []req.Resource{r})
	require.NoError(t, e)
	require.False(t, list[0].Favorite)
	a, e = s.Favorite(ctx, 1, r, false)
	require.NoError(t, e)
	require.True(t, a.Changed)
	a, e = s.Favorite(ctx, 1, r, false)
	require.NoError(t, e)
	require.False(t, a.Changed)
}

func TestMutualIntersectionUsesBoundedCandidateCursor(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	changeForTest(t, s, 1, 2, Follow)
	changeForTest(t, s, 2, 1, Follow)
	changeForTest(t, s, 1, 3, Follow)
	page, err := s.ListUsers(ctx, 1, "mutual", req.CursorSearch{Limit: 1}, 0)
	require.NoError(t, err)
	require.Empty(t, page.List)
	require.True(t, page.HasMore)
	require.NotEmpty(t, page.NextCursor)
	page, err = s.ListUsers(ctx, 1, "mutual", req.CursorSearch{Limit: 1, Cursor: page.NextCursor}, 0)
	require.NoError(t, err)
	require.Len(t, page.List, 1)
	require.Equal(t, "2", page.List[0].AccountNo)
	require.False(t, page.HasMore)
}
func TestHistoryDevicesOutOfOrderRewindClearDelete(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceShortVideo, ResourceID: "visible"}
	opened, e := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "request-0001"})
	require.NoError(t, e)
	duplicate, e := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "request-0001"})
	require.NoError(t, e)
	require.Equal(t, opened, duplicate)
	report := req.HistoryReport{Resource: r, SessionToken: opened.SessionToken, SessionVersion: opened.SessionVersion, Epoch: opened.Epoch, Sequence: 2, PositionMs: 5000, Checkpoint: true}
	out, e := s.ReportHistory(ctx, 1, report)
	require.NoError(t, e)
	require.True(t, out.Persisted)
	require.True(t, out.Accepted)
	report.Sequence = 1
	report.PositionMs = 9000
	out, e = s.ReportHistory(ctx, 1, report)
	require.NoError(t, e)
	require.False(t, out.Accepted)
	report.Sequence = 3
	report.PositionMs = 1000
	out, e = s.ReportHistory(ctx, 1, report)
	require.NoError(t, e)
	require.True(t, out.Accepted)
	list, e := s.HistoryProgress(ctx, 1, []req.Resource{r})
	require.NoError(t, e)
	require.Equal(t, int64(1000), list.List[0].PositionMs)
	other, e := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "device-0002"})
	require.NoError(t, e)
	require.Greater(t, other.SessionVersion, opened.SessionVersion)
	report.Sequence = 4
	_, e = s.ReportHistory(ctx, 1, report)
	require.ErrorIs(t, e, ErrConflict)
	report.SessionToken = other.SessionToken
	report.SessionVersion = other.SessionVersion
	require.NoError(t, s.DeleteHistory(ctx, 1, nil))
	_, e = s.ReportHistory(ctx, 1, report)
	require.ErrorIs(t, e, ErrConflict)
	third, e := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "device-0003"})
	require.NoError(t, e)
	require.Greater(t, third.Epoch, opened.Epoch)
	require.NoError(t, s.DeleteHistory(ctx, 1, &r))
	report.SessionToken = third.SessionToken
	report.SessionVersion = third.SessionVersion
	report.Epoch = third.Epoch
	_, e = s.ReportHistory(ctx, 1, report)
	require.ErrorIs(t, e, ErrConflict)
}
func TestRedisOutageFailsClosedButRelationshipsRemainCorrect(t *testing.T) {
	s := testService(t)
	s.Redis = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 10 * time.Millisecond, MaxRetries: -1})
	defer s.Redis.Close()
	require.ErrorIs(t, s.RateLimit(context.Background(), 1, false), ErrUnavailable)
	changeForTest(t, s, 1, 2, Follow)
	rows, e := s.BatchStatus(context.Background(), 1, []req.Target{target(2)})
	require.NoError(t, e)
	require.True(t, rows[0].Following)
}

type testSink struct {
	Calls atomic.Int32
	Fail  atomic.Bool
}

func (*testSink) Name() string { return "test" }
func (s *testSink) Deliver(context.Context, Notification) error {
	s.Calls.Add(1)
	if s.Fail.Load() {
		return errors.New("temporary")
	}
	return nil
}

func TestRecordLiveStartedWithoutSinkDoesNotCreateBacklog(t *testing.T) {
	s := testService(t)
	require.NoError(t, s.repository().Transaction(context.Background(), func(tx *gorm.DB) error {
		return s.RecordLiveStarted(tx, 2, "no-channel-session")
	}))
	var count int64
	require.NoError(t, s.DB.Model(&model.SocialOutboxEvent{}).
		Where("event_type = ? AND object_no = ?", model.EventLiveStarted, "no-channel-session").
		Count(&count).Error)
	require.Zero(t, count)
}

func TestOutboxDedupFailureReplayAndRevocation(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	sink := &testSink{}
	s.Sink = sink
	changeForTest(t, s, 1, 2, Follow)
	changeForTest(t, s, 1, 2, Subscribe)
	require.NoError(t, s.repository().Transaction(ctx, func(tx *gorm.DB) error { return s.RecordLiveStarted(tx, 2, "session1") }))
	require.NoError(t, s.repository().Transaction(ctx, func(tx *gorm.DB) error { return s.RecordLiveStarted(tx, 2, "session1") }))
	for i := 0; i < 6; i++ {
		require.NoError(t, s.TickOutbox(ctx))
	}
	require.Equal(t, int32(1), sink.Calls.Load())
	var e model.SocialOutboxEvent
	require.NoError(t, s.DB.Where("event_type = ?", model.EventLiveStarted).First(&e).Error)
	require.NoError(t, s.DB.Model(&e).Updates(map[string]interface{}{"status": model.EventPending, "cursor": 0, "next_attempt_at": 0}).Error)
	require.NoError(t, s.TickOutbox(ctx))
	require.Equal(t, int32(1), sink.Calls.Load())
	changeForTest(t, s, 1, 2, Unsubscribe)
	require.NoError(t, s.repository().Transaction(ctx, func(tx *gorm.DB) error { return s.RecordLiveStarted(tx, 2, "session2") }))
	for i := 0; i < 5; i++ {
		require.NoError(t, s.TickOutbox(ctx))
	}
	require.Equal(t, int32(1), sink.Calls.Load())
	changeForTest(t, s, 1, 2, Subscribe)
	require.NoError(t, s.repository().Transaction(ctx, func(tx *gorm.DB) error { return s.RecordLiveStarted(tx, 2, "session3") }))
	s.Sink = nil
	for i := 0; i < 12; i++ {
		_ = s.DB.Model(&model.SocialOutboxEvent{}).Where("status = ?", model.EventPending).Update("next_attempt_at", 0).Error
		_ = s.TickOutbox(ctx)
	}
	// First会自动叠加已有主键，使用新变量校验失败状态。
	var failed model.SocialOutboxEvent
	require.NoError(t, s.DB.Where("object_no = ?", "session3").First(&failed).Error)
	require.Equal(t, model.EventFailed, failed.Status)
	require.Equal(t, "CHANNEL_UNAVAILABLE", failed.LastError)
	s.Sink = sink
	_, err := s.ReplayEvent(ctx, failed.EventID)
	require.NoError(t, err)
	require.NoError(t, s.TickOutbox(ctx))
	require.Equal(t, int32(2), sink.Calls.Load())
}
func TestMySQLConcurrency(t *testing.T) {
	dsn := os.Getenv("SOCIAL_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set SOCIAL_TEST_MYSQL_DSN to a disposable social_test_* database")
	}
	cfg, err := mysqlDriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "social_test_"), "refuse non-test database")
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(64)
	require.NoError(t, db.Migrator().DropTable(append(model.Models(), &testUser{})...))
	s := fixture(t, db)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.ChangeRelationship(ctx, 1, target(2), Follow, "")
			if e != nil {
				errs <- e
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	assertCounters(t, s, 1, 1, 0)
	assertCounters(t, s, 2, 0, 1)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			action := Follow
			if i%3 == 0 {
				action = Unfollow
			}
			_, _ = s.ChangeRelationship(ctx, 1, target(2), action, "")
			if i%7 == 0 {
				_, _ = s.ChangeRelationship(ctx, 2, target(1), Block, "")
			}
		}(i)
	}
	wg.Wait()
	following, blocked, e := s.CheckRelationship(ctx, s.DB, 1, 2)
	require.NoError(t, e)
	require.True(t, blocked)
	require.False(t, following)
	assertCounters(t, s, 1, 0, 0)
	assertCounters(t, s, 2, 0, 0)
	changeForTest(t, s, 2, 1, Unblock)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, e := s.ChangeRelationship(ctx, 1, target(2), Block, "")
		if e != nil {
			t.Error(e)
		}
	}()
	go func() {
		defer wg.Done()
		_, e := s.ChangeRelationship(ctx, 2, target(1), Block, "")
		if e != nil {
			t.Error(e)
		}
	}()
	wg.Wait()
	assertCounters(t, s, 1, 0, 0)
	assertCounters(t, s, 2, 0, 0)
}
func BenchmarkFollowUnfollow(b *testing.B) {
	s := testService(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := s.ChangeRelationship(ctx, 1, target(2), Follow, ""); e != nil {
			b.Fatal(e)
		}
		if _, e := s.ChangeRelationship(ctx, 1, target(2), Unfollow, ""); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkBatchStatus(b *testing.B) {
	s := testService(b)
	changeForTest(b, s, 1, 2, Follow)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := s.BatchStatus(ctx, 1, []req.Target{target(2), target(3), target(4)}); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkHistoryCheckpoint(b *testing.B) {
	s := testService(b)
	ctx := context.Background()
	r := req.Resource{ResourceType: model.ResourceShortVideo, ResourceID: "visible"}
	session, e := s.OpenHistory(ctx, 1, req.HistoryOpen{Resource: r, RequestID: "benchmark-0001"})
	require.NoError(b, e)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, e = s.ReportHistory(ctx, 1, req.HistoryReport{Resource: r, SessionToken: session.SessionToken, SessionVersion: session.SessionVersion, Epoch: session.Epoch, Sequence: uint64(i + 1), PositionMs: int64(i % 10000), Checkpoint: true})
		if e != nil {
			b.Fatal(e)
		}
	}
}

func TestCounterAndSessionOverflowFailWithoutPartialChange(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	changeForTest(t, s, 1, 2, Follow)
	changeForTest(t, s, 1, 2, Unfollow)
	require.NoError(t, s.DB.Model(&model.SocialUserCounter{}).Where("user_id=1").Update("version", repo.MaxSafeInteger).Error)
	_, err := s.ChangeRelationship(ctx, 1, target(2), Follow, "")
	require.ErrorIs(t, err, repo.ErrCounterInvariant)
	following, err := repo.Following(s.DB, 1, 2)
	require.NoError(t, err)
	require.False(t, following)
}
func TestOutboxCrashLeaseRecoveryAndStaleCompletion(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	changeForTest(t, s, 1, 2, Follow)
	first, ok, err := s.claimEvent(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = s.claimEvent(ctx)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, s.DB.Model(&model.SocialOutboxEvent{}).Where("id=?", first.ID).Update("next_attempt_at", 0).Error)
	second, ok, err := s.claimEvent(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEqual(t, first.LeaseToken, second.LeaseToken)
	stale := s.DB.Model(&model.SocialOutboxEvent{}).Where("id=? AND lease_token=?", first.ID, first.LeaseToken).Update("status", model.EventDone)
	require.NoError(t, stale.Error)
	require.Zero(t, stale.RowsAffected)
}

func TestMySQLUTCTimestampWithProjectLocalDSN(t *testing.T) {
	dsn := os.Getenv("SOCIAL_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires disposable MySQL")
	}
	cfg, err := mysqlDriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "social_test_"))
	cfg.ParseTime = true
	cfg.Loc = time.FixedZone("Asia/Shanghai", 8*3600)
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, _ := db.DB()
	defer pool.Close()
	s := &Service{DB: db}
	before := time.Now().UTC()
	require.NoError(t, s.repository().Transaction(context.Background(), func(tx *gorm.DB) error { return event(tx, model.EventRelationship, 1, 2, "", "timezone_test", "") }))
	var row model.SocialOutboxEvent
	require.NoError(t, db.Where("action = ?", "timezone_test").Order("id DESC").First(&row).Error)
	require.WithinDuration(t, before, row.CreatedAt.Time, 3*time.Second)
}

func TestDeliveryReceiptFailureIsAtLeastOnce(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	sink := &testSink{}
	s.Sink = sink
	changeForTest(t, s, 1, 2, Follow)
	changeForTest(t, s, 1, 2, Subscribe)
	require.NoError(t, s.consumeRelationshipEvents(ctx))
	require.NoError(t, s.repository().Transaction(ctx, func(tx *gorm.DB) error { return s.RecordLiveStarted(tx, 2, "receipt-failure") }))
	require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register("test:receipt_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "social_event_consumer" {
			tx.AddError(errors.New("receipt commit failed"))
		}
	}))
	require.Error(t, s.TickOutbox(ctx))
	require.Equal(t, int32(1), sink.Calls.Load())
	require.NoError(t, s.DB.Callback().Create().Remove("test:receipt_failure"))
	require.NoError(t, s.DB.Model(&model.SocialOutboxEvent{}).Where("object_no = ?", "receipt-failure").Update("next_attempt_at", 0).Error)
	require.NoError(t, s.TickOutbox(ctx))
	require.Equal(t, int32(2), sink.Calls.Load(), "外部发送可重复，要求渠道按幂等键去重，不能宣称端到端exactly-once")
	var n int64
	require.NoError(t, s.DB.Model(&model.SocialEventConsumer{}).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestCommonRelationshipsRespectBlockPrivacy(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	changeForTest(t, s, 1, 3, Follow)
	changeForTest(t, s, 2, 3, Follow)
	list, err := s.ListUsers(ctx, 1, "common", req.CursorSearch{}, 2)
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	changeForTest(t, s, 2, 1, Block)
	_, err = s.ListUsers(ctx, 1, "common", req.CursorSearch{}, 2)
	require.ErrorIs(t, err, ErrBlocked)
}
