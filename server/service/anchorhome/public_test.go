package anchorhome_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/glebarez/sqlite"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"tb_live_module/global"
	"tb_live_module/initialize"
	"tb_live_module/internal/modulekit"
	alb "tb_live_module/model/anchorhome"
	req "tb_live_module/model/anchorhome/request"
	gift "tb_live_module/model/gift"
	guardModel "tb_live_module/model/guard"
	live "tb_live_module/model/live"
	rankModel "tb_live_module/model/ranking"
	social "tb_live_module/model/social"
	svc "tb_live_module/service/anchorhome"
	guardSvc "tb_live_module/service/guard"
	ranking "tb_live_module/service/ranking"
	"testing"
	"time"
)

type publicFixture struct {
	db      *gorm.DB
	s       *svc.PublicService
	now     time.Time
	queries *atomic.Int64
}
type publicQueryLogger struct {
	logger.Interface
	count *atomic.Int64
}

func (l publicQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.count.Add(1)
	l.Interface.Trace(ctx, begin, fc, err)
}

func publicDB(t *testing.T, real bool) *gorm.DB {
	t.Helper()
	var dialect gorm.Dialector
	if !real {
		dialect = sqlite.Open("file:" + t.TempDir() + "/home.db")
	} else {
		dsn := os.Getenv("ANCHOR_HOME_PUBLIC_MYSQL_DSN")
		if dsn == "" {
			t.Skip("isolated MySQL opt-in")
		}
		c, err := driver.ParseDSN(dsn)
		require.NoError(t, err)
		// Guard before opening any network connection or performing fixture DDL.
		require.Equal(t, "tcp", c.Net)
		require.Equal(t, "127.0.0.1:13339", c.Addr)
		require.Equal(t, "anchor_home_test_public", c.DBName)
		c.ParseTime = true
		c.Loc = time.UTC
		c.Timeout = 3 * time.Second
		c.ReadTimeout = 10 * time.Second
		c.WriteTimeout = 10 * time.Second
		dialect = mysql.Open(c.FormatDSN())
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	if real {
		pool.SetMaxOpenConns(20)
	} else {
		pool.SetMaxOpenConns(1)
	}
	t.Cleanup(func() { pool.Close() })
	if real {
		require.NoError(t, db.Exec("DROP DATABASE anchor_home_test_public").Error)
		require.NoError(t, db.Exec("CREATE DATABASE anchor_home_test_public CHARACTER SET utf8mb4").Error)
		require.NoError(t, db.Exec("USE anchor_home_test_public").Error)
	}
	return db
}

func newPublicFixture(t *testing.T, real bool) publicFixture {
	t.Helper()
	db := publicDB(t, real)
	models := []any{&live.LiveAccount{}, &live.LiveAnchor{}, &live.LiveRoom{}, &live.LiveSession{}, &live.LiveCategory{}, &gift.Order{}, &gift.Revenue{}, &guardModel.Plan{}, &guardModel.Membership{}, &guardModel.Ledger{}}
	models = append(models, social.Models()...)
	require.NoError(t, db.AutoMigrate(models...))
	require.NoError(t, svc.MigrateAlbumSchema(db))
	require.NoError(t, initialize.MigrateRanking(db, "Asia/Kuala_Lumpur"))
	for id := uint(1); id <= 6; id++ {
		phone := fmt.Sprintf("+1000000000%d", id)
		require.NoError(t, db.Create(&live.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: fmt.Sprintf("u%d", id), Username: fmt.Sprintf("synthetic_%d", id), Nickname: fmt.Sprintf("观众%d", id), Password: "SECRET_PASSWORD_HASH", Phone: &phone, Status: 1}).Error)
	}
	a := live.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: 1}, UserId: 1, AnchorNo: "host", Nickname: "主播名片", Avatar: "/safe-avatar.png", Signature: "直播简介", ApplyStatus: live.AnchorApplyStatusApproved, Status: live.AnchorStatusNormal, CertStatus: live.AnchorCertStatusApproved, LivePermission: 1, Level: 3, CountryCode: "MY", Language: "zh", CategoryId: 1, Remark: "PRIVATE_REMARK"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&live.LiveCategory{GVA_MODEL: global.GVA_MODEL{ID: 1}, Code: "music", Name: "音乐", Status: 1}).Error)
	require.NoError(t, db.Create(&live.LiveRoom{GVA_MODEL: global.GVA_MODEL{ID: 1}, RoomNo: "room", AnchorId: 1, StreamName: "PRIVATE_STREAM", PublishSecretHash: "PRIVATE_PUBLISH", Status: 1, Visibility: 1, LiveStatus: live.LiveRoomLiving, CurrentSessionId: 1, Title: "唱歌中"}).Error)
	require.NoError(t, db.Create(&live.LiveSession{GVA_MODEL: global.GVA_MODEL{ID: 1}, SessionNo: "session", RoomId: 1, AnchorId: 1, Status: live.LiveSessionLiving}).Error)
	require.NoError(t, db.Create(&social.SocialUserCounter{UserID: 1, FollowingCount: 2, FansCount: 5}).Error)
	al, err := svc.NewAlbumService(db, svc.AlbumOptions{StorageDir: filepath.Join(t.TempDir(), "private-media")})
	require.NoError(t, err)
	old := svc.DefaultAlbum
	svc.DefaultAlbum = al
	t.Cleanup(func() { svc.DefaultAlbum = old; al.Close() })
	now := time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC)
	queries := &atomic.Int64{}
	db = db.Session(&gorm.Session{Logger: publicQueryLogger{Interface: logger.Default.LogMode(logger.Silent), count: queries}})
	return publicFixture{db: db, s: &svc.PublicService{DB: db, Now: func() time.Time { return now }}, now: now, queries: queries}
}

