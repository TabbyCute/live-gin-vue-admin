package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	socialReq "tb_live_module/model/social/request"
	socialService "tb_live_module/service/social"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"tb_live_module/config"
	"tb_live_module/global"
	model "tb_live_module/model/live"
	req "tb_live_module/model/live/request"
	res "tb_live_module/model/live/response"
	socialModel "tb_live_module/model/social"
)

func presenceFixture(t *testing.T) (*PresenceService, model.LiveRoom, model.LiveSession) {
	t.Helper()
	addr := os.Getenv("PRESENCE_TEST_REDIS")
	if addr == "" {
		t.Skip("isolated Redis required: PRESENCE_TEST_REDIS")
	}
	require.True(t, strings.HasPrefix(addr, "127.0.0.1:"), "tests only accept loopback Redis")
	social, room, session := setupAccessTest(t)
	room.RoomNo = "pr_" + presenceRandom()[:24]
	session.SessionNo = "ps_" + presenceRandom()
	require.NoError(t, social.DB.Model(&room).Updates(map[string]any{"room_no": room.RoomNo, "visibility": model.LiveRoomVisibilityPublic}).Error)
	require.NoError(t, social.DB.Model(&session).Update("session_no", session.SessionNo).Error)
	require.NoError(t, social.DB.AutoMigrate(model.PresenceModels()...))
	r := redis.NewClient(&redis.Options{Addr: addr, DB: 15, ContextTimeoutEnabled: true, MaxRetries: -1})
	require.NoError(t, r.Ping(context.Background()).Err())
	social.Redis = r
	s := &PresenceService{DB: social.DB, Redis: r, Key: []byte(presenceRandom() + strings.Repeat("test-presence-key", 3)), Config: config.LivePresence{}, Social: social}
	t.Cleanup(func() {
		keys := presenceKeys(session.SessionNo)
		keys = append(keys, keys[0]+":lease")
		_ = r.Del(context.Background(), keys...).Err()
		r.Close()
	})
	return s, room, session
}
func presenceViewer() PresenceIdentity { return PresenceIdentity{Actor: 601, TokenVersion: 1} }
func openPresence(t *testing.T, s *PresenceService, room string, id string) res.PresenceSession {
	t.Helper()
	out, e := s.Open(context.Background(), presenceViewer(), s.IPKey(presenceRandom()), req.PresenceOpen{RoomNo: room, RequestID: id})
	require.NoError(t, e)
	return out
}
func TestPresenceLifecycleIdempotencyAndGuests(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	a := openPresence(t, s, room.RoomNo, "request_0001")
	b := openPresence(t, s, room.RoomNo, "request_0001")
	require.Equal(t, a.PresenceToken, b.PresenceToken)
	require.Equal(t, "1", b.ViewCount)
	c := openPresence(t, s, room.RoomNo, "request_0002")
	require.Equal(t, "2", c.ConnectionCount)
	require.Equal(t, "1", c.MemberCount)
	require.Equal(t, "1", c.ViewerCount)
	guest, e := s.NewGuest()
	require.NoError(t, e)
	g, e := s.GuestIdentity(guest.GuestToken)
	require.NoError(t, e)
	go1, e := s.Open(ctx, g, s.IPKey("guest-network"), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: "guest_request1"})
	require.NoError(t, e)
	require.Equal(t, "1", go1.GuestCount)
	require.Equal(t, "2", go1.TotalOnlineCount)
	_, e = s.Like(ctx, g, req.PresenceLike{PresenceToken: go1.PresenceToken, RequestID: "guest_like1", Sequence: "1", Count: "1"})
	require.ErrorIs(t, e, ErrPresenceAccess)
	h, e := s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "1", Playing: true, Visible: true})
	require.NoError(t, e)
	require.Equal(t, "0", h.ConnectionDurationMs)
	time.Sleep(12 * time.Millisecond)
	h, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "2", Playing: true, Visible: true})
	require.NoError(t, e)
	require.NotEqual(t, "0", h.ConnectionDurationMs)
	dup, e := s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "2", Playing: true, Visible: true})
	require.NoError(t, e)
	require.Equal(t, h.ConnectionDurationMs, dup.ConnectionDurationMs)
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "1", Playing: true, Visible: true})
	require.ErrorIs(t, e, ErrPresenceSequence)
	left, e := s.Leave(ctx, presenceViewer(), req.PresenceLeave{PresenceToken: a.PresenceToken, Sequence: "3", Reason: "user_leave"})
	require.NoError(t, e)
	require.Equal(t, "2", left.ConnectionCount)
	require.Equal(t, "1", left.MemberCount)
	left, e = s.Leave(ctx, presenceViewer(), req.PresenceLeave{PresenceToken: a.PresenceToken, Sequence: "3", Reason: "user_leave"})
	require.NoError(t, e)
	require.Equal(t, "2", left.ConnectionCount)
	_, e = s.Leave(ctx, presenceViewer(), req.PresenceLeave{PresenceToken: c.PresenceToken, Sequence: "1", Reason: "user_leave"})
	require.NoError(t, e)
	stats, e := s.Stats(ctx, g, room.RoomNo)
	require.NoError(t, e)
	require.Equal(t, "0", stats.MemberCount)
	require.Equal(t, "1", stats.GuestCount)
	require.Equal(t, "2", stats.PeakOnlineCount)
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	var n int64
	require.NoError(t, s.DB.Model(&model.LiveViewerVisit{}).Count(&n).Error)
	require.EqualValues(t, 3, n)
	require.NoError(t, s.DB.Model(&session).Updates(map[string]any{"status": model.LiveSessionEnded, "ended_at": time.Now().UnixMilli()}).Error)
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	require.NoError(t, s.DB.First(&session, session.ID).Error)
	require.Positive(t, session.StatsFinalizedAt)
	_, e = s.Open(ctx, presenceViewer(), s.IPKey("a"), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: "after_end_1"})
	require.ErrorIs(t, e, ErrPresenceAccess)
	var cp model.LivePresenceCheckpoint
	require.NoError(t, s.DB.First(&cp, "session_no=?", session.SessionNo).Error)
	require.EqualValues(t, 3, cp.ViewCount)
	require.EqualValues(t, 2, cp.ViewerCount)
}
func TestPresenceLikeRaceAndAuthorization(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	o := openPresence(t, s, room.RoomNo, "like_open_01")
	r := req.PresenceLike{PresenceToken: o.PresenceToken, Sequence: "1", RequestID: "like_request1", Count: "5"}
	var wg sync.WaitGroup
	var passed atomic.Int64
	for n := 0; n < 40; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.Like(ctx, presenceViewer(), r)
			if e == nil && v.AcceptedCount == "5" {
				passed.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 40, passed.Load())
	v, e := s.Stats(ctx, presenceViewer(), room.RoomNo)
	require.NoError(t, e)
	require.Equal(t, "5", v.LikeCount)
	r.RequestID = "another_like1"
	_, e = s.Like(ctx, presenceViewer(), r)
	require.ErrorIs(t, e, ErrPresenceSequence)
	r.Sequence = "2"
	r.Count = "20"
	_, e = s.Like(ctx, presenceViewer(), r)
	require.ErrorIs(t, e, ErrPresenceLimited)
	_, e = s.Heartbeat(ctx, PresenceIdentity{Actor: 602, TokenVersion: 1}, req.PresenceHeartbeat{PresenceToken: o.PresenceToken, Sequence: "1"})
	require.ErrorIs(t, e, ErrPresenceExpired)
	tkn, _, e := s.validate(ctx, presenceViewer(), o.PresenceToken, false)
	require.NoError(t, e)
	require.NoError(t, s.Redis.HSet(ctx, presenceKeys(session.SessionNo)[1], tkn.Connection, mutateMeta(t, s, session.SessionNo, tkn.Connection, func(m map[string]any) { m["authAt"] = 0 })).Err())
	require.NoError(t, s.DB.Model(&model.LiveAccount{}).Where("id=601").Update("status", model.LiveAccountStatusBanned).Error)
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: o.PresenceToken, Sequence: "1"})
	require.ErrorIs(t, e, ErrPresenceAccess)
}
func mutateMeta(t *testing.T, s *PresenceService, session, c string, f func(map[string]any)) string {
	t.Helper()
	raw, e := s.Redis.HGet(context.Background(), presenceKeys(session)[1], c).Result()
	require.NoError(t, e)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	f(m)
	b, e := json.Marshal(m)
	require.NoError(t, e)
	return string(b)
}
func TestPresenceExpirationConcurrencyAndBackpressure(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	a := openPresence(t, s, room.RoomNo, "expire_open1")
	b := openPresence(t, s, room.RoomNo, "expire_open2")
	tkn, _, e := s.validate(ctx, presenceViewer(), a.PresenceToken, false)
	require.NoError(t, e)
	require.NoError(t, s.Redis.ZAdd(ctx, presenceKeys(session.SessionNo)[2], redis.Z{Score: 1, Member: a.ConnectionNo}).Err())
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.run(ctx, tkn, map[string]any{"op": "expire", "batch": 1}) }()
	}
	wg.Wait()
	stats, e := s.Stats(ctx, presenceViewer(), room.RoomNo)
	require.NoError(t, e)
	require.Equal(t, "1", stats.ConnectionCount)
	require.Equal(t, "1", stats.MemberCount)
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "1"})
	require.ErrorIs(t, e, ErrPresenceExpired)
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	var rows []model.LiveRoomEvent
	require.NoError(t, s.DB.Where("event_type=?", "room.viewer.timeout").Find(&rows).Error)
	require.Len(t, rows, 1)
	// MySQL failure does not acknowledge or discard a pending visit.
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: b.PresenceToken, Sequence: "1"})
	require.NoError(t, e)
	require.NoError(t, s.DB.Migrator().DropTable(&model.LiveViewerVisit{}))
	require.Error(t, s.FlushSession(ctx, session.SessionNo))
	require.Positive(t, s.Redis.ZCard(ctx, presenceKeys(session.SessionNo)[7]).Val())
	require.NoError(t, s.DB.AutoMigrate(&model.LiveViewerVisit{}))
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	// Worker stale -> bounded admission stops, even though Redis is still healthy.
	require.NoError(t, s.Redis.HSet(ctx, presenceKeys(session.SessionNo)[0], "committed", 1).Err())
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: b.PresenceToken, Sequence: "1"})
	require.ErrorIs(t, e, ErrPresenceCapacity)
	_, e = s.Open(ctx, presenceViewer(), s.IPKey("stale-worker"), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: "stale_worker_open"})
	require.ErrorIs(t, e, ErrPresenceCapacity)
}
func TestPresenceViewersAndCorruption(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	a := openPresence(t, s, room.RoomNo, "viewer_open1")
	_ = openPresence(t, s, room.RoomNo, "viewer_open2")
	page, e := s.Viewers(ctx, presenceViewer(), req.PresenceQuery{RoomNo: room.RoomNo, Limit: 1}, true)
	require.NoError(t, e)
	require.Len(t, page.List, 1)
	raw, _ := json.Marshal(page)
	for _, secret := range []string{"presenceToken", "connectionNo", "tokenVersion", "password", "visitorKey", "redis", "phone"} {
		require.NotContains(t, string(raw), secret)
	}
	// Corrupt key type must fail before a write changes any statistics.
	before := s.Redis.HGet(ctx, presenceKeys(session.SessionNo)[0], "view").Val()
	require.NoError(t, s.Redis.Del(ctx, presenceKeys(session.SessionNo)[6]).Err())
	require.NoError(t, s.Redis.Set(ctx, presenceKeys(session.SessionNo)[6], "bad", time.Minute).Err())
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "1"})
	require.ErrorIs(t, e, ErrPresenceCorrupt)
	require.Equal(t, before, s.Redis.HGet(ctx, presenceKeys(session.SessionNo)[0], "view").Val())
}
func TestPresenceCredentialAndIntegerBounds(t *testing.T) {
	s := &PresenceService{Key: []byte(strings.Repeat("k", 32))}
	i := presenceViewer()
	token := s.sign("connection", presenceToken{Room: "room", Session: "session", Connection: "pc_" + strings.Repeat("a", 64), Actor: i.key(), Epoch: strings.Repeat("b", 32), Created: time.Now().UnixMilli()})
	_, e := s.parseToken(i, token)
	require.NoError(t, e)
	_, e = s.parseToken(i, token+"x")
	require.Error(t, e)
	_, e = s.parseToken(PresenceIdentity{Visitor: "a"}, token)
	require.Error(t, e)
	for _, raw := range []string{"0", "01", "-1", "1.0", "1e3", "9007199254740992", "18446744073709551615", " 1", "+1"} {
		_, e := presenceInt(raw, presenceMaxInteger)
		require.Error(t, e, raw)
	}
	for _, session := range []string{"s_1", "s-2"} {
		for _, key := range presenceKeys(session) {
			require.Contains(t, key, "{"+session+"}")
		}
	}
	require.NotContains(t, presenceStateLua, "redis.call('KEYS'")
}
func FuzzPresenceCredentials(f *testing.F) {
	f.Add("token")
	f.Add("eyJzIjoieCJ9.invalid")
	f.Fuzz(func(t *testing.T, v string) {
		s := &PresenceService{Key: []byte(strings.Repeat("k", 32))}
		_, _ = s.parseToken(presenceViewer(), v)
	})
}
func TestPresenceStatsOwnership(t *testing.T) {
	require.ErrorIs(t, (&RoomService{}).UpdateSessionStats(req.LiveSessionStatsReq{}), ErrPresenceStatsOwned)
}

