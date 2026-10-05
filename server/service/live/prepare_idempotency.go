package live

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	livePrepareResultPrefix          = "v1_"
	livePrepareCacheOperationTimeout = time.Second
	defaultPushURLRefreshLimit       = 5
	hardPushURLRefreshLimit          = 20
)

var errLivePrepareResultCacheMiss = errors.New("prepare result cache miss")

type livePrepareResultCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

type redisLivePrepareResultCache struct {
	client redis.UniversalClient
}

func (c redisLivePrepareResultCache) Get(ctx context.Context, key string) (string, error) {
	value, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", errLivePrepareResultCacheMiss
	}
	return value, err
}

func (c redisLivePrepareResultCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (s *RoomService) prepareCache() livePrepareResultCache {
	if s.prepareResultCache != nil {
		return s.prepareResultCache
	}
	if global.GVA_REDIS == nil {
		return nil
	}
	return redisLivePrepareResultCache{client: global.GVA_REDIS}
}

func normalizeLiveIdempotencyKey(raw string, required bool) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		if required {
			return "", ErrLiveIdempotencyKeyInvalid
		}
		return "", nil
	}
	if key != raw || len(key) < 8 || len(key) > 128 {
		return "", ErrLiveIdempotencyKeyInvalid
	}
	for i := 0; i < len(key); i++ {
		ch := key[i]
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == ':') {
			return "", ErrLiveIdempotencyKeyInvalid
		}
	}
	return key, nil
}

func livePrepareCacheKey(publishTokenKey string, userID uint64, idempotencyKey string) string {
	mac := hmac.New(sha256.New, []byte("live:prepare-result:key:v1:"+publishTokenKey))
	_, _ = mac.Write([]byte(strconv.FormatUint(userID, 10)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(idempotencyKey))
	return "live:prepare-result:v1:" + strconv.FormatUint(userID, 10) + ":" + hex.EncodeToString(mac.Sum(nil))
}

func livePrepareResultCipherKey(publishTokenKey string) [sha256.Size]byte {
	return sha256.Sum256([]byte("live:prepare-result:cipher:v1:" + publishTokenKey))
}

func encryptLivePrepareResult(publishTokenKey, cacheKey string, result liveRes.LiveSessionPrepareResp) (string, error) {
	plainText, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	key := livePrepareResultCipherKey(publishTokenKey)
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
	sealed = gcm.Seal(sealed, nonce, plainText, []byte(cacheKey))
	return livePrepareResultPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptLivePrepareResult(publishTokenKey, cacheKey, value string) (liveRes.LiveSessionPrepareResp, error) {
	var result liveRes.LiveSessionPrepareResp
	if !strings.HasPrefix(value, livePrepareResultPrefix) {
		return result, errors.New("invalid prepare result prefix")
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(value, livePrepareResultPrefix))
	if err != nil {
		return result, err
	}
	key := livePrepareResultCipherKey(publishTokenKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return result, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return result, errors.New("invalid encrypted prepare result")
	}
	plainText, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(cacheKey))
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(plainText, &result); err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	return result, nil
}

func (s *RoomService) cacheLivePrepareResult(
	userID uint64,
	idempotencyKey string,
	publishConfig livePublishTokenConfig,
	result liveRes.LiveSessionPrepareResp,
) error {
	if idempotencyKey == "" {
		return nil
	}
	cache := s.prepareCache()
	if cache == nil {
		return ErrLiveIdempotencyUnavailable
	}
	ttl := time.Until(time.UnixMilli(result.PrepareDeadlineAt))
	if ttl <= 0 {
		return ErrLiveSessionStateInvalid
	}
	cacheKey := livePrepareCacheKey(publishConfig.publishTokenKey, userID, idempotencyKey)
	value, err := encryptLivePrepareResult(publishConfig.publishTokenKey, cacheKey, result)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), livePrepareCacheOperationTimeout)
	defer cancel()
	if err = cache.Set(ctx, cacheKey, value, ttl); err != nil {
		logLivePrepareCacheError("保存prepare幂等结果失败", err)
		return ErrLiveIdempotencyUnavailable
	}
	return nil
}

