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
	"time"
	"unicode/utf8"

	"tb_live_module/global"
	srsclient "tb_live_module/internal/srs"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	liveSessionNoMaxLength       = 96
	liveSessionNoTimeLayout      = "20060102150405.000"
	liveSessionNoCollisionChecks = 10000
	defaultPrepareTimeoutSeconds = 120
	defaultEndScanBatchSize      = 100
	defaultEndRetryBaseSeconds   = 5
	defaultEndRetryMaxSeconds    = 60
	defaultSRSRequestSeconds     = 3
)

var (
	ErrLiveRoomNotFound              = errors.New("直播间不存在")
	ErrLiveRoomNoRequired            = errors.New("直播间编号不能为空")
	ErrLiveRoomNoExists              = errors.New("直播间编号已存在")
	ErrLiveRoomUnavailable           = errors.New("直播间当前不可开播")
	ErrLiveRoomActiveSessionExists   = errors.New("直播间已有活动场次")
	ErrLiveRoomActiveSessionNotFound = errors.New("当前没有活动直播场次")
	ErrLiveRoomStatusReasonRequired  = errors.New("禁用直播间时必须填写原因")
	ErrLiveSessionNotFound           = errors.New("直播场次不存在")
	ErrLiveSessionStateInvalid       = errors.New("直播场次状态不允许当前操作")
	ErrLivePublishTokenInvalid       = errors.New("推流凭证无效")
	ErrLivePublishTokenConfigInvalid = errors.New("推流凭证配置无效")
)

type RoomService struct {
	srsController srsclient.Controller
}

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
	permission, anchor, err := (&AnchorService{}).checkLivePermission(userID)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	if !permission.Allow || anchor == nil {
		return liveRes.LiveSessionPrepareResp{}, ErrLiveRoomUnavailable
	}
	if err = (&CategoryService{}).EnsureEnabledCategory(req.CategoryId); err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
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

	var room liveModel.LiveRoom
	var session liveModel.LiveSession
	var publishToken string
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var txErr error
		room, txErr = ensureRoomForAnchor(tx, *anchor)
		if txErr != nil {
			return txErr
		}
		room, txErr = getRoomByIDForUpdate(tx, room.ID)
		if txErr != nil {
			return txErr
		}
		if room.Status != liveModel.LiveRoomStatusNormal {
			return ErrLiveRoomUnavailable
		}
		if room.CurrentSessionId != 0 {
			var current liveModel.LiveSession
			if lookupErr := tx.First(&current, room.CurrentSessionId).Error; lookupErr == nil && isActiveSession(current.Status) {
				return ErrLiveRoomActiveSessionExists
			}
			if updateErr := clearRoomCurrentSession(tx, room.ID); updateErr != nil {
				return updateErr
			}
		}

		preparedAt := time.Now()
		sessionNo, sessionNoErr := nextLiveSessionNo(tx, preparedAt, anchor.AnchorNo, room.RoomNo)
		if sessionNoErr != nil {
			return sessionNoErr
		}
		session = liveModel.LiveSession{
			SessionNo: sessionNo, RoomId: room.ID, AnchorId: anchor.ID,
			CategoryId: req.CategoryId, Title: strings.TrimSpace(req.Title), CoverURL: strings.TrimSpace(req.CoverURL),
			Status:            liveModel.LiveSessionPreparing,
			PrepareDeadlineAt: preparedAt.Add(time.Duration(prepareTimeoutSeconds) * time.Second).UnixMilli(),
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
		return tx.First(&room, room.ID).Error
	})
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, normalizeRoomError(err)
	}
	return liveRes.LiveSessionPrepareResp{
		RoomNo: room.RoomNo, SessionNo: session.SessionNo, StreamName: room.StreamName,
		PrepareDeadlineAt: session.PrepareDeadlineAt,
		PublishToken:      publishToken, PushURL: publishConfig.streamURL(room.StreamName, publishToken),
		PlayURL: publishConfig.playURL(room.StreamName),
	}, nil
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
		switch session.Status {
		case liveModel.LiveSessionPreparing:
			if session.PrepareDeadlineAt <= 0 || session.PrepareDeadlineAt <= now {
				return ErrLiveSessionStateInvalid
			}
			publishIP := strings.TrimSpace(req.IP)
			publishUpdates["status"] = liveModel.LiveSessionLiving
			publishUpdates["started_at"] = now
			publishUpdates["publish_ip"] = publishIP
			publishUpdates["prepare_deadline_at"] = int64(0)
			publishUpdates["reconnect_deadline_at"] = int64(0)
			session.Status, session.PrepareDeadlineAt, session.StartedAt, session.PublishIP, session.ReconnectDeadlineAt =
				liveModel.LiveSessionLiving, 0, now, publishIP, 0
			if err := tx.Model(&liveModel.LiveAnchor{}).Where("id = ?", session.AnchorId).Update("last_live_at", now).Error; err != nil {
				return err
			}
		case liveModel.LiveSessionLiving:
			if session.ReconnectDeadlineAt == 0 {
				// 活跃连接的重复on_publish不产生写库；不同连接不能覆盖当前发布者身份。
				if !srsPublisherMatches(session, req) {
					return ErrLiveSessionStateInvalid
				}
				result = toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo)
				return nil
			}
			if session.ReconnectDeadlineAt <= now {
				return ErrLiveSessionStateInvalid
			}
			publishUpdates["reconnect_deadline_at"] = int64(0)
			session.ReconnectDeadlineAt = 0
		default:
			return ErrLiveSessionStateInvalid
		}
		if err := tx.Model(&liveModel.LiveSession{}).Where("id = ?", session.ID).Updates(publishUpdates).Error; err != nil {
			return err
		}
		if err := tx.Model(&liveModel.LiveRoom{}).Where("id = ?", room.ID).Updates(map[string]interface{}{
			"live_status": liveModel.LiveRoomLiving, "live_started_at": session.StartedAt,
		}).Error; err != nil {
			return err
		}
		result = toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo)
		return nil
	})
	return result, err
}

