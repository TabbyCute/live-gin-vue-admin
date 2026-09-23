package live

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrLiveRoomNotFound             = errors.New("直播间不存在")
	ErrLiveRoomNoRequired           = errors.New("直播间编号不能为空")
	ErrLiveRoomNoExists             = errors.New("直播间编号已存在")
	ErrLiveRoomUnavailable          = errors.New("直播间当前不可开播")
	ErrLiveRoomActiveSessionExists  = errors.New("直播间已有活动场次")
	ErrLiveRoomStatusReasonRequired = errors.New("禁用直播间时必须填写原因")
	ErrLiveSessionNotFound          = errors.New("直播场次不存在")
	ErrLiveSessionStateInvalid      = errors.New("直播场次状态不允许当前操作")
	ErrLivePublishTokenInvalid      = errors.New("推流凭证无效")
	ErrLiveHookUnauthorized         = errors.New("SRS 回调鉴权失败")
)

type RoomService struct{}

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

	publishToken, err := randomSecret(32)
	if err != nil {
		return liveRes.LiveSessionPrepareResp{}, err
	}
	var room liveModel.LiveRoom
	var session liveModel.LiveSession
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

		session = liveModel.LiveSession{
			SessionNo: newExternalNo("S"), RoomId: room.ID, AnchorId: anchor.ID,
			CategoryId: req.CategoryId, Title: strings.TrimSpace(req.Title), CoverURL: strings.TrimSpace(req.CoverURL),
			Status: liveModel.LiveSessionPreparing,
		}
		if txErr = tx.Create(&session).Error; txErr != nil {
			return normalizeActiveSessionError(txErr)
		}
		updates := map[string]interface{}{
			"category_id":         req.CategoryId,
			"title":               session.Title,
			"cover_url":           session.CoverURL,
			"visibility":          *req.Visibility,
			"publish_secret_hash": secretHash(publishToken),
			"stream_key_version":  gorm.Expr("stream_key_version + 1"),
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
		PublishToken: publishToken, PushURL: buildStreamURL(global.GVA_CONFIG.Live.PushBaseURL, room.StreamName, publishToken),
		PlayURL: buildStreamURL(global.GVA_CONFIG.Live.PlayBaseURL, room.StreamName, ""),
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

func (s *RoomService) RequestOwnerEnd(userID uint64, sessionNo string) error {
	anchor, err := (&AnchorService{}).getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAnchorNotFound
	}
	if err != nil {
		return err
	}
	return s.requestEndByQuery("session_no = ? AND anchor_id = ?", []interface{}{sessionNo, anchor.ID}, liveModel.LiveSessionEndByAnchor, "")
}

