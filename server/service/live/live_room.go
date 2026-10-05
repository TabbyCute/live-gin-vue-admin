package live

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"tb_live_module/global"
	srsclient "tb_live_module/internal/srs"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	liveSessionNoMaxLength                 = 96
	liveSessionNoTimeLayout                = "20060102150405.000"
	liveSessionNoCollisionChecks           = 10000
	defaultPrepareTimeoutSeconds           = 120
	defaultStreamReadyTimeoutSeconds       = 15
	defaultStreamReadyRetryBaseSeconds     = 1
	defaultStreamReadyRetryMaxSeconds      = 4
	defaultStreamReconcileIntervalSeconds  = 5
	defaultStreamReconcileMissingThreshold = 3
	defaultMaxConcurrentStreams            = 10000
	defaultTaskMaxBatchesPerRun            = 10
	defaultStateTransitionConcurrency      = 16
	defaultEndScanBatchSize                = 100
	defaultEndStopConcurrency              = 8
	defaultEndRetryBaseSeconds             = 5
	defaultEndRetryMaxSeconds              = 60
	defaultEndWarningSeconds               = 120
	defaultEndCriticalSeconds              = 900
	defaultEndManualRetrySeconds           = 300
	defaultSRSRequestSeconds               = 3
)

var (
	ErrLiveRoomNotFound               = errors.New("直播间不存在")
	ErrLiveRoomNoRequired             = errors.New("直播间编号不能为空")
	ErrLiveRoomNoExists               = errors.New("直播间编号已存在")
	ErrLiveRoomUnavailable            = errors.New("直播间当前不可开播")
	ErrLiveRoomActiveSessionExists    = errors.New("直播间已有活动场次")
	ErrLiveRoomActiveSessionNotFound  = errors.New("当前没有活动直播场次")
	ErrLiveRoomStatusReasonRequired   = errors.New("禁用直播间时必须填写原因")
	ErrLiveSessionNotFound            = errors.New("直播场次不存在")
	ErrLiveSessionStateInvalid        = errors.New("直播场次状态不允许当前操作")
	ErrLivePublishTokenInvalid        = errors.New("推流凭证无效")
	ErrLivePublishTokenConfigInvalid  = errors.New("推流凭证配置无效")
	ErrLiveIdempotencyKeyInvalid      = errors.New("Idempotency-Key格式错误")
	ErrLiveIdempotencyUnavailable     = errors.New("幂等结果存储暂时不可用")
	ErrLiveIdempotencyConflict        = errors.New("Idempotency-Key对应的推流凭证已失效")
	ErrLivePushURLRefreshLimit        = errors.New("本场推流地址重新签发次数已达上限")
	ErrLiveTaskCapacityExceeded       = errors.New("直播定时任务待处理数量超过单轮容量")
	ErrLiveManualConfirmationTooEarly = errors.New("SRS尚未达到严重降级或场次尚未进入人工关注状态")
)

type RoomService struct {
	srsController      srsclient.Controller
	prepareResultCache livePrepareResultCache
	snapshotCache      livePublisherSnapshotCache
}