var _ = fmt.Sprint
var _ = global.GVA_DB
var _ = socialModel.ResourceLiveRoom

func TestPresenceWatchUnionPauseAndCap(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	a := openPresence(t, s, room.RoomNo, "watch_open1")
	b := openPresence(t, s, room.RoomNo, "watch_open2")
	for _, o := range []res.PresenceSession{a, b} {
		_, e := s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: o.PresenceToken, Sequence: "1", Playing: true, Visible: true})
		require.NoError(t, e)
	}
	now := time.Now().UnixMilli()
	for _, o := range []res.PresenceSession{a, b} {
		require.NoError(t, s.Redis.HSet(ctx, presenceKeys(session.SessionNo)[1], o.ConnectionNo, mutateMeta(t, s, session.SessionNo, o.ConnectionNo, func(m map[string]any) { m["last"] = now - 60000 })).Err())
	}
	ah, e := s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "2", Playing: true, Visible: true})
	require.NoError(t, e)
	require.Equal(t, "30000", ah.ConnectionDurationMs)
	bh, e := s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: b.PresenceToken, Sequence: "2", Playing: true, Visible: true})
	require.NoError(t, e)
	require.Equal(t, "30000", bh.ConnectionDurationMs)
	total, e := strconv.ParseUint(bh.TotalWatchDurationMs, 10, 64)
	require.NoError(t, e)
	require.Less(t, total, uint64(31000), "overlapping devices must not double watch time")
	paused, e := s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "3", Playing: false, Visible: true})
	require.NoError(t, e)
	require.Equal(t, ah.ConnectionDurationMs, paused.ConnectionDurationMs)
	invisible, e := s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: a.PresenceToken, Sequence: "4", Playing: true, Visible: false})
	require.NoError(t, e)
	require.Equal(t, paused.ConnectionDurationMs, invisible.ConnectionDurationMs)
}
func TestPresenceRedisLossRecoveryAndClusterTags(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	o := openPresence(t, s, room.RoomNo, "recovery_open1")
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	require.Error(t, s.RecoverLostSession(ctx, session.SessionNo, "isolated loss recovery test"), "must not delete existing real keys")
	require.NoError(t, s.Redis.Del(ctx, presenceKeys(session.SessionNo)...).Err()) // isolated namespace only, simulates eviction/data loss
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	_, e := s.Open(ctx, presenceViewer(), s.IPKey("a"), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: "recovery_open2"})
	require.ErrorIs(t, e, ErrPresenceCorrupt)
	require.NoError(t, s.RecoverLostSession(ctx, session.SessionNo, "isolated loss recovery test"))
	v, e := s.Open(ctx, presenceViewer(), s.IPKey("a"), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: "recovery_open2"})
	require.NoError(t, e)
	require.True(t, v.DataLost)
	var recovery model.LiveRoomEvent
	require.NoError(t, s.DB.Where("event_type=?", "room.presence.recovered").First(&recovery).Error)
	require.Contains(t, recovery.Payload, "isolated loss recovery test")
	require.Equal(t, "operator", recovery.SourceType)
	require.Equal(t, "1", v.ViewerCount)
	require.Equal(t, "2", v.ViewCount)
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: o.PresenceToken, Sequence: "1"})
	require.Error(t, e)
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	var n int64
	require.NoError(t, s.DB.Model(&model.LiveViewerVisit{}).Count(&n).Error)
	require.EqualValues(t, 2, n)
}
func TestPresenceSourceEventsReferenceCommittedFacts(t *testing.T) {
	s, room, session := presenceFixture(t)
	testPresenceSourceFacts(t, s, room, session)
}
func TestPresenceMySQLSourceEventsReferenceCommittedFacts(t *testing.T) {
	s, room, session := presenceFixture(t)
	presenceMySQL(t, s, room, session)
	testPresenceSourceFacts(t, s, room, session)
}
func testPresenceSourceFacts(t *testing.T, s *PresenceService, room model.LiveRoom, session model.LiveSession) {
	ctx := context.Background()
	_ = openPresence(t, s, room.RoomNo, "event_open_1")
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	require.NoError(t, s.DB.Exec("CREATE TABLE gift_order (order_no VARCHAR(64) PRIMARY KEY,room_no VARCHAR(64),session_no VARCHAR(64),sender_user_id INTEGER)").Error)
	require.NoError(t, s.DB.Exec("CREATE TABLE gift_event_outbox (id INTEGER PRIMARY KEY,event_id TEXT,event_type TEXT,gift_order_no TEXT,room_no TEXT,session_no TEXT,created_at BIGINT)").Error)
	require.NoError(t, s.DB.Exec("INSERT INTO gift_event_outbox VALUES (1,'not_committed','gift.sent','missing',?,?,?)", room.RoomNo, session.SessionNo, time.Now().UnixMilli()).Error)
	require.NoError(t, s.ProjectRoomFacts(ctx))
	var n int64
	require.NoError(t, s.DB.Model(&model.LiveRoomEvent{}).Where("source_type='gift'").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.DB.Exec("INSERT INTO gift_order VALUES ('order_fact',?,?,601)", room.RoomNo, session.SessionNo).Error)
	require.NoError(t, s.DB.Exec("INSERT INTO gift_event_outbox VALUES (2,'real_event','gift.sent','order_fact',?,?,?)", room.RoomNo, session.SessionNo, time.Now().UnixMilli()).Error)
	_, e := s.Social.ChangeRelationship(ctx, 601, socialReq.Target{AccountNo: "host"}, socialService.Follow, "")
	require.NoError(t, e)
	require.NoError(t, s.ProjectRoomFacts(ctx))
	require.NoError(t, s.ProjectRoomFacts(ctx))
	var rows []model.LiveRoomEvent
	require.NoError(t, s.DB.Where("source_type IN ?", []string{"gift", "social"}).Find(&rows).Error)
	require.Len(t, rows, 2)
	raw, _ := json.Marshal(rows)
	require.Contains(t, string(raw), "order_fact")
	require.NotContains(t, string(raw), "amount")
	require.NotContains(t, string(raw), "balance")
}

