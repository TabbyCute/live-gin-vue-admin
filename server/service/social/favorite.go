package social

import (
	"context"
	"gorm.io/gorm"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	repo "tb_live_module/repository/social"
)

func (s *Service) Favorite(ctx context.Context, actor uint64, r req.Resource, enabled bool) (res.Change, error) {
	if err := s.ready(); err != nil {
		return res.Change{}, err
	}
	if !validResource(r) {
		return res.Change{}, ErrInvalid
	}
	changed := false
	err := s.repository().Transaction(ctx, func(tx *gorm.DB) error {
		changed = false
		if _, err := repo.LockUsers(tx, actor); err != nil {
			return err
		}
		if err := s.Directory.Validate(ctx, tx, []uint64{actor}); err != nil {
			return err
		}
		var result *gorm.DB
		if enabled {
			visible, err := s.resources(ctx, tx, actor, []req.Resource{r})
			if err != nil {
				return err
			}
			if _, ok := visible[r]; !ok {
				return ErrResource
			}
			changed, err = repo.InsertAbsent(tx, &model.SocialFavorite{UserID: actor, ResourceType: r.ResourceType, ResourceID: r.ResourceID}, "user_id = ? AND resource_type = ? AND resource_id = ?", actor, r.ResourceType, r.ResourceID)
			return err
		} else {
			result = tx.Where("user_id = ? AND resource_type = ? AND resource_id = ?", actor, r.ResourceType, r.ResourceID).Delete(&model.SocialFavorite{})
		}
		if result.Error != nil {
			return result.Error
		}
		changed = result.RowsAffected > 0
		return nil
	})
	return res.Change{Changed: changed && err == nil}, err
}
func (s *Service) FavoriteStatus(ctx context.Context, actor uint64, refs []req.Resource) ([]res.FavoriteStatus, error) {
	visible, err := s.resources(ctx, s.db(ctx), actor, refs)
	if err != nil {
		return nil, err
	}
	query := s.db(ctx).Where("user_id = ?", actor)
	pairs := make([][]interface{}, 0, len(refs))
	for _, r := range refs {
		pairs = append(pairs, []interface{}{r.ResourceType, r.ResourceID})
	}
	var rows []model.SocialFavorite
	if err = query.Where("(resource_type,resource_id) IN ?", pairs).Find(&rows).Error; err != nil {
		return nil, err
	}
	favorites := map[req.Resource]bool{}
	for _, row := range rows {
		favorites[req.Resource{ResourceType: row.ResourceType, ResourceID: row.ResourceID}] = true
	}
	out := []res.FavoriteStatus{}
	seen := map[req.Resource]bool{}
	for _, r := range refs {
		if seen[r] {
			continue
		}
		seen[r] = true
		_, ok := visible[r]
		out = append(out, res.FavoriteStatus{ResourceType: r.ResourceType, ResourceID: r.ResourceID, Favorite: ok && favorites[r]})
	}
	return out, nil
}