func (s *RoomService) OnUnpublish(req liveReq.SRSHookReq) error {
	window := global.GVA_CONFIG.Live.ReconnectWindowSeconds
	if window <= 0 {
		window = 20
	}
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
				"disconnect_count": gorm.Expr("disconnect_count + 1"),
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

func (s *RoomService) ProcessPendingSessions(now int64) error {
	batchSize := liveEndScanBatchSize()
	var runErrors []error
	// 准备超时没有真正开播，不访问SRS也不做直播结算，直接取消场次并释放房间。
	// (status, prepare_deadline_at)联合索引使查询只触达已到期的准备中场次。
	var prepareTimeoutIDs []uint
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND prepare_deadline_at <= ?", liveModel.LiveSessionPreparing, now).
		Order("prepare_deadline_at ASC, id ASC").Limit(batchSize).Pluck("id", &prepareTimeoutIDs).Error; err != nil {
		return err
	}
	for _, sessionID := range prepareTimeoutIDs {
		if err := s.cancelPrepareTimeout(sessionID, now); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
			runErrors = append(runErrors, err)
		}
	}

	// 先把超过重连窗口的直播中场次改为结束中，本轮随后即可尝试确认SRS已无发布流并完成结算。
	var timeoutIDs []uint
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND reconnect_deadline_at > 0 AND reconnect_deadline_at <= ?", liveModel.LiveSessionLiving, now).
		Order("reconnect_deadline_at ASC, id ASC").Limit(batchSize).Pluck("id", &timeoutIDs).Error; err != nil {
		return err
	}
	for _, sessionID := range timeoutIDs {
		if err := s.markDisconnectTimeout(sessionID, now); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
			runErrors = append(runErrors, err)
		}
	}

	// 只读取到达重试时间的结束中场次；(status, stop_next_retry_at)联合索引避免扫描历史数据。
	var endingIDs []uint
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND stop_next_retry_at <= ?", liveModel.LiveSessionEnding, now).
		Order("stop_next_retry_at ASC, id ASC").Limit(batchSize).Pluck("id", &endingIDs).Error; err != nil {
		return errors.Join(append(runErrors, err)...)
	}
	for _, sessionID := range endingIDs {
		if err := s.processEndingSession(sessionID, now); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
			runErrors = append(runErrors, err)
		}
	}
	return errors.Join(runErrors...)
}