func checkPublicProfile(t *testing.T, f publicFixture) {
	ctx := context.Background()
	for i, status := range []string{alb.PhotoApproved, alb.PhotoPending, alb.PhotoRejected, alb.PhotoRemoved, alb.PhotoDeleted} {
		require.NoError(t, f.db.Create(&alb.Photo{PhotoNo: fmt.Sprintf("photo%d", i), OwnerID: 1, AnchorNo: "host", StorageKey: fmt.Sprintf("%032d", i), Status: status, Version: 1, Sort: i + 1, Width: 200, Height: 300, Bytes: 100, Reason: "PRIVATE_REASON"}).Error)
	}
	f.queries.Store(0)
	p, err := f.s.Profile(ctx, 0, "host")
	require.NoError(t, err)
	require.LessOrEqual(t, f.queries.Load(), int64(12))
	require.Equal(t, "主播名片", p.Nickname)
	require.Equal(t, []string{"音乐"}, p.Tags)
	require.Equal(t, "5", p.FansCount)
	require.Len(t, p.Photos, 1)
	require.Equal(t, "photo0", p.Photos[0].PhotoNo)
	require.NotNil(t, p.Live)
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	for _, secret := range []string{"SECRET_", "PRIVATE_", "userId", "phone", "income", "balance", "streamName", "storageKey", "reason", "birthday"} {
		require.NotContains(t, string(raw), secret)
	}
	require.NoError(t, f.db.Model(&live.LiveRoom{}).Where("id=1").Update("visibility", live.LiveRoomVisibilityFollowers).Error)
	p, err = f.s.Profile(ctx, 0, "host")
	require.NoError(t, err)
	require.Nil(t, p.Live)
	p, err = f.s.Profile(ctx, 2, "host")
	require.NoError(t, err)
	require.Nil(t, p.Live)
	require.NoError(t, f.db.Create(&social.SocialFollow{UserID: 2, TargetUserID: 1}).Error)
	require.NoError(t, f.db.Create(&social.SocialFollow{UserID: 1, TargetUserID: 2}).Error)
	p, err = f.s.Profile(ctx, 2, "host")
	require.NoError(t, err)
	require.NotNil(t, p.Live)
	require.True(t, p.Relationship.Mutual)
	require.NoError(t, f.db.Model(&live.LiveRoom{}).Where("id=1").Update("visibility", live.LiveRoomVisibilityPrivate).Error)
	p, err = f.s.Profile(ctx, 2, "host")
	require.NoError(t, err)
	require.Nil(t, p.Live)
	p, err = f.s.Profile(ctx, 1, "host")
	require.NoError(t, err)
	require.NotNil(t, p.Live)
	require.True(t, p.IsSelf)
	for _, pair := range [][2]uint64{{1, 2}, {2, 1}} {
		b := social.SocialBlock{UserID: pair[0], TargetUserID: pair[1]}
		require.NoError(t, f.db.Create(&b).Error)
		_, err = f.s.Profile(ctx, 2, "host")
		require.ErrorIs(t, err, svc.ErrNotFound)
		require.NoError(t, f.db.Delete(&b).Error)
	}
	for _, field := range []struct {
		m     any
		name  string
		value any
		reset any
	}{{&live.LiveAccount{}, "status", 2, 1}, {&live.LiveAnchor{}, "status", 2, 1}, {&live.LiveAnchor{}, "apply_status", 1, live.AnchorApplyStatusApproved}} {
		require.NoError(t, f.db.Model(field.m).Where("id=1").Update(field.name, field.value).Error)
		_, err = f.s.Profile(ctx, 0, "host")
		require.ErrorIs(t, err, svc.ErrNotFound)
		require.NoError(t, f.db.Model(field.m).Where("id=1").Update(field.name, field.reset).Error)
	}
	for _, bad := range []string{"", strings.Repeat("a", 33), "../host", "host' OR 1=1--"} {
		_, err = f.s.Profile(ctx, 0, bad)
		require.Error(t, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = f.s.Profile(cancelled, 0, "host")
	require.ErrorIs(t, err, context.Canceled)
}

func checkPublicSupporters(t *testing.T, f publicFixture) {
	ctx := context.Background()
	at := f.now.UnixMilli()
	for i, user := range []string{"u3", "u4"} {
		amount := int64(100)
		order := gift.Order{OrderNo: fmt.Sprintf("o%d", i), RequestID: fmt.Sprintf("request-%d", i), SenderUserID: uint64(i + 3), SenderAccountNo: user, AnchorNo: "host", RoomNo: "room", SessionNo: "session", CategoryNo: "music", GiftNo: "gift", UnitPrice: amount, Quantity: 1, GrossCoinAmount: amount, Status: "succeeded", WalletTransactionNo: fmt.Sprintf("wt%d", i), WalletLedgerNo: fmt.Sprintf("wl%d", i), CreatedAt: at}
		require.NoError(t, f.db.Create(&order).Error)
		require.NoError(t, f.db.Create(&gift.Revenue{RevenueNo: fmt.Sprintf("r%d", i), GiftOrderNo: order.OrderNo, BusinessNo: order.OrderNo, EntryType: "gift", GrossCoinAmount: amount, Quantity: 1, CreatedAt: at}).Error)
	}
	rank := ranking.Service{DB: f.db, Timezone: "Asia/Kuala_Lumpur", Now: func() time.Time { return f.now }}
	_, err := rank.Consume(ctx, 20)
	require.NoError(t, err)
	q := req.SupportersQuery{ProfileQuery: req.ProfileQuery{AnchorNo: "host"}, Period: "all", Page: 1, PageSize: 1}
	b, err := f.s.Supporters(ctx, 0, q)
	require.NoError(t, err)
	require.Equal(t, "2", b.Total)
	require.Equal(t, "u3", b.List[0].SubjectNo)
	require.Equal(t, "100", b.List[0].Score)
	require.Empty(t, b.LagCount)
	// A refund source changes the same projection, never a parallel synthetic score.
	require.NoError(t, f.db.Create(&gift.Revenue{RevenueNo: "refund0", GiftOrderNo: "o0", BusinessNo: "refund00", EntryType: "refund", GrossCoinAmount: -100, Quantity: -1, CreatedAt: at + 1}).Error)
	_, err = rank.Consume(ctx, 20)
	require.NoError(t, err)
	for _, period := range []string{"day", "month", "all"} {
		q.Period = period
		b, err = f.s.Supporters(ctx, 0, q)
		require.NoError(t, err)
		require.Equal(t, "1", b.Total)
		require.Equal(t, "u4", b.List[0].SubjectNo)
	}
	require.NoError(t, f.db.Create(&social.SocialBlock{UserID: 4, TargetUserID: 2}).Error)
	b, err = f.s.Supporters(ctx, 2, q)
	require.NoError(t, err)
	require.Equal(t, "0", b.Total)
	require.Empty(t, b.List)
	require.NoError(t, f.db.Model(&live.LiveAccount{}).Where("id=4").Update("status", 2).Error)
	b, err = f.s.Supporters(ctx, 0, q)
	require.NoError(t, err)
	require.Empty(t, b.List)
	for _, bad := range []req.SupportersQuery{{ProfileQuery: q.ProfileQuery, Period: "week"}, {ProfileQuery: q.ProfileQuery, Page: 101}, {ProfileQuery: q.ProfileQuery, PageSize: 51}, {ProfileQuery: q.ProfileQuery, Page: -1}} {
		_, err = f.s.Supporters(ctx, 0, bad)
		require.ErrorIs(t, err, modulekit.Invalid)
	}
	require.NoError(t, f.db.Model(&rankModel.Checkpoint{}).Where("consumer_name='gift-revenue'").Update("last_error", "INTERNAL_PROJECTION_ERROR").Error)
	_, err = f.s.Supporters(ctx, 0, q)
	require.ErrorIs(t, err, modulekit.Invariant)
}

func seedPublicGuard(t *testing.T, f publicFixture, user uint64, segments []guardSvc.Segment) {
	t.Helper()
	raw, err := json.Marshal(segments)
	require.NoError(t, err)
	until := int64(0)
	for _, seg := range segments {
		until = max(until, seg.End)
	}
	no := fmt.Sprintf("member%d", user)
	ledger := fmt.Sprintf("ledger%d", user)
	require.NoError(t, f.db.Create(&guardModel.Ledger{LedgerNo: ledger, MembershipNo: no, SourceNo: fmt.Sprintf("source%d", user), AfterSnapshot: string(raw)}).Error)
	require.NoError(t, f.db.Create(&guardModel.Membership{MembershipNo: no, UserID: user, AnchorID: 1, Projection: string(raw), LedgerNo: ledger, ValidUntil: until, Status: "expired"}).Error)
}
func checkPublicGuards(t *testing.T, f publicFixture) {
	ctx := context.Background()
	at := f.now.UnixMilli()
	low := guardModel.PlanSnapshot{PlanNo: "silver", Name: "白银", Level: 1, Priority: 1, BenefitConfig: `{"publicBenefits":["守护身份展示"],"internal":"PRIVATE_BENEFIT"}`}
	high := guardModel.PlanSnapshot{PlanNo: "gold", Name: "黄金", Level: 2, Priority: 2}
	seedPublicGuard(t, f, 3, []guardSvc.Segment{{Plan: low, Start: at - 1000, End: at + 1000}, {Plan: high, Start: at - 1000, End: at}})
	seedPublicGuard(t, f, 4, []guardSvc.Segment{{Plan: high, Start: at - 1000, End: at}})
	seedPublicGuard(t, f, 5, []guardSvc.Segment{{Plan: high, Start: at + 1000, End: at + 2000}})
	q := req.GuardsQuery{ProfileQuery: req.ProfileQuery{AnchorNo: "host"}, Page: 1, PageSize: 50}
	g, err := f.s.Guards(ctx, 0, q)
	require.NoError(t, err)
	require.Equal(t, "1", g.Total)
	require.Len(t, g.List, 1)
	require.Equal(t, "silver", g.List[0].Plan.PlanNo)
	require.Equal(t, []string{"守护身份展示"}, g.List[0].Plan.Benefits)
	raw, _ := json.Marshal(g)
	for _, private := range []string{"PRIVATE_", "membershipNo", "expiresAt", "ledgerNo", "benefitConfig"} {
		require.NotContains(t, string(raw), private)
	}
	require.NoError(t, f.db.Create(&social.SocialBlock{UserID: 2, TargetUserID: 3}).Error)
	g, err = f.s.Guards(ctx, 2, q)
	require.NoError(t, err)
	require.Empty(t, g.List)
	require.Equal(t, "0", g.Total)
	require.NoError(t, f.db.Model(&guardModel.Membership{}).Where("user_id=3").Update("projection", strings.ReplaceAll(stringMustJSON([]guardSvc.Segment{{Plan: low, Start: at - 1000, End: at + 1000}}), "白银", "伪造")).Error)
	_, err = f.s.Guards(ctx, 0, q)
	require.ErrorIs(t, err, modulekit.Invariant)
}
func stringMustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func checkPublicSettings(t *testing.T, f publicFixture) {
	ctx := context.Background()
	settings, err := f.s.Settings(ctx, 1)
	require.NoError(t, err)
	updated, err := f.s.SaveSettings(ctx, 1, req.SaveSettings{Signature: "新的简介", CategoryID: 1, Version: settings.Version})
	require.NoError(t, err)
	require.NotEqual(t, settings.Version, updated.Version)
	_, err = f.s.SaveSettings(ctx, 1, req.SaveSettings{Signature: "新的简介", CategoryID: 1, Version: settings.Version})
	require.NoError(t, err)
	_, err = f.s.SaveSettings(ctx, 1, req.SaveSettings{Signature: "来自旧窗口", CategoryID: 1, Version: settings.Version})
	require.ErrorIs(t, err, modulekit.Conflict)
	var a live.LiveAnchor
	require.NoError(t, f.db.First(&a, 1).Error)
	require.Equal(t, "主播名片", a.Nickname)
	require.Equal(t, "PRIVATE_REMARK", a.Remark)
	require.Equal(t, uint32(3), a.Level)
	_, err = f.s.SaveSettings(ctx, 2, req.SaveSettings{Signature: "冒充", Version: updated.Version})
	require.ErrorIs(t, err, svc.ErrForbidden)
	for _, r := range []req.SaveSettings{{Signature: strings.Repeat("字", 256), Version: updated.Version}, {Signature: "\x00", Version: updated.Version}, {Signature: "test", CategoryID: 999, Version: updated.Version}, {Signature: "test", Version: "bad"}} {
		_, err = f.s.SaveSettings(ctx, 1, r)
		require.ErrorIs(t, err, modulekit.Invalid)
	}
}

func TestPublicHomeSQLiteContracts(t *testing.T) {
	for name, fn := range map[string]func(*testing.T, publicFixture){"profile": checkPublicProfile, "net_supporters": checkPublicSupporters, "guard_expiry": checkPublicGuards, "settings_cas": checkPublicSettings} {
		t.Run(name, func(t *testing.T) { fn(t, newPublicFixture(t, false)) })
	}
}
func TestMySQLPublicHomeContracts(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*testing.T, publicFixture)
	}{{"profile", checkPublicProfile}, {"net_supporters", checkPublicSupporters}, {"guard_expiry", checkPublicGuards}, {"settings_cas", checkPublicSettings}} {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newPublicFixture(t, true)) })
	}
}

