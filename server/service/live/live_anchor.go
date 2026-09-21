package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAnchorNotFound            = errors.New("主播不存在")
	ErrAnchorAlreadyExists       = errors.New("该用户已存在主播记录")
	ErrAnchorApplyPending        = errors.New("主播申请正在审核中")
	ErrAnchorAlreadyApproved     = errors.New("已经是主播")
	ErrInvalidApplyStatus        = errors.New("主播申请状态不允许当前操作")
	ErrRejectReasonRequired      = errors.New("审核拒绝时必须填写拒绝原因")
	ErrStatusReasonRequired      = errors.New("禁用或封禁主播时必须填写原因")
	ErrInvalidBanUntil           = errors.New("临时封禁截止时间必须晚于当前时间，永久封禁请传 0")
	ErrCancelledCannotRestore    = errors.New("已注销主播不能通过状态接口恢复")
	ErrInvalidPermission         = errors.New("直播权限关闭时不能开启 PK 权限")
	ErrRecommendPermissionClosed = errors.New("主播推荐权限已关闭，不能设置为运营推荐")
	ErrInvalidCertState          = errors.New("认证状态、认证类型和认证名称不一致")
	ErrInvalidBirthday           = errors.New("生日格式必须为 YYYY-MM-DD 且不能晚于今天")
	ErrInvalidDateRange          = errors.New("创建时间筛选范围无效")
	ErrAnchorNoTooLong           = errors.New("渠道号生成的主播编号超过长度限制")
	ErrInvalidChannel            = errors.New("渠道 ID 必须在 1 到 999999 之间")
)

const certificationRequiredForLive = false

type AnchorService struct{}

func (s *AnchorService) GetMyAnchorInfo(userID uint64) (liveRes.MyAnchorInfoResp, error) {
	anchor, err := s.getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveRes.MyAnchorInfoResp{IsAnchor: false, Anchor: nil}, nil
	}
	if err != nil {
		return liveRes.MyAnchorInfoResp{}, err
	}
	info := toMyAnchorInfo(*anchor)
	return liveRes.MyAnchorInfoResp{IsAnchor: true, Anchor: &info}, nil
}

func (s *AnchorService) ApplyAnchor(userID uint64, req liveReq.AnchorApplyReq) (*liveModel.LiveAnchor, error) {
	if global.GVA_DB == nil {
		return nil, errors.New("数据库未初始化")
	}
	if req.ChannelId == 0 || req.ChannelId > 999999 {
		return nil, ErrInvalidChannel
	}
	birthday, err := parseBirthday(req.Birthday)
	if err != nil {
		return nil, err
	}
	if err = (&CategoryService{}).EnsureEnabledCategory(req.CategoryId); err != nil {
		return nil, err
	}

	var existing liveModel.LiveAnchor
	err = global.GVA_DB.Where("user_id = ?", userID).First(&existing).Error
	switch {
	case err == nil:
		return s.reapplyAnchor(existing.ID, req, birthday)
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}

	// AnchorNo 依赖数据库自增 ID。先用事务内的唯一占位值创建记录，
	// 获得 ID 后再替换成“渠道号 + 六位序列(ID+100000)”，整个过程原子提交。
	anchor := liveModel.LiveAnchor{
		UserId:              userID,
		AnchorNo:            temporaryAnchorNo(),
		Nickname:            strings.TrimSpace(req.Nickname),
		Avatar:              strings.TrimSpace(req.Avatar),
		Cover:               strings.TrimSpace(req.Cover),
		Signature:           strings.TrimSpace(req.Signature),
		Gender:              req.Gender,
		Birthday:            birthday,
		CountryCode:         strings.ToUpper(strings.TrimSpace(req.CountryCode)),
		RegionCode:          strings.TrimSpace(req.RegionCode),
		CityCode:            strings.TrimSpace(req.CityCode),
		Language:            strings.TrimSpace(req.Language),
		CategoryId:          req.CategoryId,
		Level:               1,
		TagIds:              cloneUint64s(req.TagIds),
		ApplyStatus:         liveModel.AnchorApplyStatusPending,
		ApplyAt:             time.Now().UnixMilli(),
		Status:              liveModel.AnchorStatusNormal,
		LivePermission:      liveModel.AnchorPermissionDisabled,
		PkPermission:        liveModel.AnchorPermissionDisabled,
		RecommendPermission: liveModel.AnchorPermissionEnabled,
		WithdrawPermission:  liveModel.AnchorPermissionDisabled,
		CertStatus:          liveModel.AnchorCertStatusNone,
		RiskLevel:           liveModel.AnchorRiskNormal,
		Source:              "app",
		ChannelId:           req.ChannelId,
	}

	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		if createErr := tx.Create(&anchor).Error; createErr != nil {
			return createErr
		}
		anchorNo := formatAnchorNo(anchor.ChannelId, anchor.ID)
		if len(anchorNo) > 32 {
			return ErrAnchorNoTooLong
		}
		if updateErr := tx.Model(&liveModel.LiveAnchor{}).
			Where("id = ?", anchor.ID).
			Update("anchor_no", anchorNo).Error; updateErr != nil {
			return updateErr
		}
		anchor.AnchorNo = anchorNo
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrAnchorNoTooLong) {
			return nil, err
		}
		// user_id 唯一索引是并发申请的最终防线。冲突后重新读取并转换成稳定业务错误，
		// 绝不把 MySQL duplicate entry 原文暴露给客户端。
		if conflictErr := s.applyConflictError(userID); conflictErr != nil {
			return nil, conflictErr
		}
		return nil, err
	}
	return &anchor, nil
}

