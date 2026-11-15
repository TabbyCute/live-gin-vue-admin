package anchorhome

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"tb_live_module/global"
	"tb_live_module/internal/modulekit"
	req "tb_live_module/model/anchorhome/request"
	res "tb_live_module/model/anchorhome/response"
	guardRes "tb_live_module/model/guard/response"
	model "tb_live_module/model/live"
	rankRes "tb_live_module/model/ranking/response"
	"tb_live_module/service/guard"
	"tb_live_module/service/live"
	"tb_live_module/service/ranking"
	"tb_live_module/service/social"
	"time"
	"unicode"
	"unicode/utf8"
)

type PublicService struct {
	DB  *gorm.DB
	Now func() time.Time
}

var DefaultPublic = &PublicService{}

func (s *PublicService) database(ctx context.Context) *gorm.DB {
	db := s.DB
	if db == nil {
		db = global.GVA_DB
	}
	if db == nil {
		return nil
	}
	return db.WithContext(ctx)
}
func (s *PublicService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s *PublicService) snapshot(ctx context.Context, fn func(*gorm.DB) error) error {
	db := s.database(ctx)
	if db == nil {
		return modulekit.Unavailable
	}
	return db.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func (s *PublicService) Profile(ctx context.Context, viewer uint64, anchorNo string) (res.Profile, error) {
	out := res.Profile{Tags: []string{}, Photos: []res.PublicPhoto{}}
	err := s.snapshot(ctx, func(tx *gorm.DB) error {
		a, err := VisibleAnchor(ctx, tx, viewer, anchorNo)
		if err != nil {
			return err
		}
		var account model.LiveAccount
		if err = tx.Select("account_no").First(&account, a.UserId).Error; err != nil {
			return err
		}
		counts, relation, err := social.PublicHomeSnapshot(ctx, tx, viewer, a.UserId)
		if err != nil {
			return err
		}
		photos, err := DefaultAlbum.ApprovedPhotosFromDB(ctx, tx, viewer, anchorNo)
		if err != nil {
			return err
		}
		out = res.Profile{AnchorNo: a.AnchorNo, AccountNo: account.AccountNo, Nickname: a.Nickname, Avatar: a.Avatar, Signature: a.Signature, Gender: a.Gender,
			CountryCode: a.CountryCode, Language: a.Language, Level: a.Level, Certified: a.CertStatus == model.AnchorCertStatusApproved,
			Tags: []string{}, Photos: photos, FollowingCount: strconv.FormatUint(counts.FollowingCount, 10), FansCount: strconv.FormatUint(counts.FansCount, 10), IsSelf: viewer > 0 && viewer == a.UserId,
			Relationship: res.Relationship{Following: relation.Following, FollowsYou: relation.FollowsYou, Mutual: relation.Mutual}, GeneratedAt: s.now().UnixMilli()}
		if a.CategoryId > 0 {
			var category model.LiveCategory
			err = tx.Select("name").Where("id=? AND status=?", a.CategoryId, model.LiveCategoryStatusEnabled).Take(&category).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				out.Tags = append(out.Tags, category.Name)
			}
		}
		rooms, err := (live.SocialDirectory{}).Online(ctx, tx, viewer, []uint64{a.UserId})
		if err != nil {
			return err
		}
		if len(rooms) > 0 {
			room := rooms[0]
			out.Live = &res.LiveCard{RoomNo: room.RoomNo, SessionNo: room.SessionNo, Title: room.Title, CoverURL: room.CoverURL}
		}
		return nil
	})
	return out, err
}

