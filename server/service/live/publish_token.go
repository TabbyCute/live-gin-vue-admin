package live

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"path"
	"strings"
	"time"

	"tb_live_module/config"
	liveModel "tb_live_module/model/live"
)

const (
	livePublishTokenVersion    = 1
	livePublishTokenScope      = "live:publish"
	livePublishTokenPrefix     = "v1_"
	livePublishTokenClockSkew  = 60
	livePublishTokenIDByteSize = 8
)

// livePublishTokenClaims 是 pt 解密后的 V1 载荷。
// 时间字段统一使用 Unix 秒；token_id 是每次签发生成的 8 字节随机数十六进制文本。
type livePublishTokenClaims struct {
	Version           uint8  `json:"ver"`
	Scope             string `json:"scope"`
	TokenID           string `json:"token_id"`
	SessionNo         string `json:"session_no"`
	App               string `json:"app"`
	Vhost             string `json:"vhost"`
	Stream            string `json:"stream"`
	CredentialVersion uint32 `json:"credential_version"`
	IssuedAt          int64  `json:"iat"`
	NotBefore         int64  `json:"nbf"`
	ExpiresAt         int64  `json:"exp"`
}

type livePublishTokenConfig struct {
	publishTokenKey  string
	app              string
	vhost            string
	pushTokenSeconds int
	pushBaseURL      *url.URL
	playBaseURL      *url.URL
}

func resolveLivePublishTokenConfig(cfg config.Live) (livePublishTokenConfig, error) {
	publishTokenKey := strings.TrimSpace(cfg.PublishTokenKey)
	if publishTokenKey == "" {
		return livePublishTokenConfig{}, fmt.Errorf("%w: live.publish-token-key 不能为空", ErrLivePublishTokenConfigInvalid)
	}
	app := strings.Trim(strings.TrimSpace(cfg.SRS.App), "/")
	if app == "" || strings.Contains(app, "/") {
		return livePublishTokenConfig{}, fmt.Errorf("%w: live.srs.app 必须是非空的单段应用名", ErrLivePublishTokenConfigInvalid)
	}
	if cfg.PushTokenSeconds <= 0 {
		return livePublishTokenConfig{}, fmt.Errorf("%w: live.push-token-seconds 必须大于 0", ErrLivePublishTokenConfigInvalid)
	}
	pushBaseURL, err := parseLiveStreamBaseURL(cfg.SRS.PushBaseURL, true)
	if err != nil {
		return livePublishTokenConfig{}, err
	}
	playBaseURL, err := parseLiveStreamBaseURL(cfg.SRS.PlayBaseURL, false)
	if err != nil {
		return livePublishTokenConfig{}, err
	}
	return livePublishTokenConfig{
		publishTokenKey: publishTokenKey, app: app, vhost: pushBaseURL.Hostname(),
		pushTokenSeconds: cfg.PushTokenSeconds, pushBaseURL: pushBaseURL, playBaseURL: playBaseURL,
	}, nil
}

func parseLiveStreamBaseURL(raw string, required bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return nil, fmt.Errorf("%w: live.srs.push-base-url 不能为空", ErrLivePublishTokenConfigInvalid)
		}
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("%w: 无效的流媒体基础地址", ErrLivePublishTokenConfigInvalid)
	}
	return parsed, nil
}

