package social

import (
	"context"
	"gorm.io/gorm"
	model "tb_live_module/model/social"
)

type AccessRelationship struct {
	Following bool
	Blocked   bool
}

// VisibleOwnerScope 供资源模块在同一 SQL 快照内执行关系授权。
// owner/visibility 必须来自服务端固定列名，禁止传入客户端字段。
// 每个 EXISTS 都按关系唯一索引点查，避免为热门主播加载整份粉丝集合。
func VisibleOwnerScope(actor uint64, owner, visibility string, public, followers, private uint8) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if actor == 0 {
			return db.Where(visibility+" = ?", public)
		}
		return db.Where("NOT EXISTS (SELECT 1 FROM social_block b WHERE b.user_id = ? AND b.target_user_id = "+owner+")", actor).
			Where("NOT EXISTS (SELECT 1 FROM social_block b WHERE b.user_id = "+owner+" AND b.target_user_id = ?)", actor).
			Where("("+visibility+" = ? OR ("+visibility+" IN ? AND "+owner+" = ?) OR ("+visibility+" = ? AND EXISTS (SELECT 1 FROM social_follow f WHERE f.user_id = ? AND f.target_user_id = "+owner+")))", public, []int{int(followers), int(private)}, actor, followers, actor)
	}
}

// BatchAccessRelationships 只有数据库事实；live模块不直接操作social表。
func (s *Service) BatchAccessRelationships(ctx context.Context, db *gorm.DB, actor uint64, ids []uint64) (map[uint64]AccessRelationship, error) {
	out := map[uint64]AccessRelationship{}
	if len(ids) > 100 {
		return nil, ErrInvalid
	}
	if len(ids) == 0 || actor == 0 {
		return out, nil
	}
	if db == nil {
		return nil, ErrUnavailable
	}
	db = db.WithContext(ctx)
	var follows []model.SocialFollow
	if err := db.Where("user_id = ? AND target_user_id IN ?", actor, ids).Find(&follows).Error; err != nil {
		return nil, err
	}
	for _, f := range follows {
		r := out[f.TargetUserID]
		r.Following = true
		out[f.TargetUserID] = r
	}
	var blocks []model.SocialBlock
	if err := db.Where("(user_id = ? AND target_user_id IN ?) OR (target_user_id = ? AND user_id IN ?)", actor, ids, actor, ids).Find(&blocks).Error; err != nil {
		return nil, err
	}
	for _, b := range blocks {
		id := b.TargetUserID
		if id == actor {
			id = b.UserID
		}
		r := out[id]
		r.Blocked = true
		out[id] = r
	}
	return out, nil
}

// FansCounts 同步权威读，用于替换旧 LiveAnchor.fans_count 的读取。
func (s *Service) FansCounts(ctx context.Context, db *gorm.DB, ids []uint64) (map[uint64]uint64, error) {
	if len(ids) > 100 {
		return nil, ErrInvalid
	}
	out := map[uint64]uint64{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []model.SocialUserCounter
	if err := db.WithContext(ctx).Select("user_id", "fans_count").Where("user_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.UserID] = r.FansCount
	}
	return out, nil
}