func (s *AnchorService) reapplyAnchor(anchorID uint, req liveReq.AnchorApplyReq, birthday *time.Time) (*liveModel.LiveAnchor, error) {
	updates, err := publicProfileUpdates(req.Nickname, req.Avatar, req.Cover, req.Signature, req.Gender, birthday,
		req.CountryCode, req.RegionCode, req.CityCode, req.Language, req.CategoryId, req.TagIds)
	if err != nil {
		return nil, err
	}
	updates["apply_status"] = liveModel.AnchorApplyStatusPending
	updates["apply_at"] = time.Now().UnixMilli()
	updates["audit_at"] = int64(0)
	updates["audit_user_id"] = uint64(0)
	updates["reject_reason"] = ""
	updates["live_permission"] = liveModel.AnchorPermissionDisabled
	updates["pk_permission"] = liveModel.AnchorPermissionDisabled
	updates["withdraw_permission"] = liveModel.AnchorPermissionDisabled
	// 锁定既有申请，确保两个并发“重新申请”请求只有一个能从拒绝态进入审核中。
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		anchor, lookupErr := s.getAnchorByIDForUpdate(tx, anchorID)
		if lookupErr != nil {
			return normalizeAnchorLookupError(lookupErr)
		}
		switch anchor.ApplyStatus {
		case liveModel.AnchorApplyStatusPending:
			return ErrAnchorApplyPending
		case liveModel.AnchorApplyStatusApproved:
			return ErrAnchorAlreadyApproved
		case liveModel.AnchorApplyStatusNone, liveModel.AnchorApplyStatusRejected:
			// 被拒后复用原主播记录和 AnchorNo，清空上一轮审核结果；
			// ChannelId 属于首次来源信息，重提申请不能借机改写渠道归属。
		default:
			return ErrInvalidApplyStatus
		}
		return tx.Model(&liveModel.LiveAnchor{}).Where("id = ?", anchor.ID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	refreshed, err := s.getAnchorByID(global.GVA_DB, anchorID)
	return refreshed, err
}

func (s *AnchorService) applyConflictError(userID uint64) error {
	anchor, err := s.getAnchorByUserID(global.GVA_DB, userID)
	if err != nil {
		return nil
	}
	switch anchor.ApplyStatus {
	case liveModel.AnchorApplyStatusPending:
		return ErrAnchorApplyPending
	case liveModel.AnchorApplyStatusApproved:
		return ErrAnchorAlreadyApproved
	default:
		return ErrAnchorAlreadyExists
	}
}

func (s *AnchorService) GetApplyStatus(userID uint64) (liveRes.AnchorApplyStatusResp, error) {
	anchor, err := s.getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveRes.AnchorApplyStatusResp{IsApplied: false, ApplyStatus: liveModel.AnchorApplyStatusNone}, nil
	}
	if err != nil {
		return liveRes.AnchorApplyStatusResp{}, err
	}
	return liveRes.AnchorApplyStatusResp{
		IsApplied:    anchor.ApplyStatus != liveModel.AnchorApplyStatusNone,
		AnchorNo:     anchor.AnchorNo,
		ApplyStatus:  anchor.ApplyStatus,
		ApplyAt:      anchor.ApplyAt,
		AuditAt:      anchor.AuditAt,
		RejectReason: anchor.RejectReason,
	}, nil
}

func (s *AnchorService) UpdateMyAnchorProfile(userID uint64, req liveReq.AnchorProfileUpdateReq) error {
	anchor, err := s.getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAnchorNotFound
	}
	if err != nil {
		return err
	}
	birthday, err := parseBirthday(req.Birthday)
	if err != nil {
		return err
	}
	if err = (&CategoryService{}).EnsureEnabledCategory(req.CategoryId); err != nil {
		return err
	}
	updates, err := publicProfileUpdates(req.Nickname, req.Avatar, req.Cover, req.Signature, req.Gender, birthday,
		req.CountryCode, req.RegionCode, req.CityCode, req.Language, req.CategoryId, req.TagIds)
	if err != nil {
		return err
	}
	// map 白名单确保空字符串、0、nil 生日等合法零值也会真正写入，
	// 同时从结构上阻断客户端修改审核、权限、风控和运营字段。
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", anchor.ID).Updates(updates).Error
}

