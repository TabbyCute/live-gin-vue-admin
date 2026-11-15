package anchorhome

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"regexp"
	"tb_live_module/model/live"
	"tb_live_module/model/system"
)

var opaque = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// VisibleAnchor reads current authorization facts, never a cache or JWT snapshot.
func VisibleAnchor(ctx context.Context, db *gorm.DB, viewer uint64, anchorNo string) (live.LiveAnchor, error) {
	var a live.LiveAnchor
	if db == nil {
		return a, ErrUnavailable
	}
	if len(anchorNo) > 32 || !opaque.MatchString(anchorNo) {
		return a, ErrInvalid
	}
	db = db.WithContext(ctx)
	q := db.Model(&live.LiveAnchor{}).Where("anchor_no=? AND apply_status=? AND status=?", anchorNo, live.AnchorApplyStatusApproved, live.AnchorStatusNormal).
		Where("EXISTS (SELECT 1 FROM live_account u WHERE u.id=live_anchor.user_id AND u.status=1 AND u.deleted_at IS NULL)")
	if viewer > 0 {
		var n int64
		if err := db.Model(&live.LiveAccount{}).Where("id=? AND status=1", viewer).Count(&n).Error; err != nil {
			return a, err
		}
		if n != 1 {
			return a, ErrForbidden
		}
		q = q.Where("NOT EXISTS (SELECT 1 FROM social_block b WHERE (b.user_id=? AND b.target_user_id=live_anchor.user_id) OR (b.target_user_id=? AND b.user_id=live_anchor.user_id))", viewer, viewer)
	}
	err := q.Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return a, err
}

func ownerAnchor(db *gorm.DB, owner uint64) (live.LiveAnchor, error) {
	var a live.LiveAnchor
	if owner == 0 {
		return a, ErrForbidden
	}
	err := db.Model(&live.LiveAnchor{}).Where("user_id=? AND status=? AND apply_status=?", owner, live.AnchorStatusNormal, live.AnchorApplyStatusApproved).
		Where("EXISTS (SELECT 1 FROM live_account u WHERE u.id=live_anchor.user_id AND u.status=1 AND u.deleted_at IS NULL)").Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrForbidden
	}
	return a, err
}

func albumAdmin(db *gorm.DB, actor uint64) error {
	// Keep the optional row-lock clause while isolating each model's SELECT/WHERE.
	db = db.Session(&gorm.Session{})
	if actor == 0 {
		return ErrForbidden
	}
	var user system.SysUser
	err := db.Select("id", "enable", "authority_id").Where("id=? AND enable=1", actor).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if system.IsSuperAdmin(actor) {
		return nil
	}
	var n int64
	err = db.Clauses(clause.Locking{Strength: "SHARE"}).Table("sys_user_authority ua").Joins("JOIN sys_authorities a ON a.authority_id=ua.sys_authority_authority_id AND a.deleted_at IS NULL").Where("ua.sys_user_id=? AND ua.sys_authority_authority_id=888", actor).Count(&n).Error
	if err != nil {
		return err
	}
	if n != 1 || user.AuthorityId != 888 {
		return ErrForbidden
	}
	return nil
}