// processEndingSession 使用stop_next_retry_at作为数据库租约。
// 人工请求和一个或多个服务实例上的扫描任务可以同时进入，但只有一个执行者能调用SRS。
func (s *RoomService) processEndingSession(sessionID uint, now int64) error {
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
	var room liveModel.LiveRoom
	if err := global.GVA_DB.Select("id", "stream_name").First(&room, session.RoomId).Error; err != nil {
		s.recordStopFailure(session, now, err)
		return normalizeRoomError(err)
	}

	controller, err := s.liveSRSController()
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
	if err != nil {
		s.recordStopFailure(session, now, err)
		return err
	}
	return s.finalizeSession(session.ID, now)
}

func (s *RoomService) liveSRSController() (srsclient.Controller, error) {
	if s.srsController != nil {
		return s.srsController, nil
	}
	return srsclient.NewClient(global.GVA_CONFIG.Live.SRS)
}

func (s *RoomService) recordStopFailure(session liveModel.LiveSession, now int64, stopErr error) {
	message := truncateRunes(stopErr.Error(), 500)
	nextRetryAt := now + liveStopRetryMillis(session.StopAttempts)
	_ = global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("id = ? AND status = ? AND stop_attempts = ? AND stop_next_retry_at = ?",
			session.ID, liveModel.LiveSessionEnding, session.StopAttempts, session.StopNextRetryAt).
		Updates(map[string]interface{}{
			"stop_next_retry_at": nextRetryAt,
			"stop_last_error":    message,
		}).Error
}

func liveEndScanBatchSize() int {
	size := global.GVA_CONFIG.Live.EndScanBatchSize
	if size <= 0 {
		return defaultEndScanBatchSize
	}
	if size > 1000 {
		return 1000
	}
	return size
}

func livePrepareTimeoutSeconds() int {
	timeout := global.GVA_CONFIG.Live.PrepareTimeoutSeconds
	if timeout <= 0 {
		return defaultPrepareTimeoutSeconds
	}
	return timeout
}

func liveStopLeaseMillis() int64 {
	return liveSRSRequestTimeout().Milliseconds() + 2000
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
				if err = cancelPreparingSession(tx, session, liveModel.LiveSessionEndByAdmin, reason, now); err != nil {
					return err
				}
				liveStatus = liveModel.LiveRoomOffline
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
	for _, session := range sessions {
		room, anchor := rooms[session.RoomId], anchors[session.AnchorId]
		list = append(list, toLiveSessionAdminItem(session, room, anchor))
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
	return toLiveSessionAdminItem(session, room, anchor), nil
}

func (s *RoomService) RequestAdminEnd(sessionID uint, reason string) error {
	return s.requestEndByQuery("id = ?", []interface{}{sessionID}, liveModel.LiveSessionEndByAdmin, strings.TrimSpace(reason))
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
			"stop_requested_at": requestedAt, "stop_attempts": uint32(0),
			"stop_next_retry_at": requestedAt, "stop_last_error": "",
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
	}
}

func toLiveSessionAdminItem(session liveModel.LiveSession, room liveModel.LiveRoom, anchor liveModel.LiveAnchor) liveRes.LiveSessionAdminItem {
	return liveRes.LiveSessionAdminItem{
		ID: session.ID, CreatedAt: session.CreatedAt,
		LiveSessionInfo: toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo),
		RoomID:          session.RoomId, AnchorID: session.AnchorId, PublishIP: session.PublishIP,
	}
}

// publicStreamInfo 保证未采集流信息时统一返回空对象，而不是 null 或需要二次解析的 JSON 字符串。
// 该字段只允许写入经过公开白名单清洗后的内容，不得保存 SRS 原始对象中的 IP、token 或内部连接标识。
func publicStreamInfo(value []byte) json.RawMessage {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" || !json.Valid(value) ||
		!strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(append([]byte(nil), value...))
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