func (s *AnchorService) GetAnchorPublicDetail(anchorNo string) (liveRes.AnchorPublicDetailResp, error) {
	if global.GVA_DB == nil {
		return liveRes.AnchorPublicDetailResp{}, errors.New("数据库未初始化")
	}
	var anchor liveModel.LiveAnchor
	err := global.GVA_DB.Where("anchor_no = ? AND apply_status = ? AND status = ?", anchorNo,
		liveModel.AnchorApplyStatusApproved, liveModel.AnchorStatusNormal).First(&anchor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveRes.AnchorPublicDetailResp{}, ErrAnchorNotFound
	}
	if err != nil {
		return liveRes.AnchorPublicDetailResp{}, err
	}
	return toAnchorPublicDetail(anchor), nil
}

func (s *AnchorService) CheckLivePermission(userID uint64) (liveRes.AnchorPermissionCheckResp, error) {
	result, _, err := s.checkLivePermission(userID)
	return result, err
}

func (s *AnchorService) CheckPkPermission(userID uint64) (liveRes.AnchorPermissionCheckResp, error) {
	// PK 资格严格继承直播资格。即使后台出现 pk_permission=1、live_permission=0 的脏数据，
	// 这里仍先拒绝直播资格，避免复制两套状态判断后逐渐产生差异。
	result, anchor, err := s.checkLivePermission(userID)
	if err != nil || !result.Allow {
		return result, err
	}
	if anchor.PkPermission != liveModel.AnchorPermissionEnabled {
		return denied("PK_PERMISSION_DISABLED", "主播 PK 权限未开启", 0), nil
	}
	return allowed(), nil
}

func (s *AnchorService) checkLivePermission(userID uint64) (liveRes.AnchorPermissionCheckResp, *liveModel.LiveAnchor, error) {
	anchor, err := s.getAnchorByUserID(global.GVA_DB, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return denied("NOT_ANCHOR", "当前用户不是主播", 0), nil, nil
	}
	if err != nil {
		return liveRes.AnchorPermissionCheckResp{}, nil, err
	}

	switch anchor.ApplyStatus {
	case liveModel.AnchorApplyStatusApproved:
	case liveModel.AnchorApplyStatusRejected:
		return denied("APPLY_REJECTED", "主播申请未通过", 0), anchor, nil
	default:
		return denied("APPLY_PENDING", "主播申请尚未审核通过", 0), anchor, nil
	}

	switch anchor.Status {
	case liveModel.AnchorStatusNormal:
	case liveModel.AnchorStatusDisabled:
		return denied("ANCHOR_DISABLED", "主播账号已被禁用", 0), anchor, nil
	case liveModel.AnchorStatusBanned:
		// BanUntil=0 表示永久封禁，未来时间表示临时封禁。已过期但 status 仍为 2 时，
		// check 保持只读并继续拒绝，等待专门的恢复机制处理状态，GET 不隐式写库。
		message := "主播账号当前处于封禁状态"
		if anchor.BanUntil > 0 && anchor.BanUntil <= time.Now().UnixMilli() {
			message = "主播封禁已到期，等待系统恢复账号状态"
		}
		return denied("ANCHOR_BANNED", message, anchor.BanUntil), anchor, nil
	case liveModel.AnchorStatusCancelled:
		return denied("ANCHOR_CANCELLED", "主播账号已注销", 0), anchor, nil
	default:
		return denied("ANCHOR_DISABLED", "主播账号状态异常", 0), anchor, nil
	}

	// 第一版仅禁止高风险主播；中风险不擅自扩大限制。风险等级只保存事实状态，
	// 不在 risk/update 中联动改权限，所有实时决策集中在这里。
	if !canLiveByRiskLevel(anchor.RiskLevel) {
		return denied("RISK_LEVEL_BLOCKED", "当前风险等级不允许开播", 0), anchor, nil
	}
	if anchor.LivePermission != liveModel.AnchorPermissionEnabled {
		return denied("LIVE_PERMISSION_DISABLED", "主播开播权限未开启", 0), anchor, nil
	}
	if certificationRequiredForLive && anchor.CertStatus != liveModel.AnchorCertStatusApproved {
		return denied("CERTIFICATION_REQUIRED", "完成主播认证后才能开播", 0), anchor, nil
	}
	return allowed(), anchor, nil
}