func (s *RoomService) OnPublish(streamName, param string) (liveRes.LiveSessionInfo, error) {
	publishToken := hookPublishToken(param)
	var result liveRes.LiveSessionInfo
	err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var roomRef liveModel.LiveRoom
		if err := tx.Select("id", "anchor_id").Where("stream_name = ?", streamName).First(&roomRef).Error; err != nil {
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
		now := time.Now().UnixMilli()
		switch session.Status {
		case liveModel.LiveSessionPreparing:
			if err := tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", session.ID, liveModel.LiveSessionPreparing).
				Updates(map[string]interface{}{"status": liveModel.LiveSessionLiving, "started_at": now, "reconnect_deadline_at": int64(0)}).Error; err != nil {
				return err
			}
			session.Status, session.StartedAt, session.ReconnectDeadlineAt = liveModel.LiveSessionLiving, now, 0
			if err := tx.Model(&liveModel.LiveAnchor{}).Where("id = ?", session.AnchorId).Update("last_live_at", now).Error; err != nil {
				return err
			}
		case liveModel.LiveSessionLiving:
			if session.ReconnectDeadlineAt == 0 {
				// SRS 可能重复发送 on_publish；已在直播中时按幂等成功处理。
				break
			}
			if session.ReconnectDeadlineAt < now {
				return ErrLiveSessionStateInvalid
			}
			if err := tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", session.ID, liveModel.LiveSessionLiving).
				Update("reconnect_deadline_at", int64(0)).Error; err != nil {
				return err
			}
			session.ReconnectDeadlineAt = 0
		default:
			return ErrLiveSessionStateInvalid
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

func (s *RoomService) OnUnpublish(streamName string) error {
	window := global.GVA_CONFIG.Live.ReconnectWindowSeconds
	if window <= 0 {
		window = 20
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var room liveModel.LiveRoom
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("stream_name = ?", streamName).First(&room).Error; err != nil {
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
	// 先结算上一轮已经进入“结束中”的场次，让结束中成为可观察且可恢复的状态。
	var endingIDs []uint
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).Where("status = ?", liveModel.LiveSessionEnding).
		Order("id ASC").Limit(100).Pluck("id", &endingIDs).Error; err != nil {
		return err
	}
	for _, sessionID := range endingIDs {
		if err := s.finalizeSession(sessionID, now); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
			return err
		}
	}

	var timeoutIDs []uint
	if err := global.GVA_DB.Model(&liveModel.LiveSession{}).
		Where("status = ? AND reconnect_deadline_at > 0 AND reconnect_deadline_at <= ?", liveModel.LiveSessionLiving, now).
		Order("id ASC").Limit(100).Pluck("id", &timeoutIDs).Error; err != nil {
		return err
	}
	for _, sessionID := range timeoutIDs {
		if err := s.markDisconnectTimeout(sessionID, now); err != nil && !errors.Is(err, ErrLiveSessionStateInvalid) {
			return err
		}
	}
	return nil
}

func (s *RoomService) GetPublicRoom(roomNo string) (liveRes.LiveRoomPublicItem, error) {
	items, _, err := s.getPublicRooms(roomNo, 0, 1, 1)
	if err != nil {
		return liveRes.LiveRoomPublicItem{}, err
	}
	if len(items) == 0 {
		return liveRes.LiveRoomPublicItem{}, ErrLiveRoomNotFound
	}
	return items[0], nil
}

func (s *RoomService) GetPublicRoomList(req liveReq.LiveRoomPublicListReq) ([]liveRes.LiveRoomPublicItem, int64, error) {
	return s.getPublicRooms("", req.CategoryId, req.Page, req.PageSize)
}

func (s *RoomService) getPublicRooms(roomNo string, categoryID uint64, page, pageSize int) ([]liveRes.LiveRoomPublicItem, int64, error) {
	db := global.GVA_DB.Table("live_room AS r").
		Joins("JOIN live_anchor AS a ON a.id = r.anchor_id AND a.deleted_at IS NULL").
		Joins("JOIN live_session AS s ON s.id = r.current_session_id AND s.deleted_at IS NULL").
		Where("r.deleted_at IS NULL AND r.status = ? AND r.live_status = ? AND r.visibility = ?", liveModel.LiveRoomStatusNormal, liveModel.LiveRoomLiving, liveModel.LiveRoomVisibilityPublic).
		Where("a.status = ? AND a.apply_status = ?", liveModel.AnchorStatusNormal, liveModel.AnchorApplyStatusApproved)
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
		LiveStartedAt  int64
		SessionNo      string
		ViewCount      uint64
		LikeCount      uint64
	}
	err := db.Select("r.room_no, a.anchor_no, a.nickname AS anchor_nickname, a.avatar AS anchor_avatar, r.category_id, r.title, r.cover_url, r.notice, r.live_started_at, s.session_no, s.view_count, s.like_count").
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
			LiveStartedAt: row.LiveStartedAt, SessionNo: row.SessionNo, ViewCount: row.ViewCount, LikeCount: row.LikeCount,
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

func (s *RoomService) GetAdminRoomDetail(roomID uint) (liveRes.LiveRoomAdminItem, error) {
	var room liveModel.LiveRoom
	if err := global.GVA_DB.First(&room, roomID).Error; err != nil {
		return liveRes.LiveRoomAdminItem{}, normalizeRoomError(err)
	}
	var anchor liveModel.LiveAnchor
	if err := global.GVA_DB.First(&anchor, room.AnchorId).Error; err != nil {
		return liveRes.LiveRoomAdminItem{}, err
	}
	return toLiveRoomAdminItem(room, anchor), nil
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
				if err = markSessionEnding(tx, session.ID, liveModel.LiveSessionEndByAdmin, reason, now); err != nil {
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
		list = append(list, liveRes.LiveSessionAdminItem{ID: session.ID, CreatedAt: session.CreatedAt,
			LiveSessionInfo: toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo), RoomID: session.RoomId, AnchorID: session.AnchorId})
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
	return liveRes.LiveSessionAdminItem{ID: session.ID, CreatedAt: session.CreatedAt,
		LiveSessionInfo: toLiveSessionInfo(session, room.RoomNo, anchor.AnchorNo), RoomID: session.RoomId, AnchorID: session.AnchorId}, nil
}

func (s *RoomService) RequestAdminEnd(sessionID uint, reason string) error {
	return s.requestEndByQuery("id = ?", []interface{}{sessionID}, liveModel.LiveSessionEndByAdmin, strings.TrimSpace(reason))
}

func (s *RoomService) requestEndByQuery(query string, args []interface{}, endReason uint8, failureReason string) error {
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
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
		now := time.Now().UnixMilli()
		if session.Status == liveModel.LiveSessionPreparing {
			if err := cancelPreparingSession(tx, session, endReason, failureReason, now); err != nil {
				return err
			}
			return nil
		}
		if session.Status != liveModel.LiveSessionLiving {
			return ErrLiveSessionStateInvalid
		}
		if err := markSessionEnding(tx, session.ID, endReason, failureReason, now); err != nil {
			return err
		}
		return tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", session.RoomId, session.ID).
			Update("live_status", liveModel.LiveRoomEnding).Error
	})
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
		if err := markSessionEnding(tx, session.ID, liveModel.LiveSessionEndDisconnectTimeout, "断流超过重连窗口", endedAt); err != nil {
			return err
		}
		return tx.Model(&liveModel.LiveRoom{}).Where("id = ? AND current_session_id = ?", session.RoomId, session.ID).
			Update("live_status", liveModel.LiveRoomEnding).Error
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
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLiveSessionStateInvalid
		}
		if err := clearRoomIfCurrent(tx, session.RoomId, session.ID); err != nil {
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

func markSessionEnding(tx *gorm.DB, sessionID uint, endReason uint8, failureReason string, endedAt int64) error {
	result := tx.Model(&liveModel.LiveSession{}).Where("id = ? AND status = ?", sessionID, liveModel.LiveSessionLiving).
		Updates(map[string]interface{}{
			"status": liveModel.LiveSessionEnding, "ended_at": endedAt, "end_reason": endReason,
			"failure_reason": failureReason, "reconnect_deadline_at": int64(0),
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
			"failure_reason": reason, "stats_finalized_at": endedAt,
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
		if err = markSessionEnding(tx, session.ID, liveModel.LiveSessionEndAnchorBlocked, reason, now); err != nil {
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
		Title: session.Title, CoverURL: session.CoverURL, Status: session.Status, StartedAt: session.StartedAt,
		EndedAt: session.EndedAt, DurationMs: session.DurationMs, LastUnpublishAt: session.LastUnpublishAt,
		ReconnectDeadlineAt: session.ReconnectDeadlineAt, DisconnectCount: session.DisconnectCount,
		EndReason: session.EndReason, FailureReason: session.FailureReason, ViewCount: session.ViewCount,
		ViewerCount: session.ViewerCount, PeakOnlineCount: session.PeakOnlineCount, LikeCount: session.LikeCount,
		GiftCount: session.GiftCount, GiftCoinAmount: session.GiftCoinAmount, GiftUserCount: session.GiftUserCount,
		StatsFinalizedAt: session.StatsFinalizedAt,
	}
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

func newExternalNo(prefix string) string {
	value := strings.ReplaceAll(uuid.NewString(), "-", "")
	return prefix + value[:23]
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
	return values.Get("token")
}

func buildStreamURL(baseURL, streamName, token string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	result := baseURL + "/" + url.PathEscape(streamName)
	if token != "" {
		result += "?token=" + url.QueryEscape(token)
	}
	return result
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

func ValidateLiveHookToken(token string) error {
	expected := strings.TrimSpace(global.GVA_CONFIG.Live.SRSHookToken)
	if expected == "" || token == "" || !secureStringEqual(secretHash(token), secretHash(expected)) {
		return ErrLiveHookUnauthorized
	}
	return nil
}

func secureStringEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