func TestPresenceConnectionLimitsAndLikeMinutes(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	var first res.PresenceSession
	for n := 0; n < 4; n++ {
		first = openPresence(t, s, room.RoomNo, fmt.Sprintf("limit_open_%d", n))
	}
	_, e := s.Open(ctx, presenceViewer(), s.IPKey("overflow"), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: "limit_open_5"})
	require.ErrorIs(t, e, ErrPresenceLimited)
	tkn, _, e := s.validate(ctx, presenceViewer(), first.PresenceToken, false)
	require.NoError(t, e)
	// A configured per-minute limit applies even when its fixed second has reset.
	s.Config.LikeSecond = 10
	s.Config.LikeMinute = 10
	for n := 1; n <= 10; n++ {
		r := req.PresenceLike{PresenceToken: first.PresenceToken, RequestID: fmt.Sprintf("like_limit_%d", n), Sequence: fmt.Sprint(n), Count: "1"}
		_, e = s.Like(ctx, presenceViewer(), r)
		require.NoError(t, e)
	}
	// Reset only second counters in this isolated fixture; minute quota must still reject.
	p := "live:presence:v1:{like-" + s.mac("like-actor", tkn.Actor) + "}:"
	require.NoError(t, s.Redis.Del(ctx, p+"second").Err())
	_, e = s.Like(ctx, presenceViewer(), req.PresenceLike{PresenceToken: first.PresenceToken, RequestID: "like_limit_11", Sequence: "11", Count: "1"})
	require.ErrorIs(t, e, ErrPresenceLimited)
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	var events []model.LiveRoomEvent
	require.NoError(t, s.DB.Where("event_type=?", "room.viewer.liked").Find(&events).Error)
	require.Len(t, events, 1, "likes are aggregated at checkpoint, not one SQL event per tap")
	require.Contains(t, events[0].Payload, `"acceptedCount":"10"`)
	// Account-wide admission cap holds across rooms. IP is only an auxiliary dimension.
	for n := 0; n < 12; n++ {
		require.NoError(t, s.admit(ctx, presenceToken{Actor: "g:limit-fixture", Connection: fmt.Sprintf("reservation_%s_%d", session.SessionNo, n)}, s.IPKey("reservation-net"), "reserve"))
	}
	require.ErrorIs(t, s.admit(ctx, presenceToken{Actor: "g:limit-fixture", Connection: "overflow-" + session.SessionNo}, s.IPKey("reservation-net"), "reserve"), ErrPresenceLimited)
}
func TestPresenceCursorPrivacyAndPermissionChanges(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	for n := 0; n < 5; n++ {
		id := uint(700 + n)
		account := model.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: fmt.Sprintf("presence%d", id), Username: fmt.Sprintf("presence%d", id), Password: "private", Status: model.LiveAccountStatusNormal, TokenVersion: 1}
		require.NoError(t, s.DB.Create(&account).Error)
		_, e := s.Open(ctx, PresenceIdentity{Actor: uint64(id), TokenVersion: 1}, s.IPKey(fmt.Sprint(id)), req.PresenceOpen{RoomNo: room.RoomNo, RequestID: fmt.Sprintf("page_open_%d", id)})
		require.NoError(t, e)
	}
	got := map[string]bool{}
	cursor := ""
	for n := 0; n < 5; n++ {
		page, e := s.Viewers(ctx, presenceViewer(), req.PresenceQuery{RoomNo: room.RoomNo, Limit: 1, Cursor: cursor}, false)
		require.NoError(t, e)
		require.Len(t, page.List, 1)
		require.False(t, got[page.List[0].User.AccountNo])
		got[page.List[0].User.AccountNo] = true
		cursor = page.NextCursor
	}
	require.Empty(t, cursor)
	require.Len(t, got, 5)
	require.NoError(t, s.DB.Create(&socialModel.SocialBlock{UserID: 501, TargetUserID: 700}).Error)
	page, e := s.Viewers(ctx, presenceViewer(), req.PresenceQuery{RoomNo: room.RoomNo, Limit: 100}, false)
	require.NoError(t, e)
	require.Len(t, page.List, 4)
	o := openPresence(t, s, room.RoomNo, "permission_open")
	require.NoError(t, s.Redis.HSet(ctx, presenceKeys(session.SessionNo)[1], o.ConnectionNo, mutateMeta(t, s, session.SessionNo, o.ConnectionNo, func(m map[string]any) { m["authAt"] = 0 })).Err())
	require.NoError(t, s.DB.Model(&room).Update("visibility", model.LiveRoomVisibilityPrivate).Error)
	_, e = s.Heartbeat(ctx, presenceViewer(), req.PresenceHeartbeat{PresenceToken: o.PresenceToken, Sequence: "1"})
	require.ErrorIs(t, e, ErrPresenceAccess)
}
func TestPresenceCheckpointVersionsNeverRegress(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	o := openPresence(t, s, room.RoomNo, "version_open1")
	_, e := s.Leave(ctx, presenceViewer(), req.PresenceLeave{PresenceToken: o.PresenceToken, Sequence: "1", Reason: "user_leave"})
	require.NoError(t, e)
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	var v model.LiveViewerVisit
	require.NoError(t, s.DB.First(&v, "visit_no=?", o.ConnectionNo).Error)
	require.Equal(t, "closed", v.Status)
	old := v
	old.Revision = 1
	old.Status = "online"
	old.LeftAt = 0
	require.NoError(t, persistPresenceVisits(s.DB, []model.LiveViewerVisit{old}))
	require.NoError(t, s.DB.First(&v, "visit_no=?", o.ConnectionNo).Error)
	require.Equal(t, "closed", v.Status)
	require.Positive(t, v.LeftAt)
}