func (s *AnchorService) GetAnchorList(req *liveReq.AnchorAdminListReq) ([]liveRes.AnchorAdminListItemResp, int64, error) {
	if global.GVA_DB == nil {
		return nil, 0, errors.New("数据库未初始化")
	}
	db := global.GVA_DB.Model(&liveModel.LiveAnchor{})
	if req.AnchorId > 0 {
		db = db.Where("id = ?", req.AnchorId)
	}
	if req.AnchorNo != "" {
		db = db.Where("anchor_no LIKE ?", "%"+strings.TrimSpace(req.AnchorNo)+"%")
	}
	if req.UserId > 0 {
		db = db.Where("user_id = ?", req.UserId)
	}
	if req.Nickname != "" {
		db = db.Where("nickname LIKE ?", "%"+strings.TrimSpace(req.Nickname)+"%")
	}
	if req.AnchorType != nil {
		db = db.Where("anchor_type = ?", *req.AnchorType)
	}
	if req.CategoryId > 0 {
		db = db.Where("category_id = ?", req.CategoryId)
	}
	if req.AgencyId > 0 {
		db = db.Where("agency_id = ?", req.AgencyId)
	}
	if req.ApplyStatus != nil {
		db = db.Where("apply_status = ?", *req.ApplyStatus)
	}
	if req.CertStatus != nil {
		db = db.Where("cert_status = ?", *req.CertStatus)
	}
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}
	if req.LivePermission != nil {
		db = db.Where("live_permission = ?", *req.LivePermission)
	}
	if req.PkPermission != nil {
		db = db.Where("pk_permission = ?", *req.PkPermission)
	}
	if req.RecommendPermission != nil {
		db = db.Where("recommend_permission = ?", *req.RecommendPermission)
	}
	if req.WithdrawPermission != nil {
		db = db.Where("withdraw_permission = ?", *req.WithdrawPermission)
	}
	if req.IsSigned != nil {
		db = db.Where("is_signed = ?", *req.IsSigned)
	}
	if req.IsRecommended != nil {
		db = db.Where("is_recommended = ?", *req.IsRecommended)
	}
	if req.RiskLevel != nil {
		db = db.Where("risk_level = ?", *req.RiskLevel)
	}
	if req.Source != "" {
		db = db.Where("source = ?", strings.TrimSpace(req.Source))
	}
	if req.ChannelId > 0 {
		db = db.Where("channel_id = ?", req.ChannelId)
	}
	var err error
	db, err = applyCreatedAtRange(db, req.CreatedAtStart, req.CreatedAtEnd)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err = db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var anchors []liveModel.LiveAnchor
	if err = db.Scopes(req.PageInfo.Paginate()).Order("id DESC").Find(&anchors).Error; err != nil {
		return nil, 0, err
	}
	list := make([]liveRes.AnchorAdminListItemResp, 0, len(anchors))
	for _, anchor := range anchors {
		list = append(list, toAnchorAdminListItem(anchor))
	}
	return list, total, nil
}

func (s *AnchorService) GetAnchorAdminDetail(anchorID uint) (liveRes.AnchorAdminDetailResp, error) {
	anchor, err := s.getAnchorByID(global.GVA_DB, anchorID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveRes.AnchorAdminDetailResp{}, ErrAnchorNotFound
	}
	if err != nil {
		return liveRes.AnchorAdminDetailResp{}, err
	}
	return toAnchorAdminDetail(*anchor), nil
}

func (s *AnchorService) AuditAnchor(adminID uint64, req liveReq.AnchorAuditReq) error {
	if req.ApplyStatus != liveModel.AnchorApplyStatusApproved && req.ApplyStatus != liveModel.AnchorApplyStatusRejected {
		return ErrInvalidApplyStatus
	}
	if req.ApplyStatus == liveModel.AnchorApplyStatusRejected && strings.TrimSpace(req.RejectReason) == "" {
		return ErrRejectReasonRequired
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		anchor, err := s.getAnchorByIDForUpdate(tx, req.AnchorId)
		if err != nil {
			return normalizeAnchorLookupError(err)
		}
		// 审核历史只能从“审核中”走向“通过/拒绝”；已通过资格的撤销由状态或权限接口完成。
		if anchor.ApplyStatus != liveModel.AnchorApplyStatusPending {
			return ErrInvalidApplyStatus
		}
		updates := map[string]interface{}{
			"apply_status":  req.ApplyStatus,
			"audit_at":      time.Now().UnixMilli(),
			"audit_user_id": adminID,
		}
		if req.ApplyStatus == liveModel.AnchorApplyStatusApproved {
			updates["reject_reason"] = ""
			// 审核和业务权限保持分离：第一版不自动开启直播、PK 或提现权限。
		} else {
			updates["reject_reason"] = strings.TrimSpace(req.RejectReason)
			updates["live_permission"] = liveModel.AnchorPermissionDisabled
			updates["pk_permission"] = liveModel.AnchorPermissionDisabled
		}
		return tx.Model(&liveModel.LiveAnchor{}).Where("id = ?", anchor.ID).Updates(updates).Error
	})
}