func TestMySQLPublicSettingsRaceAndHomeLoad(t *testing.T) {
	f := newPublicFixture(t, true)
	ctx := context.Background()
	settings, err := f.s.Settings(ctx, 1)
	require.NoError(t, err)
	var successes atomic.Int64
	var conflicts atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, e := f.s.SaveSettings(ctx, 1, req.SaveSettings{Signature: fmt.Sprintf("编辑%d", i), CategoryID: 1, Version: settings.Version})
			if e == nil {
				successes.Add(1)
			} else if e == modulekit.Conflict {
				conflicts.Add(1)
			} else {
				t.Errorf("unexpected concurrent edit error: %v", e)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	require.Equal(t, int64(1), successes.Load())
	require.Equal(t, int64(11), conflicts.Load())
	// A large history must not be visited when the current live card is resolved.
	sessions := make([]live.LiveSession, 2000)
	for i := range sessions {
		sessions[i] = live.LiveSession{SessionNo: fmt.Sprintf("history%d", i), RoomId: 1, AnchorId: 1, Status: live.LiveSessionEnded}
	}
	require.NoError(t, f.db.CreateInBatches(sessions, 100).Error)
	const reads = 160
	durations := make([]time.Duration, reads)
	sem := make(chan struct{}, 8)
	f.queries.Store(0)
	for i := 0; i < reads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			at := time.Now()
			p, e := f.s.Profile(ctx, 2, "host")
			durations[i] = time.Since(at)
			if e != nil || p.Live == nil {
				t.Errorf("home read failed: %v", e)
			}
		}(i)
	}
	wg.Wait()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	require.Less(t, durations[len(durations)-1], 5*time.Second)
	require.LessOrEqual(t, f.queries.Load(), int64(reads*14))
	t.Logf("profile concurrent=8 reads=%d history=2000 p50=%s p95=%s max=%s SQL/request=%.1f", reads, durations[reads/2], durations[reads*95/100], durations[reads-1], float64(f.queries.Load())/reads)
}