func generateLivePublishToken(
	cfg livePublishTokenConfig,
	sessionNo string,
	stream string,
	credentialVersion uint32,
	now time.Time,
) (string, error) {
	tokenID, err := randomSecret(livePublishTokenIDByteSize)
	if err != nil {
		return "", err
	}
	iat := now.Unix()
	claims := livePublishTokenClaims{
		Version: livePublishTokenVersion, Scope: livePublishTokenScope, TokenID: tokenID,
		SessionNo: sessionNo, App: cfg.app, Vhost: cfg.vhost, Stream: stream,
		CredentialVersion: credentialVersion,
		IssuedAt:          iat, NotBefore: iat - livePublishTokenClockSkew,
		ExpiresAt: iat + int64(cfg.pushTokenSeconds),
	}
	plainText, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	// publish-token-key 可以是任意长度；固定用 SHA-256 派生 AES-256 所需的 32 字节密钥。
	key := sha256.Sum256([]byte(cfg.publishTokenKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := make([]byte, 0, len(nonce)+len(plainText)+gcm.Overhead())
	sealed = append(sealed, nonce...)
	sealed = gcm.Seal(sealed, nonce, plainText, nil)
	return livePublishTokenPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptLivePublishToken(cfg livePublishTokenConfig, token string) (livePublishTokenClaims, error) {
	var claims livePublishTokenClaims
	if !strings.HasPrefix(token, livePublishTokenPrefix) {
		return claims, ErrLivePublishTokenInvalid
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, livePublishTokenPrefix))
	if err != nil {
		return claims, ErrLivePublishTokenInvalid
	}
	key := sha256.Sum256([]byte(cfg.publishTokenKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return claims, ErrLivePublishTokenInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return claims, ErrLivePublishTokenInvalid
	}
	plainText, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil || json.Unmarshal(plainText, &claims) != nil {
		return livePublishTokenClaims{}, ErrLivePublishTokenInvalid
	}
	return claims, nil
}

func validateLivePublishTokenClaims(
	cfg livePublishTokenConfig,
	claims livePublishTokenClaims,
	requestApp string,
	requestVhost string,
	requestStream string,
	sessionNo string,
	credentialVersion uint32,
	now time.Time,
) error {
	tokenID, err := hex.DecodeString(claims.TokenID)
	nowUnix := now.Unix()
	if err != nil || len(tokenID) != livePublishTokenIDByteSize ||
		claims.Version != livePublishTokenVersion || claims.Scope != livePublishTokenScope ||
		claims.SessionNo == "" || claims.SessionNo != sessionNo ||
		claims.App != cfg.app || claims.Vhost != cfg.vhost || claims.Stream != requestStream ||
		claims.CredentialVersion == 0 || claims.CredentialVersion != credentialVersion ||
		claims.IssuedAt <= 0 || claims.NotBefore > claims.IssuedAt || claims.ExpiresAt <= claims.IssuedAt ||
		claims.IssuedAt > nowUnix+livePublishTokenClockSkew || nowUnix < claims.NotBefore || nowUnix >= claims.ExpiresAt {
		return ErrLivePublishTokenInvalid
	}
	if requestApp != "" && requestApp != claims.App {
		return ErrLivePublishTokenInvalid
	}
	// SRS 未配置具体 vhost 时会发送 __defaultVhost__；此时仍以 pt 内绑定的配置域名为准。
	if requestVhost != "" && requestVhost != "__defaultVhost__" && !strings.EqualFold(requestVhost, claims.Vhost) {
		return ErrLivePublishTokenInvalid
	}
	return nil
}

func nextLiveCredentialVersion(room liveModel.LiveRoom) (uint32, error) {
	version := room.StreamKeyVersion
	if version == 0 {
		version = 1
	}
	// 新房间的数据库默认版本为 1；已签发过凭证时才轮换到下一代。
	if strings.TrimSpace(room.PublishSecretHash) == "" {
		return version, nil
	}
	if version == math.MaxUint32 {
		return 0, fmt.Errorf("%w: credential_version 已达上限", ErrLivePublishTokenConfigInvalid)
	}
	return version + 1, nil
}

func (cfg livePublishTokenConfig) streamURL(stream, publishToken string) string {
	return buildConfiguredLiveStreamURL(cfg.pushBaseURL, cfg.app, stream, "pt", publishToken)
}

func (cfg livePublishTokenConfig) playURL(stream string) string {
	return buildConfiguredLiveStreamURL(cfg.playBaseURL, cfg.app, stream, "", "")
}

func buildConfiguredLiveStreamURL(baseURL *url.URL, app, stream, tokenName, token string) string {
	if baseURL == nil {
		return ""
	}
	result := *baseURL
	basePath := strings.TrimRight(result.Path, "/")
	if path.Base(basePath) != app {
		basePath += "/" + app
	}
	result.Path = basePath + "/" + stream
	result.RawPath = ""
	if tokenName != "" && token != "" {
		query := result.Query()
		query.Set(tokenName, token)
		result.RawQuery = query.Encode()
	}
	return result.String()
}