func TestPresenceFullEventQueueDrainsBeforeCleanup(t *testing.T) {
	s, room, session := presenceFixture(t)
	ctx := context.Background()
	_ = openPresence(t, s, room.RoomNo, "fullqueue_open")
	key := presenceKeys(session.SessionNo)[8]
	// This script writes only into the test-owned session; no scans or global reset.
	require.NoError(t, s.Redis.Eval(ctx, `for i=1,20000 do redis.call('XADD',KEYS[1],'*','type','room.viewer.entered','actor','u:601','connection','fixture','at',ARGV[1],'payload','{}') end;return 1`, []string{key}, time.Now().UnixMilli()).Err())
	require.NoError(t, s.DB.Model(&session).Updates(map[string]any{"status": model.LiveSessionEnded, "ended_at": time.Now().UnixMilli()}).Error)
	before := s.Redis.XLen(ctx, key).Val()
	require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	require.Less(t, s.Redis.XLen(ctx, key).Val(), before, "a full queue must not prevent the worker from draining events")
	for n := 0; n < 3; n++ {
		require.NoError(t, s.FlushSession(ctx, session.SessionNo))
	}
	require.Zero(t, s.Redis.ZCard(ctx, presenceKeys(session.SessionNo)[2]).Val(), "cleanup resumes after capacity is available")
	// Worker cancellation must also wait for its independent fact projector to exit.
	run, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { s.Run(run); close(done) }()
	stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}
}
