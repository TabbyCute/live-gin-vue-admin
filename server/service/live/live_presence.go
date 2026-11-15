package live

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"tb_live_module/config"
	model "tb_live_module/model/live"
	req "tb_live_module/model/live/request"
	res "tb_live_module/model/live/response"
	socialModel "tb_live_module/model/social"
	socialReq "tb_live_module/model/social/request"
	socialService "tb_live_module/service/social"
)

//go:embed presence_scripts/state.lua
var presenceStateLua string

//go:embed presence_scripts/admission.lua
var presenceAdmissionLua string

const presenceMaxInteger uint64 = 9007199254740991

var presenceNumber = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var presenceDecimal = regexp.MustCompile(`^[1-9][0-9]{0,15}$`)

type PresenceError struct {
	Code    int
	Message string
}

func (e *PresenceError) Error() string { return e.Message }

var (
	ErrPresenceInvalid     = &PresenceError{49501, "观众请求参数无效"}
	ErrPresenceExpired     = &PresenceError{49502, "观众会话已失效，请重新进入"}
	ErrPresenceAccess      = &PresenceError{49503, "无权访问此直播场次"}
	ErrPresenceSequence    = &PresenceError{49504, "请求序号已过期或幂等参数冲突"}
	ErrPresenceLimited     = &PresenceError{49505, "操作过于频繁，请稍后重试"}
	ErrPresenceUnavailable = &PresenceError{49506, "观众统计暂不可用，请稍后重试"}
	ErrPresenceCapacity    = &PresenceError{49507, "观众统计繁忙，请稍后重试"}
	ErrPresenceCorrupt     = &PresenceError{49508, "观众统计状态异常，正在等待恢复"}
	ErrPresenceStatsOwned  = errors.New("场次观众统计由presence汇总，外部统计写入已停用")
)

type PresenceService struct {
	DB          *gorm.DB
	Redis       redis.UniversalClient
	Config      config.LivePresence
	Key         []byte
	Log         *zap.Logger
	Social      *socialService.Service
	Failures    atomic.Uint64
	nextSession atomic.Uint64
}

var DefaultPresence = &PresenceService{}

type PresenceIdentity struct {
	Actor        uint64
	Visitor      string
	TokenVersion uint64
}

func (i PresenceIdentity) key() string {
	if i.Actor > 0 {
		return "u:" + strconv.FormatUint(i.Actor, 10)
	}
	return "g:" + i.Visitor
}
func (s *PresenceService) ready() error {
	if s.DB == nil || s.Redis == nil || len(s.Key) < 32 {
		return ErrPresenceUnavailable
	}
	return nil
}
func (s *PresenceService) mac(purpose, value string) string {
	h := hmac.New(sha256.New, s.Key)
	h.Write([]byte(purpose + "\x00" + value))
	return hex.EncodeToString(h.Sum(nil))
}
func (s *PresenceService) IPKey(ip string) string { return s.mac("network", ip) }
func presenceRandom() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic("secure random source unavailable")
	}
	return hex.EncodeToString(b)
}
func presenceInt(s string, max uint64) (uint64, error) {
	if !presenceDecimal.MatchString(s) {
		return 0, ErrPresenceInvalid
	}
	n, e := strconv.ParseUint(s, 10, 64)
	if e != nil || n > max {
		return 0, ErrPresenceInvalid
	}
	return n, nil
}
func presenceUint(n uint64) string { return strconv.FormatUint(n, 10) }

type presenceToken struct {
	Room       string `json:"r"`
	Session    string `json:"s"`
	Connection string `json:"c"`
	Actor      string `json:"a"`
	Epoch      string `json:"e"`
	Created    int64  `json:"t"`
}