func (s *RoomService) loadLivePrepareResult(
	userID uint64,
	idempotencyKey string,
	publishConfig livePublishTokenConfig,
	room liveModel.LiveRoom,
	session liveModel.LiveSession,
	now time.Time,
) (liveRes.LiveSessionPrepareResp, bool, error) {
	cache := s.prepareCache()
	if cache == nil {
		return liveRes.LiveSessionPrepareResp{}, false, ErrLiveIdempotencyUnavailable
	}
	cacheKey := livePrepareCacheKey(publishConfig.publishTokenKey, userID, idempotencyKey)
	ctx, cancel := context.WithTimeout(context.Background(), livePrepareCacheOperationTimeout)
	defer cancel()
	value, err := cache.Get(ctx, cacheKey)
	if errors.Is(err, errLivePrepareResultCacheMiss) {
		return liveRes.LiveSessionPrepareResp{}, false, nil
	}
	if err != nil {
		logLivePrepareCacheError("读取prepare幂等结果失败", err)
		return liveRes.LiveSessionPrepareResp{}, false, ErrLiveIdempotencyUnavailable
	}
	result, err := decryptLivePrepareResult(publishConfig.publishTokenKey, cacheKey, value)
	if err != nil {
		logLivePrepareCacheError("解密prepare幂等结果失败", err)
		return liveRes.LiveSessionPrepareResp{}, false, ErrLiveIdempotencyUnavailable
	}
	if !validCachedLivePrepareResult(publishConfig, room, session, result, now) {
		// 找到了该幂等键曾经提交的结果，但数据库中的当前凭证已经被后续操作轮换。
		// 此时不能把旧键当成新刷新请求再次执行，否则同一键会产生多个不同结果。
		return liveRes.LiveSessionPrepareResp{}, false, ErrLiveIdempotencyConflict
	}
	return result, true, nil
}

func validCachedLivePrepareResult(
	publishConfig livePublishTokenConfig,
	room liveModel.LiveRoom,
	session liveModel.LiveSession,
	result liveRes.LiveSessionPrepareResp,
	now time.Time,
) bool {
	if session.Status != liveModel.LiveSessionPreparing && session.Status != liveModel.LiveSessionLiving {
		return false
	}
	if result.RoomNo != room.RoomNo || result.SessionNo != session.SessionNo ||
		result.StreamName != room.StreamName || result.PrepareDeadlineAt <= 0 ||
		result.PrepareDeadlineAt > session.PrepareDeadlineAt || now.UnixMilli() >= result.PrepareDeadlineAt ||
		result.PublishToken == "" || !secureStringEqual(room.PublishSecretHash, secretHash(result.PublishToken)) ||
		result.PushURL != publishConfig.streamURL(room.StreamName, result.PublishToken) ||
		result.PlayURL != publishConfig.playURL(room.StreamName) {
		return false
	}
	claims, err := decryptLivePublishToken(publishConfig, result.PublishToken)
	if err != nil {
		return false
	}
	return validateLivePublishTokenClaims(
		publishConfig, claims, "", "", room.StreamName,
		session.SessionNo, room.StreamKeyVersion, now,
	) == nil
}

func prepareRequestMatchesCurrentSession(
	req liveReq.LiveSessionPrepareReq,
	room liveModel.LiveRoom,
	session liveModel.LiveSession,
) bool {
	return req.Visibility != nil && req.CategoryId == session.CategoryId &&
		strings.TrimSpace(req.Title) == session.Title && strings.TrimSpace(req.CoverURL) == session.CoverURL &&
		*req.Visibility == room.Visibility
}

func livePushURLRefreshLimit() uint32 {
	limit := global.GVA_CONFIG.Live.PushURLRefreshLimit
	if limit <= 0 {
		limit = defaultPushURLRefreshLimit
	}
	if limit > hardPushURLRefreshLimit {
		limit = hardPushURLRefreshLimit
	}
	return uint32(limit)
}

