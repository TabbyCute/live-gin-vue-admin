package live

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	socialService "tb_live_module/service/social"
)

var ErrLiveAccessDenied = errors.New("无权访问当前直播间")
var ErrMediaGatewayRequired = errors.New("受保护直播尚未配置可信媒体鉴权网关")

type LiveAccessService struct{}
type LiveAccessSnapshot struct {
	RoomNo     string
	AnchorNo   string
	SessionNo  string
	StreamName string
	Title      string
	CoverURL   string
	UserID     uint64
	Visibility uint8
	IsLive     bool
}

func (s *LiveAccessService) CheckLiveAccess(ctx context.Context, actor uint64, roomNo string, playing bool) (LiveAccessSnapshot, error) {
	rows, err := s.BatchCheckLiveAccess(ctx, global.GVA_DB, actor, []string{roomNo}, playing)
	if err != nil {
		return LiveAccessSnapshot{}, err
	}
	r, ok := rows[roomNo]
	if !ok {
		socialService.Default.Metrics.AccessDenied.Add(1)
		return r, ErrLiveAccessDenied
	}
	return r, nil
}
func (s *LiveAccessService) BatchCheckLiveAccess(ctx context.Context, db *gorm.DB, actor uint64, nos []string, playing bool) (map[string]LiveAccessSnapshot, error) {
	out := map[string]LiveAccessSnapshot{}
	if len(nos) == 0 {
		return out, nil
	}
	if len(nos) > 100 || db == nil {
		return nil, ErrLiveAccessDenied
	}
	if actor > 0 {
		if err := (SocialDirectory{}).Validate(ctx, db, []uint64{actor}); err != nil {
			return nil, err
		}
	}
	var rows []LiveAccessSnapshot
	if err := liveAccessQuery(ctx, db, actor, playing).Where("r.room_no IN ?", nos).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.RoomNo] = r
	}
	return out, nil
}

// liveAccessQuery 资料、历史、在线列表和播放鉴权共用同一查询规则。
// 房间、场次、账号与社交权限在单条 SQL 中读取，避免跨语句授权快照不一致。
func liveAccessQuery(ctx context.Context, db *gorm.DB, actor uint64, playing bool) *gorm.DB {
	living := fmt.Sprintf("r.live_status = %d AND s.status = %d AND s.id = r.current_session_id AND s.room_id = r.id AND s.anchor_id = r.anchor_id", liveModel.LiveRoomLiving, liveModel.LiveSessionLiving)
	sessionJoin := "LEFT JOIN live_session s ON s.id=r.current_session_id AND s.deleted_at IS NULL"
	if db.Dialector.Name() == "mysql" {
		// 只按当前场次主键读取；禁止优化器从主播索引遍历全部历史场次。
		sessionJoin = "LEFT JOIN live_session s FORCE INDEX (PRIMARY) ON s.id=r.current_session_id AND s.deleted_at IS NULL"
	}
	query := db.WithContext(ctx).Table("live_room r").
		Select("r.room_no,r.title,r.cover_url,r.stream_name,r.visibility,a.user_id,a.anchor_no,COALESCE(s.session_no,'') AS session_no,COALESCE(("+living+"),false) AS is_live").
		Joins("JOIN live_anchor a ON a.id=r.anchor_id AND a.deleted_at IS NULL").
		Joins("JOIN live_account u ON u.id=a.user_id AND u.deleted_at IS NULL").
		Joins(sessionJoin).
		Where("r.deleted_at IS NULL AND r.status = ? AND a.status = ? AND a.apply_status = ? AND a.live_permission = ? AND a.risk_level < ? AND u.status = ?", liveModel.LiveRoomStatusNormal, liveModel.AnchorStatusNormal, liveModel.AnchorApplyStatusApproved, liveModel.AnchorPermissionEnabled, liveModel.AnchorRiskHigh, liveModel.LiveAccountStatusNormal).
		Scopes(socialService.VisibleOwnerScope(actor, "a.user_id", "r.visibility", liveModel.LiveRoomVisibilityPublic, liveModel.LiveRoomVisibilityFollowers, liveModel.LiveRoomVisibilityPrivate))
	if certificationRequiredForLive {
		query = query.Where("a.cert_status = ?", liveModel.AnchorCertStatusApproved)
	}
	if playing {
		query = query.Where(living)
	}
	return query
}