func (s *PresenceService) sign(purpose string, v any) string {
	b, _ := json.Marshal(v)
	p := base64.RawURLEncoding.EncodeToString(b)
	return p + "." + s.mac(purpose, p)
}
func (s *PresenceService) verify(purpose, token string, v any) error {
	if len(token) > 2048 {
		return ErrPresenceInvalid
	}
	p := strings.Split(token, ".")
	if len(p) != 2 || !hmac.Equal([]byte(s.mac(purpose, p[0])), []byte(p[1])) {
		return ErrPresenceExpired
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(p[0])
	if e != nil || json.Unmarshal(b, v) != nil {
		return ErrPresenceExpired
	}
	return nil
}
func (s *PresenceService) NewGuest() (res.PresenceGuest, error) {
	if e := s.ready(); e != nil {
		return res.PresenceGuest{}, e
	}
	v := struct {
		ID      string `json:"id"`
		Expires int64  `json:"expires"`
	}{presenceRandom(), time.Now().Add(24 * time.Hour).UnixMilli()}
	return res.PresenceGuest{GuestToken: s.sign("guest", v), ExpiresInSeconds: 86400}, nil
}
func (s *PresenceService) GuestIdentity(token string) (PresenceIdentity, error) {
	var v struct {
		ID      string `json:"id"`
		Expires int64  `json:"expires"`
	}
	e := s.verify("guest", token, &v)
	if e != nil || len(v.ID) != 32 || !presenceNumber.MatchString(v.ID) || v.Expires <= time.Now().UnixMilli() || v.Expires > time.Now().Add(25*time.Hour).UnixMilli() {
		return PresenceIdentity{}, ErrPresenceExpired
	}
	return PresenceIdentity{Visitor: v.ID}, nil
}
func (s *PresenceService) parseToken(i PresenceIdentity, token string) (presenceToken, error) {
	var t presenceToken
	if e := s.verify("connection", token, &t); e != nil {
		return t, e
	}
	if t.Actor != i.key() || !presenceNumber.MatchString(t.Room) || !presenceNumber.MatchString(t.Session) || len(t.Epoch) != 32 || len(t.Connection) != 67 || !strings.HasPrefix(t.Connection, "pc_") || t.Created <= 0 || time.Now().UnixMilli()-t.Created > 7*86400000 {
		return t, ErrPresenceExpired
	}
	return t, nil
}
func presenceKeys(session string) []string {
	p := "live:presence:v1:{" + session + "}:"
	out := []string{}
	for _, k := range []string{"stats", "connections", "expiry", "members", "guests", "actors", "seen", "pending", "events", "active", "likes"} {
		out = append(out, p+k)
	}
	return out
}
func presenceRedisContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Second)
}
func (s *PresenceService) run(ctx context.Context, t presenceToken, a map[string]any) ([]any, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	a["epoch"] = t.Epoch
	a["connection"] = t.Connection
	a["actor"] = t.Actor
	c := s.Config.Bounds()
	a["ttl"] = c.TTLSeconds * 1000
	a["maxDelta"] = c.HeartbeatSeconds * 2000
	b, e := json.Marshal(a)
	if e != nil {
		return nil, ErrPresenceInvalid
	}
	rc, cancel := presenceRedisContext(ctx)
	defer cancel()
	v, e := s.Redis.Eval(rc, presenceStateLua, presenceKeys(t.Session), string(b)).Slice()
	if e != nil {
		s.failure("redis_operation", t.Session)
		return nil, ErrPresenceUnavailable
	}
	if len(v) == 0 {
		return nil, ErrPresenceCorrupt
	}
	switch v[0] {
	case "ok":
		return v, nil
	case "expired":
		return nil, ErrPresenceExpired
	case "ended":
		return nil, ErrPresenceAccess
	case "limited":
		return nil, ErrPresenceLimited
	case "capacity", "busy":
		return nil, ErrPresenceCapacity
	case "sequence", "conflict":
		return nil, ErrPresenceSequence
	default:
		s.failure("state_corrupt", t.Session)
		return nil, ErrPresenceCorrupt
	}
}
func (s *PresenceService) failure(code, session string) {
	n := s.Failures.Add(1)
	if s.Log != nil && (n == 1 || n%100 == 0) {
		s.Log.Warn("live presence operation failed", zap.String("code", code), zap.String("sessionNo", session), zap.Uint64("failureCount", n))
	}
}

type presenceMeta struct {
	LikeAt         int64  `json:"likeAt"`
	Likes          uint64 `json:"likes"`
	Actor          string `json:"actor"`
	Joined         int64  `json:"joined"`
	Last           int64  `json:"last"`
	Seq            string `json:"seq"`
	Duration       uint64 `json:"duration"`
	Closed         bool   `json:"closed"`
	Reason         string `json:"reason"`
	Left           int64  `json:"left"`
	Revision       uint64 `json:"revision"`
	AuthAt         int64  `json:"authAt"`
	IP             string `json:"ip"`
	History        string `json:"history"`
	HistoryVersion uint64 `json:"historyVersion"`
	HistoryEpoch   uint64 `json:"historyEpoch"`
}
type presenceActor struct {
	Connections json.RawMessage `json:"connections"`
	Duration    uint64          `json:"duration"`
	LastActive  int64           `json:"lastActive"`
	Activity    string          `json:"activity"`
}
type presenceResult struct {
	ViewerDuration string `json:"viewerSessionDurationMs"`
	res.PresenceStats
	Meta     presenceMeta `json:"meta"`
	Accepted string       `json:"acceptedCount"`
	Closed   bool         `json:"closed"`
}

