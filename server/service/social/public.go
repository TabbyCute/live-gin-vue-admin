package social

import (
	"context"
	"gorm.io/gorm"
	res "tb_live_module/model/social/response"
)

// PublicHomeSnapshot reads counters and both follow directions in one statement.
// The resource module authorizes the owner in this same database snapshot first.
func PublicHomeSnapshot(ctx context.Context, db *gorm.DB, viewer, owner uint64) (res.Counters, res.Relationship, error) {
	var r struct {
		FollowingCount uint64
		FansCount      uint64
		Following      bool
		FollowsYou     bool
	}
	err := db.WithContext(ctx).Table("live_account AS u").Select(`COALESCE(c.following_count,0) AS following_count, COALESCE(c.fans_count,0) AS fans_count,
EXISTS(SELECT 1 FROM social_follow f WHERE f.user_id=? AND f.target_user_id=u.id) AS following,
EXISTS(SELECT 1 FROM social_follow f WHERE f.user_id=u.id AND f.target_user_id=?) AS follows_you`, viewer, viewer).
		Joins("LEFT JOIN social_user_counter c ON c.user_id=u.id").Where("u.id=?", owner).Scan(&r).Error
	return res.Counters{FollowingCount: r.FollowingCount, FansCount: r.FansCount}, res.Relationship{Following: r.Following, FollowsYou: r.FollowsYou, Mutual: r.Following && r.FollowsYou}, err
}