func (s *AnchorService) UpdateAnchorStatus(req liveReq.AnchorStatusUpdateReq) error {
	if req.Status == nil {
		return errors.New("主播状态不能为空")
	}
	now := time.Now().UnixMilli()
	status := *req.Status
	reason := strings.TrimSpace(req.StatusReason)
	if (status == liveModel.AnchorStatusDisabled || status == liveModel.AnchorStatusBanned) && reason == "" {
		return ErrStatusReasonRequired
	}
	if status == liveModel.AnchorStatusBanned && req.BanUntil != 0 && req.BanUntil <= now {
		return ErrInvalidBanUntil
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		anchor, err := s.getAnchorByIDForUpdate(tx, req.AnchorId)
		if err != nil {
			return normalizeAnchorLookupError(err)
		}
		// 注销保留主播 ID 和历史数据，是不可由普通状态接口逆转的强状态。
		if anchor.Status == liveModel.AnchorStatusCancelled && status != liveModel.AnchorStatusCancelled {
			return ErrCancelledCannotRestore
		}
		updates := map[string]interface{}{"status": status}
		switch status {
		case liveModel.AnchorStatusNormal:
			updates["status_reason"] = ""
			updates["ban_until"] = int64(0)
		case liveModel.AnchorStatusDisabled:
			updates["status_reason"] = reason
			updates["ban_until"] = int64(0)
		case liveModel.AnchorStatusBanned:
			updates["status_reason"] = reason
			updates["ban_until"] = req.BanUntil
		case liveModel.AnchorStatusCancelled:
			updates["status_reason"] = reason
			updates["ban_until"] = int64(0)
		default:
			return errors.New("无效的主播状态")
		}
		return tx.Model(&liveModel.LiveAnchor{}).Where("id = ?", anchor.ID).Updates(updates).Error
	})
}

func (s *AnchorService) UpdateAnchorPermission(req liveReq.AnchorPermissionUpdateReq) error {
	if req.LivePermission == nil || req.PkPermission == nil || req.RecommendPermission == nil || req.WithdrawPermission == nil {
		return errors.New("主播权限字段不能为空")
	}
	if *req.LivePermission == liveModel.AnchorPermissionDisabled && *req.PkPermission == liveModel.AnchorPermissionEnabled {
		return ErrInvalidPermission
	}
	if _, err := s.getAnchorByID(global.GVA_DB, req.AnchorId); err != nil {
		return normalizeAnchorLookupError(err)
	}
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Updates(map[string]interface{}{
		"live_permission":      *req.LivePermission,
		"pk_permission":        *req.PkPermission,
		"recommend_permission": *req.RecommendPermission,
		"withdraw_permission":  *req.WithdrawPermission,
	}).Error
}

func (s *AnchorService) UpdateAnchorProfile(req liveReq.AnchorAdminProfileUpdateReq) error {
	if _, err := s.getAnchorByID(global.GVA_DB, req.AnchorId); err != nil {
		return normalizeAnchorLookupError(err)
	}
	birthday, err := parseBirthday(req.Birthday)
	if err != nil {
		return err
	}
	if err = (&CategoryService{}).EnsureEnabledCategory(req.CategoryId); err != nil {
		return err
	}
	updates, err := publicProfileUpdates(req.Nickname, req.Avatar, req.Cover, req.Signature, req.Gender, birthday,
		req.CountryCode, req.RegionCode, req.CityCode, req.Language, req.CategoryId, req.TagIds)
	if err != nil {
		return err
	}
	updates["anchor_type"] = req.AnchorType
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Updates(updates).Error
}

func (s *AnchorService) UpdateAnchorRecommend(req liveReq.AnchorRecommendUpdateReq) error {
	if req.IsRecommended == nil {
		return errors.New("运营推荐状态不能为空")
	}
	anchor, err := s.getAnchorByID(global.GVA_DB, req.AnchorId)
	if err != nil {
		return normalizeAnchorLookupError(err)
	}
	// RecommendPermission 是“能否进入推荐系统”，IsRecommended 是人工运营标记。
	// 前者关闭时拒绝设置后者，避免保存相互矛盾的数据。
	if *req.IsRecommended == 1 && anchor.RecommendPermission != liveModel.AnchorPermissionEnabled {
		return ErrRecommendPermissionClosed
	}
	if req.NewcomerUntil < 0 {
		return errors.New("新人期截止时间不能为负数")
	}
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Updates(map[string]interface{}{
		"is_recommended":   *req.IsRecommended,
		"newcomer_until":   req.NewcomerUntil,
		"sort":             req.Sort,
		"recommend_weight": req.RecommendWeight,
	}).Error
}