func (s *PresenceService) sessionResult(t presenceToken, v []any) (res.PresenceSession, error) {
	out := res.PresenceSession{}
	if len(v) < 2 {
		return out, ErrPresenceCorrupt
	}
	var raw presenceResult
	if json.Unmarshal([]byte(fmt.Sprint(v[1])), &raw) != nil {
		return out, ErrPresenceCorrupt
	}
	out.PresenceStats = raw.PresenceStats
	out.RoomNo = t.Room
	out.SessionNo = t.Session
	out.WorkerHealthy = time.Now().UnixMilli()-out.CheckpointAt < 60000
	out.ConnectionDurationMs = presenceUint(raw.Meta.Duration)
	out.Closed = raw.Meta.Closed || raw.Closed
	out.AcceptedCount = raw.Accepted
	out.ViewerSessionDurationMs = raw.ViewerDuration
	c := s.Config.Bounds()
	out.HeartbeatIntervalSeconds = c.HeartbeatSeconds
	out.ExpiresInSeconds = c.TTLSeconds
	return out, nil
}
func (s *PresenceService) admit(ctx context.Context, t presenceToken, ip, op string) error {
	c := s.Config.Bounds()
	rc, cancel := presenceRedisContext(ctx)
	defer cancel()
	p := "live:presence:v1:{admission}:"
	n, e := s.Redis.Eval(rc, presenceAdmissionLua, []string{p + "actor:" + s.mac("actor", t.Actor), p + "ip:" + ip, p + "global"}, op, t.Connection, c.TTLSeconds*1000, 12, c.IPConnections, c.GlobalConnections).Int()
	if e != nil {
		return ErrPresenceUnavailable
	}
	if n == -2 {
		return ErrPresenceCorrupt
	}
	if n < 0 {
		return ErrPresenceLimited
	}
	return nil
}
func (s *PresenceService) Rate(ctx context.Context, scope, subject string, limit int) error {
	if e := s.ready(); e != nil {
		return e
	}
	rc, cancel := presenceRedisContext(ctx)
	defer cancel()
	n, e := s.Redis.Eval(rc, `local n=redis.call('INCR',KEYS[1]);if n==1 then redis.call('EXPIRE',KEYS[1],60)end;return n`, []string{"live:presence:v1:rate:" + scope + ":" + s.mac("rate", subject)}).Int()
	if e != nil {
		return ErrPresenceUnavailable
	}
	if n > limit {
		return ErrPresenceLimited
	}
	return nil
}
func (s *PresenceService) access(ctx context.Context, i PresenceIdentity, room, session string) error {
	if i.Actor == 0 && i.Visitor == "" {
		return ErrPresenceAccess
	}
	if i.Actor > 0 {
		var a struct {
			Status       uint8
			TokenVersion uint64
		}
		e := s.DB.WithContext(ctx).Model(&model.LiveAccount{}).Select("status,token_version").Where("id=?", i.Actor).Take(&a).Error
		if e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrPresenceAccess
			}
			return ErrPresenceUnavailable
		}
		if a.Status != model.LiveAccountStatusNormal || a.TokenVersion != i.TokenVersion {
			return ErrPresenceAccess
		}
	}
	rows, e := (&LiveAccessService{}).BatchCheckLiveAccess(ctx, s.DB, i.Actor, []string{room}, true)
	if e != nil {
		return ErrPresenceUnavailable
	}
	r, ok := rows[room]
	if !ok || (session != "" && r.SessionNo != session) {
		return ErrPresenceAccess
	}
	return nil
}
func (s *PresenceService) ensure(ctx context.Context, room, session string) (model.LivePresenceCheckpoint, error) {
	var cp model.LivePresenceCheckpoint
	var ls model.LiveSession
	if e := s.DB.WithContext(ctx).Where("session_no=?", session).First(&ls).Error; e != nil {
		return cp, ErrPresenceUnavailable
	}
	if ls.Status != model.LiveSessionLiving {
		return cp, ErrPresenceAccess
	}
	cp = model.LivePresenceCheckpoint{SessionNo: session, RoomNo: room, Epoch: presenceRandom(), ViewCount: ls.ViewCount, ViewerCount: ls.ViewerCount, PeakOnlineCount: uint64(ls.PeakOnlineCount), LikeCount: ls.LikeCount, TotalWatchDurationMs: ls.TotalWatchDurationMs, LastError: "initializing"}
	// Durable identity precedes Redis initialization. A DB commit failure can never
	// leave a Redis epoch whose checkpoint row was rolled back.
	if e := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&cp).Error; e != nil {
		return cp, ErrPresenceUnavailable
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("session_no=?", session).First(&cp).Error; e != nil {
			return e
		}
		if cp.FinalizedAt > 0 || (cp.DataLost && cp.LastError == "redis_state_lost") {
			return ErrPresenceCorrupt
		}
		t := presenceToken{Session: session, Epoch: cp.Epoch}
		rc, cancel := presenceRedisContext(ctx)
		exists, e := s.Redis.Exists(rc, presenceKeys(session)[0]).Result()
		cancel()
		if e != nil {
			return ErrPresenceUnavailable
		}
		if exists == 0 && cp.LastError != "initializing" {
			return ErrPresenceCorrupt
		}
		if exists == 0 {
			_, e = s.run(ctx, t, map[string]any{"op": "init", "room": room, "view": presenceUint(cp.ViewCount), "viewer": presenceUint(cp.ViewerCount), "peak": presenceUint(cp.PeakOnlineCount), "likes": presenceUint(cp.LikeCount), "watch": presenceUint(cp.TotalWatchDurationMs), "lost": map[bool]string{true: "1", false: "0"}[cp.DataLost]})
			if e != nil {
				return e
			}
		}
		if cp.LastError == "initializing" {
			if cp.DataLost {
				var after string
				for {
					var rows []model.LiveViewerSession
					if e := tx.Where("session_no=? AND visitor_key>?", session, after).Order("visitor_key").Limit(100).Find(&rows).Error; e != nil {
						return e
					}
					if len(rows) == 0 {
						break
					}
					members := []any{}
					for _, r := range rows {
						members = append(members, r.VisitorKey)
					}
					rc, cancel := presenceRedisContext(ctx)
					pipe := s.Redis.Pipeline()
					pipe.SAdd(rc, presenceKeys(session)[6], members...)
					for _, r := range rows {
						b, _ := json.Marshal(map[string]any{"connections": []string{}, "timeline": time.Now().UnixMilli(), "duration": r.WatchDurationMs, "lastActive": r.LastSeenAt, "activity": "heartbeat", "index": ""})
						pipe.HSet(rc, presenceKeys(session)[5], r.VisitorKey, string(b))
					}
					pipe.Expire(rc, presenceKeys(session)[6], 7*24*time.Hour)
					pipe.Expire(rc, presenceKeys(session)[5], 7*24*time.Hour)
					_, e := pipe.Exec(rc)
					cancel()
					if e != nil {
						return ErrPresenceUnavailable
					}
					after = rows[len(rows)-1].VisitorKey
				}
			}
			return tx.Model(&cp).Update("last_error", "").Error
		}
		return nil
	})
	if err != nil {
		var b *PresenceError
		if errors.As(err, &b) {
			return cp, err
		}
		return cp, ErrPresenceUnavailable
	}
	return cp, nil
}
func (s *PresenceService) Open(ctx context.Context, i PresenceIdentity, ip string, r req.PresenceOpen) (res.PresenceSession, error) {
	out := res.PresenceSession{}
	if !presenceNumber.MatchString(r.RoomNo) || !presenceNumber.MatchString(r.RequestID) || len(r.RequestID) < 8 {
		return out, ErrPresenceInvalid
	}
	if e := s.ready(); e != nil {
		return out, e
	}
	if e := s.access(ctx, i, r.RoomNo, ""); e != nil {
		return out, e
	}
	var ls model.LiveSession
	e := s.DB.WithContext(ctx).Table("live_session s").Select("s.*").Joins("JOIN live_room r ON r.current_session_id=s.id").Where("r.room_no=? AND s.status=? AND s.deleted_at IS NULL", r.RoomNo, model.LiveSessionLiving).Take(&ls).Error
	if e != nil {
		return out, ErrPresenceAccess
	}
	cp, e := s.ensure(ctx, r.RoomNo, ls.SessionNo)
	if e != nil {
		return out, e
	}
	t := presenceToken{Room: r.RoomNo, Session: ls.SessionNo, Actor: i.key(), Epoch: cp.Epoch}
	t.Connection = "pc_" + s.mac("connection-id", t.Session+":"+t.Epoch+":"+t.Actor+":"+r.RequestID)
	// Reserve globally before per-room admission. Failed reservations expire automatically;
	// no compensation may remove a concurrent successful retry of the same request.
	if e = s.admit(ctx, t, ip, "reserve"); e != nil {
		return out, e
	}
	var history string
	var historyVersion, historyEpoch uint64
	// History is one recent-resource row, not another online/duration counter. Request IDs
	// are deterministic, so retrying a successful open does not create a second history row.
	rc, cancel := presenceRedisContext(ctx)
	raw, e := s.Redis.HGet(rc, presenceKeys(t.Session)[1], t.Connection).Result()
	cancel()
	if e != nil && !errors.Is(e, redis.Nil) {
		return out, ErrPresenceUnavailable
	}
	if raw == "" && i.Actor > 0 && s.Social != nil {
		h, err := s.Social.OpenHistory(ctx, i.Actor, socialReq.HistoryOpen{Resource: socialReq.Resource{ResourceType: socialModel.ResourceLiveRoom, ResourceID: r.RoomNo}, RequestID: t.Connection[3:]})
		if err != nil {
			return out, ErrPresenceUnavailable
		}
		history, historyVersion, historyEpoch = h.SessionToken, h.SessionVersion, h.Epoch
	}
	v, e := s.run(ctx, t, map[string]any{"op": "open", "roomLimit": s.Config.Bounds().RoomConnections, "ip": ip, "history": history, "historyVersion": historyVersion, "historyEpoch": historyEpoch})
	if e != nil {
		return out, e
	}
	out, e = s.sessionResult(t, v)
	if e != nil {
		return out, e
	}
	var parsed presenceResult
	_ = json.Unmarshal([]byte(fmt.Sprint(v[1])), &parsed)
	t.Created = parsed.Meta.Joined
	out.PresenceToken = s.sign("connection", t)
	out.ConnectionNo = t.Connection
	return out, nil
}
func (s *PresenceService) validate(ctx context.Context, i PresenceIdentity, token string, recheck bool) (presenceToken, presenceMeta, error) {
	t, e := s.parseToken(i, token)
	if e != nil {
		return t, presenceMeta{}, e
	}
	if e = s.ready(); e != nil {
		return t, presenceMeta{}, e
	}
	rc, cancel := presenceRedisContext(ctx)
	p := s.Redis.Pipeline()
	mcmd := p.HGet(rc, presenceKeys(t.Session)[1], t.Connection)
	health := p.HGet(rc, presenceKeys(t.Session)[0], "committed")
	_, e = p.Exec(rc)
	cancel()
	if e != nil {
		if errors.Is(e, redis.Nil) {
			return t, presenceMeta{}, ErrPresenceExpired
		}
		return t, presenceMeta{}, ErrPresenceUnavailable
	}
	var m presenceMeta
	if json.Unmarshal([]byte(mcmd.Val()), &m) != nil || m.Actor != t.Actor || m.Joined != t.Created {
		return t, m, ErrPresenceCorrupt
	}
	if m.Closed && !recheck {
		return t, m, nil
	}
	at, _ := strconv.ParseInt(health.Val(), 10, 64)
	now := time.Now().UnixMilli()
	if now-at > 60000 {
		return t, m, ErrPresenceCapacity
	}
	if recheck && (now-m.AuthAt >= int64(s.Config.Bounds().RecheckSeconds)*1000 || now-at > 10000) {
		if e = s.access(ctx, i, t.Room, t.Session); e != nil {
			return t, m, e
		}
		if _, e = s.run(ctx, t, map[string]any{"op": "auth"}); e != nil {
			return t, m, e
		}
	}
	return t, m, nil
}
func (s *PresenceService) Heartbeat(ctx context.Context, i PresenceIdentity, r req.PresenceHeartbeat) (res.PresenceSession, error) {
	if _, e := presenceInt(r.Sequence, presenceMaxInteger); e != nil {
		return res.PresenceSession{}, e
	}
	t, m, e := s.validate(ctx, i, r.PresenceToken, true)
	if e != nil {
		return res.PresenceSession{}, e
	}
	if e = s.admit(ctx, t, m.IP, "reserve"); e != nil {
		return res.PresenceSession{}, e
	}
	v, e := s.run(ctx, t, map[string]any{"op": "heartbeat", "sequence": r.Sequence, "playing": r.Playing, "visible": r.Visible})
	if e != nil {
		return res.PresenceSession{}, e
	}
	return s.sessionResult(t, v)
}
func (s *PresenceService) Leave(ctx context.Context, i PresenceIdentity, r req.PresenceLeave) (res.PresenceSession, error) {
	if _, e := presenceInt(r.Sequence, presenceMaxInteger); e != nil {
		return res.PresenceSession{}, e
	}
	switch r.Reason {
	case "user_leave", "room_change", "account_change", "page_hidden", "page_close":
	default:
		return res.PresenceSession{}, ErrPresenceInvalid
	}
	t, m, e := s.validate(ctx, i, r.PresenceToken, false)
	if e != nil {
		return res.PresenceSession{}, e
	}
	v, e := s.run(ctx, t, map[string]any{"op": "leave", "sequence": r.Sequence, "reason": r.Reason})
	if e != nil {
		return res.PresenceSession{}, e
	}
	// Release is advisory; TTL remains the correctness fallback when Redis fails here.
	_ = s.admit(ctx, t, m.IP, "release")
	return s.sessionResult(t, v)
}
func (s *PresenceService) Like(ctx context.Context, i PresenceIdentity, r req.PresenceLike) (res.PresenceSession, error) {
	if i.Actor == 0 {
		return res.PresenceSession{}, ErrPresenceAccess
	}
	if !presenceNumber.MatchString(r.RequestID) || len(r.RequestID) < 8 {
		return res.PresenceSession{}, ErrPresenceInvalid
	}
	if _, e := presenceInt(r.Sequence, presenceMaxInteger); e != nil {
		return res.PresenceSession{}, e
	}
	if _, e := presenceInt(r.Count, 20); e != nil {
		return res.PresenceSession{}, e
	}
	t, _, e := s.validate(ctx, i, r.PresenceToken, true)
	if e != nil {
		return res.PresenceSession{}, e
	}
	if e = s.reserveLikes(ctx, t, r); e != nil {
		return res.PresenceSession{}, e
	}
	c := s.Config.Bounds()
	v, e := s.run(ctx, t, map[string]any{"op": "like", "sequence": r.Sequence, "request": r.RequestID, "count": r.Count, "likeSecond": c.LikeSecond, "likeMinute": c.LikeMinute})
	if e != nil {
		return res.PresenceSession{}, e
	}
	return s.sessionResult(t, v)
}