type publicStreamSnapshot struct {
	VideoCodec      string `json:"videoCodec,omitempty"`
	VideoProfile    string `json:"videoProfile,omitempty"`
	VideoLevel      string `json:"videoLevel,omitempty"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	AudioCodec      string `json:"audioCodec,omitempty"`
	AudioProfile    string `json:"audioProfile,omitempty"`
	AudioSampleRate int    `json:"audioSampleRate,omitempty"`
	AudioChannels   int    `json:"audioChannels,omitempty"`
	CollectedAt     int64  `json:"collectedAt,omitempty"`
}

type liveBatchErrorSample struct {
	message string
	cause   error
}

type liveDeadlineCandidate struct {
	ID         uint
	DeadlineAt int64
}

func (e liveBatchErrorSample) Error() string { return e.message }
func (e liveBatchErrorSample) Unwrap() error { return e.cause }

// BackfillApprovedAnchorRooms 为升级前已经审核通过的主播补齐唯一直播间。
// 新审核链路会在同一事务内创建房间；这里仅作为存量数据迁移和异常兜底。
func (s *RoomService) BackfillApprovedAnchorRooms(db *gorm.DB) error {
	const batchSize = 200
	var lastID uint
	for {
		var anchors []liveModel.LiveAnchor
		missingRoom := db.Table("live_room").Select("1").
			Where("live_room.anchor_id = live_anchor.id AND live_room.deleted_at IS NULL")
		if err := db.Where("apply_status = ? AND id > ?", liveModel.AnchorApplyStatusApproved, lastID).
			Where("NOT EXISTS (?)", missingRoom).Order("id ASC").Limit(batchSize).Find(&anchors).Error; err != nil {
			return err
		}
		if len(anchors) == 0 {
			return nil
		}
		for _, candidate := range anchors {
			if err := db.Transaction(func(tx *gorm.DB) error {
				var anchor liveModel.LiveAnchor
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&anchor, candidate.ID).Error; err != nil {
					return err
				}
				if anchor.ApplyStatus != liveModel.AnchorApplyStatusApproved {
					return nil
				}
				_, err := ensureRoomForAnchor(tx, anchor)
				return err
			}); err != nil {
				return err
			}
		}
		lastID = anchors[len(anchors)-1].ID
	}
}

func (s *RoomService) GetOwnerRoom(userID uint64) (liveRes.LiveRoomOwnerInfoResp, error) {
	anchor, err := (&AnchorService{}).getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveRes.LiveRoomOwnerInfoResp{}, ErrAnchorNotFound
	}
	if err != nil {
		return liveRes.LiveRoomOwnerInfoResp{}, err
	}
	var room liveModel.LiveRoom
	err = global.GVA_DB.Where("anchor_id = ?", anchor.ID).First(&room).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveRes.LiveRoomOwnerInfoResp{HasRoom: false}, nil
	}
	if err != nil {
		return liveRes.LiveRoomOwnerInfoResp{}, err
	}
	info, err := s.roomInfo(room, *anchor)
	if err != nil {
		return liveRes.LiveRoomOwnerInfoResp{}, err
	}
	return liveRes.LiveRoomOwnerInfoResp{HasRoom: true, Room: &info}, nil
}

func (s *RoomService) SaveOwnerRoom(userID uint64, req liveReq.LiveRoomSaveReq) (liveRes.LiveRoomInfo, error) {
	anchor, err := (&AnchorService{}).getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveRes.LiveRoomInfo{}, ErrAnchorNotFound
	}
	if err != nil {
		return liveRes.LiveRoomInfo{}, err
	}
	if anchor.ApplyStatus != liveModel.AnchorApplyStatusApproved || anchor.Status != liveModel.AnchorStatusNormal {
		return liveRes.LiveRoomInfo{}, ErrLiveRoomUnavailable
	}
	if err = (&CategoryService{}).EnsureEnabledCategory(req.CategoryId); err != nil {
		return liveRes.LiveRoomInfo{}, err
	}
	var room liveModel.LiveRoom
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		room, txErr = ensureRoomForAnchor(tx, *anchor)
		if txErr != nil {
			return txErr
		}
		updates := map[string]interface{}{
			"category_id": req.CategoryId,
			"title":       strings.TrimSpace(req.Title),
			"cover_url":   strings.TrimSpace(req.CoverURL),
			"notice":      strings.TrimSpace(req.Notice),
			"visibility":  *req.Visibility,
		}
		if txErr = tx.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).Updates(updates).Error; txErr != nil {
			return txErr
		}
		return tx.First(&room, room.ID).Error
	})
	if err != nil {
		return liveRes.LiveRoomInfo{}, normalizeRoomError(err)
	}
	return s.roomInfo(room, *anchor)
}

func (s *RoomService) UpdateOwnerRoomStatus(userID uint64, status uint8) error {
	anchor, err := (&AnchorService{}).getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAnchorNotFound
	}
	if err != nil {
		return err
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		room, lockErr := getRoomByAnchorForUpdate(tx, anchor.ID)
		if lockErr != nil {
			return normalizeRoomError(lockErr)
		}
		if room.Status == liveModel.LiveRoomStatusDisabled {
			return ErrLiveRoomUnavailable
		}
		if room.CurrentSessionId != 0 {
			return ErrLiveRoomActiveSessionExists
		}
		reason := ""
		if status == liveModel.LiveRoomStatusClosed {
			reason = "主播主动关闭"
		}
		return tx.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).Updates(map[string]interface{}{
			"status":            status,
			"status_reason":     reason,
			"status_changed_at": time.Now().UnixMilli(),
		}).Error
	})
}

func (s *RoomService) PrepareSession(userID uint64, req liveReq.LiveSessionPrepareReq) (liveRes.LiveSessionPrepareResp, error) {
	if global.GVA_DB == nil {
		return liveRes.LiveSessionPrepareResp{}, errors.New("数据库未初始化")
	}
	publishConfig, err := resolveLivePublishTokenConfig(global.GVA_CONFIG.Live)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	prepareTimeoutSeconds := livePrepareTimeoutSeconds()
	if publishConfig.pushTokenSeconds < prepareTimeoutSeconds {
		return liveRes.LiveSessionPrepareResp{}, fmt.Errorf(
			"%w: live.push-token-seconds 不能小于 live.prepare-timeout-seconds",
			ErrLivePublishTokenConfigInvalid,
		)
	}
	idempotencyKey, err := normalizeLiveIdempotencyKey(req.IdempotencyKey, false)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}

	var anchor liveModel.LiveAnchor
	var room liveModel.LiveRoom
	var session liveModel.LiveSession
	var publishToken string
	var result liveRes.LiveSessionPrepareResp
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		// prepare与后台封禁、权限关闭使用同一主播行锁。锁顺序固定为
		// anchor -> category -> room -> session，避免先读取旧权限再创建场次的竞态。
		lockedAnchor, txErr := (&AnchorService{}).getAnchorByUserIDForUpdate(tx, userID)
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			return ErrLiveRoomUnavailable
		}
		if txErr != nil {
			return txErr
		}
		if permission := livePermissionForAnchor(lockedAnchor, time.Now().UnixMilli()); !permission.Allow {
			return ErrLiveRoomUnavailable
		}
		anchor = *lockedAnchor
		categoryEnabled, txErr := (&CategoryService{}).categoryEnabledForShare(tx, req.CategoryId)
		if txErr != nil {
			return txErr
		}
		// ensureRoomForAnchor对存量房间使用SELECT FOR UPDATE；新建房间由当前事务独占，
		// 不再重复查询房间，减少prepare热路径的一次数据库往返。
		room, txErr = ensureRoomForAnchor(tx, anchor)
		if txErr != nil {
			return txErr
		}
		if room.Status != liveModel.LiveRoomStatusNormal {
			return ErrLiveRoomUnavailable
		}
		if room.CurrentSessionId != 0 {
			var current liveModel.LiveSession
			lookupErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, room.CurrentSessionId).Error
			if lookupErr == nil && isActiveSession(current.Status) {
				if idempotencyKey != "" &&
					(current.Status == liveModel.LiveSessionPreparing || current.Status == liveModel.LiveSessionLiving) &&
					prepareRequestMatchesCurrentSession(req, room, current) {
					now := time.Now()
					cached, ok, cacheErr := s.loadLivePrepareResult(
						userID, idempotencyKey, publishConfig, room, current, now,
					)
					if cacheErr != nil {
						return cacheErr
					}
					if ok {
						session = current
						result = cached
						return nil
					}
					// Redis键被提前淘汰时，同参数重试仍可在尚未publish的准备窗口内安全轮换凭证。
					// 旧pt哈希会在同一事务中失效，且不延长prepare截止时间。
					if current.Status == liveModel.LiveSessionPreparing &&
						current.PrepareDeadlineAt > now.UnixMilli() && !hasSRSPublisher(current) {
						session = current
						rotated, rotateErr := s.rotatePreparingPushURL(
							tx, userID, idempotencyKey, publishConfig, &room, &session, now,
						)
						if rotateErr != nil {
							return rotateErr
						}
						result = rotated
						return nil
					}
				}
				return ErrLiveRoomActiveSessionExists
			}
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				// 数据库错误不能当作“旧指针”清理，否则瞬时故障可能放行第二个活动场次。
				return lookupErr
			}
			if updateErr := clearRoomCurrentSession(tx, room.ID); updateErr != nil {
				return updateErr
			}
		}
		if !categoryEnabled {
			return ErrLiveCategoryNotFound
		}

		preparedAt := time.Now()
		sessionNo, sessionNoErr := nextLiveSessionNo(tx, preparedAt, anchor.AnchorNo, room.RoomNo)
		if sessionNoErr != nil {
			return sessionNoErr
		}
		session = liveModel.LiveSession{
			SessionNo: sessionNo, RoomId: room.ID, AnchorId: anchor.ID,
			CategoryId: req.CategoryId, Title: strings.TrimSpace(req.Title), CoverURL: strings.TrimSpace(req.CoverURL),
			Status:                      liveModel.LiveSessionPreparing,
			PrepareDeadlineAt:           preparedAt.Add(time.Duration(prepareTimeoutSeconds) * time.Second).UnixMilli(),
			PublishCredentialIssueCount: 1,
		}
		if txErr = tx.Create(&session).Error; txErr != nil {
			return normalizeActiveSessionError(txErr)
		}
		credentialVersion, versionErr := nextLiveCredentialVersion(room)
		if versionErr != nil {
			return versionErr
		}
		publishToken, txErr = generateLivePublishToken(
			publishConfig, session.SessionNo, room.StreamName, credentialVersion, preparedAt,
		)
		if txErr != nil {
			return txErr
		}
		updates := map[string]interface{}{
			"category_id":         req.CategoryId,
			"title":               session.Title,
			"cover_url":           session.CoverURL,
			"visibility":          *req.Visibility,
			"publish_secret_hash": secretHash(publishToken),
			"stream_key_version":  credentialVersion,
			"live_status":         liveModel.LiveRoomPreparing,
			"current_session_id":  session.ID,
			"live_started_at":     int64(0),
		}
		if txErr = tx.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).Updates(updates).Error; txErr != nil {
			return txErr
		}
		if txErr = tx.First(&room, room.ID).Error; txErr != nil {
			return txErr
		}
		result = buildLivePrepareResponse(publishConfig, room, session, publishToken)
		return s.cacheLivePrepareResult(userID, idempotencyKey, publishConfig, result)
	})
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, normalizeRoomError(err)
	}
	return result, nil
}

func (s *RoomService) GetOwnerCurrentSession(userID uint64) (*liveRes.LiveSessionInfo, error) {
	anchor, err := (&AnchorService{}).getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAnchorNotFound
	}
	if err != nil {
		return nil, err
	}
	var room liveModel.LiveRoom
	if err = global.GVA_DB.Where("anchor_id = ?", anchor.ID).First(&room).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if room.CurrentSessionId == 0 {
		return nil, nil
	}
	var session liveModel.LiveSession
	if err = global.GVA_DB.First(&session, room.CurrentSessionId).Error; err != nil {
		return nil, normalizeSessionError(err)
	}
	result := toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo)
	return &result, nil
}

func (s *RoomService) GetOwnerSessionHistory(userID uint64, req liveReq.LiveSessionHistoryReq) ([]liveRes.LiveSessionInfo, int64, error) {
	anchor, err := (&AnchorService{}).getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, ErrAnchorNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	var room liveModel.LiveRoom
	if err = global.GVA_DB.Where("anchor_id = ?", anchor.ID).First(&room).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return []liveRes.LiveSessionInfo{}, 0, nil
	} else if err != nil {
		return nil, 0, err
	}
	db := global.GVA_DB.Model(&liveModel.LiveSession{}).Where("room_id = ?", room.ID)
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}
	var total int64
	if err = db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var sessions []liveModel.LiveSession
	if err = db.Scopes(req.PageInfo.Paginate()).Order("id DESC").Find(&sessions).Error; err != nil {
		return nil, 0, err
	}
	list := make([]liveRes.LiveSessionInfo, 0, len(sessions))
	for _, session := range sessions {
		list = append(list, toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo))
	}
	return list, total, nil
}

func (s *RoomService) RequestOwnerEnd(userID uint64) error {
	var stopSessionID uint
	err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		// 主播端不接收 sessionNo；统一按 anchor -> room -> session 加锁并使用房间当前指针。
		var anchor liveModel.LiveAnchor
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&anchor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAnchorNotFound
			}
			return err
		}
		room, err := getRoomByAnchorForUpdate(tx, anchor.ID)
		if err != nil {
			return normalizeRoomError(err)
		}
		if room.CurrentSessionId == 0 {
			// 结束接口采用幂等语义。定时任务可能刚好已经完成结算并清除了当前场次。
			return nil
		}
		var session liveModel.LiveSession
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, room.CurrentSessionId).Error; err != nil {
			return normalizeSessionError(err)
		}
		if session.RoomId != room.ID || session.AnchorId != anchor.ID {
			return ErrLiveSessionStateInvalid
		}
		shouldStop, transitionErr := transitionLiveSessionToEnd(
			tx, session, liveModel.LiveSessionEndByAnchor, "", time.Now().UnixMilli(),
		)
		if shouldStop {
			stopSessionID = session.ID
		}
		return transitionErr
	})
	if err != nil {
		return err
	}
	if stopSessionID > 0 {
		// 结束意图已经可靠落库。SRS失败或超时只会让场次保留在结束中，由扫描任务继续重试。
		_ = s.processEndingSession(stopSessionID, time.Now().UnixMilli())
	}
	return nil
}

func (s *RoomService) OnPublish(req liveReq.SRSHookReq) (liveRes.LiveSessionInfo, error) {
	publishToken := hookPublishToken(req.Param)
	publishConfig, err := resolveLivePublishTokenConfig(global.GVA_CONFIG.Live)
	if err != nil {
		return liveRes.LiveSessionInfo{}, err
	}
	claims, err := decryptLivePublishToken(publishConfig, publishToken)
	if err != nil {
		return liveRes.LiveSessionInfo{}, ErrLivePublishTokenInvalid
	}
	// 流就绪任务必须用SRS运行时stream ID精确匹配快照；缺失时拒绝发布，
	// 避免仅凭可复用的app/stream误把旧连接或其他SRS节点的流判为当前场次。
	if strings.TrimSpace(req.StreamID) == "" {
		return liveRes.LiveSessionInfo{}, ErrLiveSessionStateInvalid
	}

	// 仅当Hook声称来自不同SRS进程时访问一次轻量HTTP API做权威校验。
	// 正常首次发布和同连接重复Hook不会增加外部请求；伪造或迟到的旧server_id不能推动代际回滚。
	runtime, err := loadLiveSRSRuntime(global.GVA_DB)
	if err != nil {
		return liveRes.LiveSessionInfo{}, err
	}
	hookServerID := strings.TrimSpace(req.ServerID)
	if runtime.ServerID != "" && hookServerID != "" && runtime.ServerID != hookServerID {
		observation, verifyErr := s.verifyCurrentSRSRuntime(hookServerID, time.Now().UnixMilli())
		if verifyErr != nil {
			return liveRes.LiveSessionInfo{}, errors.Join(ErrLiveSessionStateInvalid, verifyErr)
		}
		runtime = observation.Runtime
	}

	var result liveRes.LiveSessionInfo
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var roomRef liveModel.LiveRoom
		if err := tx.Select("id", "anchor_id").Where("stream_name = ?", req.Stream).First(&roomRef).Error; err != nil {
			return normalizeRoomError(err)
		}
		// 所有同时涉及主播、房间、场次的写事务统一按 anchor -> room -> session 加锁。
		var anchor liveModel.LiveAnchor
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&anchor, roomRef.AnchorId).Error; err != nil {
			return err
		}
		room, err := getRoomByIDForUpdate(tx, roomRef.ID)
		if err != nil {
			return normalizeRoomError(err)
		}
		if publishToken == "" || !secureStringEqual(room.PublishSecretHash, secretHash(publishToken)) {
			return ErrLivePublishTokenInvalid
		}
		if room.Status != liveModel.LiveRoomStatusNormal || room.CurrentSessionId == 0 {
			return ErrLiveRoomUnavailable
		}
		if anchor.ApplyStatus != liveModel.AnchorApplyStatusApproved ||
			anchor.Status != liveModel.AnchorStatusNormal ||
			anchor.LivePermission != liveModel.AnchorPermissionEnabled ||
			anchor.RiskLevel == liveModel.AnchorRiskHigh {
			return ErrLiveRoomUnavailable
		}
		var session liveModel.LiveSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, room.CurrentSessionId).Error; err != nil {
			return normalizeSessionError(err)
		}
		if err = validateLivePublishTokenClaims(
			publishConfig, claims, req.App, req.Vhost, req.Stream,
			session.SessionNo, room.StreamKeyVersion, time.Now(),
		); err != nil {
			return err
		}

		now := time.Now().UnixMilli()
		publishUpdates := livePublisherUpdates(req)
		if runtime.ServerID == hookServerID && runtime.Generation > 0 {
			publishUpdates["srs_generation"] = runtime.Generation
		}
		streamReadyDeadline := now + liveStreamReadyTimeoutMillis()
		switch session.Status {
		case liveModel.LiveSessionPreparing:
			if session.PrepareDeadlineAt <= 0 || session.PrepareDeadlineAt <= now {
				return ErrLiveSessionStateInvalid
			}
			if hasSRSPublisher(session) {
				// SRS可能对同一连接重复发送on_publish；保持幂等，但不能让另一连接覆盖当前发布者。
				if !srsPublisherMatches(session, req) {
					if !canTakeoverAfterSRSRestart(session, req, runtime) {
						return ErrLiveSessionStateInvalid
					}
					publishUpdates["publisher_epoch"] = gorm.Expr("publisher_epoch + 1")
					publishUpdates["media_state"] = liveModel.LiveMediaPublishing
					publishUpdates["media_state_changed_at"] = now
					publishUpdates["media_last_confirmed_at"] = now
				} else {
					result = toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo)
					return nil
				}
			}
			publishIP := strings.TrimSpace(req.IP)
			if strings.TrimSpace(session.PublishIP) == "" {
				publishUpdates["publish_ip"] = publishIP
				session.PublishIP = publishIP
			}
			publishUpdates["stream_info"] = nil
			publishUpdates["stream_ready_deadline_at"] = streamReadyDeadline
			publishUpdates["stream_probe_attempts"] = uint32(0)
			publishUpdates["stream_probe_next_at"] = now
			publishUpdates["stream_probe_last_error"] = ""
			publishUpdates["publisher_epoch"] = gorm.Expr("publisher_epoch + 1")
			publishUpdates["media_state"] = liveModel.LiveMediaPublishing
			publishUpdates["media_state_changed_at"] = now
			publishUpdates["media_last_confirmed_at"] = now
			if session.PrepareDeadlineAt < streamReadyDeadline {
				publishUpdates["prepare_deadline_at"] = streamReadyDeadline
				session.PrepareDeadlineAt = streamReadyDeadline
			}
			session.StreamInfo = nil
		case liveModel.LiveSessionLiving:
			if session.ReconnectDeadlineAt == 0 {
				// 活跃连接的重复on_publish不产生写库；不同连接不能覆盖当前发布者身份。
				if !srsPublisherMatches(session, req) {
					if !canTakeoverAfterSRSRestart(session, req, runtime) {
						return ErrLiveSessionStateInvalid
					}
					publishUpdates["publisher_epoch"] = gorm.Expr("publisher_epoch + 1")
					publishUpdates["reconnect_deadline_at"] = int64(0)
					publishUpdates["stream_info"] = nil
					publishUpdates["stream_ready_deadline_at"] = streamReadyDeadline
					publishUpdates["stream_probe_attempts"] = uint32(0)
					publishUpdates["stream_probe_next_at"] = now
					publishUpdates["stream_probe_last_error"] = ""
					publishUpdates["stream_reconcile_missing_count"] = uint32(0)
					publishUpdates["stream_reconcile_last_error"] = ""
					publishUpdates["media_state"] = liveModel.LiveMediaPublishing
					publishUpdates["media_state_changed_at"] = now
					publishUpdates["media_last_confirmed_at"] = now
					session.StreamInfo = nil
				} else {
					result = toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo)
					return nil
				}
			} else if session.ReconnectDeadlineAt <= now {
				return ErrLiveSessionStateInvalid
			} else {
				publishUpdates["publisher_epoch"] = gorm.Expr("publisher_epoch + 1")
				publishUpdates["reconnect_deadline_at"] = int64(0)
				publishUpdates["stream_info"] = nil
				publishUpdates["stream_ready_deadline_at"] = streamReadyDeadline
				publishUpdates["stream_probe_attempts"] = uint32(0)
				publishUpdates["stream_probe_next_at"] = now
				publishUpdates["stream_probe_last_error"] = ""
				publishUpdates["stream_reconcile_missing_count"] = uint32(0)
				publishUpdates["stream_reconcile_last_error"] = ""
				publishUpdates["media_state"] = liveModel.LiveMediaPublishing
				publishUpdates["media_state_changed_at"] = now
				publishUpdates["media_last_confirmed_at"] = now
				session.ReconnectDeadlineAt = 0
				session.StreamInfo = nil
			}
		default:
			return ErrLiveSessionStateInvalid
		}
		if err := tx.Model(&liveModel.LiveSession{}).Where("id = ?", session.ID).Updates(publishUpdates).Error; err != nil {
			return err
		}
		// GORM 的 map Updates 不会把表达式结果回填到当前结构体。响应至少要反映本次
		// 已经提交的媒体事实，不能让 SRS 收到成功但客户端仍看到旧的 unknown/disconnected。
		session.MediaState = liveModel.LiveMediaPublishing
		session.MediaStateChangedAt = now
		session.MediaLastConfirmedAt = now
		session.ReconnectDeadlineAt = 0
		session.StreamReadyDeadlineAt = streamReadyDeadline
		if session.Status == liveModel.LiveSessionLiving {
			if err := tx.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).Updates(map[string]interface{}{
				"live_status": liveModel.LiveRoomLiving, "live_started_at": session.StartedAt,
			}).Error; err != nil {
				return err
			}
		}
		result = toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo)
		return nil
	})
	return result, err
}

func canTakeoverAfterSRSRestart(
	session liveModel.LiveSession,
	req liveReq.SRSHookReq,
	runtime liveModel.LiveSRSRuntime,
) bool {
	hookServerID := strings.TrimSpace(req.ServerID)
	oldServerID := strings.TrimSpace(session.SRSServerID)
	return hookServerID != "" && oldServerID != "" && oldServerID != hookServerID &&
		runtime.HealthState == liveModel.LiveSRSHealthHealthy && runtime.ServerID == hookServerID &&
		runtime.Generation > session.SRSGeneration
}

func (s *RoomService) OnUnpublish(req liveReq.SRSHookReq) error {
	window := liveReconnectWindowSeconds()
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var room liveModel.LiveRoom
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("stream_name = ?", req.Stream).First(&room).Error; err != nil {
			return normalizeRoomError(err)
		}
		if room.CurrentSessionId == 0 {
			return nil
		}
		var session liveModel.LiveSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, room.CurrentSessionId).Error; err != nil {
			return normalizeSessionError(err)
		}
		if session.Status == liveModel.LiveSessionPreparing {
			if !hasSRSPublisher(session) || !srsPublisherMatches(session, req) {
				return nil
			}
			// 尚未取得有效视频尺寸时仍属于准备中。当前连接断开后清除连接身份，
			// 允许主播在准备截止时间前重新推流，且旧连接不能被扫描任务误判为已就绪。
			now := time.Now().UnixMilli()
			return tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", session.ID, liveModel.LiveSessionPreparing).
				Updates(map[string]interface{}{
					"srs_server_id": "", "srs_stream_id": "", "srs_client_id": "",
					"srs_vhost": "", "srs_app": "", "stream_info": nil,
					"stream_ready_deadline_at": int64(0), "stream_probe_attempts": uint32(0),
					"stream_probe_next_at": int64(0), "stream_probe_last_error": "",
					"stream_reconcile_missing_count": uint32(0), "stream_reconcile_last_error": "",
					"media_state":            liveModel.LiveMediaDisconnected,
					"media_state_changed_at": now, "media_last_confirmed_at": now,
				}).Error
		}
		if session.Status != liveModel.LiveSessionLiving {
			return nil
		}
		if !srsPublisherMatches(session, req) {
			// 新连接已经接管后，旧连接的延迟on_unpublish只能幂等忽略。
			return nil
		}
		if session.ReconnectDeadlineAt > 0 {
			// SRS 可能重复发送同一次 on_unpublish，不能重复累计或延长重连窗口。
			return nil
		}
		now := time.Now().UnixMilli()
		return tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", session.ID, liveModel.LiveSessionLiving).
			Updates(map[string]interface{}{
				"last_unpublish_at": now, "reconnect_deadline_at": now + int64(window)*1000,
				"disconnect_count":         gorm.Expr("disconnect_count + 1"),
				"stream_ready_deadline_at": int64(0), "stream_probe_attempts": uint32(0),
				"stream_probe_next_at": int64(0), "stream_probe_last_error": "",
				"stream_reconcile_checked_at": now, "stream_reconcile_missing_count": uint32(0),
				"stream_reconcile_last_error": "",
				"media_state":                 liveModel.LiveMediaDisconnected,
				"media_state_changed_at":      now, "media_last_confirmed_at": now,
			}).Error
	})
}

func livePublisherUpdates(req liveReq.SRSHookReq) map[string]interface{} {
	updates := make(map[string]interface{}, 5)
	for column, value := range map[string]string{
		"srs_server_id": strings.TrimSpace(req.ServerID),
		"srs_stream_id": strings.TrimSpace(req.StreamID),
		"srs_client_id": strings.TrimSpace(req.ClientID),
		"srs_vhost":     strings.TrimSpace(req.Vhost),
		"srs_app":       strings.Trim(strings.TrimSpace(req.App), "/"),
	} {
		if value != "" {
			updates[column] = value
		}
	}
	return updates
}

// srsPublisherMatches 只接受当前发布连接的重复或断流回调。
// 已保存的连接标识不能被缺失或不同的回调标识绕过；旧数据未保存某一字段时则跳过该字段比较。
func srsPublisherMatches(session liveModel.LiveSession, req liveReq.SRSHookReq) bool {
	for _, pair := range [][2]string{
		{strings.TrimSpace(session.SRSServerID), strings.TrimSpace(req.ServerID)},
		{strings.TrimSpace(session.SRSStreamID), strings.TrimSpace(req.StreamID)},
		{strings.TrimSpace(session.SRSClientID), strings.TrimSpace(req.ClientID)},
	} {
		if pair[0] != "" && pair[0] != pair[1] {
			return false
		}
	}
	return true
}

func hasSRSPublisher(session liveModel.LiveSession) bool {
	return strings.TrimSpace(session.SRSStreamID) != ""
}

func (s *RoomService) UpdateSessionStats(req liveReq.LiveSessionStatsReq) error {
	updates := map[string]interface{}{
		"view_count": req.ViewCount, "viewer_count": req.ViewerCount, "peak_online_count": req.PeakOnlineCount,
		"like_count": req.LikeCount, "gift_count": req.GiftCount, "gift_coin_amount": req.GiftCoinAmount,
		"gift_user_count": req.GiftUserCount,
	}
	result := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("session_no = ? AND status IN ?", req.SessionNo, []int{int(liveModel.LiveSessionPreparing), int(liveModel.LiveSessionLiving), int(liveModel.LiveSessionEnding)}).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Where("session_no = ? AND status IN ?", req.SessionNo, []int{int(liveModel.LiveSessionPreparing), int(liveModel.LiveSessionLiving), int(liveModel.LiveSessionEnding)}).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrLiveSessionStateInvalid
		}
	}
	return nil
}

// ProcessPendingSessions 只推进可由数据库事实确定的超时状态，不发起任何SRS网络请求。
// 即使SRS完全不可用，准备超时和重连超时也能按扫描周期持续处理。
func (s *RoomService) ProcessPendingSessions(now int64) error {
	batchSize := liveEndScanBatchSize()
	var runErrors []error
	// 准备超时没有真正开播，不访问SRS也不做直播结算，直接取消场次并释放房间。
	// (status, prepare_deadline_at)联合索引使查询只触达已到期的准备中场次。
	prepareBatches := liveEndScanMaxBatchesPerRun()
	prepareProcessed := 0
	var prepareCursor liveDeadlineCandidate
	for batch := 0; batch < prepareBatches; batch++ {
		var candidates []liveDeadlineCandidate
		query := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Select("id, prepare_deadline_at AS deadline_at").
			Where("status = ? AND srs_stream_id = '' AND prepare_deadline_at <= ?", liveModel.LiveSessionPreparing, now).
			Order("prepare_deadline_at ASC, id ASC").Limit(batchSize)
		if prepareCursor.ID > 0 {
			query = query.Where(
				"prepare_deadline_at > ? OR (prepare_deadline_at = ? AND id > ?)",
				prepareCursor.DeadlineAt, prepareCursor.DeadlineAt, prepareCursor.ID,
			)
		}
		if err := query.Scan(&candidates).Error; err != nil {
			return summarizeLiveBatchErrors("直播场次生命周期", append(runErrors, err))
		}
		if len(candidates) == 0 {
			break
		}
		prepareTimeoutIDs := make([]uint, 0, len(candidates))
		for _, candidate := range candidates {
			prepareTimeoutIDs = append(prepareTimeoutIDs, candidate.ID)
		}
		prepareCursor = candidates[len(candidates)-1]
		prepareProcessed += len(prepareTimeoutIDs)
		runErrors = append(runErrors, runLiveSessionIDWorkers(
			prepareTimeoutIDs, liveStateTransitionConcurrency(),
			func(sessionID uint) error { return s.cancelPrepareTimeout(sessionID, now) },
		)...)
		if len(prepareTimeoutIDs) < batchSize {
			break
		}
	}
	if prepareProcessed >= batchSize*prepareBatches {
		var remaining int64
		if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Where("status = ? AND srs_stream_id = '' AND prepare_deadline_at <= ?", liveModel.LiveSessionPreparing, now).
			Limit(1).Count(&remaining).Error; err != nil {
			runErrors = append(runErrors, err)
		} else if remaining > 0 {
			runErrors = append(runErrors, fmt.Errorf(
				"%w: 准备超时扫描单轮已处理%d项", ErrLiveTaskCapacityExceeded, prepareProcessed,
			))
		}
	}

	// 把超过重连窗口的直播中场次改为结束中；独立的SRS收尾任务随后负责断流和结算。
	timeoutBatches := liveEndScanMaxBatchesPerRun()
	timeoutProcessed := 0
	var timeoutCursor liveDeadlineCandidate
	for batch := 0; batch < timeoutBatches; batch++ {
		var candidates []liveDeadlineCandidate
		query := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Select("id, reconnect_deadline_at AS deadline_at").
			Where("status = ? AND reconnect_deadline_at > 0 AND reconnect_deadline_at <= ?", liveModel.LiveSessionLiving, now).
			Order("reconnect_deadline_at ASC, id ASC").Limit(batchSize)
		if timeoutCursor.ID > 0 {
			query = query.Where(
				"reconnect_deadline_at > ? OR (reconnect_deadline_at = ? AND id > ?)",
				timeoutCursor.DeadlineAt, timeoutCursor.DeadlineAt, timeoutCursor.ID,
			)
		}
		if err := query.Scan(&candidates).Error; err != nil {
			return summarizeLiveBatchErrors("直播场次生命周期", append(runErrors, err))
		}
		if len(candidates) == 0 {
			break
		}
		timeoutIDs := make([]uint, 0, len(candidates))
		for _, candidate := range candidates {
			timeoutIDs = append(timeoutIDs, candidate.ID)
		}
		timeoutCursor = candidates[len(candidates)-1]
		timeoutProcessed += len(timeoutIDs)
		runErrors = append(runErrors, runLiveSessionIDWorkers(
			timeoutIDs, liveStateTransitionConcurrency(),
			func(sessionID uint) error { return s.markDisconnectTimeout(sessionID, now) },
		)...)
		if len(timeoutIDs) < batchSize {
			break
		}
	}
	if timeoutProcessed >= batchSize*timeoutBatches {
		var remaining int64
		if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Where("status = ? AND reconnect_deadline_at > 0 AND reconnect_deadline_at <= ?", liveModel.LiveSessionLiving, now).
			Limit(1).Count(&remaining).Error; err != nil {
			runErrors = append(runErrors, err)
		} else if remaining > 0 {
			runErrors = append(runErrors, fmt.Errorf(
				"%w: 重连超时扫描单轮已处理%d项", ErrLiveTaskCapacityExceeded, timeoutProcessed,
			))
		}
	}

	return summarizeLiveBatchErrors("直播场次生命周期", runErrors)
}

func runLiveSessionIDWorkers(ids []uint, concurrency int, run func(uint) error) []error {
	if len(ids) == 0 {
		return nil
	}
	if concurrency > len(ids) {
		concurrency = len(ids)
	}
	jobs := make(chan uint, concurrency)
	errs := make(chan error, len(ids))
	var workers sync.WaitGroup
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range jobs {
				if err := run(id); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
					errs <- err
				}
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	workers.Wait()
	close(errs)
	runErrors := make([]error, 0, len(errs))
	for err := range errs {
		runErrors = append(runErrors, err)
	}
	return runErrors
}

// ProcessEndingSessions 只处理需要访问SRS的结束中场次，与准备/重连超时扫描完全隔离。
// 有界工作池既消除逐场串行请求造成的队头阻塞，也限制SRS故障时的并发连接和goroutine数量。
// stop_next_retry_at同时充当多实例租约；不同服务实例即使读到相同ID，也只有一个能真正调用SRS。
func (s *RoomService) ProcessEndingSessions(now int64) error {
	if err := promoteEndingStallAlerts(now); err != nil {
		return err
	}
	batchSize := liveEndStopBatchSize()
	if batchSize <= 0 {
		return nil
	}
	maxItems := batchSize * liveEndStopMaxBatchesPerRun()
	var endingIDs []uint
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND stop_next_retry_at <= ?", liveModel.LiveSessionEnding, now).
		Order("stop_recovery_state DESC, stop_requested_at ASC, id ASC").Limit(maxItems).Pluck("id", &endingIDs).Error; err != nil {
		return err
	}
	if len(endingIDs) == 0 {
		return nil
	}

	workerCount := liveEndStopConcurrency()
	if workerCount > len(endingIDs) {
		workerCount = len(endingIDs)
	}
	controller, controllerErr := s.liveSRSController()
	var runErrors []error
	if controllerErr == nil {
		if batchController, ok := controller.(srsclient.BatchStopController); ok {
			runErrors = append(runErrors, s.processEndingSessionsBatch(
				endingIDs, now, workerCount, batchController,
			)...)
			if len(endingIDs) >= maxItems {
				runErrors = append(runErrors, endingSessionCapacityErrors(now, len(endingIDs))...)
			}
			return summarizeLiveBatchErrors("SRS场次收尾", runErrors)
		}
	}
	jobs := make(chan uint, len(endingIDs))
	errs := make(chan error, len(endingIDs))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for sessionID := range jobs {
				// 批次后部可能已等待多个SRS超时周期，不能继续使用扫描开始时的旧时间认领租约。
				attemptAt := observedUnixMilli(now)
				if err := s.processEndingSessionWithController(
					sessionID, attemptAt, controller, controllerErr,
				); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
					errs <- err
				}
			}
		}()
	}
	for _, sessionID := range endingIDs {
		jobs <- sessionID
	}
	close(jobs)
	workers.Wait()
	close(errs)

	runErrors = make([]error, 0, len(errs))
	for err := range errs {
		runErrors = append(runErrors, err)
	}
	if len(endingIDs) >= maxItems {
		runErrors = append(runErrors, endingSessionCapacityErrors(now, len(endingIDs))...)
	}
	return summarizeLiveBatchErrors("SRS场次收尾", runErrors)
}

func (s *RoomService) processEndingSessionsBatch(
	endingIDs []uint,
	now int64,
	workerCount int,
	controller srsclient.BatchStopController,
) []error {
	sessions, err := claimEndingSessionsBatch(endingIDs, now)
	if err != nil {
		return []error{err}
	}
	if len(sessions) == 0 {
		return nil
	}
	runErrors := make([]error, 0)
	pending := sessions[:0]
	for _, session := range sessions {
		if mediaStopAlreadyConfirmed(session) {
			if finalizeErr := s.finalizeSession(session.ID, observedUnixMilli(now)); finalizeErr != nil {
				runErrors = append(runErrors, finalizeErr)
			}
			continue
		}
		pending = append(pending, session)
	}
	sessions = pending
	if len(sessions) == 0 {
		return runErrors
	}
	roomIDs := make([]uint, 0, len(sessions))
	for _, session := range sessions {
		roomIDs = append(roomIDs, session.RoomId)
	}
	var rooms []liveModel.LiveRoom
	if err = global.GVA_DB.Select("id", "stream_name").Where("id IN ?", roomIDs).Find(&rooms).Error; err != nil {
		return []error{err}
	}
	roomByID := make(map[uint]string, len(rooms))
	for _, room := range rooms {
		roomByID[room.ID] = room.StreamName
	}
	refs := make([]srsclient.StreamRef, 0, len(sessions))
	for _, session := range sessions {
		refs = append(refs, srsclient.StreamRef{
			StreamID: session.SRSStreamID,
			Vhost:    session.SRSVhost,
			App:      session.SRSApp,
			Stream:   roomByID[session.RoomId],
		})
	}
	stopErrors := controller.StopPublishers(context.Background(), refs, workerCount)
	if len(stopErrors) != len(sessions) {
		return []error{fmt.Errorf("SRS批量停止结果数量错误: want=%d got=%d", len(sessions), len(stopErrors))}
	}
	var batchSRSFailure error
	for _, stopErr := range stopErrors {
		if stopErr != nil {
			batchSRSFailure = stopErr
			break
		}
	}
	completedObservationAt := observedUnixMilli(now)
	if batchSRSFailure != nil {
		_ = observeLiveSRSFailure(completedObservationAt, batchSRSFailure)
	} else {
		_, _ = observeLiveSRSSuccess(srsclient.RuntimeIdentity{}, completedObservationAt)
	}
	byID := make(map[uint]int, len(sessions))
	ids := make([]uint, 0, len(sessions))
	for index, session := range sessions {
		ids = append(ids, session.ID)
		byID[session.ID] = index
	}
	runErrors = append(runErrors, runLiveSessionIDWorkers(ids, liveStateTransitionConcurrency(), func(sessionID uint) error {
		index := byID[sessionID]
		session := sessions[index]
		completedAt := observedUnixMilli(now)
		if stopErr := stopErrors[index]; stopErr != nil {
			if recordErr := s.recordStopFailure(session, completedAt, stopErr); recordErr != nil {
				return errors.Join(stopErr, recordErr)
			}
			return stopErr
		}
		return s.finalizeSession(session.ID, completedAt)
	})...)
	return runErrors
}

func mediaStopAlreadyConfirmed(session liveModel.LiveSession) bool {
	return session.MediaState == liveModel.LiveMediaStopped &&
		session.MediaStopSource == liveModel.LiveMediaStopSourceAdmin &&
		session.MediaStopConfirmedAt > 0
}

func claimEndingSessionsBatch(endingIDs []uint, now int64) ([]liveModel.LiveSession, error) {
	leaseUntil := now + liveStopBatchLeaseMillis()
	dialect := global.GVA_DB.Dialector.Name()
	if dialect == "mysql" || dialect == "postgres" {
		var sessions []liveModel.LiveSession
		err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Select(
					"id", "room_id", "srs_stream_id", "srs_vhost", "srs_app",
					"stop_requested_at", "stop_attempts", "stop_next_retry_at", "stop_recovery_state",
					"media_state", "media_stop_source", "media_stop_confirmed_at",
				).
				Where("id IN ? AND status = ? AND stop_next_retry_at <= ?", endingIDs, liveModel.LiveSessionEnding, now).
				Order("stop_next_retry_at ASC, id ASC").Find(&sessions).Error; err != nil {
				return err
			}
			if len(sessions) == 0 {
				return nil
			}
			claimedIDs := make([]uint, 0, len(sessions))
			for _, session := range sessions {
				claimedIDs = append(claimedIDs, session.ID)
			}
			claim := tx.Model(&liveModel.LiveSession{}).
				Where("id IN ? AND status = ? AND stop_next_retry_at <= ?", claimedIDs, liveModel.LiveSessionEnding, now).
				Updates(map[string]interface{}{
					"stop_attempts":      gorm.Expr("stop_attempts + 1"),
					"stop_next_retry_at": leaseUntil,
				})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected != int64(len(sessions)) {
				return fmt.Errorf("批量认领SRS收尾任务不完整: want=%d got=%d", len(sessions), claim.RowsAffected)
			}
			for index := range sessions {
				sessions[index].StopAttempts++
				sessions[index].StopNextRetryAt = leaseUntil
			}
			return nil
		})
		return sessions, err
	}

	// 不支持SKIP LOCKED的数据库继续使用逐行CAS，保证测试和兼容部署的正确性。
	sessions := make([]liveModel.LiveSession, 0, len(endingIDs))
	for _, sessionID := range endingIDs {
		claim := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Where("id = ? AND status = ? AND stop_next_retry_at <= ?", sessionID, liveModel.LiveSessionEnding, now).
			Updates(map[string]interface{}{
				"stop_attempts":      gorm.Expr("stop_attempts + 1"),
				"stop_next_retry_at": leaseUntil,
			})
		if claim.Error != nil {
			return nil, claim.Error
		}
		if claim.RowsAffected == 0 {
			continue
		}
		var session liveModel.LiveSession
		if err := global.GVA_DB.Select(
			"id", "room_id", "srs_stream_id", "srs_vhost", "srs_app", "stop_requested_at",
			"stop_attempts", "stop_next_retry_at", "stop_recovery_state",
			"media_state", "media_stop_source", "media_stop_confirmed_at",
		).First(&session, sessionID).Error; err != nil {
			return nil, normalizeSessionError(err)
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func endingSessionCapacityErrors(now int64, processed int) []error {
	var remaining int64
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND stop_next_retry_at <= ?", liveModel.LiveSessionEnding, now).
		Limit(1).Count(&remaining).Error; err != nil {
		return []error{err}
	}
	if remaining == 0 {
		return nil
	}
	return []error{fmt.Errorf(
		"%w: SRS场次收尾单轮已认领%d项", ErrLiveTaskCapacityExceeded, processed,
	)}
}

func summarizeLiveBatchErrors(operation string, runErrors []error) error {
	if len(runErrors) == 0 {
		return nil
	}
	const maxSamples = 5
	samples := make([]error, 0, maxSamples)
	seen := make(map[string]struct{}, maxSamples)
	for _, runErr := range runErrors {
		if runErr == nil {
			continue
		}
		message := truncateRunes(runErr.Error(), 500)
		if _, exists := seen[message]; exists {
			continue
		}
		seen[message] = struct{}{}
		samples = append(samples, liveBatchErrorSample{message: message, cause: runErr})
		if len(samples) == maxSamples {
			break
		}
	}
	if len(samples) == 0 {
		return nil
	}
	return fmt.Errorf("%s失败%d项（最多展示%d种错误）: %w", operation, len(runErrors), maxSamples, errors.Join(samples...))
}

// ProcessStreamReadiness 按数据库中的下一次探测时间批量查询SRS。
// on_publish只证明连接建立；只有publish.active且宽高均有效时才正式进入直播中。
// stream_probe_next_at同时作为多实例处理租约，避免重复请求和失控的后台goroutine。
func (s *RoomService) ProcessStreamReadiness(now int64) error {
	batchSize := liveStreamReadyBatchSize()
	if batchSize <= 0 {
		return nil
	}
	var runErrors []error
	maxBatches := liveStreamReadyMaxBatchesPerRun()
	processed := 0
	var snapshotByID map[string]srsclient.PublisherSnapshot
	var snapshotGeneration uint64
	var snapshotErr error
	snapshotLoaded := false
	var expiredCursor liveDeadlineCandidate

	for batch := 0; batch < maxBatches; batch++ {
		batchProcessed := 0
		// 已超过硬截止时间的场次优先处理，不能被大量普通探测任务挤到下一轮。
		// 该查询和下面的到期探测查询分别命中联合索引，避免OR条件扩大扫描范围。
		var expiredCandidates []liveDeadlineCandidate
		expiredQuery := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Select("id, stream_ready_deadline_at AS deadline_at").
			Where("status IN ? AND srs_stream_id <> '' AND stream_ready_deadline_at > 0",
				[]int{int(liveModel.LiveSessionPreparing), int(liveModel.LiveSessionLiving)}).
			Where("stream_ready_deadline_at <= ?", now).
			Order("stream_ready_deadline_at ASC, id ASC").Limit(batchSize)
		if expiredCursor.ID > 0 {
			expiredQuery = expiredQuery.Where(
				"stream_ready_deadline_at > ? OR (stream_ready_deadline_at = ? AND id > ?)",
				expiredCursor.DeadlineAt, expiredCursor.DeadlineAt, expiredCursor.ID,
			)
		}
		if err := expiredQuery.Scan(&expiredCandidates).Error; err != nil {
			return summarizeLiveBatchErrors("流就绪探测", append(runErrors, err))
		}
		expiredIDs := make([]uint, 0, len(expiredCandidates))
		for _, candidate := range expiredCandidates {
			expiredIDs = append(expiredIDs, candidate.ID)
		}
		if len(expiredCandidates) > 0 {
			expiredCursor = expiredCandidates[len(expiredCandidates)-1]
		}
		if len(expiredIDs) > 0 {
			batchProcessed += len(expiredIDs)
			runErrors = append(runErrors, runLiveSessionIDWorkers(
				expiredIDs, liveStateTransitionConcurrency(),
				func(sessionID uint) error { return s.expireStreamReadiness(sessionID, observedUnixMilli(now)) },
			)...)
		}

		remaining := batchSize - len(expiredIDs)
		if remaining > 0 {
			sessions, err := claimStreamReadinessSessions(now, remaining)
			if err != nil {
				return summarizeLiveBatchErrors("流就绪探测", append(runErrors, err))
			}
			batchProcessed += len(sessions)
			if len(sessions) > 0 {
				if !snapshotLoaded {
					snapshotLoaded = true
					controller, controllerErr := s.liveSRSController()
					if controllerErr != nil {
						snapshotErr = controllerErr
						_ = observeLiveSRSFailure(observedUnixMilli(now), controllerErr)
					} else {
						requestTimeout := streamReadinessRequestTimeout(sessions, now)
						ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
						publisherBatch, loadErr := s.loadPublisherSnapshotBatch(ctx, controller)
						cancel()
						observation, observeErr := observePublisherBatch(publisherBatch, observedUnixMilli(now), loadErr)
						snapshotErr = errors.Join(loadErr, observeErr)
						if snapshotErr == nil {
							snapshotGeneration = observation.Runtime.Generation
							snapshotByID = make(map[string]srsclient.PublisherSnapshot, len(publisherBatch.Publishers))
							for _, snapshot := range publisherBatch.Publishers {
								if id := strings.TrimSpace(snapshot.StreamID); id != "" {
									snapshotByID[id] = snapshot
								}
							}
						}
					}
				}
				if snapshotErr != nil {
					runErrors = append(runErrors, snapshotErr)
					runErrors = append(runErrors, s.recordStreamProbeBatchFailureConcurrent(
						sessions, observedUnixMilli(now), snapshotErr,
					)...)
				} else {
					runErrors = append(runErrors, s.applyStreamReadinessSnapshots(
						sessions, snapshotByID, snapshotGeneration, now,
					)...)
				}
			}
		}

		processed += batchProcessed
		if batchProcessed < batchSize {
			return summarizeLiveBatchErrors("流就绪探测", runErrors)
		}
	}

	if pending, err := hasPendingStreamReadiness(now); err != nil {
		runErrors = append(runErrors, err)
	} else if pending {
		runErrors = append(runErrors, fmt.Errorf(
			"%w: 流就绪探测单轮已处理%d项", ErrLiveTaskCapacityExceeded, processed,
		))
	}
	return summarizeLiveBatchErrors("流就绪探测", runErrors)
}

func claimStreamReadinessSessions(now int64, limit int) ([]liveModel.LiveSession, error) {
	leaseUntil := now + liveStreamProbeLeaseMillis()
	statuses := []int{int(liveModel.LiveSessionPreparing), int(liveModel.LiveSessionLiving)}
	dialect := global.GVA_DB.Dialector.Name()
	if dialect == "mysql" || dialect == "postgres" {
		var sessions []liveModel.LiveSession
		err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Select(
					"id", "room_id", "anchor_id", "status", "srs_stream_id", "srs_vhost", "srs_app",
					"stream_ready_deadline_at", "stream_probe_attempts", "stream_probe_next_at",
				).
				Where("status IN ? AND srs_stream_id <> '' AND stream_ready_deadline_at > ?", statuses, now).
				Where("stream_probe_next_at > 0 AND stream_probe_next_at <= ?", now).
				Order("stream_probe_next_at ASC, id ASC").Limit(limit).Find(&sessions).Error; err != nil {
				return err
			}
			if len(sessions) == 0 {
				return nil
			}
			ids := make([]uint, 0, len(sessions))
			for _, session := range sessions {
				ids = append(ids, session.ID)
			}
			claim := tx.Model(&liveModel.LiveSession{}).
				Where("id IN ? AND status IN ? AND srs_stream_id <> '' AND stream_ready_deadline_at > ?", ids, statuses, now).
				Where("stream_probe_next_at > 0 AND stream_probe_next_at <= ?", now).
				Update("stream_probe_next_at", leaseUntil)
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected != int64(len(sessions)) {
				return fmt.Errorf("批量认领流就绪任务不完整: want=%d got=%d", len(sessions), claim.RowsAffected)
			}
			for index := range sessions {
				sessions[index].StreamProbeNextAt = leaseUntil
			}
			return nil
		})
		return sessions, err
	}

	// SQLite等不支持SKIP LOCKED的数据库保留逐行CAS，测试和单实例部署仍具备正确租约语义。
	var candidateIDs []uint
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status IN ? AND srs_stream_id <> '' AND stream_ready_deadline_at > ?", statuses, now).
		Where("stream_probe_next_at > 0 AND stream_probe_next_at <= ?", now).
		Order("stream_probe_next_at ASC, id ASC").Limit(limit).Pluck("id", &candidateIDs).Error; err != nil {
		return nil, err
	}
	claimedIDs := make([]uint, 0, len(candidateIDs))
	for _, sessionID := range candidateIDs {
		claim := global.GVA_DB.Model(&liveModel.LiveSession{}).
			Where("id = ? AND status IN ? AND srs_stream_id <> '' AND stream_ready_deadline_at > ?", sessionID, statuses, now).
			Where("stream_probe_next_at > 0 AND stream_probe_next_at <= ?", now).
			Update("stream_probe_next_at", leaseUntil)
		if claim.Error != nil {
			return nil, claim.Error
		}
		if claim.RowsAffected == 1 {
			claimedIDs = append(claimedIDs, sessionID)
		}
	}
	if len(claimedIDs) == 0 {
		return nil, nil
	}
	var sessions []liveModel.LiveSession
	if err := global.GVA_DB.Select(
		"id", "room_id", "anchor_id", "status", "srs_stream_id", "srs_vhost", "srs_app",
		"stream_ready_deadline_at", "stream_probe_attempts", "stream_probe_next_at",
	).Where("id IN ?", claimedIDs).Order("id ASC").Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *RoomService) recordStreamProbeBatchFailureConcurrent(
	sessions []liveModel.LiveSession,
	now int64,
	probeErr error,
) []error {
	ids := make([]uint, 0, len(sessions))
	byID := make(map[uint]liveModel.LiveSession, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
		byID[session.ID] = session
	}
	return runLiveSessionIDWorkers(ids, liveStateTransitionConcurrency(), func(sessionID uint) error {
		session := byID[sessionID]
		observedAt := observedUnixMilli(now)
		if session.StreamReadyDeadlineAt <= observedAt {
			return s.expireStreamReadiness(session.ID, observedAt)
		}
		return s.recordStreamProbeFailure(session, session.StreamProbeNextAt, observedAt, probeErr)
	})
}

func (s *RoomService) applyStreamReadinessSnapshots(
	sessions []liveModel.LiveSession,
	snapshotByID map[string]srsclient.PublisherSnapshot,
	generation uint64,
	now int64,
) []error {
	ids := make([]uint, 0, len(sessions))
	byID := make(map[uint]liveModel.LiveSession, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
		byID[session.ID] = session
	}
	return runLiveSessionIDWorkers(ids, liveStateTransitionConcurrency(), func(sessionID uint) error {
		session := byID[sessionID]
		completedAt := observedUnixMilli(now)
		// 批量处理本身也可能跨过硬截止时间；每个场次按真正开始落库的时间再次判断。
		if session.StreamReadyDeadlineAt <= completedAt {
			return s.expireStreamReadiness(session.ID, completedAt)
		}
		snapshot, exists := snapshotByID[strings.TrimSpace(session.SRSStreamID)]
		if exists && snapshot.Active && snapshot.Width > 0 && snapshot.Height > 0 {
			if err := s.applyReadyPublisherSnapshot(session.ID, snapshot, generation, completedAt); err == nil {
				return nil
			} else if !errors.Is(err, ErrLiveSessionStateInvalid) {
				return err
			}
			return s.recordStreamProbeFailure(
				session, session.StreamProbeNextAt, completedAt,
				errors.New("SRS发布流身份与当前场次不匹配"),
			)
		}
		reason := errors.New("SRS发布流暂未出现")
		if exists && !snapshot.Active {
			reason = errors.New("SRS发布流尚未处于active状态")
		} else if exists {
			reason = fmt.Errorf("SRS尚未返回有效视频尺寸: width=%d height=%d", snapshot.Width, snapshot.Height)
		}
		return s.recordStreamProbeFailure(session, session.StreamProbeNextAt, completedAt, reason)
	})
}

func hasPendingStreamReadiness(now int64) (bool, error) {
	statuses := []int{int(liveModel.LiveSessionPreparing), int(liveModel.LiveSessionLiving)}
	var count int64
	err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status IN ? AND srs_stream_id <> ''", statuses).
		Where("(stream_ready_deadline_at > 0 AND stream_ready_deadline_at <= ?) OR "+
			"(stream_ready_deadline_at > ? AND stream_probe_next_at > 0 AND stream_probe_next_at <= ?)", now, now, now).
		Limit(1).Count(&count).Error
	return count > 0, err
}

// ProcessLiveStreamReconciliation 批量核对已经稳定进入直播中的场次是否仍存在于SRS。
// 只有SRS列表查询成功时才会累计缺失次数；网络错误、超时或SRS业务错误只记录错误，
// 不会把正常直播误判为断流。连续缺失达到配置阈值后，场次进入与on_unpublish相同的重连窗口。
func (s *RoomService) ProcessLiveStreamReconciliation(now int64) error {
	intervalMillis := liveStreamReconcileIntervalMillis()
	eligibleBefore := now - intervalMillis
	batchSize := liveStreamReconcileBatchSize()
	if batchSize <= 0 {
		return nil
	}

	candidates, err := loadLiveStreamReconcileCandidates(eligibleBefore, batchSize)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}

	controller, err := s.liveSRSController()
	if err != nil {
		_ = observeLiveSRSFailure(observedUnixMilli(now), err)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveSRSRequestTimeout())
	defer cancel()
	publisherBatch, err := s.loadPublisherSnapshotBatch(ctx, controller)
	completedAt := observedUnixMilli(now)
	observation, observeErr := observePublisherBatch(publisherBatch, completedAt, err)
	err = errors.Join(err, observeErr)
	if err != nil {
		// 故障事实只写全局live_srs_runtime；不再为同一次单实例故障更新最多一万条场次。
		// checked_at保持不变，恢复信号到来时这些场次会立即重新进入对账。
		return err
	}

	snapshotByID := make(map[string]srsclient.PublisherSnapshot, len(publisherBatch.Publishers))
	for _, snapshot := range publisherBatch.Publishers {
		if id := strings.TrimSpace(snapshot.StreamID); id != "" {
			snapshotByID[id] = snapshot
		}
	}
	var runErrors []error
	maxBatches := liveStreamReconcileMaxBatchesPerRun()
	processed := 0
	for batch := 0; ; batch++ {
		processed += len(candidates)
		if err = s.applyLiveStreamReconcileBatch(
			candidates, snapshotByID, eligibleBefore, completedAt, observation,
		); err != nil {
			runErrors = append(runErrors, err)
			break
		}
		if len(candidates) < batchSize {
			break
		}
		if batch+1 >= maxBatches {
			if pending, pendingErr := hasPendingLiveStreamReconciliation(eligibleBefore); pendingErr != nil {
				runErrors = append(runErrors, pendingErr)
			} else if pending {
				runErrors = append(runErrors, fmt.Errorf(
					"%w: 直播流对账单轮已处理%d项", ErrLiveTaskCapacityExceeded, processed,
				))
			}
			break
		}
		candidates, err = loadLiveStreamReconcileCandidates(eligibleBefore, batchSize)
		if err != nil {
			runErrors = append(runErrors, err)
			break
		}
		if len(candidates) == 0 {
			break
		}
	}
	return summarizeLiveBatchErrors("直播流对账", runErrors)
}

func hasPendingLiveStreamReconciliation(eligibleBefore int64) (bool, error) {
	var count int64
	err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND reconnect_deadline_at = 0 AND stream_ready_deadline_at = 0", liveModel.LiveSessionLiving).
		Where("srs_stream_id <> '' AND stream_reconcile_checked_at <= ?", eligibleBefore).
		Limit(1).Count(&count).Error
	return count > 0, err
}

func loadLiveStreamReconcileCandidates(eligibleBefore int64, batchSize int) ([]liveModel.LiveSession, error) {
	var candidates []liveModel.LiveSession
	err := global.GVA_DB.
		Select(
			"id", "room_id", "anchor_id", "srs_server_id", "srs_stream_id", "srs_vhost", "srs_app",
			"srs_generation",
			"stream_reconcile_checked_at", "stream_reconcile_missing_count",
		).
		Where("status = ? AND reconnect_deadline_at = 0 AND stream_ready_deadline_at = 0", liveModel.LiveSessionLiving).
		Where("srs_stream_id <> '' AND stream_reconcile_checked_at <= ?", eligibleBefore).
		Order("stream_reconcile_checked_at ASC, id ASC").Limit(batchSize).
		Find(&candidates).Error
	return candidates, err
}

func (s *RoomService) applyLiveStreamReconcileBatch(
	candidates []liveModel.LiveSession,
	snapshotByID map[string]srsclient.PublisherSnapshot,
	eligibleBefore, completedAt int64,
	observation liveSRSObservation,
) error {
	roomIDs := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		roomIDs = append(roomIDs, candidate.RoomId)
	}
	var rooms []liveModel.LiveRoom
	if err := global.GVA_DB.Select("id", "anchor_id", "current_session_id", "stream_name").
		Where("id IN ?", roomIDs).Find(&rooms).Error; err != nil {
		return err
	}
	roomByID := make(map[uint]liveModel.LiveRoom, len(rooms))
	for _, room := range rooms {
		roomByID[room.ID] = room
	}

	threshold := uint32(liveStreamReconcileMissingThreshold())
	presentIDs := make([]uint, 0, len(candidates))
	missingIDs := make([]uint, 0, len(candidates))
	mismatchIDs := make([]uint, 0)
	transitionCandidates := make([]liveModel.LiveSession, 0)
	var runErrors []error
	for _, candidate := range candidates {
		room, roomExists := roomByID[candidate.RoomId]
		if !roomExists || room.CurrentSessionId != candidate.ID || room.AnchorId != candidate.AnchorId {
			continue
		}
		snapshot, exists := snapshotByID[strings.TrimSpace(candidate.SRSStreamID)]
		if exists && snapshot.Active && publisherSnapshotMatches(candidate, room, snapshot) {
			presentIDs = append(presentIDs, candidate.ID)
			continue
		}
		instanceInvalidated := observation.Restarted && strings.TrimSpace(candidate.SRSServerID) != "" &&
			strings.TrimSpace(candidate.SRSServerID) != strings.TrimSpace(observation.Runtime.ServerID)
		if instanceInvalidated || candidate.StreamReconcileMissingCount+1 >= threshold {
			if instanceInvalidated && candidate.StreamReconcileMissingCount+1 < threshold {
				candidate.StreamReconcileMissingCount = threshold - 1
			}
			transitionCandidates = append(transitionCandidates, candidate)
			continue
		}
		if exists && snapshot.Active {
			mismatchIDs = append(mismatchIDs, candidate.ID)
		} else {
			missingIDs = append(missingIDs, candidate.ID)
		}
	}

	// 正常存在和尚未达到断流阈值的场次都只做条件批量更新；只有真正切换到重连窗口时逐场加锁事务。
	if err := bulkUpdateLiveStreamPresent(
		presentIDs, eligibleBefore, completedAt, observation.Runtime.Generation,
	); err != nil {
		runErrors = append(runErrors, err)
	}
	if err := bulkUpdateLiveStreamMissing(missingIDs, eligibleBefore, completedAt, threshold, ""); err != nil {
		runErrors = append(runErrors, err)
	}
	if err := bulkUpdateLiveStreamMissing(
		mismatchIDs, eligibleBefore, completedAt, threshold,
		"SRS存在相同stream ID，但vhost、app或stream与当前场次不匹配",
	); err != nil {
		runErrors = append(runErrors, err)
	}
	for _, candidate := range transitionCandidates {
		snapshot, exists := snapshotByID[strings.TrimSpace(candidate.SRSStreamID)]
		forceMissing := observation.Restarted && strings.TrimSpace(candidate.SRSServerID) != "" &&
			strings.TrimSpace(candidate.SRSServerID) != strings.TrimSpace(observation.Runtime.ServerID)
		if err := s.applyLiveStreamReconcileObservation(
			candidate, snapshot, exists, eligibleBefore, completedAt, forceMissing,
		); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
			runErrors = append(runErrors, err)
		}
	}
	return errors.Join(runErrors...)
}

func liveStreamReconcileBaseQuery(ids []uint, eligibleBefore int64) *gorm.DB {
	return global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("id IN ? AND status = ? AND reconnect_deadline_at = 0 AND stream_ready_deadline_at = 0",
			ids, liveModel.LiveSessionLiving).
		Where("srs_stream_id <> '' AND stream_reconcile_checked_at <= ?", eligibleBefore)
}

func bulkUpdateLiveStreamPresent(ids []uint, eligibleBefore, checkedAt int64, generation uint64) error {
	if len(ids) == 0 {
		return nil
	}
	updates := map[string]interface{}{
		"stream_reconcile_checked_at":    checkedAt,
		"stream_reconcile_last_error":    "",
		"stream_reconcile_missing_count": uint32(0),
		"stream_reconcile_last_seen_at":  checkedAt,
		"media_state":                    liveModel.LiveMediaPublishing,
		"media_state_changed_at":         checkedAt,
		"media_last_confirmed_at":        checkedAt,
	}
	if generation > 0 {
		updates["srs_generation"] = generation
	}
	return liveStreamReconcileBaseQuery(ids, eligibleBefore).Updates(updates).Error
}

func bulkUpdateLiveStreamMissing(
	ids []uint,
	eligibleBefore, checkedAt int64,
	threshold uint32,
	lastError string,
) error {
	if len(ids) == 0 {
		return nil
	}
	return liveStreamReconcileBaseQuery(ids, eligibleBefore).
		Where("stream_reconcile_missing_count < ?", threshold-1).
		Updates(map[string]interface{}{
			"stream_reconcile_checked_at":    checkedAt,
			"stream_reconcile_last_error":    lastError,
			"stream_reconcile_missing_count": gorm.Expr("stream_reconcile_missing_count + 1"),
		}).Error
}

func (s *RoomService) recordLiveStreamReconcileBatchFailure(
	candidates []liveModel.LiveSession,
	eligibleBefore, checkedAt int64,
	reconcileErr error,
) error {
	message := truncateRunes(reconcileErr.Error(), 500)
	ids := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return liveStreamReconcileBaseQuery(ids, eligibleBefore).Updates(map[string]interface{}{
		"stream_reconcile_checked_at": checkedAt,
		"stream_reconcile_last_error": message,
	}).Error
}

func (s *RoomService) recordLiveStreamReconcileFailureBatches(
	candidates []liveModel.LiveSession,
	eligibleBefore, checkedAt int64,
	batchSize int,
	reconcileErr error,
) error {
	var runErrors []error
	for batch := 0; ; batch++ {
		if err := s.recordLiveStreamReconcileBatchFailure(
			candidates, eligibleBefore, checkedAt, reconcileErr,
		); err != nil {
			runErrors = append(runErrors, err)
			break
		}
		if len(candidates) < batchSize || batch+1 >= liveStreamReconcileMaxBatchesPerRun() {
			break
		}
		var err error
		candidates, err = loadLiveStreamReconcileCandidates(eligibleBefore, batchSize)
		if err != nil {
			runErrors = append(runErrors, err)
			break
		}
		if len(candidates) == 0 {
			break
		}
	}
	return errors.Join(runErrors...)
}

func (s *RoomService) applyLiveStreamReconcileObservation(
	candidate liveModel.LiveSession,
	snapshot srsclient.PublisherSnapshot,
	snapshotExists bool,
	eligibleBefore, checkedAt int64,
	forceMissing bool,
) error {
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id").First(&sessionRef, candidate.ID).Error; err != nil {
			return normalizeSessionError(err)
		}
		room, err := getRoomByIDForUpdate(tx, sessionRef.RoomId)
		if err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, candidate.ID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if room.CurrentSessionId != session.ID || room.AnchorId != session.AnchorId ||
			session.Status != liveModel.LiveSessionLiving || session.ReconnectDeadlineAt != 0 ||
			session.StreamReadyDeadlineAt != 0 || strings.TrimSpace(session.SRSStreamID) == "" ||
			strings.TrimSpace(session.SRSStreamID) != strings.TrimSpace(candidate.SRSStreamID) ||
			session.StreamReconcileCheckedAt > eligibleBefore {
			return ErrLiveSessionStateInvalid
		}

		publisherPresent := snapshotExists && snapshot.Active && publisherSnapshotMatches(session, room, snapshot)
		updates := map[string]interface{}{
			"stream_reconcile_checked_at": checkedAt,
			"stream_reconcile_last_error": "",
		}
		if publisherPresent {
			updates["stream_reconcile_missing_count"] = uint32(0)
			updates["stream_reconcile_last_seen_at"] = checkedAt
			return tx.Model(&liveModel.LiveSession{}).Where("id = ?", session.ID).Updates(updates).Error
		}

		missingCount := session.StreamReconcileMissingCount
		threshold := uint32(liveStreamReconcileMissingThreshold())
		if forceMissing {
			missingCount = threshold
		} else if missingCount < threshold {
			missingCount++
		}
		updates["stream_reconcile_missing_count"] = missingCount
		if snapshotExists && snapshot.Active {
			updates["stream_reconcile_last_error"] = "SRS存在相同stream ID，但vhost、app或stream与当前场次不匹配"
		}
		if missingCount >= threshold {
			updates["last_unpublish_at"] = checkedAt
			updates["reconnect_deadline_at"] = checkedAt + int64(liveReconnectWindowSeconds())*1000
			updates["disconnect_count"] = gorm.Expr("disconnect_count + 1")
			updates["media_state"] = liveModel.LiveMediaDisconnected
			updates["media_state_changed_at"] = checkedAt
			updates["media_last_confirmed_at"] = checkedAt
		}
		return tx.Model(&liveModel.LiveSession{}).Where("id = ?", session.ID).Updates(updates).Error
	})
}

func (s *RoomService) recordStreamProbeBatchFailure(
	sessions []liveModel.LiveSession,
	claimedNextAt int64,
	now int64,
	probeErr error,
	runErrors *[]error,
) {
	for _, session := range sessions {
		if session.StreamReadyDeadlineAt <= now {
			if err := s.expireStreamReadiness(session.ID, now); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
				*runErrors = append(*runErrors, err)
			}
			continue
		}
		if err := s.recordStreamProbeFailure(session, claimedNextAt, now, probeErr); err != nil {
			*runErrors = append(*runErrors, err)
		}
	}
}

func (s *RoomService) recordStreamProbeFailure(
	session liveModel.LiveSession,
	claimedNextAt int64,
	now int64,
	probeErr error,
) error {
	attempt := session.StreamProbeAttempts + 1
	nextAt := now + liveStreamProbeRetryMillis(attempt)
	if nextAt > session.StreamReadyDeadlineAt {
		nextAt = session.StreamReadyDeadlineAt
	}
	result := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("id = ? AND status IN ? AND srs_stream_id = ? AND stream_probe_next_at = ?",
			session.ID, []int{int(liveModel.LiveSessionPreparing), int(liveModel.LiveSessionLiving)},
			session.SRSStreamID, claimedNextAt).
		Updates(map[string]interface{}{
			"stream_probe_attempts": attempt, "stream_probe_next_at": nextAt,
			"stream_probe_last_error": truncateRunes(probeErr.Error(), 500),
		})
	return result.Error
}

func (s *RoomService) expireStreamReadiness(sessionID uint, now int64) error {
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id").First(&sessionRef, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		room, err := getRoomByIDForUpdate(tx, sessionRef.RoomId)
		if err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if session.StreamReadyDeadlineAt <= 0 || session.StreamReadyDeadlineAt > now || !hasSRSPublisher(session) {
			return ErrLiveSessionStateInvalid
		}
		if room.CurrentSessionId != session.ID || room.AnchorId != session.AnchorId {
			return ErrLiveSessionStateInvalid
		}
		failureReason := fmt.Sprintf("推流后%d秒未获取到有效视频尺寸", global.GVA_CONFIG.Live.StreamReadyTimeoutSeconds)
		if global.GVA_CONFIG.Live.StreamReadyTimeoutSeconds <= 0 {
			failureReason = fmt.Sprintf("推流后%d秒未获取到有效视频尺寸", defaultStreamReadyTimeoutSeconds)
		}
		switch session.Status {
		case liveModel.LiveSessionPreparing:
			if err := markPreparingPublisherEnding(
				tx, session.ID, liveModel.LiveSessionEndPrepareTimeout, failureReason, now,
			); err != nil {
				return err
			}
		case liveModel.LiveSessionLiving:
			if err := markSessionEnding(
				tx, session.ID, liveModel.LiveSessionEndSystemError, failureReason, now, now,
			); err != nil {
				return err
			}
		default:
			return ErrLiveSessionStateInvalid
		}
		roomUpdate := tx.Model(&liveModel.LiveRoom{}).
			Where("id = ? AND current_session_id = ?", session.RoomId, session.ID).
			Update("live_status", liveModel.LiveRoomEnding)
		if roomUpdate.Error != nil {
			return roomUpdate.Error
		}
		if roomUpdate.RowsAffected != 1 {
			return ErrLiveSessionStateInvalid
		}
		return nil
	})
}

func (s *RoomService) applyReadyPublisherSnapshot(
	sessionID uint,
	snapshot srsclient.PublisherSnapshot,
	generation uint64,
	now int64,
) error {
	if !snapshot.Active || snapshot.Width <= 0 || snapshot.Height <= 0 {
		return ErrLiveSessionStateInvalid
	}
	streamInfo, err := json.Marshal(publicStreamSnapshot{
		VideoCodec: snapshot.VideoCodec, VideoProfile: snapshot.VideoProfile,
		VideoLevel: snapshot.VideoLevel, Width: snapshot.Width, Height: snapshot.Height,
		AudioCodec: snapshot.AudioCodec, AudioProfile: snapshot.AudioProfile,
		AudioSampleRate: snapshot.AudioSampleRate, AudioChannels: snapshot.AudioChannels,
		CollectedAt: now,
	})
	if err != nil {
		return err
	}

	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id", "anchor_id").First(&sessionRef, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		var anchor liveModel.LiveAnchor
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&anchor, sessionRef.AnchorId).Error; err != nil {
			return err
		}
		room, err := getRoomByIDForUpdate(tx, sessionRef.RoomId)
		if err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if !publisherSnapshotMatches(session, room, snapshot) {
			return ErrLiveSessionStateInvalid
		}

		switch session.Status {
		case liveModel.LiveSessionPreparing:
			if session.StreamReadyDeadlineAt <= 0 || session.StreamReadyDeadlineAt <= now {
				return ErrLiveSessionStateInvalid
			}
			result := tx.Model(&liveModel.LiveSession{}).
				Where("id = ? AND status = ? AND srs_stream_id = ?", session.ID, liveModel.LiveSessionPreparing, snapshot.StreamID).
				Updates(map[string]interface{}{
					"status": liveModel.LiveSessionLiving, "started_at": now,
					"prepare_deadline_at": int64(0), "reconnect_deadline_at": int64(0),
					"stream_info": streamInfo, "stream_ready_deadline_at": int64(0),
					"stream_probe_attempts": uint32(0), "stream_probe_next_at": int64(0),
					"stream_probe_last_error": "", "stream_reconcile_checked_at": now,
					"stream_reconcile_missing_count": uint32(0), "stream_reconcile_last_seen_at": now,
					"stream_reconcile_last_error": "",
					"media_state":                 liveModel.LiveMediaPublishing,
					"media_state_changed_at":      now, "media_last_confirmed_at": now,
					"srs_generation": generation,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrLiveSessionStateInvalid
			}
			if err = tx.Model(&liveModel.LiveAnchor{}).Where("id = ?", session.AnchorId).
				Update("last_live_at", now).Error; err != nil {
				return err
			}
			roomUpdate := tx.Model(&liveModel.LiveRoom{}).
				Where("id = ? AND current_session_id = ?", room.ID, session.ID).
				Updates(map[string]interface{}{
					"live_status": liveModel.LiveRoomLiving, "live_started_at": now,
				})
			if roomUpdate.Error != nil {
				return roomUpdate.Error
			}
			if roomUpdate.RowsAffected != 1 {
				return ErrLiveSessionStateInvalid
			}
			return nil
		case liveModel.LiveSessionLiving:
			if session.StreamReadyDeadlineAt <= 0 || session.StreamReadyDeadlineAt <= now {
				return ErrLiveSessionStateInvalid
			}
			result := tx.Model(&liveModel.LiveSession{}).
				Where("id = ? AND status = ? AND srs_stream_id = ?", session.ID, liveModel.LiveSessionLiving, snapshot.StreamID).
				Updates(map[string]interface{}{
					"stream_info": streamInfo, "stream_ready_deadline_at": int64(0),
					"stream_probe_attempts": uint32(0), "stream_probe_next_at": int64(0),
					"stream_probe_last_error": "", "stream_reconcile_checked_at": now,
					"stream_reconcile_missing_count": uint32(0), "stream_reconcile_last_seen_at": now,
					"stream_reconcile_last_error": "",
					"media_state":                 liveModel.LiveMediaPublishing,
					"media_state_changed_at":      now, "media_last_confirmed_at": now,
					"srs_generation": generation,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrLiveSessionStateInvalid
			}
			return nil
		default:
			return ErrLiveSessionStateInvalid
		}
	})
}

func publisherSnapshotMatches(
	session liveModel.LiveSession,
	room liveModel.LiveRoom,
	snapshot srsclient.PublisherSnapshot,
) bool {
	if room.CurrentSessionId != session.ID || room.AnchorId != session.AnchorId {
		return false
	}
	if strings.TrimSpace(snapshot.StreamID) == "" || strings.TrimSpace(session.SRSStreamID) != strings.TrimSpace(snapshot.StreamID) {
		return false
	}
	if strings.TrimSpace(snapshot.Stream) != strings.TrimSpace(room.StreamName) {
		return false
	}
	if expected := strings.Trim(strings.TrimSpace(session.SRSApp), "/"); expected != "" && expected != strings.Trim(strings.TrimSpace(snapshot.App), "/") {
		return false
	}
	if expected := strings.TrimSpace(session.SRSVhost); expected != "" && !strings.EqualFold(expected, strings.TrimSpace(snapshot.Vhost)) {
		return false
	}
	return true
}

// processEndingSession 使用stop_next_retry_at作为数据库租约。
// 人工请求和一个或多个服务实例上的扫描任务可以同时进入，但只有一个执行者能调用SRS。
func (s *RoomService) processEndingSession(sessionID uint, now int64) error {
	controller, controllerErr := s.liveSRSController()
	return s.processEndingSessionWithController(sessionID, now, controller, controllerErr)
}

func (s *RoomService) processEndingSessionWithController(
	sessionID uint,
	now int64,
	controller srsclient.Controller,
	controllerErr error,
) error {
	leaseUntil := now + liveStopLeaseMillis()
	claim := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("id = ? AND status = ? AND stop_next_retry_at <= ?", sessionID, liveModel.LiveSessionEnding, now).
		Updates(map[string]interface{}{
			"stop_attempts":      gorm.Expr("stop_attempts + 1"),
			"stop_next_retry_at": leaseUntil,
		})
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil
	}

	var session liveModel.LiveSession
	if err := global.GVA_DB.First(&session, sessionID).Error; err != nil {
		return normalizeSessionError(err)
	}
	if session.Status != liveModel.LiveSessionEnding {
		return nil
	}
	// 人工确认是已经持久化并审计的媒体事实。若进程恰好在确认事务提交后、最终结算前
	// 退出，恢复扫描必须直接完成幂等结算，不能再次依赖仍处于故障中的 SRS。
	if mediaStopAlreadyConfirmed(session) {
		return s.finalizeSession(session.ID, observedUnixMilli(now))
	}
	var room liveModel.LiveRoom
	if err := global.GVA_DB.Select("id", "stream_name").First(&room, session.RoomId).Error; err != nil {
		return errors.Join(normalizeRoomError(err), s.recordStopFailure(session, observedUnixMilli(now), err))
	}

	err := controllerErr
	if err == nil {
		app := strings.Trim(strings.TrimSpace(session.SRSApp), "/")
		if app == "" {
			app = strings.Trim(strings.TrimSpace(global.GVA_CONFIG.Live.SRS.App), "/")
		}
		vhost := strings.TrimSpace(session.SRSVhost)
		if vhost == "" {
			if pushURL, parseErr := url.Parse(strings.TrimSpace(global.GVA_CONFIG.Live.SRS.PushBaseURL)); parseErr == nil {
				vhost = pushURL.Hostname()
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), liveSRSRequestTimeout())
		defer cancel()
		err = controller.StopPublisher(ctx, srsclient.StreamRef{
			StreamID: session.SRSStreamID,
			Vhost:    vhost,
			App:      app,
			Stream:   room.StreamName,
		})
	}
	completedAt := observedUnixMilli(now)
	if err != nil {
		_ = observeLiveSRSFailure(completedAt, err)
		return errors.Join(err, s.recordStopFailure(session, completedAt, err))
	}
	_, _ = observeLiveSRSSuccess(srsclient.RuntimeIdentity{}, completedAt)
	return s.finalizeSession(session.ID, completedAt)
}

func (s *RoomService) liveSRSController() (srsclient.Controller, error) {
	if s.srsController != nil {
		return s.srsController, nil
	}
	return srsclient.NewClient(global.GVA_CONFIG.Live.SRS)
}

func (s *RoomService) recordStopFailure(session liveModel.LiveSession, now int64, stopErr error) error {
	message := truncateRunes(stopErr.Error(), 500)
	delay := liveStopRetryMillis(session.StopAttempts)
	if session.StopRecoveryState == liveModel.LiveStopRecoveryManualAttention {
		if manualDelay := int64(liveEndManualRetrySeconds()) * 1000; manualDelay > delay {
			delay = manualDelay
		}
	}
	delay += liveStopRetryJitterMillis(session.ID, session.StopAttempts, delay)
	nextRetryAt := now + delay
	return global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("id = ? AND status = ? AND stop_attempts = ? AND stop_next_retry_at = ?",
			session.ID, liveModel.LiveSessionEnding, session.StopAttempts, session.StopNextRetryAt).
		Updates(map[string]interface{}{
			"stop_next_retry_at": nextRetryAt,
			"stop_last_error":    message,
		}).Error
}

func promoteEndingStallAlerts(now int64) error {
	criticalBefore := now - int64(liveEndCriticalSeconds())*1000
	critical := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND stop_requested_at > 0 AND stop_requested_at <= ? AND ending_alert_level < ?",
			liveModel.LiveSessionEnding, criticalBefore, liveModel.LiveEndingAlertCritical).
		Updates(map[string]interface{}{
			"ending_alert_level":  liveModel.LiveEndingAlertCritical,
			"ending_alerted_at":   now,
			"stop_recovery_state": liveModel.LiveStopRecoveryManualAttention,
		})
	if critical.Error != nil {
		return critical.Error
	}
	warningBefore := now - int64(liveEndWarningSeconds())*1000
	warning := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND stop_requested_at > 0 AND stop_requested_at <= ? AND ending_alert_level < ?",
			liveModel.LiveSessionEnding, warningBefore, liveModel.LiveEndingAlertWarning).
		Updates(map[string]interface{}{
			"ending_alert_level": liveModel.LiveEndingAlertWarning,
			"ending_alerted_at":  now,
		})
	if warning.Error != nil {
		return warning.Error
	}
	if global.GVA_LOG != nil && (critical.RowsAffected > 0 || warning.RowsAffected > 0) {
		global.GVA_LOG.Warn("直播场次Ending滞留告警",
			zap.Int64("newCritical", critical.RowsAffected),
			zap.Int64("newWarning", warning.RowsAffected),
		)
	}
	return nil
}

func liveEndScanBatchSize() int {
	return boundedLiveBatchSize(global.GVA_CONFIG.Live.EndScanBatchSize, defaultEndScanBatchSize)
}

func liveEndScanMaxBatchesPerRun() int {
	return liveTaskMaxBatchesPerRun(global.GVA_CONFIG.Live.EndScanMaxBatchesPerRun, liveEndScanBatchSize())
}

func liveStreamReadyBatchSize() int {
	return boundedLiveBatchSize(global.GVA_CONFIG.Live.StreamReadyBatchSize, liveEndScanBatchSize())
}

func liveStreamReadyMaxBatchesPerRun() int {
	return liveTaskMaxBatchesPerRun(global.GVA_CONFIG.Live.StreamReadyMaxBatchesPerRun, liveStreamReadyBatchSize())
}

func liveStreamReconcileBatchSize() int {
	return boundedLiveBatchSize(global.GVA_CONFIG.Live.StreamReconcileBatchSize, liveEndScanBatchSize())
}

func liveStreamReconcileMaxBatchesPerRun() int {
	return liveTaskMaxBatchesPerRun(
		global.GVA_CONFIG.Live.StreamReconcileMaxBatchesPerRun, liveStreamReconcileBatchSize(),
	)
}

func liveEndStopBatchSize() int {
	return boundedLiveBatchSize(global.GVA_CONFIG.Live.EndStopBatchSize, liveEndScanBatchSize())
}

func liveEndStopMaxBatchesPerRun() int {
	return liveTaskMaxBatchesPerRun(global.GVA_CONFIG.Live.EndStopMaxBatchesPerRun, liveEndStopBatchSize())
}

func liveTaskMaxBatchesPerRun(configured, batchSize int) int {
	batches := configured
	if batches <= 0 {
		batches = defaultTaskMaxBatchesPerRun
	}
	// 配置的单轮总容量不得低于声明的最大并发流数，防止只调小批次后静默产生积压。
	required := (liveMaxConcurrentStreams() + batchSize - 1) / batchSize
	if batches < required {
		batches = required
	}
	// 批次数仍有硬上限，避免极端错误配置变成无界排空；生产建议批量1000，1万路仅需10批。
	if batches > 1000 {
		return 1000
	}
	return batches
}

func liveMaxConcurrentStreams() int {
	maximum := global.GVA_CONFIG.Live.MaxConcurrentStreams
	if maximum <= 0 {
		return defaultMaxConcurrentStreams
	}
	if maximum > 100000 {
		return 100000
	}
	return maximum
}

func liveStateTransitionConcurrency() int {
	workers := global.GVA_CONFIG.Live.StateTransitionConcurrency
	if workers <= 0 {
		workers = defaultStateTransitionConcurrency
	}
	if workers > 64 {
		return 64
	}
	return workers
}

func boundedLiveBatchSize(size, fallback int) int {
	if size <= 0 {
		size = fallback
	}
	if size <= 0 {
		size = defaultEndScanBatchSize
	}
	if size > 1000 {
		return 1000
	}
	return size
}

func liveEndStopConcurrency() int {
	workers := global.GVA_CONFIG.Live.EndStopConcurrency
	if workers <= 0 {
		return defaultEndStopConcurrency
	}
	// 每个worker最多持有一个SRS HTTP连接和一个轻量goroutine，防止错误配置压垮SRS或本服务。
	if workers > 64 {
		return 64
	}
	return workers
}

func livePrepareTimeoutSeconds() int {
	timeout := global.GVA_CONFIG.Live.PrepareTimeoutSeconds
	if timeout <= 0 {
		return defaultPrepareTimeoutSeconds
	}
	return timeout
}

func liveStreamReadyTimeoutMillis() int64 {
	timeout := global.GVA_CONFIG.Live.StreamReadyTimeoutSeconds
	if timeout <= 0 {
		timeout = defaultStreamReadyTimeoutSeconds
	}
	return int64(timeout) * 1000
}

func liveStreamProbeLeaseMillis() int64 {
	return liveSRSRequestTimeout().Milliseconds() + 2000
}

func liveStreamReconcileIntervalMillis() int64 {
	interval := global.GVA_CONFIG.Live.StreamReconcileIntervalSeconds
	if interval <= 0 {
		interval = defaultStreamReconcileIntervalSeconds
	}
	return int64(interval) * 1000
}

func liveStreamReconcileMissingThreshold() int {
	threshold := global.GVA_CONFIG.Live.StreamReconcileMissingThreshold
	if threshold <= 0 {
		return defaultStreamReconcileMissingThreshold
	}
	// 防止错误配置导致计数长期增长；实际业务通常使用2到3次。
	if threshold > 1000 {
		return 1000
	}
	return threshold
}

func liveReconnectWindowSeconds() int {
	window := global.GVA_CONFIG.Live.ReconnectWindowSeconds
	if window <= 0 {
		return 20
	}
	return window
}

func streamReadinessRequestTimeout(sessions []liveModel.LiveSession, now int64) time.Duration {
	timeout := liveSRSRequestTimeout()
	for _, session := range sessions {
		remainingMillis := session.StreamReadyDeadlineAt - now
		if remainingMillis <= 0 {
			return time.Millisecond
		}
		if remaining := time.Duration(remainingMillis) * time.Millisecond; remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return time.Millisecond
	}
	return timeout
}

// observedUnixMilli keeps deterministic synthetic timestamps usable in tests, while ensuring
// production decisions made after an SRS request never move backwards from the real wall clock.
func observedUnixMilli(reference int64) int64 {
	if current := time.Now().UnixMilli(); current > reference {
		return current
	}
	return reference
}

func liveStreamProbeRetryMillis(attempt uint32) int64 {
	base := global.GVA_CONFIG.Live.StreamReadyRetryBaseSeconds
	if base <= 0 {
		base = defaultStreamReadyRetryBaseSeconds
	}
	maximum := global.GVA_CONFIG.Live.StreamReadyRetryMaxSeconds
	if maximum < base {
		maximum = defaultStreamReadyRetryMaxSeconds
		if maximum < base {
			maximum = base
		}
	}
	delay := int64(base)
	for step := uint32(1); step < attempt && delay < int64(maximum); step++ {
		delay *= 2
		if delay > int64(maximum) {
			delay = int64(maximum)
		}
	}
	return delay * 1000
}

func liveStopLeaseMillis() int64 {
	return liveSRSRequestTimeout().Milliseconds() + 2000
}

func liveStopBatchLeaseMillis() int64 {
	lease := int64((60 * time.Second).Milliseconds())
	if minimum := liveStopLeaseMillis(); minimum > lease {
		return minimum
	}
	return lease
}

func liveSRSRequestTimeout() time.Duration {
	timeout := global.GVA_CONFIG.Live.SRS.APIRequestTimeoutSeconds
	if timeout <= 0 {
		timeout = defaultSRSRequestSeconds
	}
	return time.Duration(timeout) * time.Second
}

func liveStopRetryMillis(attempt uint32) int64 {
	base := global.GVA_CONFIG.Live.EndRetryBaseSeconds
	if base <= 0 {
		base = defaultEndRetryBaseSeconds
	}
	maximum := global.GVA_CONFIG.Live.EndRetryMaxSeconds
	if maximum < base {
		maximum = defaultEndRetryMaxSeconds
		if maximum < base {
			maximum = base
		}
	}
	delay := int64(base)
	// attempt已经包含本次失败；采用5、10、20……秒的有上限指数退避。
	for step := uint32(1); step < attempt && delay < int64(maximum); step++ {
		delay *= 2
		if delay > int64(maximum) {
			delay = int64(maximum)
		}
	}
	return delay * 1000
}

func liveEndWarningSeconds() int {
	seconds := global.GVA_CONFIG.Live.EndWarningSeconds
	if seconds <= 0 {
		return defaultEndWarningSeconds
	}
	return seconds
}

func liveEndCriticalSeconds() int {
	seconds := global.GVA_CONFIG.Live.EndCriticalSeconds
	if seconds <= liveEndWarningSeconds() {
		seconds = defaultEndCriticalSeconds
		if seconds <= liveEndWarningSeconds() {
			seconds = liveEndWarningSeconds() + 60
		}
	}
	return seconds
}

func liveEndManualRetrySeconds() int {
	seconds := global.GVA_CONFIG.Live.EndManualRetrySeconds
	if seconds <= 0 {
		return defaultEndManualRetrySeconds
	}
	if seconds > 3600 {
		return 3600
	}
	return seconds
}

// liveStopRetryJitterMillis 使用场次ID和尝试次数生成稳定的0~20%正抖动。
// 无需全局随机锁，也能避免SRS恢复时所有失败场次在同一毫秒再次冲击媒体服务。
func liveStopRetryJitterMillis(sessionID uint, attempt uint32, delay int64) int64 {
	if delay <= 0 {
		return 0
	}
	window := delay / 5
	if window <= 0 {
		return 0
	}
	seed := uint64(sessionID)*11400714819323198485 + uint64(attempt)*0x9e3779b97f4a7c15
	return int64(seed % uint64(window+1))
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (s *RoomService) GetPublicRoom(roomNo string) (liveRes.LiveRoomPublicItemV2, error) {
	// 详情按 room_no 唯一索引命中，主播和场次都按主键关联。
	// current_session_id 优先返回活动场次；没有活动场次时通过 last_session_id 返回最近一次已结束直播。
	// 一次 SQL 只选择公开 DTO 需要的数据，避免 COUNT、分页排序、内部控制字段和大 JSON 扩展字段。
	var row struct {
		Room    liveModel.LiveRoom    `gorm:"embedded;embeddedPrefix:room_"`
		Anchor  liveModel.LiveAnchor  `gorm:"embedded;embeddedPrefix:anchor_"`
		Session liveModel.LiveSession `gorm:"embedded;embeddedPrefix:session_"`
	}
	result := global.GVA_DB.Table("live_room AS r").
		Select(`
			r.room_no AS room_room_no,
			r.category_id AS room_category_id,
			r.title AS room_title,
			r.cover_url AS room_cover_url,
			r.notice AS room_notice,
			r.stream_name AS room_stream_name,
			r.live_status AS room_live_status,
			a.anchor_no AS anchor_anchor_no,
			a.nickname AS anchor_nickname,
			a.avatar AS anchor_avatar,
			a.cover AS anchor_cover,
			a.signature AS anchor_signature,
			a.gender AS anchor_gender,
			a.country_code AS anchor_country_code,
			a.region_code AS anchor_region_code,
			a.city_code AS anchor_city_code,
			a.language AS anchor_language,
			a.anchor_type AS anchor_anchor_type,
			a.category_id AS anchor_category_id,
			a.level AS anchor_level,
			a.tag_ids AS anchor_tag_ids,
			a.cert_status AS anchor_cert_status,
			a.cert_type AS anchor_cert_type,
			a.cert_name AS anchor_cert_name,
			a.is_signed AS anchor_is_signed,
			a.fans_count AS anchor_fans_count,
			a.total_live_count AS anchor_total_live_count,
			a.total_live_duration_ms AS anchor_total_live_duration_ms,
			a.max_online_count AS anchor_max_online_count,
			a.total_view_count AS anchor_total_view_count,
			a.last_live_at AS anchor_last_live_at,
			a.is_recommended AS anchor_is_recommended,
			COALESCE(ls.session_no, '') AS session_session_no,
			COALESCE(ls.category_id, 0) AS session_category_id,
			COALESCE(ls.title, '') AS session_title,
			COALESCE(ls.cover_url, '') AS session_cover_url,
			ls.stream_info AS session_stream_info,
			COALESCE(ls.status, 0) AS session_status,
			COALESCE(ls.started_at, 0) AS session_started_at,
			COALESCE(ls.ended_at, 0) AS session_ended_at,
			COALESCE(ls.duration_ms, 0) AS session_duration_ms,
			COALESCE(ls.view_count, 0) AS session_view_count,
			COALESCE(ls.viewer_count, 0) AS session_viewer_count,
			COALESCE(ls.peak_online_count, 0) AS session_peak_online_count,
			COALESCE(ls.like_count, 0) AS session_like_count`).
		Joins("JOIN live_anchor AS a ON a.id = r.anchor_id AND a.deleted_at IS NULL").
		Joins(`LEFT JOIN live_session AS ls
			ON ls.id = CASE WHEN r.current_session_id > 0 THEN r.current_session_id ELSE r.last_session_id END
			AND ls.deleted_at IS NULL`).
		Where("r.deleted_at IS NULL AND r.room_no = ?", strings.TrimSpace(roomNo)).
		Where("r.status = ? AND r.visibility = ?", liveModel.LiveRoomStatusNormal, liveModel.LiveRoomVisibilityPublic).
		Where("a.status = ? AND a.apply_status = ?", liveModel.AnchorStatusNormal, liveModel.AnchorApplyStatusApproved).
		Limit(1).
		Scan(&row)
	if result.Error != nil {
		return liveRes.LiveRoomPublicItemV2{}, result.Error
	}
	if result.RowsAffected == 0 {
		return liveRes.LiveRoomPublicItemV2{}, ErrLiveRoomNotFound
	}

	var latestSession liveRes.LiveSessionPublicSummary
	if row.Session.SessionNo != "" {
		latestSession = liveRes.LiveSessionPublicSummary{
			SessionNo: row.Session.SessionNo, CategoryId: row.Session.CategoryId,
			Title: row.Session.Title, CoverURL: row.Session.CoverURL, StreamInfo: publicStreamInfo(row.Session.StreamInfo),
			Status:    row.Session.Status,
			StartedAt: row.Session.StartedAt, EndedAt: row.Session.EndedAt, DurationMs: row.Session.DurationMs,
			ViewCount: row.Session.ViewCount, ViewerCount: row.Session.ViewerCount,
			PeakOnlineCount: row.Session.PeakOnlineCount, LikeCount: row.Session.LikeCount,
		}
	}
	return liveRes.LiveRoomPublicItemV2{
		RoomNo: row.Room.RoomNo, CategoryId: row.Room.CategoryId, Title: row.Room.Title,
		CoverURL: row.Room.CoverURL, Notice: row.Room.Notice, StreamName: row.Room.StreamName,
		LiveStatus: row.Room.LiveStatus, AnchorInfo: toAnchorPublicDetail(row.Anchor),
		LatestSession: latestSession,
	}, nil
}

func (s *RoomService) GetPublicRoomList(req liveReq.LiveRoomPublicListReq) ([]liveRes.LiveRoomPublicItem, int64, error) {
	return s.getPublicRooms("", req.CategoryId, req.Page, req.PageSize, true)
}

func (s *RoomService) getPublicRooms(roomNo string, categoryID uint64, page, pageSize int, livingOnly bool) ([]liveRes.LiveRoomPublicItem, int64, error) {
	sessionJoin := "LEFT JOIN live_session AS s ON s.id = r.current_session_id AND s.deleted_at IS NULL"
	if livingOnly {
		sessionJoin = "JOIN live_session AS s ON s.id = r.current_session_id AND s.deleted_at IS NULL"
	}
	db := global.GVA_DB.Table("live_room AS r").
		Joins("JOIN live_anchor AS a ON a.id = r.anchor_id AND a.deleted_at IS NULL").
		Joins(sessionJoin).
		Where("r.deleted_at IS NULL AND r.status = ? AND r.visibility = ?", liveModel.LiveRoomStatusNormal, liveModel.LiveRoomVisibilityPublic).
		Where("a.status = ? AND a.apply_status = ?", liveModel.AnchorStatusNormal, liveModel.AnchorApplyStatusApproved)
	if livingOnly {
		db = db.Where("r.live_status = ?", liveModel.LiveRoomLiving)
	}
	if roomNo != "" {
		db = db.Where("r.room_no = ?", roomNo)
	}
	if categoryID > 0 {
		db = db.Where("r.category_id = ?", categoryID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	var rows []struct {
		RoomNo         string
		AnchorNo       string
		AnchorNickname string
		AnchorAvatar   string
		CategoryId     uint64
		Title          string
		CoverURL       string
		Notice         string
		LiveStatus     uint8
		LiveStartedAt  int64
		SessionNo      string
		ViewCount      uint64
		LikeCount      uint64
	}
	err := db.Select("r.room_no, a.anchor_no, a.nickname AS anchor_nickname, a.avatar AS anchor_avatar, r.category_id, r.title, r.cover_url, r.notice, r.live_status, r.live_started_at, COALESCE(s.session_no, '') AS session_no, COALESCE(s.view_count, 0) AS view_count, COALESCE(s.like_count, 0) AS like_count").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Order("r.recommend_weight DESC, r.live_started_at DESC, r.id DESC").Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	list := make([]liveRes.LiveRoomPublicItem, 0, len(rows))
	for _, row := range rows {
		list = append(list, liveRes.LiveRoomPublicItem{
			RoomNo: row.RoomNo, AnchorNo: row.AnchorNo, AnchorNickname: row.AnchorNickname, AnchorAvatar: row.AnchorAvatar,
			CategoryId: row.CategoryId, Title: row.Title, CoverURL: row.CoverURL, Notice: row.Notice,
			LiveStatus: row.LiveStatus, LiveStartedAt: row.LiveStartedAt,
			SessionNo: row.SessionNo, ViewCount: row.ViewCount, LikeCount: row.LikeCount,
		})
	}
	return list, total, nil
}

func (s *RoomService) GetAdminRoomList(req liveReq.LiveRoomAdminListReq) ([]liveRes.LiveRoomAdminItem, int64, error) {
	db := global.GVA_DB.Model(&liveModel.LiveRoom{})
	if req.RoomNo != "" {
		db = db.Where("room_no LIKE ?", "%"+strings.TrimSpace(req.RoomNo)+"%")
	}
	if req.Title != "" {
		db = db.Where("title LIKE ?", "%"+strings.TrimSpace(req.Title)+"%")
	}
	if req.CategoryId > 0 {
		db = db.Where("category_id = ?", req.CategoryId)
	}
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}
	if req.LiveStatus != nil {
		db = db.Where("live_status = ?", *req.LiveStatus)
	}
	if req.AnchorNo != "" {
		var anchorIDs []uint
		if err := global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("anchor_no LIKE ?", "%"+strings.TrimSpace(req.AnchorNo)+"%").Pluck("id", &anchorIDs).Error; err != nil {
			return nil, 0, err
		}
		if len(anchorIDs) == 0 {
			return []liveRes.LiveRoomAdminItem{}, 0, nil
		}
		db = db.Where("anchor_id IN ?", anchorIDs)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rooms []liveModel.LiveRoom
	if err := db.Scopes(req.PageInfo.Paginate()).Order("id DESC").Find(&rooms).Error; err != nil {
		return nil, 0, err
	}
	anchors, err := loadAnchorMap(roomsAnchorIDs(rooms))
	if err != nil {
		return nil, 0, err
	}
	list := make([]liveRes.LiveRoomAdminItem, 0, len(rooms))
	for _, room := range rooms {
		list = append(list, toLiveRoomAdminItem(room, anchors[room.AnchorId]))
	}
	return list, total, nil
}

func (s *RoomService) GetAdminRoomDetail(roomID uint) (liveRes.LiveRoomAdminDetailResp, error) {
	var room liveModel.LiveRoom
	if err := global.GVA_DB.First(&room, roomID).Error; err != nil {
		return liveRes.LiveRoomAdminDetailResp{}, normalizeRoomError(err)
	}
	var anchor liveModel.LiveAnchor
	if err := global.GVA_DB.First(&anchor, room.AnchorId).Error; err != nil {
		return liveRes.LiveRoomAdminDetailResp{}, err
	}
	result := liveRes.LiveRoomAdminDetailResp{LiveRoomAdminItem: toLiveRoomAdminItem(room, anchor)}
	if room.CurrentSessionId == 0 {
		return result, nil
	}
	var session liveModel.LiveSession
	if err := global.GVA_DB.First(&session, room.CurrentSessionId).Error; err != nil {
		return liveRes.LiveRoomAdminDetailResp{}, normalizeSessionError(err)
	}
	current := toLiveSessionAdminItem(session, room, anchor)
	if runtime, runtimeErr := loadLiveSRSRuntime(global.GVA_DB); runtimeErr == nil {
		applySRSRuntimeToAdminItem(&current, session, runtime)
	}
	result.CurrentSession = &current
	return result, nil
}

func (s *RoomService) UpdateAdminRoom(req liveReq.LiveRoomAdminUpdateReq) error {
	roomNo := strings.TrimSpace(req.RoomNo)
	if roomNo == "" {
		return ErrLiveRoomNoRequired
	}
	if err := (&CategoryService{}).EnsureEnabledCategory(req.CategoryId); err != nil {
		return err
	}
	var duplicateCount int64
	if err := global.GVA_DB.Model(&liveModel.LiveRoom{}).
		Where("room_no = ? AND id <> ?", roomNo, req.RoomID).Count(&duplicateCount).Error; err != nil {
		return err
	}
	if duplicateCount > 0 {
		return ErrLiveRoomNoExists
	}
	result := global.GVA_DB.Model(&liveModel.LiveRoom{}).Where("id = ?", req.RoomID).Updates(map[string]interface{}{
		"room_no": roomNo, "category_id": req.CategoryId, "title": strings.TrimSpace(req.Title), "cover_url": strings.TrimSpace(req.CoverURL),
		"notice": strings.TrimSpace(req.Notice), "visibility": *req.Visibility, "recommend_weight": req.RecommendWeight,
	})
	if result.Error != nil {
		return normalizeRoomNoError(result.Error)
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := global.GVA_DB.Model(&liveModel.LiveRoom{}).Where("id = ?", req.RoomID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrLiveRoomNotFound
		}
	}
	return nil
}

func (s *RoomService) UpdateAdminRoomStatus(req liveReq.LiveRoomAdminStatusReq) error {
	status, reason := *req.Status, strings.TrimSpace(req.StatusReason)
	if status == liveModel.LiveRoomStatusDisabled && reason == "" {
		return ErrLiveRoomStatusReasonRequired
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		room, err := getRoomByIDForUpdate(tx, req.RoomID)
		if err != nil {
			return normalizeRoomError(err)
		}
		now := time.Now().UnixMilli()
		liveStatus := room.LiveStatus
		if status != liveModel.LiveRoomStatusNormal && room.CurrentSessionId != 0 {
			var session liveModel.LiveSession
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, room.CurrentSessionId).Error; err != nil {
				return normalizeSessionError(err)
			}
			switch session.Status {
			case liveModel.LiveSessionPreparing:
				if hasSRSPublisher(session) {
					if err = markPreparingPublisherEnding(tx, session.ID, liveModel.LiveSessionEndByAdmin, reason, now); err != nil {
						return err
					}
					liveStatus = liveModel.LiveRoomEnding
				} else {
					if err = cancelPreparingSession(tx, session, liveModel.LiveSessionEndByAdmin, reason, now); err != nil {
						return err
					}
					liveStatus = liveModel.LiveRoomOffline
				}
			case liveModel.LiveSessionLiving:
				if err = markSessionEnding(tx, session.ID, liveModel.LiveSessionEndByAdmin, reason, now, now); err != nil {
					return err
				}
				liveStatus = liveModel.LiveRoomEnding
			case liveModel.LiveSessionEnding:
				liveStatus = liveModel.LiveRoomEnding
			default:
				if err = clearRoomIfCurrent(tx, room.ID, session.ID); err != nil {
					return err
				}
				liveStatus = liveModel.LiveRoomOffline
			}
		}
		if status == liveModel.LiveRoomStatusNormal {
			reason = ""
		}
		updates := map[string]interface{}{"status": status, "status_reason": reason, "status_changed_at": now}
		if status != liveModel.LiveRoomStatusNormal {
			updates["live_status"] = liveStatus
		}
		return tx.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).Updates(updates).Error
	})
}

func (s *RoomService) GetAdminSessionList(req liveReq.LiveSessionAdminListReq) ([]liveRes.LiveSessionAdminItem, int64, error) {
	db := global.GVA_DB.Model(&liveModel.LiveSession{})
	if req.SessionNo != "" {
		db = db.Where("session_no LIKE ?", "%"+strings.TrimSpace(req.SessionNo)+"%")
	}
	if req.CategoryId > 0 {
		db = db.Where("category_id = ?", req.CategoryId)
	}
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}
	if req.StartedAtStart > 0 {
		db = db.Where("started_at >= ?", req.StartedAtStart)
	}
	if req.StartedAtEnd > 0 {
		db = db.Where("started_at <= ?", req.StartedAtEnd)
	}
	if req.RoomNo != "" {
		var roomIDs []uint
		if err := global.GVA_DB.Model(&liveModel.LiveRoom{}).Where("room_no LIKE ?", "%"+strings.TrimSpace(req.RoomNo)+"%").Pluck("id", &roomIDs).Error; err != nil {
			return nil, 0, err
		}
		if len(roomIDs) == 0 {
			return []liveRes.LiveSessionAdminItem{}, 0, nil
		}
		db = db.Where("room_id IN ?", roomIDs)
	}
	if req.AnchorNo != "" {
		var anchorIDs []uint
		if err := global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("anchor_no LIKE ?", "%"+strings.TrimSpace(req.AnchorNo)+"%").Pluck("id", &anchorIDs).Error; err != nil {
			return nil, 0, err
		}
		if len(anchorIDs) == 0 {
			return []liveRes.LiveSessionAdminItem{}, 0, nil
		}
		db = db.Where("anchor_id IN ?", anchorIDs)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var sessions []liveModel.LiveSession
	if err := db.Scopes(req.PageInfo.Paginate()).Order("id DESC").Find(&sessions).Error; err != nil {
		return nil, 0, err
	}
	rooms, err := loadRoomMap(sessionRoomIDs(sessions))
	if err != nil {
		return nil, 0, err
	}
	anchors, err := loadAnchorMap(sessionAnchorIDs(sessions))
	if err != nil {
		return nil, 0, err
	}
	list := make([]liveRes.LiveSessionAdminItem, 0, len(sessions))
	runtime, runtimeErr := loadLiveSRSRuntime(global.GVA_DB)
	if runtimeErr != nil {
		return nil, 0, runtimeErr
	}
	for _, session := range sessions {
		room, anchor := rooms[session.RoomId], anchors[session.AnchorId]
		item := toLiveSessionAdminItem(session, room, anchor)
		applySRSRuntimeToAdminItem(&item, session, runtime)
		list = append(list, item)
	}
	return list, total, nil
}

func (s *RoomService) GetAdminSessionDetail(sessionID uint) (liveRes.LiveSessionAdminItem, error) {
	var session liveModel.LiveSession
	if err := global.GVA_DB.First(&session, sessionID).Error; err != nil {
		return liveRes.LiveSessionAdminItem{}, normalizeSessionError(err)
	}
	var room liveModel.LiveRoom
	var anchor liveModel.LiveAnchor
	if err := global.GVA_DB.First(&room, session.RoomId).Error; err != nil {
		return liveRes.LiveSessionAdminItem{}, err
	}
	if err := global.GVA_DB.First(&anchor, session.AnchorId).Error; err != nil {
		return liveRes.LiveSessionAdminItem{}, err
	}
	item := toLiveSessionAdminItem(session, room, anchor)
	runtime, runtimeErr := loadLiveSRSRuntime(global.GVA_DB)
	if runtimeErr != nil {
		return liveRes.LiveSessionAdminItem{}, runtimeErr
	}
	applySRSRuntimeToAdminItem(&item, session, runtime)
	return item, nil
}

func applySRSRuntimeToAdminItem(
	item *liveRes.LiveSessionAdminItem,
	session liveModel.LiveSession,
	runtime liveModel.LiveSRSRuntime,
) {
	item.SRSHealthState = runtime.HealthState
	item.EffectiveMediaState = effectiveLiveMediaState(session, runtime)
}

func (s *RoomService) RequestAdminEnd(sessionID uint, reason string) error {
	return s.requestEndByQuery("id = ?", []interface{}{sessionID}, liveModel.LiveSessionEndByAdmin, strings.TrimSpace(reason))
}

type MediaStopActor struct {
	UserID    uint
	Username  string
	ClientIP  string
	UserAgent string
	RequestID string
}

// ConfirmMediaStopped 只确认已经处于Ending且长期无法自动收尾的场次。
// 它不提供Living直达Ended的旁路；审计事实与媒体确认在同一事务落库，随后复用幂等结算。
func (s *RoomService) ConfirmMediaStopped(
	req liveReq.LiveSessionConfirmMediaStoppedReq,
	actor MediaStopActor,
) error {
	if req.ExpectedPublisherEpoch == nil {
		return ErrLiveSessionStateInvalid
	}
	now := time.Now().UnixMilli()
	err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id", "anchor_id").First(&sessionRef, req.SessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		var anchor liveModel.LiveAnchor
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&anchor, sessionRef.AnchorId).Error; err != nil {
			return err
		}
		room, err := getRoomByIDForUpdate(tx, sessionRef.RoomId)
		if err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, req.SessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if session.Status == liveModel.LiveSessionEnded && session.MediaStopSource == liveModel.LiveMediaStopSourceAdmin {
			return nil
		}
		if room.CurrentSessionId != session.ID || session.Status != liveModel.LiveSessionEnding ||
			session.PublisherEpoch != *req.ExpectedPublisherEpoch {
			return ErrLiveSessionStateInvalid
		}
		runtime, err := loadLiveSRSRuntime(tx)
		if err != nil {
			return err
		}
		if session.StopRecoveryState != liveModel.LiveStopRecoveryManualAttention &&
			!liveSRSRuntimeStaleForManualConfirmation(runtime, now) {
			return ErrLiveManualConfirmationTooEarly
		}
		audit := liveModel.LiveSessionMediaAudit{
			CreatedAt: now, SessionID: session.ID, RoomID: room.ID,
			OperatorID: actor.UserID, OperatorName: truncateRunes(strings.TrimSpace(actor.Username), 128),
			Reason:         truncateRunes(strings.TrimSpace(req.Reason), 500),
			Evidence:       truncateRunes(strings.TrimSpace(req.Evidence), 1000),
			ClientIP:       truncateRunes(strings.TrimSpace(actor.ClientIP), 64),
			UserAgent:      truncateRunes(strings.TrimSpace(actor.UserAgent), 500),
			RequestID:      truncateRunes(strings.TrimSpace(actor.RequestID), 128),
			PublisherEpoch: session.PublisherEpoch, SRSGeneration: runtime.Generation,
			SRSHealthState: runtime.HealthState, SRSLastSuccessAt: runtime.LastSuccessAt,
			PreviousMediaState: session.MediaState,
		}
		if err = tx.Create(&audit).Error; err != nil {
			return err
		}
		result := tx.Model(&liveModel.LiveSession{}).
			Where("id = ? AND status = ? AND publisher_epoch = ?", session.ID, liveModel.LiveSessionEnding, session.PublisherEpoch).
			Updates(map[string]interface{}{
				"media_state":             liveModel.LiveMediaStopped,
				"media_state_changed_at":  now,
				"media_last_confirmed_at": now,
				"media_stop_source":       liveModel.LiveMediaStopSourceAdmin,
				"media_stop_confirmed_at": now,
				"media_stop_confirmed_by": actor.UserID,
				// 立即可重放：若提交后进程退出，下一轮扫描会依据人工确认事实
				// 直接 finalize，而不是把任务隐藏到一个永不到达的时间点。
				"stop_next_retry_at": now,
				"stop_last_error":    "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLiveSessionStateInvalid
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 结算自身是幂等事务；即使此刻与已经在途的SRS停止任务竞争，也只会累计一次统计。
	return s.finalizeSession(req.SessionID, now)
}

func (s *RoomService) requestEndByQuery(query string, args []interface{}, endReason uint8, failureReason string) error {
	var stopSessionID uint
	err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id").Where(query, args...).First(&sessionRef).Error; err != nil {
			return normalizeSessionError(err)
		}
		if _, err := getRoomByIDForUpdate(tx, sessionRef.RoomId); err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, sessionRef.ID).Error; err != nil {
			return normalizeSessionError(err)
		}
		shouldStop, transitionErr := transitionLiveSessionToEnd(tx, session, endReason, failureReason, time.Now().UnixMilli())
		if shouldStop {
			stopSessionID = session.ID
		}
		return transitionErr
	})
	if err != nil {
		return err
	}
	if stopSessionID > 0 {
		_ = s.processEndingSession(stopSessionID, time.Now().UnixMilli())
	}
	return nil
}

func transitionLiveSessionToEnd(tx *gorm.DB, session liveModel.LiveSession, endReason uint8, failureReason string, now int64) (bool, error) {
	if session.Status == liveModel.LiveSessionPreparing {
		if hasSRSPublisher(session) {
			if err := markPreparingPublisherEnding(tx, session.ID, endReason, failureReason, now); err != nil {
				return false, err
			}
			err := tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", session.RoomId, session.ID).
				Update("live_status", liveModel.LiveRoomEnding).Error
			return err == nil, err
		}
		return false, cancelPreparingSession(tx, session, endReason, failureReason, now)
	}
	if session.Status == liveModel.LiveSessionEnding {
		return true, nil
	}
	if session.Status == liveModel.LiveSessionEnded {
		return false, nil
	}
	if session.Status != liveModel.LiveSessionLiving {
		return false, ErrLiveSessionStateInvalid
	}
	if err := markSessionEnding(tx, session.ID, endReason, failureReason, now, now); err != nil {
		return false, err
	}
	err := tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", session.RoomId, session.ID).
		Update("live_status", liveModel.LiveRoomEnding).Error
	return err == nil, err
}

func (s *RoomService) markDisconnectTimeout(sessionID uint, now int64) error {
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id").First(&sessionRef, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if _, err := getRoomByIDForUpdate(tx, sessionRef.RoomId); err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if session.Status != liveModel.LiveSessionLiving || session.ReconnectDeadlineAt == 0 || session.ReconnectDeadlineAt > now {
			return ErrLiveSessionStateInvalid
		}
		endedAt := session.LastUnpublishAt
		if endedAt == 0 {
			endedAt = session.ReconnectDeadlineAt
		}
		if err := markSessionEnding(tx, session.ID, liveModel.LiveSessionEndDisconnectTimeout, "断流超过重连窗口", endedAt, now); err != nil {
			return err
		}
		return tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", session.RoomId, session.ID).
			Update("live_status", liveModel.LiveRoomEnding).Error
	})
}

func (s *RoomService) cancelPrepareTimeout(sessionID uint, now int64) error {
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id").First(&sessionRef, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if _, err := getRoomByIDForUpdate(tx, sessionRef.RoomId); err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if session.Status != liveModel.LiveSessionPreparing || session.PrepareDeadlineAt > now {
			return ErrLiveSessionStateInvalid
		}
		if hasSRSPublisher(session) {
			if err := markPreparingPublisherEnding(
				tx, session.ID, liveModel.LiveSessionEndPrepareTimeout, "等待有效视频尺寸超时", now,
			); err != nil {
				return err
			}
			return tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", session.RoomId, session.ID).
				Update("live_status", liveModel.LiveRoomEnding).Error
		}
		return cancelPreparingSession(
			tx, session, liveModel.LiveSessionEndPrepareTimeout, "准备开播超时", now,
		)
	})
}

func (s *RoomService) finalizeSession(sessionID uint, now int64) error {
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var sessionRef liveModel.LiveSession
		if err := tx.Select("id", "room_id", "anchor_id").First(&sessionRef, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		var anchor liveModel.LiveAnchor
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&anchor, sessionRef.AnchorId).Error; err != nil {
			return err
		}
		if _, err := getRoomByIDForUpdate(tx, sessionRef.RoomId); err != nil {
			return normalizeRoomError(err)
		}
		var session liveModel.LiveSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, sessionID).Error; err != nil {
			return normalizeSessionError(err)
		}
		if session.Status == liveModel.LiveSessionEnded {
			// SRS同步响应、人工请求和扫描任务可能同时观察到停止成功。
			// 第一个事务已经完成所有统计；后续调用必须幂等返回，不能重复累计。
			return nil
		}
		if session.Status != liveModel.LiveSessionEnding {
			return ErrLiveSessionStateInvalid
		}
		if session.StartedAt == 0 {
			stopSource := session.MediaStopSource
			if stopSource == liveModel.LiveMediaStopSourceUnknown {
				stopSource = liveModel.LiveMediaStopSourceSRS
			}
			result := tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", session.ID, liveModel.LiveSessionEnding).
				Updates(map[string]interface{}{
					"status": liveModel.LiveSessionCancelled, "prepare_deadline_at": int64(0),
					"reconnect_deadline_at": int64(0), "stats_finalized_at": now,
					"stop_next_retry_at": int64(0), "stop_last_error": "",
					"media_state": liveModel.LiveMediaStopped, "media_state_changed_at": now,
					"media_last_confirmed_at": now, "media_stop_source": stopSource,
					"media_stop_confirmed_at": now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrLiveSessionStateInvalid
			}
			return clearRoomIfCurrent(tx, session.RoomId, session.ID)
		}
		endedAt := session.EndedAt
		if endedAt == 0 {
			endedAt = now
		}
		var duration uint64
		if session.StartedAt > 0 && endedAt > session.StartedAt {
			duration = uint64(endedAt - session.StartedAt)
		}
		result := tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", session.ID, liveModel.LiveSessionEnding).
			Updates(map[string]interface{}{
				"status": liveModel.LiveSessionEnded, "ended_at": endedAt, "duration_ms": duration,
				"reconnect_deadline_at": int64(0), "stats_finalized_at": now,
				"stop_next_retry_at": int64(0), "stop_last_error": "",
				"media_state": liveModel.LiveMediaStopped, "media_state_changed_at": now,
				"media_last_confirmed_at": now, "media_stop_source": func() uint8 {
					if session.MediaStopSource != liveModel.LiveMediaStopSourceUnknown {
						return session.MediaStopSource
					}
					return liveModel.LiveMediaStopSourceSRS
				}(), "media_stop_confirmed_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLiveSessionStateInvalid
		}
		if err := completeRoomCurrentSession(tx, session.RoomId, session.ID); err != nil {
			return err
		}
		return tx.Model(&liveModel.LiveAnchor{}).Where("id = ?", session.AnchorId).Updates(map[string]interface{}{
			"total_live_count":       gorm.Expr("total_live_count + 1"),
			"total_live_duration_ms": gorm.Expr("total_live_duration_ms + ?", duration),
			"total_view_count":       gorm.Expr("total_view_count + ?", session.ViewCount),
			"max_online_count":       gorm.Expr("CASE WHEN max_online_count < ? THEN ? ELSE max_online_count END", session.PeakOnlineCount, session.PeakOnlineCount),
			"last_live_end_at":       endedAt,
		}).Error
	})
}

func ensureRoomForAnchor(tx *gorm.DB, anchor liveModel.LiveAnchor) (liveModel.LiveRoom, error) {
	room, err := getRoomByAnchorForUpdate(tx, anchor.ID)
	if err == nil {
		return room, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return liveModel.LiveRoom{}, err
	}
	roomNo := anchor.AnchorNo
	room = liveModel.LiveRoom{
		RoomNo: roomNo, AnchorId: anchor.ID, CategoryId: anchor.CategoryId,
		Title: anchor.Nickname + "的直播间", CoverURL: anchor.Cover, StreamName: roomNo,
		Status: liveModel.LiveRoomStatusNormal, StatusChangedAt: time.Now().UnixMilli(),
		LiveStatus: liveModel.LiveRoomOffline, Visibility: liveModel.LiveRoomVisibilityPublic,
	}
	if err = tx.Create(&room).Error; err != nil {
		return liveModel.LiveRoom{}, normalizeRoomNoError(err)
	}
	return room, nil
}

func getRoomByAnchorForUpdate(tx *gorm.DB, anchorID uint) (liveModel.LiveRoom, error) {
	var room liveModel.LiveRoom
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("anchor_id = ?", anchorID).First(&room).Error
	return room, err
}

func getRoomByIDForUpdate(tx *gorm.DB, roomID uint) (liveModel.LiveRoom, error) {
	var room liveModel.LiveRoom
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&room, roomID).Error
	return room, err
}

func markSessionEnding(tx *gorm.DB, sessionID uint, endReason uint8, failureReason string, endedAt, requestedAt int64) error {
	result := tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", sessionID, liveModel.LiveSessionLiving).
		Updates(map[string]interface{}{
			"status": liveModel.LiveSessionEnding, "ended_at": endedAt, "end_reason": endReason,
			"failure_reason": failureReason, "reconnect_deadline_at": int64(0),
			"stream_ready_deadline_at": int64(0), "stream_probe_attempts": uint32(0),
			"stream_probe_next_at": int64(0), "stream_probe_last_error": "",
			"stop_requested_at": requestedAt, "stop_attempts": uint32(0),
			"stop_next_retry_at": requestedAt, "stop_last_error": "",
			"stop_recovery_state": liveModel.LiveStopRecoveryNormal,
			"ending_alert_level":  liveModel.LiveEndingAlertNone, "ending_alerted_at": int64(0),
			"media_state": liveModel.LiveMediaStopping, "media_state_changed_at": requestedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLiveSessionStateInvalid
	}
	return nil
}

func markPreparingPublisherEnding(
	tx *gorm.DB,
	sessionID uint,
	endReason uint8,
	failureReason string,
	now int64,
) error {
	result := tx.Model(&liveModel.LiveSession{}).
		Where("id = ? AND status = ? AND srs_stream_id <> ''", sessionID, liveModel.LiveSessionPreparing).
		Updates(map[string]interface{}{
			"status": liveModel.LiveSessionEnding, "ended_at": now, "end_reason": endReason,
			"failure_reason": failureReason, "prepare_deadline_at": int64(0),
			"stream_ready_deadline_at": int64(0), "stream_probe_attempts": uint32(0),
			"stream_probe_next_at": int64(0), "stream_probe_last_error": "",
			"stop_requested_at": now, "stop_attempts": uint32(0),
			"stop_next_retry_at": now, "stop_last_error": "",
			"stop_recovery_state": liveModel.LiveStopRecoveryNormal,
			"ending_alert_level":  liveModel.LiveEndingAlertNone, "ending_alerted_at": int64(0),
			"media_state": liveModel.LiveMediaStopping, "media_state_changed_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLiveSessionStateInvalid
	}
	return nil
}

func cancelPreparingSession(tx *gorm.DB, session liveModel.LiveSession, endReason uint8, reason string, endedAt int64) error {
	result := tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", session.ID, liveModel.LiveSessionPreparing).
		Updates(map[string]interface{}{
			"status": liveModel.LiveSessionCancelled, "ended_at": endedAt, "end_reason": endReason,
			"failure_reason": reason, "stats_finalized_at": endedAt, "prepare_deadline_at": int64(0),
			"stream_ready_deadline_at": int64(0), "stream_probe_attempts": uint32(0),
			"stream_probe_next_at": int64(0), "stream_probe_last_error": "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLiveSessionStateInvalid
	}
	return clearRoomIfCurrent(tx, session.RoomId, session.ID)
}

// endActiveSessionForAnchor 与主播状态、权限修改共用同一事务，确保封禁或关闭开播权限后
// 不会留下仍可继续推流的准备中/直播中场次。
func endActiveSessionForAnchor(tx *gorm.DB, anchorID uint, reason string) error {
	if !tx.Migrator().HasTable(&liveModel.LiveRoom{}) {
		return nil
	}
	room, err := getRoomByAnchorForUpdate(tx, anchorID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil || room.CurrentSessionId == 0 {
		return err
	}
	var session liveModel.LiveSession
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, room.CurrentSessionId).Error; err != nil {
		return normalizeSessionError(err)
	}
	now := time.Now().UnixMilli()
	switch session.Status {
	case liveModel.LiveSessionPreparing:
		if hasSRSPublisher(session) {
			if err = markPreparingPublisherEnding(tx, session.ID, liveModel.LiveSessionEndAnchorBlocked, reason, now); err != nil {
				return err
			}
			return tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", room.ID, session.ID).
				Update("live_status", liveModel.LiveRoomEnding).Error
		}
		return cancelPreparingSession(tx, session, liveModel.LiveSessionEndAnchorBlocked, reason, now)
	case liveModel.LiveSessionLiving:
		if err = markSessionEnding(tx, session.ID, liveModel.LiveSessionEndAnchorBlocked, reason, now, now); err != nil {
			return err
		}
		return tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", room.ID, session.ID).
			Update("live_status", liveModel.LiveRoomEnding).Error
	case liveModel.LiveSessionEnding:
		return nil
	default:
		return clearRoomIfCurrent(tx, room.ID, session.ID)
	}
}

func clearRoomCurrentSession(tx *gorm.DB, roomID uint) error {
	return tx.Model(&liveModel.LiveRoom{}).Where("id = ?", roomID).Updates(map[string]interface{}{
		"live_status": liveModel.LiveRoomOffline, "current_session_id": uint(0), "live_started_at": int64(0),
	}).Error
}

func clearRoomIfCurrent(tx *gorm.DB, roomID, sessionID uint) error {
	return tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", roomID, sessionID).Updates(map[string]interface{}{
		"live_status": liveModel.LiveRoomOffline, "current_session_id": uint(0), "live_started_at": int64(0),
	}).Error
}

// completeRoomCurrentSession 在已实际开播的场次完成结算时，同时保存最后一场直播指针。
// 取消的准备中场次仍使用 clearRoomIfCurrent，不得覆盖最近一次真正完成的直播。
func completeRoomCurrentSession(tx *gorm.DB, roomID, sessionID uint) error {
	result := tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", roomID, sessionID).Updates(map[string]interface{}{
		"live_status": liveModel.LiveRoomOffline, "current_session_id": uint(0),
		"last_session_id": sessionID, "live_started_at": int64(0),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLiveSessionStateInvalid
	}
	return nil
}

func (s *RoomService) roomInfo(room liveModel.LiveRoom, anchor liveModel.LiveAnchor) (liveRes.LiveRoomInfo, error) {
	result := liveRes.LiveRoomInfo{
		RoomNo: room.RoomNo, AnchorNo: anchor.AnchorNo, CategoryId: room.CategoryId, Title: room.Title,
		CoverURL: room.CoverURL, Notice: room.Notice, StreamName: room.StreamName, StreamKeyVersion: room.StreamKeyVersion,
		Status: room.Status, StatusReason: room.StatusReason, LiveStatus: room.LiveStatus,
		LiveStartedAt: room.LiveStartedAt, Visibility: room.Visibility,
	}
	if room.CurrentSessionId != 0 {
		var session liveModel.LiveSession
		if err := global.GVA_DB.First(&session, room.CurrentSessionId).Error; err != nil {
			return liveRes.LiveRoomInfo{}, normalizeSessionError(err)
		}
		current := toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo)
		result.CurrentSession = &current
	}
	return result, nil
}

func toLiveSessionInfo(session liveModel.LiveSession, roomNo, anchorNo string) liveRes.LiveSessionInfo {
	return liveRes.LiveSessionInfo{
		SessionNo: session.SessionNo, RoomNo: roomNo, AnchorNo: anchorNo, CategoryId: session.CategoryId,
		Title: session.Title, CoverURL: session.CoverURL, StreamInfo: publicStreamInfo(session.StreamInfo),
		Status: session.Status, PrepareDeadlineAt: session.PrepareDeadlineAt, StartedAt: session.StartedAt,
		EndedAt: session.EndedAt, DurationMs: session.DurationMs, LastUnpublishAt: session.LastUnpublishAt,
		ReconnectDeadlineAt: session.ReconnectDeadlineAt, DisconnectCount: session.DisconnectCount,
		EndReason: session.EndReason, FailureReason: session.FailureReason, ViewCount: session.ViewCount,
		ViewerCount: session.ViewerCount, PeakOnlineCount: session.PeakOnlineCount, LikeCount: session.LikeCount,
		GiftCount: session.GiftCount, GiftCoinAmount: session.GiftCoinAmount, GiftUserCount: session.GiftUserCount,
		StatsFinalizedAt: session.StatsFinalizedAt,
		MediaState:       session.MediaState, MediaStateChangedAt: session.MediaStateChangedAt,
		MediaLastConfirmedAt: session.MediaLastConfirmedAt,
	}
}

func toLiveSessionAdminItem(session liveModel.LiveSession, room liveModel.LiveRoom, anchor liveModel.LiveAnchor) liveRes.LiveSessionAdminItem {
	return liveRes.LiveSessionAdminItem{
		ID: session.ID, CreatedAt: session.CreatedAt,
		LiveSessionInfo: toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo),
		RoomID:          session.RoomId, AnchorID: session.AnchorId, PublishIP: session.PublishIP,
		PublisherEpoch: session.PublisherEpoch, SRSGeneration: session.SRSGeneration,
		StopRequestedAt: session.StopRequestedAt, StopAttempts: session.StopAttempts,
		StopNextRetryAt: session.StopNextRetryAt, StopLastError: session.StopLastError,
		StopRecoveryState: session.StopRecoveryState, EndingAlertLevel: session.EndingAlertLevel,
		EndingAlertedAt: session.EndingAlertedAt, MediaStopSource: session.MediaStopSource,
		MediaStopConfirmedAt: session.MediaStopConfirmedAt, MediaStopConfirmedBy: session.MediaStopConfirmedBy,
	}
}

// publicStreamInfo 只有在真实采集到有效宽高后才向前端返回流信息；其他情况统一返回空对象。
// 即使数据库中存在升级前或人工写入的额外键，也会在读取时再次按白名单重建JSON，
// 不允许SRS原始对象中的IP、token或内部连接标识意外透传。
func publicStreamInfo(value []byte) json.RawMessage {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" || !json.Valid(value) ||
		!strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return json.RawMessage(`{}`)
	}
	var snapshot publicStreamSnapshot
	if err := json.Unmarshal(value, &snapshot); err != nil || snapshot.Width <= 0 || snapshot.Height <= 0 {
		return json.RawMessage(`{}`)
	}
	sanitized, err := json.Marshal(snapshot)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(sanitized)
}

func toLiveRoomAdminItem(room liveModel.LiveRoom, anchor liveModel.LiveAnchor) liveRes.LiveRoomAdminItem {
	return liveRes.LiveRoomAdminItem{
		ID: room.ID, CreatedAt: room.CreatedAt, UpdatedAt: room.UpdatedAt, RoomNo: room.RoomNo,
		AnchorID: room.AnchorId, AnchorNo: anchor.AnchorNo, AnchorNickname: anchor.Nickname,
		CategoryId: room.CategoryId, Title: room.Title, CoverURL: room.CoverURL, Notice: room.Notice,
		StreamName: room.StreamName, StreamKeyVersion: room.StreamKeyVersion, Status: room.Status,
		StatusReason: room.StatusReason, StatusChangedAt: room.StatusChangedAt, LiveStatus: room.LiveStatus,
		CurrentSessionID: room.CurrentSessionId, LiveStartedAt: room.LiveStartedAt,
		Visibility: room.Visibility, RecommendWeight: room.RecommendWeight,
	}
}

// nextLiveSessionNo 生成“毫秒时间-anchorNo-roomNo”格式的场次编号。
// 同一直播间的准备事务会先锁定 live_room；这里再查询包含软删除记录在内的历史编号，
// 遇到毫秒碰撞时顺延逻辑毫秒，uk_live_session_no 仍作为最终并发兜底。
func nextLiveSessionNo(tx *gorm.DB, now time.Time, anchorNo, roomNo string) (string, error) {
	baseTime := now.Truncate(time.Millisecond)
	for offset := 0; offset < liveSessionNoCollisionChecks; offset++ {
		candidateTime := baseTime.Add(time.Duration(offset) * time.Millisecond)
		timePart := strings.ReplaceAll(candidateTime.Format(liveSessionNoTimeLayout), ".", "")
		candidate := timePart + "-" + anchorNo + "-" + roomNo
		if utf8.RuneCountInString(candidate) > liveSessionNoMaxLength {
			return "", fmt.Errorf("直播场次编号长度超过%d个字符", liveSessionNoMaxLength)
		}
		var count int64
		if err := tx.Unscoped().Model(&liveModel.LiveSession{}).
			Where("session_no = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", errors.New("生成唯一直播场次编号失败")
}

func randomSecret(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func secretHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hookPublishToken(param string) string {
	values, err := url.ParseQuery(strings.TrimPrefix(strings.TrimSpace(param), "?"))
	if err != nil {
		return ""
	}
	if token := values.Get("pt"); token != "" {
		return token
	}
	return values.Get("token")
}

func isActiveSession(status uint8) bool {
	return status == liveModel.LiveSessionPreparing || status == liveModel.LiveSessionLiving || status == liveModel.LiveSessionEnding
}

func normalizeRoomError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrLiveRoomNotFound
	}
	return err
}

func normalizeRoomNoError(err error) error {
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "uk_live_room_no") || strings.Contains(message, "live_room.room_no") ||
			(strings.Contains(message, "duplicate") && strings.Contains(message, "room_no")) {
			return ErrLiveRoomNoExists
		}
	}
	return err
}

func normalizeSessionError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrLiveSessionNotFound
	}
	return err
}

func normalizeActiveSessionError(err error) error {
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "uk_live_session_active_room") || strings.Contains(message, "live_session.active_room_id") {
			return ErrLiveRoomActiveSessionExists
		}
	}
	return err
}

func roomsAnchorIDs(rooms []liveModel.LiveRoom) []uint {
	ids := make([]uint, 0, len(rooms))
	for _, room := range rooms {
		ids = append(ids, room.AnchorId)
	}
	return ids
}

func sessionRoomIDs(sessions []liveModel.LiveSession) []uint {
	ids := make([]uint, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.RoomId)
	}
	return ids
}

func sessionAnchorIDs(sessions []liveModel.LiveSession) []uint {
	ids := make([]uint, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.AnchorId)
	}
	return ids
}

func loadAnchorMap(ids []uint) (map[uint]liveModel.LiveAnchor, error) {
	result := make(map[uint]liveModel.LiveAnchor)
	if len(ids) == 0 {
		return result, nil
	}
	var anchors []liveModel.LiveAnchor
	if err := global.GVA_DB.Where("id IN ?", ids).Find(&anchors).Error; err != nil {
		return nil, err
	}
	for _, anchor := range anchors {
		result[anchor.ID] = anchor
	}
	return result, nil
}

func loadRoomMap(ids []uint) (map[uint]liveModel.LiveRoom, error) {
	result := make(map[uint]liveModel.LiveRoom)
	if len(ids) == 0 {
		return result, nil
	}
	var rooms []liveModel.LiveRoom
	if err := global.GVA_DB.Where("id IN ?", ids).Find(&rooms).Error; err != nil {
		return nil, err
	}
	for _, room := range rooms {
		result[room.ID] = room
	}
	return result, nil
}

func secureStringEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