func (s *AnchorService) UpdateAnchorSigned(req liveReq.AnchorSignedUpdateReq) error {
	if req.IsSigned == nil {
		return errors.New("签约状态不能为空")
	}
	if _, err := s.getAnchorByID(global.GVA_DB, req.AnchorId); err != nil {
		return normalizeAnchorLookupError(err)
	}
	// 签约、账号性质和公会归属是三个独立概念，本接口只改 is_signed。
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Update("is_signed", *req.IsSigned).Error
}

func (s *AnchorService) UpdateAnchorAgency(req liveReq.AnchorAgencyUpdateReq) error {
	anchor, err := s.getAnchorByID(global.GVA_DB, req.AnchorId)
	if err != nil {
		return normalizeAnchorLookupError(err)
	}
	if anchor.AgencyId == req.AgencyId {
		return nil
	}
	joinAt := int64(0)
	if req.AgencyId != 0 {
		joinAt = time.Now().UnixMilli()
	}
	// 当前仓库尚无 Agency 模型，因此不创建虚假的公会表或外键；
	// 接入公会模块后应在此处补充“存在且状态合法”的校验。
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Updates(map[string]interface{}{
		"agency_id":      req.AgencyId,
		"agency_join_at": joinAt,
	}).Error
}

func (s *AnchorService) UpdateAnchorRisk(req liveReq.AnchorRiskUpdateReq) error {
	if req.RiskLevel == nil {
		return errors.New("风险等级不能为空")
	}
	if _, err := s.getAnchorByID(global.GVA_DB, req.AnchorId); err != nil {
		return normalizeAnchorLookupError(err)
	}
	// 风险等级记录风险事实，不在这里偷偷改直播、PK 或提现权限；实时资格由 check 统一计算。
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Update("risk_level", *req.RiskLevel).Error
}

func (s *AnchorService) UpdateAnchorRemark(req liveReq.AnchorRemarkUpdateReq) error {
	if utf8.RuneCountInString(req.Remark) > 500 {
		return errors.New("后台备注不能超过 500 个字符")
	}
	if _, err := s.getAnchorByID(global.GVA_DB, req.AnchorId); err != nil {
		return normalizeAnchorLookupError(err)
	}
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Update("remark", req.Remark).Error
}

func (s *AnchorService) UpdateAnchorCert(req liveReq.AnchorCertUpdateReq) error {
	if req.CertStatus == nil || req.CertType == nil {
		return ErrInvalidCertState
	}
	status, certType := *req.CertStatus, *req.CertType
	name := strings.TrimSpace(req.CertName)
	// 未认证必须清空类型/展示名；认证中和已认证必须明确认证类型；
	// 已认证还必须有可展示名称。证件资料不存入 LiveAnchor。
	if status == liveModel.AnchorCertStatusNone && (certType != liveModel.AnchorCertTypeNone || name != "") {
		return ErrInvalidCertState
	}
	if (status == liveModel.AnchorCertStatusPending || status == liveModel.AnchorCertStatusApproved) && certType == liveModel.AnchorCertTypeNone {
		return ErrInvalidCertState
	}
	if status == liveModel.AnchorCertStatusApproved && name == "" {
		return ErrInvalidCertState
	}
	if _, err := s.getAnchorByID(global.GVA_DB, req.AnchorId); err != nil {
		return normalizeAnchorLookupError(err)
	}
	return global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", req.AnchorId).Updates(map[string]interface{}{
		"cert_status": status,
		"cert_type":   certType,
		"cert_name":   name,
	}).Error
}

func (s *AnchorService) getAnchorByUserID(db *gorm.DB, userID uint64) (*liveModel.LiveAnchor, error) {
	if db == nil {
		return nil, errors.New("数据库未初始化")
	}
	var anchor liveModel.LiveAnchor
	err := db.Where("user_id = ?", userID).First(&anchor).Error
	return &anchor, err
}

func (s *AnchorService) getAnchorByID(db *gorm.DB, anchorID uint) (*liveModel.LiveAnchor, error) {
	if db == nil {
		return nil, errors.New("数据库未初始化")
	}
	var anchor liveModel.LiveAnchor
	err := db.First(&anchor, anchorID).Error
	return &anchor, err
}

func (s *AnchorService) getAnchorByIDForUpdate(db *gorm.DB, anchorID uint) (*liveModel.LiveAnchor, error) {
	var anchor liveModel.LiveAnchor
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&anchor, anchorID).Error
	return &anchor, err
}

func normalizeAnchorLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAnchorNotFound
	}
	return err
}

func temporaryAnchorNo() string {
	raw := strings.ReplaceAll(uuid.NewString(), "-", "")
	return "tmp_" + raw[:28]
}