func (s *RoomService) rotatePreparingPushURL(
	tx *gorm.DB,
	userID uint64,
	idempotencyKey string,
	publishConfig livePublishTokenConfig,
	room *liveModel.LiveRoom,
	session *liveModel.LiveSession,
	now time.Time,
) (liveRes.LiveSessionPrepareResp, error) {
	issuedCount := session.PublishCredentialIssueCount
	if issuedCount == 0 {
		issuedCount = 1
	}
	if issuedCount-1 >= livePushURLRefreshLimit() {
		return liveRes.LiveSessionPrepareResp{}, ErrLivePushURLRefreshLimit
	}
	credentialVersion, err := nextLiveCredentialVersion(*room)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	publishToken, err := generateLivePublishToken(
		publishConfig, session.SessionNo, room.StreamName, credentialVersion, now,
	)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	result := buildLivePrepareResponse(publishConfig, *room, *session, publishToken)
	if err = s.cacheLivePrepareResult(userID, idempotencyKey, publishConfig, result); err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	if err = tx.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).Updates(map[string]interface{}{
		"publish_secret_hash": secretHash(publishToken),
		"stream_key_version":  credentialVersion,
	}).Error; err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	issuedCount++
	sessionUpdate := tx.Model(&liveModel.LiveSession{}).
		Where("id = ? AND status = ? AND srs_stream_id = ''", session.ID, liveModel.LiveSessionPreparing).
		Update("publish_credential_issue_count", issuedCount)
	if sessionUpdate.Error != nil {
		return liveRes.LiveSessionPrepareResp{}, sessionUpdate.Error
	}
	if sessionUpdate.RowsAffected != 1 {
		return liveRes.LiveSessionPrepareResp{}, ErrLiveSessionStateInvalid
	}
	room.PublishSecretHash = secretHash(publishToken)
	room.StreamKeyVersion = credentialVersion
	session.PublishCredentialIssueCount = issuedCount
	return result, nil
}

func buildLivePrepareResponse(
	publishConfig livePublishTokenConfig,
	room liveModel.LiveRoom,
	session liveModel.LiveSession,
	publishToken string,
) liveRes.LiveSessionPrepareResp {
	return liveRes.LiveSessionPrepareResp{
		RoomNo: room.RoomNo, SessionNo: session.SessionNo, StreamName: room.StreamName,
		PrepareDeadlineAt: session.PrepareDeadlineAt,
		PublishToken:      publishToken, PushURL: publishConfig.streamURL(room.StreamName, publishToken),
		PlayURL: publishConfig.playURL(room.StreamName),
	}
}

// RefreshOwnerPushURL 为已经创建、但尚未被SRS接受publish的准备场次轮换推流凭证。
// prepare截止时间保持不变；Redis写入失败会让事务整体回滚，避免提交一个无法幂等重放的刷新结果。
func (s *RoomService) RefreshOwnerPushURL(userID uint64, rawIdempotencyKey string) (liveRes.LiveSessionPrepareResp, error) {
	if global.GVA_DB == nil {
		return liveRes.LiveSessionPrepareResp{}, errors.New("数据库未初始化")
	}
	idempotencyKey, err := normalizeLiveIdempotencyKey(rawIdempotencyKey, true)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	publishConfig, err := resolveLivePublishTokenConfig(global.GVA_CONFIG.Live)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	if publishConfig.pushTokenSeconds < livePrepareTimeoutSeconds() {
		return liveRes.LiveSessionPrepareResp{}, fmt.Errorf(
			"%w: live.push-token-seconds 不能小于 live.prepare-timeout-seconds",
			ErrLivePublishTokenConfigInvalid,
		)
	}

	var result liveRes.LiveSessionPrepareResp
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		anchor, txErr := (&AnchorService{}).getAnchorByUserIDForUpdate(tx, userID)
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			return ErrLiveRoomUnavailable
		}
		if txErr != nil {
			return txErr
		}
		if permission := livePermissionForAnchor(anchor, time.Now().UnixMilli()); !permission.Allow {
			return ErrLiveRoomUnavailable
		}
		room, txErr := getRoomByAnchorForUpdate(tx, anchor.ID)
		if txErr != nil {
			return normalizeRoomError(txErr)
		}
		if room.Status != liveModel.LiveRoomStatusNormal || room.CurrentSessionId == 0 {
			return ErrLiveRoomActiveSessionNotFound
		}
		var session liveModel.LiveSession
		if txErr = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, room.CurrentSessionId).Error; txErr != nil {
			return normalizeSessionError(txErr)
		}
		now := time.Now()
		if session.RoomId != room.ID || session.AnchorId != anchor.ID ||
			session.Status != liveModel.LiveSessionPreparing || session.PrepareDeadlineAt <= now.UnixMilli() ||
			hasSRSPublisher(session) {
			return ErrLiveSessionStateInvalid
		}
		cached, ok, txErr := s.loadLivePrepareResult(userID, idempotencyKey, publishConfig, room, session, now)
		if txErr != nil {
			return txErr
		}
		if ok {
			result = cached
			return nil
		}
		result, txErr = s.rotatePreparingPushURL(tx, userID, idempotencyKey, publishConfig, &room, &session, now)
		return txErr
	})
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, normalizeRoomError(err)
	}
	return result, nil
}

func logLivePrepareCacheError(message string, err error) {
	if global.GVA_LOG != nil {
		global.GVA_LOG.Error(message, zap.Error(err))
	}
}