func requireProtectedMedia(visibility *uint8) error {
	if visibility == nil {
		return ErrLiveAccessDenied
	}
	if *visibility == liveModel.LiveRoomVisibilityPublic {
		return nil
	}
	if !mediaGatewayReady() {
		return ErrMediaGatewayRequired
	}
	return nil
}
func mediaGatewayReady() bool {
	c := global.GVA_CONFIG.Social
	u, err := url.Parse(c.PlaybackBaseURL)
	return c.ProtectedPlaybackEnabled && c.MediaGatewayVerified && len(c.PlaybackSigningKey) >= 32 && err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

const playbackTTL = 30 * time.Second

type PlaybackAuthorization struct {
	PlayURL               string `json:"playUrl"`
	ExpiresAt             int64  `json:"expiresAt"`
	SessionNo             string `json:"sessionNo"`
	AuthorizationRequired bool   `json:"authorizationRequired"`
}
type playbackClaims struct {
	UserID     uint64 `json:"u"`
	RoomNo     string `json:"r"`
	SessionNo  string `json:"s"`
	StreamName string `json:"n"`
	ExpiresAt  int64  `json:"e"`
	IssuedAt   int64  `json:"i"`
	Version    uint8  `json:"v"`
}

func (s *LiveAccessService) AuthorizePlayback(ctx context.Context, actor uint64, roomNo string) (PlaybackAuthorization, error) {
	if actor == 0 || !mediaGatewayReady() {
		return PlaybackAuthorization{}, ErrMediaGatewayRequired
	}
	r, err := s.CheckLiveAccess(ctx, actor, roomNo, true)
	if err != nil {
		return PlaybackAuthorization{}, err
	}
	now := time.Now()
	c := playbackClaims{UserID: actor, RoomNo: r.RoomNo, SessionNo: r.SessionNo, StreamName: r.StreamName, ExpiresAt: now.Add(playbackTTL).UnixMilli(), IssuedAt: now.UnixMilli(), Version: 1}
	b, _ := json.Marshal(c)
	payload := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(global.GVA_CONFIG.Social.PlaybackSigningKey))
	mac.Write([]byte(payload))
	token := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	u, _ := url.Parse(global.GVA_CONFIG.Social.PlaybackBaseURL)
	u.Path = strings.TrimRight(u.Path, "/") + "/" + global.GVA_CONFIG.Live.SRS.App + "/" + r.StreamName + ".m3u8"
	q := u.Query()
	q.Set("playbackToken", token)
	u.RawQuery = q.Encode()
	return PlaybackAuthorization{PlayURL: u.String(), ExpiresAt: c.ExpiresAt, SessionNo: r.SessionNo, AuthorizationRequired: true}, nil
}

var mediaPath = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:-[0-9]+)?\.(?:m3u8|ts)$`)

// ValidatePlayback 供可信媒体代理逐请求调用。Bearer必须独立校验，不能仅凭URL签名鉴权。
func (s *LiveAccessService) ValidatePlayback(ctx context.Context, actor uint64, token, requestPath string) error {
	reject := func() error { socialService.Default.Metrics.CredentialRejected.Add(1); return ErrLiveAccessDenied }
	if actor == 0 || !mediaGatewayReady() || len(token) > 2048 {
		return reject()
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return reject()
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return reject()
	}
	mac := hmac.New(sha256.New, []byte(global.GVA_CONFIG.Social.PlaybackSigningKey))
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return reject()
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return reject()
	}
	var c playbackClaims
	if json.Unmarshal(raw, &c) != nil {
		return reject()
	}
	now := time.Now().UnixMilli()
	if c.Version != 1 || c.UserID != actor || c.ExpiresAt <= now || c.IssuedAt > now+1000 || c.ExpiresAt-c.IssuedAt != playbackTTL.Milliseconds() {
		return reject()
	}
	// 只允许同一流的playlist/segment/FLV；禁止目录跳转、跨流、编码路径与未知资源。
	prefix := "/" + global.GVA_CONFIG.Live.SRS.App + "/"
	if !strings.HasPrefix(requestPath, prefix) {
		return reject()
	}
	name := strings.TrimPrefix(requestPath, prefix)
	if !mediaPath.MatchString(name) || !(name == c.StreamName+".m3u8" || (strings.HasPrefix(name, c.StreamName+"-") && strings.HasSuffix(name, ".ts"))) {
		return reject()
	}
	r, err := s.CheckLiveAccess(ctx, actor, c.RoomNo, true)
	if err != nil {
		return reject()
	}
	if r.SessionNo != c.SessionNo || r.StreamName != c.StreamName {
		return reject()
	}
	return nil
}