func formatAnchorNo(channelID uint64, anchorID uint) string {
	return fmt.Sprintf("%d%06d", channelID, uint64(anchorID)+100000)
}

func parseBirthday(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.DateOnly, strings.TrimSpace(*value))
	if err != nil || parsed.After(time.Now()) {
		return nil, ErrInvalidBirthday
	}
	return &parsed, nil
}

func tagIDsDBValue(tagIDs []uint64) (interface{}, error) {
	if tagIDs == nil {
		return nil, nil
	}
	data, err := json.Marshal(tagIDs)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func publicProfileUpdates(nickname, avatar, cover, signature string, gender uint8, birthday *time.Time,
	countryCode, regionCode, cityCode, language string, categoryID uint64, tagIDs []uint64,
) (map[string]interface{}, error) {
	tags, err := tagIDsDBValue(tagIDs)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"nickname":     strings.TrimSpace(nickname),
		"avatar":       strings.TrimSpace(avatar),
		"cover":        strings.TrimSpace(cover),
		"signature":    strings.TrimSpace(signature),
		"gender":       gender,
		"birthday":     birthday,
		"country_code": strings.ToUpper(strings.TrimSpace(countryCode)),
		"region_code":  strings.TrimSpace(regionCode),
		"city_code":    strings.TrimSpace(cityCode),
		"language":     strings.TrimSpace(language),
		"category_id":  categoryID,
		"tag_ids":      tags,
	}, nil
}

func applyCreatedAtRange(db *gorm.DB, start, end string) (*gorm.DB, error) {
	var startTime, endTime time.Time
	var err error
	if start != "" {
		startTime, err = time.Parse(time.DateOnly, start)
		if err != nil {
			return nil, ErrInvalidDateRange
		}
		db = db.Where("created_at >= ?", startTime)
	}
	if end != "" {
		endTime, err = time.Parse(time.DateOnly, end)
		if err != nil {
			return nil, ErrInvalidDateRange
		}
		// 日期筛选的结束日按闭区间理解，SQL 使用次日零点的开区间以覆盖整天。
		db = db.Where("created_at < ?", endTime.AddDate(0, 0, 1))
	}
	if !startTime.IsZero() && !endTime.IsZero() && startTime.After(endTime) {
		return nil, ErrInvalidDateRange
	}
	return db, nil
}

func canLiveByRiskLevel(level uint8) bool {
	return level != liveModel.AnchorRiskHigh
}

func allowed() liveRes.AnchorPermissionCheckResp {
	return liveRes.AnchorPermissionCheckResp{Allow: true, Code: "OK", Message: ""}
}

func denied(code, message string, banUntil int64) liveRes.AnchorPermissionCheckResp {
	return liveRes.AnchorPermissionCheckResp{Allow: false, Code: code, Message: message, BanUntil: banUntil}
}

func cloneUint64s(values []uint64) []uint64 {
	if values == nil {
		return nil
	}
	return append([]uint64(nil), values...)
}

func formatBirthday(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.DateOnly)
}

func toMyAnchorInfo(anchor liveModel.LiveAnchor) liveRes.MyAnchorInfo {
	return liveRes.MyAnchorInfo{
		AnchorNo: anchor.AnchorNo, Nickname: anchor.Nickname, Avatar: anchor.Avatar,
		Cover: anchor.Cover, Signature: anchor.Signature, Gender: anchor.Gender, Birthday: formatBirthday(anchor.Birthday),
		CountryCode: anchor.CountryCode, RegionCode: anchor.RegionCode, CityCode: anchor.CityCode, Language: anchor.Language,
		AnchorType: anchor.AnchorType, CategoryId: anchor.CategoryId, Level: anchor.Level, TagIds: cloneUint64s(anchor.TagIds),
		AgencyId: anchor.AgencyId, ApplyStatus: anchor.ApplyStatus, ApplyAt: anchor.ApplyAt, AuditAt: anchor.AuditAt,
		RejectReason: anchor.RejectReason, CertStatus: anchor.CertStatus, CertType: anchor.CertType, CertName: anchor.CertName,
		Status: anchor.Status, StatusReason: anchor.StatusReason, BanUntil: anchor.BanUntil,
		LivePermission: anchor.LivePermission, PkPermission: anchor.PkPermission,
		RecommendPermission: anchor.RecommendPermission, WithdrawPermission: anchor.WithdrawPermission,
		IsSigned: anchor.IsSigned, FansCount: anchor.FansCount, TotalLiveCount: anchor.TotalLiveCount,
		TotalLiveDuration: anchor.TotalLiveDuration, MaxOnlineCount: anchor.MaxOnlineCount, TotalViewCount: anchor.TotalViewCount,
		LastLiveAt: anchor.LastLiveAt, LastLiveEndAt: anchor.LastLiveEndAt,
	}
}