func homePage(page, size *int) error {
	if *page == 0 {
		*page = 1
	}
	if *size == 0 {
		*size = 20
	}
	if *page < 1 || *page > 100 || *size < 1 || *size > 50 {
		return modulekit.Invalid
	}
	return nil
}
func (s *PublicService) Supporters(ctx context.Context, viewer uint64, q req.SupportersQuery) (rankRes.Board, error) {
	var out rankRes.Board
	if err := homePage(&q.Page, &q.PageSize); err != nil {
		return out, err
	}
	if q.Period == "" {
		q.Period = "day"
	}
	if q.Period != "day" && q.Period != "month" && q.Period != "all" {
		return out, modulekit.Invalid
	}
	err := s.snapshot(ctx, func(tx *gorm.DB) error {
		if _, err := VisibleAnchor(ctx, tx, viewer, q.AnchorNo); err != nil {
			return err
		}
		rank := ranking.Service{DB: tx, Now: s.Now, Timezone: global.GVA_CONFIG.Ranking.Zone(), AccountEncoding: ranking.Default.AccountEncoding}
		var err error
		out, err = rank.AnchorSupportersSnapshot(ctx, viewer, q.AnchorNo, q.Period, q.Page, q.PageSize)
		return err
	})
	return out, err
}
func (s *PublicService) Guards(ctx context.Context, viewer uint64, q req.GuardsQuery) (guardRes.PublicMembers, error) {
	var out guardRes.PublicMembers
	if err := homePage(&q.Page, &q.PageSize); err != nil {
		return out, err
	}
	err := s.snapshot(ctx, func(tx *gorm.DB) error {
		a, err := VisibleAnchor(ctx, tx, viewer, q.AnchorNo)
		if err != nil {
			return err
		}
		g := guard.Service{DB: tx, Now: s.Now}
		out, err = g.AnchorMembersSnapshot(ctx, viewer, uint64(a.ID), q.Page, q.PageSize, global.GuardReady.Load() && global.GVA_CONFIG.Gift.GuardEnabled)
		return err
	})
	return out, err
}

func settingsVersion(a model.LiveAnchor) string {
	raw, _ := json.Marshal([]any{a.AnchorNo, a.Signature, a.CategoryId})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func settingsResponse(db *gorm.DB, a model.LiveAnchor) (res.Settings, error) {
	out := res.Settings{Signature: a.Signature, CategoryID: a.CategoryId, Version: settingsVersion(a), Categories: []res.Category{}}
	var rows []model.LiveCategory
	if err := db.Select("id", "name").Where("status=?", model.LiveCategoryStatusEnabled).Order("sort DESC, id").Limit(200).Find(&rows).Error; err != nil {
		return out, err
	}
	for _, c := range rows {
		out.Categories = append(out.Categories, res.Category{ID: uint64(c.ID), Name: c.Name})
	}
	return out, nil
}
func (s *PublicService) Settings(ctx context.Context, owner uint64) (res.Settings, error) {
	var out res.Settings
	err := s.snapshot(ctx, func(tx *gorm.DB) error {
		a, err := ownerAnchor(tx, owner)
		if err != nil {
			return err
		}
		out, err = settingsResponse(tx, a)
		return err
	})
	return out, err
}

// SaveSettings changes only the two editor-owned fields. The hash CAS ignores
// unrelated room/financial changes. Identical retries are harmless; competing
// edits to these fields return conflict instead of overwriting another client.
func (s *PublicService) SaveSettings(ctx context.Context, owner uint64, r req.SaveSettings) (res.Settings, error) {
	var out res.Settings
	if !utf8.ValidString(r.Signature) || utf8.RuneCountInString(r.Signature) > 255 || strings.TrimSpace(r.Signature) != r.Signature || strings.IndexFunc(r.Signature, func(c rune) bool { return unicode.IsControl(c) && c != '\n' }) >= 0 || len(r.Version) != 64 || r.CategoryID > 9007199254740991 {
		return out, modulekit.Invalid
	}
	if _, err := hex.DecodeString(r.Version); err != nil {
		return out, modulekit.Invalid
	}
	db := s.database(ctx)
	if db == nil {
		return out, modulekit.Unavailable
	}
	err := modulekit.Transaction(db, ctx, func(tx *gorm.DB) error {
		a, err := ownerAnchor(tx.Clauses(clause.Locking{Strength: "UPDATE"}), owner)
		if err != nil {
			return err
		}
		if settingsVersion(a) != r.Version && (a.Signature != r.Signature || a.CategoryId != r.CategoryID) {
			return modulekit.Conflict
		}
		if r.CategoryID > 0 {
			var n int64
			if err = tx.Model(&model.LiveCategory{}).Where("id=? AND status=?", r.CategoryID, model.LiveCategoryStatusEnabled).Count(&n).Error; err != nil {
				return err
			}
			if n != 1 {
				return modulekit.Invalid
			}
		}
		if a.Signature != r.Signature || a.CategoryId != r.CategoryID {
			if err = tx.Model(&model.LiveAnchor{}).Where("id=?", a.ID).Updates(map[string]any{"signature": r.Signature, "category_id": r.CategoryID}).Error; err != nil {
				return err
			}
		}
		a.Signature = r.Signature
		a.CategoryId = r.CategoryID
		out, err = settingsResponse(tx, a)
		return err
	})
	return out, err
}
