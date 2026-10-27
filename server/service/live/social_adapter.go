package live

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	liveModel "tb_live_module/model/live"
	socialReq "tb_live_module/model/social/request"
	socialRes "tb_live_module/model/social/response"
	socialService "tb_live_module/service/social"
)

// SocialDirectory 是社交模块访问账号/主播数据的唯一适配边界。
type SocialDirectory struct{}

func (SocialDirectory) Resolve(ctx context.Context, db *gorm.DB, targets []socialReq.Target) ([]uint64, error) {
	accounts, anchors := []string{}, []string{}
	for _, t := range targets {
		if t.AccountNo != "" {
			accounts = append(accounts, t.AccountNo)
		} else {
			anchors = append(anchors, t.AnchorNo)
		}
	}
	accountMap, anchorMap := map[string]uint64{}, map[string]uint64{}
	if len(accounts) > 0 {
		var rows []liveModel.LiveAccount
		if err := db.WithContext(ctx).Unscoped().Select("id", "account_no").Where("account_no IN ?", accounts).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			accountMap[r.AccountNo] = uint64(r.ID)
		}
	}
	if len(anchors) > 0 {
		var rows []liveModel.LiveAnchor
		if err := db.WithContext(ctx).Unscoped().Select("user_id", "anchor_no").Where("anchor_no IN ?", anchors).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			anchorMap[r.AnchorNo] = r.UserId
		}
	}
	out := []uint64{}
	for _, t := range targets {
		id := accountMap[t.AccountNo]
		if t.AnchorNo != "" {
			id = anchorMap[t.AnchorNo]
		}
		if id == 0 {
			return nil, socialService.ErrUnavailableUser
		}
		out = append(out, id)
	}
	return out, nil
}
func (SocialDirectory) Validate(ctx context.Context, db *gorm.DB, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	ids = append([]uint64(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var rows []liveModel.LiveAccount
	if err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Select("id", "status").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return err
	}
	found := map[uint64]bool{}
	for _, r := range rows {
		if r.Status == liveModel.LiveAccountStatusNormal {
			found[uint64(r.ID)] = true
		}
	}
	for _, id := range ids {
		if !found[id] {
			return socialService.ErrUnavailableUser
		}
	}
	return nil
}

// publicAccountProfiles 只选择统一公开DTO字段，账号身份和主播身份不混用。
func publicAccountProfiles(ctx context.Context, db *gorm.DB, ids []uint64) (map[uint64]socialRes.User, error) {
	out := make(map[uint64]socialRes.User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > 100 {
		return nil, socialService.ErrInvalid
	}
	var accounts []struct {
		ID             uint64
		socialRes.User `gorm:"embedded"`
	}
	err := db.WithContext(ctx).Table("live_account u").
		Select("u.id,u.account_no,u.nickname,u.avatar,u.signature,u.gender,u.country_code,COALESCE(a.anchor_no,'') AS anchor_no").
		Joins("LEFT JOIN live_anchor a ON a.user_id=u.id AND a.deleted_at IS NULL AND a.status = ? AND a.apply_status = ?", liveModel.AnchorStatusNormal, liveModel.AnchorApplyStatusApproved).
		Where("u.id IN ? AND u.deleted_at IS NULL AND u.status = ?", ids, liveModel.LiveAccountStatusNormal).Scan(&accounts).Error
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		out[a.ID] = a.User
	}
	return out, nil
}

// Profiles 最多100人，最多两条批量SQL；资料不缓存，直播与关系权限实时查询。
func (SocialDirectory) Profiles(ctx context.Context, db *gorm.DB, viewer uint64, ids []uint64) (map[uint64]socialRes.User, error) {
	out, err := publicAccountProfiles(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	liveIDs := make([]uint64, 0, len(out))
	for id, u := range out {
		if u.AnchorNo != "" {
			liveIDs = append(liveIDs, id)
		}
	}
	if len(liveIDs) == 0 {
		return out, nil
	}
	var rooms []LiveAccessSnapshot
	if err = liveAccessQuery(ctx, db, viewer, true).Where("a.user_id IN ?", liveIDs).Scan(&rooms).Error; err != nil {
		return nil, err
	}
	for _, room := range rooms {
		u := out[room.UserID]
		u.IsLive, u.RoomNo, u.SessionNo = true, room.RoomNo, room.SessionNo
		out[room.UserID] = u
	}
	return out, nil
}
func (SocialDirectory) IsAnchor(ctx context.Context, db *gorm.DB, id uint64) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&liveModel.LiveAnchor{}).Where("user_id = ? AND status = ? AND apply_status = ?", id, liveModel.AnchorStatusNormal, liveModel.AnchorApplyStatusApproved).Count(&n).Error
	return n > 0, err
}
func (SocialDirectory) Online(ctx context.Context, db *gorm.DB, actor uint64, ids []uint64) ([]socialRes.OnlineRoom, error) {
	out := []socialRes.OnlineRoom{}
	if len(ids) > 100 {
		return nil, socialService.ErrInvalid
	}
	if len(ids) == 0 {
		return out, nil
	}
	var rooms []LiveAccessSnapshot
	if err := liveAccessQuery(ctx, db, actor, true).Where("a.user_id IN ?", ids).Scan(&rooms).Error; err != nil {
		return nil, err
	}
	byUser := make(map[uint64]LiveAccessSnapshot, len(rooms))
	liveIDs := make([]uint64, 0, len(rooms))
	for _, room := range rooms {
		byUser[room.UserID] = room
		liveIDs = append(liveIDs, room.UserID)
	}
	profiles, err := publicAccountProfiles(ctx, db, liveIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		r, live := byUser[id]
		u, visible := profiles[id]
		if live && visible {
			u.IsLive, u.RoomNo, u.SessionNo = true, r.RoomNo, r.SessionNo
			out = append(out, socialRes.OnlineRoom{User: u, Title: r.Title, CoverURL: r.CoverURL})
		}
	}
	return out, nil
}
func (SocialDirectory) LiveEventValid(ctx context.Context, db *gorm.DB, id uint64, sessionNo string) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Table("live_session s").Joins("JOIN live_room r ON r.id=s.room_id AND r.current_session_id=s.id AND r.deleted_at IS NULL").Joins("JOIN live_anchor a ON a.id=r.anchor_id AND a.deleted_at IS NULL").Joins("JOIN live_account u ON u.id=a.user_id AND u.deleted_at IS NULL").Where("s.deleted_at IS NULL AND s.session_no = ? AND a.user_id = ? AND s.status = ? AND r.status = ? AND r.live_status = ? AND r.visibility <> ? AND a.status = ? AND a.apply_status = ? AND a.live_permission = ? AND a.risk_level < ? AND u.status = ?", sessionNo, id, liveModel.LiveSessionLiving, liveModel.LiveRoomStatusNormal, liveModel.LiveRoomLiving, liveModel.LiveRoomVisibilityPrivate, liveModel.AnchorStatusNormal, liveModel.AnchorApplyStatusApproved, liveModel.AnchorPermissionEnabled, liveModel.AnchorRiskHigh, liveModel.LiveAccountStatusNormal).Count(&n).Error
	return n > 0, err
}