func TestMySQLPublicSupporterAndGuardLoad(t *testing.T) {
	f := newPublicFixture(t, true)
	ctx := context.Background()
	at := f.now.UnixMilli()
	const members = 1000
	var cp rankModel.Checkpoint
	require.NoError(t, f.db.Where("consumer_name='gift-revenue'").Take(&cp).Error)
	users := make([]live.LiveAccount, 0, members)
	buckets := make([]rankModel.ScoreBucket, 0, members)
	memberships := make([]guardModel.Membership, 0, members)
	ledgers := make([]guardModel.Ledger, 0, members)
	// Long renewal histories exercise JSON selection and immutable-seal checks,
	// including 30 expired segments, a current lower tier and a future renewal.
	segments := make([]guardSvc.Segment, 0, 32)
	for i := 0; i < 30; i++ {
		segments = append(segments, guardSvc.Segment{GrantNo: fmt.Sprintf("old%d", i), Plan: guardModel.PlanSnapshot{PlanNo: "silver", Name: "白银", Priority: 1}, Start: at - int64(31-i)*1000, End: at - int64(30-i)*1000})
	}
	segments = append(segments, guardSvc.Segment{GrantNo: "current", Plan: guardModel.PlanSnapshot{PlanNo: "silver", Name: "白银", Priority: 1}, Start: at - 1000, End: at + 1000}, guardSvc.Segment{GrantNo: "future", Plan: guardModel.PlanSnapshot{PlanNo: "silver", Name: "白银", Priority: 1}, Start: at + 1000, End: at + 2000})
	projection := stringMustJSON(segments)
	for i := 0; i < members; i++ {
		id := uint(i + 100)
		no := fmt.Sprintf("load%05d", i)
		score := strconv.Itoa(members - i)
		users = append(users, live.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: no, Username: no, Password: "synthetic", Status: 1})
		buckets = append(buckets, rankModel.ScoreBucket{BucketKey: fmt.Sprintf("%064d", i+1), Generation: cp.Generation, RankType: "gift", PeriodType: "all", PeriodKey: "all", ScopeType: "anchor", ScopeNo: "host", SubjectType: "user", SubjectNo: no, ScoreCoin: score, ScoreSort: strings.Repeat("0", 65-len(score)) + score, Quantity: "1", Version: 1})
		ledgerNo := fmt.Sprintf("load-ledger%d", i)
		memberNo := fmt.Sprintf("load-member%d", i)
		memberships = append(memberships, guardModel.Membership{MembershipNo: memberNo, UserID: uint64(id), AnchorID: 1, Projection: projection, LedgerNo: ledgerNo, ValidUntil: at + 2000, Status: "active"})
		ledgers = append(ledgers, guardModel.Ledger{LedgerNo: ledgerNo, MembershipNo: memberNo, SourceNo: fmt.Sprintf("load-source%d", i), AfterSnapshot: projection})
	}
	require.NoError(t, f.db.CreateInBatches(users, 100).Error)
	require.NoError(t, f.db.CreateInBatches(buckets, 100).Error)
	require.NoError(t, f.db.CreateInBatches(memberships, 100).Error)
	require.NoError(t, f.db.CreateInBatches(ledgers, 100).Error)
	q := req.SupportersQuery{ProfileQuery: req.ProfileQuery{AnchorNo: "host"}, Period: "all", Page: 1, PageSize: 50}
	for _, kind := range []string{"supporters", "guards"} {
		t.Run(kind, func(t *testing.T) {
			const reads = 80
			times := make([]time.Duration, reads)
			sem := make(chan struct{}, 8)
			var wg sync.WaitGroup
			f.queries.Store(0)
			for i := 0; i < reads; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					start := time.Now()
					var total string
					var n int
					var err error
					if kind == "supporters" {
						out, e := f.s.Supporters(ctx, 2, q)
						err = e
						total = out.Total
						n = len(out.List)
					} else {
						out, e := f.s.Guards(ctx, 2, req.GuardsQuery{ProfileQuery: q.ProfileQuery, Page: 1, PageSize: 50})
						err = e
						total = out.Total
						n = len(out.List)
					}
					times[i] = time.Since(start)
					if err != nil || total != "1000" || n != 50 {
						t.Errorf("%s failed total=%s length=%d error=%v", kind, total, n, err)
					}
				}(i)
			}
			wg.Wait()
			sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
			require.Less(t, times[reads-1], 5*time.Second)
			require.LessOrEqual(t, f.queries.Load(), int64(reads*12))
			t.Logf("%s concurrent=8 reads=%d members=1000 renewalSegments=32 pageSize=50 p50=%s p95=%s max=%s SQL/request=%.1f", kind, reads, times[reads/2], times[reads*95/100], times[reads-1], float64(f.queries.Load())/reads)
		})
	}
	// The public board has a composite order index and never queries gift revenue.
	var explain string
	require.NoError(t, f.db.Raw("EXPLAIN FORMAT=JSON SELECT subject_no FROM ranking_score_bucket WHERE generation=? AND rank_type='gift' AND period_type='all' AND period_key='all' AND scope_type='anchor' AND scope_no='host' AND gift_no='' AND score_sort>? ORDER BY score_sort DESC,subject_no LIMIT 50", cp.Generation, strings.Repeat("0", 65)).Scan(&explain).Error)
	require.Contains(t, explain, "idx_ranking_board")
	require.NotContains(t, explain, "gift_revenue_ledger")
	t.Logf("board explain=%s", explain)
	// Older installations have another account collation. The indexed outer-key
	// conversion must handle it without illegal collation mixes or an O(N²) scan.
	require.NoError(t, f.db.Exec("ALTER TABLE live_account MODIFY account_no VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL").Error)
	upper := buckets[0]
	upper.ID = 0
	upper.BucketKey = strings.Repeat("f", 64)
	upper.SubjectNo = strings.ToUpper(upper.SubjectNo)
	require.NoError(t, f.db.Create(&upper).Error)
	board, err := f.s.Supporters(ctx, 2, q)
	require.NoError(t, err)
	require.Equal(t, "1000", board.Total, "case variants cannot resolve a different canonical identity")
	encoding, err := ranking.DiscoverAccountEncoding(f.db)
	require.NoError(t, err)
	require.Equal(t, "utf8mb4_unicode_ci", encoding.Collation)
	var plan []struct {
		Table string
		Type  string
		Key   *string
	}
	require.NoError(t, f.db.Raw(`EXPLAIN SELECT count(*) FROM ranking_score_bucket b WHERE generation=? AND rank_type='gift' AND period_type='all' AND period_key='all' AND scope_type='anchor' AND scope_no='host' AND gift_no='' AND subject_type='user' AND EXISTS (SELECT 1 FROM live_account pu WHERE pu.account_no=CONVERT(b.subject_no USING utf8mb4) COLLATE utf8mb4_unicode_ci AND BINARY pu.account_no=BINARY b.subject_no AND pu.deleted_at IS NULL AND pu.status=1)`, cp.Generation).Scan(&plan).Error)
	indexed := false
	for _, row := range plan {
		if row.Table == "pu" {
			require.Equal(t, "eq_ref", row.Type)
			require.NotNil(t, row.Key)
			require.Equal(t, "uk_live_account_account_no", *row.Key)
			indexed = true
		}
	}
	require.True(t, indexed)
}