// reserveLikes enforces account-wide budgets across rooms/instances. Reservations
// are conservative: a later room failure may consume budget, never manufacture likes.
func (s *PresenceService) reserveLikes(ctx context.Context, t presenceToken, r req.PresenceLike) error {
	c := s.Config.Bounds()
	p := "live:presence:v1:{like-" + s.mac("like-actor", t.Actor) + "}:"
	rc, cancel := presenceRedisContext(ctx)
	defer cancel()
	result, e := s.Redis.Eval(rc, `local old=redis.call('GET',KEYS[3]);if old then if old==ARGV[1] then return 1 else return -2 end end
local t=redis.call('TIME');local sec=t[1];local minute=math.floor(sec/60);local n=tonumber(ARGV[1]);local s=redis.call('HMGET',KEYS[1],'window','count');local m=redis.call('HMGET',KEYS[2],'window','count');local sn=0;local mn=0;if s[1]==tostring(sec) then sn=tonumber(s[2])end;if m[1]==tostring(minute) then mn=tonumber(m[2])end;if sn+n>tonumber(ARGV[2]) or mn+n>tonumber(ARGV[3])then return -1 end;redis.call('HSET',KEYS[1],'window',sec,'count',sn+n);redis.call('EXPIRE',KEYS[1],2);redis.call('HSET',KEYS[2],'window',minute,'count',mn+n);redis.call('EXPIRE',KEYS[2],120);redis.call('SET',KEYS[3],ARGV[1],'EX',120);return 1`, []string{p + "second", p + "minute", p + "receipt:" + s.mac("like-receipt", t.Connection+":"+r.RequestID+":"+r.Sequence)}, r.Count, c.LikeSecond, c.LikeMinute).Int()
	if e != nil {
		return ErrPresenceUnavailable
	}
	if result == -2 {
		return ErrPresenceSequence
	}
	if result < 0 {
		return ErrPresenceLimited
	}
	return nil
}