// SocialRoomResource 收藏允许正常离线房间；播放必须额外校验当前场次。
type SocialRoomResource struct{}

func (SocialRoomResource) Resolve(ctx context.Context, db *gorm.DB, actor uint64, refs []socialReq.Resource) (map[socialReq.Resource]socialRes.Resource, error) {
	nos := []string{}
	for _, r := range refs {
		nos = append(nos, r.ResourceID)
	}
	rows, err := (&LiveAccessService{}).BatchCheckLiveAccess(ctx, db, actor, nos, false)
	if err != nil {
		return nil, err
	}
	out := map[socialReq.Resource]socialRes.Resource{}
	for _, r := range refs {
		if row, ok := rows[r.ResourceID]; ok {
			out[r] = socialRes.Resource{OwnerUserID: row.UserID, ResourceType: r.ResourceType, ResourceID: r.ResourceID, Title: row.Title, CoverURL: row.CoverURL}
		}
	}
	return out, nil
}
func applySocialFansCounts(ctx context.Context, db *gorm.DB, anchors []liveModel.LiveAnchor) error {
	ids := []uint64{}
	for _, a := range anchors {
		ids = append(ids, a.UserId)
	}
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		counts, err := socialService.Default.FansCounts(ctx, db, ids[start:end])
		if err != nil {
			return err
		}
		for i := start; i < end; i++ {
			anchors[i].FansCount = counts[anchors[i].UserId]
		}
	}
	return nil
}