func toAnchorPublicDetail(anchor liveModel.LiveAnchor) liveRes.AnchorPublicDetailResp {
	return liveRes.AnchorPublicDetailResp{
		AnchorNo: anchor.AnchorNo, Nickname: anchor.Nickname, Avatar: anchor.Avatar, Cover: anchor.Cover,
		Signature: anchor.Signature, Gender: anchor.Gender, CountryCode: anchor.CountryCode, RegionCode: anchor.RegionCode,
		CityCode: anchor.CityCode, Language: anchor.Language, AnchorType: anchor.AnchorType, CategoryId: anchor.CategoryId,
		Level: anchor.Level, TagIds: cloneUint64s(anchor.TagIds), CertStatus: anchor.CertStatus, CertType: anchor.CertType,
		CertName: anchor.CertName, IsSigned: anchor.IsSigned, FansCount: anchor.FansCount,
		TotalLiveCount: anchor.TotalLiveCount, TotalLiveDuration: anchor.TotalLiveDuration,
		MaxOnlineCount: anchor.MaxOnlineCount, TotalViewCount: anchor.TotalViewCount,
		LastLiveAt: anchor.LastLiveAt, IsRecommended: anchor.IsRecommended,
	}
}

func toAnchorAdminListItem(anchor liveModel.LiveAnchor) liveRes.AnchorAdminListItemResp {
	return liveRes.AnchorAdminListItemResp{
		ID: anchor.ID, CreatedAt: anchor.CreatedAt, UserId: anchor.UserId, AnchorNo: anchor.AnchorNo,
		Nickname: anchor.Nickname, Avatar: anchor.Avatar, AnchorType: anchor.AnchorType, CategoryId: anchor.CategoryId,
		AgencyId: anchor.AgencyId, ApplyStatus: anchor.ApplyStatus, CertStatus: anchor.CertStatus, Status: anchor.Status,
		LivePermission: anchor.LivePermission, PkPermission: anchor.PkPermission,
		RecommendPermission: anchor.RecommendPermission, WithdrawPermission: anchor.WithdrawPermission,
		IsSigned: anchor.IsSigned, IsRecommended: anchor.IsRecommended, RiskLevel: anchor.RiskLevel,
		Source: anchor.Source, ChannelId: anchor.ChannelId,
	}
}

func toAnchorAdminDetail(anchor liveModel.LiveAnchor) liveRes.AnchorAdminDetailResp {
	return liveRes.AnchorAdminDetailResp{
		ID: anchor.ID, CreatedAt: anchor.CreatedAt, UpdatedAt: anchor.UpdatedAt, UserId: anchor.UserId, AnchorNo: anchor.AnchorNo,
		Nickname: anchor.Nickname, Avatar: anchor.Avatar, Cover: anchor.Cover, Signature: anchor.Signature, Gender: anchor.Gender,
		Birthday: formatBirthday(anchor.Birthday), CountryCode: anchor.CountryCode, RegionCode: anchor.RegionCode,
		CityCode: anchor.CityCode, Language: anchor.Language, AnchorType: anchor.AnchorType, CategoryId: anchor.CategoryId,
		Level: anchor.Level, TagIds: cloneUint64s(anchor.TagIds), AgencyId: anchor.AgencyId, AgencyJoinAt: anchor.AgencyJoinAt,
		ApplyStatus: anchor.ApplyStatus, ApplyAt: anchor.ApplyAt, AuditAt: anchor.AuditAt, AuditUserId: anchor.AuditUserId,
		RejectReason: anchor.RejectReason, CertStatus: anchor.CertStatus, CertType: anchor.CertType, CertName: anchor.CertName,
		Status: anchor.Status, StatusReason: anchor.StatusReason, BanUntil: anchor.BanUntil,
		LivePermission: anchor.LivePermission, PkPermission: anchor.PkPermission,
		RecommendPermission: anchor.RecommendPermission, WithdrawPermission: anchor.WithdrawPermission,
		IsSigned: anchor.IsSigned, IsRecommended: anchor.IsRecommended, NewcomerUntil: anchor.NewcomerUntil,
		Sort: anchor.Sort, RecommendWeight: anchor.RecommendWeight, FansCount: anchor.FansCount,
		TotalLiveCount: anchor.TotalLiveCount, TotalLiveDuration: anchor.TotalLiveDuration,
		MaxOnlineCount: anchor.MaxOnlineCount, TotalViewCount: anchor.TotalViewCount,
		LastLiveAt: anchor.LastLiveAt, LastLiveEndAt: anchor.LastLiveEndAt,
		Source: anchor.Source, SourceId: anchor.SourceId, ChannelId: anchor.ChannelId,
		RiskLevel: anchor.RiskLevel, Remark: anchor.Remark, Extra: anchor.Extra,
	}
}
