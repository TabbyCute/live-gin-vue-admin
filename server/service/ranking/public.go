package ranking

import (
	"context"
	"gorm.io/gorm"
	"regexp"
	"strconv"
	"tb_live_module/internal/modulekit"
	liveModel "tb_live_module/model/live"
	model "tb_live_module/model/ranking"
	req "tb_live_module/model/ranking/request"
	res "tb_live_module/model/ranking/response"
	"tb_live_module/service/live"
	"tb_live_module/service/social"
)

// AccountEncoding is discovered from the indexed identity column once at startup.
// Legacy account tables may use a different collation than binary ranking keys.
type AccountEncoding struct {
	Charset   string
	Collation string
}

var sqlEncodingName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func DiscoverAccountEncoding(db *gorm.DB) (*AccountEncoding, error) {
	if db.Dialector.Name() != "mysql" {
		return nil, nil
	}
	var row AccountEncoding
	if err := db.Raw("SELECT CHARACTER_SET_NAME AS charset,COLLATION_NAME AS collation FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='live_account' AND COLUMN_NAME='account_no'").Scan(&row).Error; err != nil {
		return nil, err
	}
	if !sqlEncodingName.MatchString(row.Charset) || !sqlEncodingName.MatchString(row.Collation) {
		return nil, ErrInvariant
	}
	return &row, nil
}

// AnchorSupportersSnapshot is a read-model boundary for the public home.
// The caller must validate the anchor and supply the SAME read-only transaction
// in Service.DB. It cannot query other scopes, history periods or private money.
// Receipt lag reconciliation belongs to the worker/admin path, never a public
// request scanning the complete revenue ledger.
func (s *Service) AnchorSupportersSnapshot(ctx context.Context, viewer uint64, anchorNo, period string, page, size int) (res.Board, error) {
	out := res.Board{List: []res.Entry{}}
	q := req.Query{ScopeType: "anchor", ScopeNo: anchorNo, Period: period, Page: page, PageSize: size}
	if period != "day" && period != "month" && period != "all" || page < 1 || page > 100 || size < 1 || size > 50 {
		return out, modulekit.Invalid
	}
	if err := s.validateQuery("gift", &q); err != nil {
		return out, err
	}
	db := s.db(ctx)
	identityMatch := "pu.account_no=ranking_score_bucket.subject_no"
	if db.Dialector.Name() == "mysql" {
		encoding := s.AccountEncoding
		if encoding == nil {
			var err error
			encoding, err = DiscoverAccountEncoding(db)
			if err != nil {
				return out, err
			}
		}
		if !sqlEncodingName.MatchString(encoding.Charset) || !sqlEncodingName.MatchString(encoding.Collation) {
			return out, ErrInvariant
		}
		// Convert the outer key, never the indexed account column. The second
		// comparison preserves exact identity even on a case-insensitive legacy index.
		identityMatch = "pu.account_no=CONVERT(ranking_score_bucket.subject_no USING " + encoding.Charset + ") COLLATE " + encoding.Collation + " AND BINARY pu.account_no=BINARY ranking_score_bucket.subject_no"
	}
	var cp model.Checkpoint
	if err := db.Where("consumer_name=?", "gift-revenue").Take(&cp).Error; err != nil {
		return out, err
	}
	if cp.Timezone != s.zone() || cp.LastError != "" {
		return out, ErrInvariant
	}
	query := func() *gorm.DB {
		return boardQuery(db, cp.Generation, "gift", q).
			Where("subject_type=?", "user").
			Where("EXISTS (?)", db.Table("live_account AS pu").Select("1").
				Where(identityMatch+" AND pu.deleted_at IS NULL AND pu.status=?", liveModel.LiveAccountStatusNormal).
				Scopes(social.VisibleOwnerScope(viewer, "pu.id", "1", 1, 2, 3)))
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return out, err
	}
	var rows []model.ScoreBucket
	if err := query().Order("score_sort DESC, subject_no ASC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return out, err
	}
	numbers := make([]string, 0, len(rows))
	for _, r := range rows {
		numbers = append(numbers, r.SubjectNo)
	}
	var users []liveModel.LiveAccount
	if len(numbers) > 0 {
		if err := db.Select("id", "account_no").Where("account_no IN ?", numbers).Find(&users).Error; err != nil {
			return out, err
		}
	}
	ids := make([]uint64, 0, len(users))
	byNo := make(map[string]uint64, len(users))
	for _, u := range users {
		ids = append(ids, uint64(u.ID))
		byNo[u.AccountNo] = uint64(u.ID)
	}
	profiles, err := (live.SocialDirectory{}).Profiles(ctx, db, viewer, ids)
	if err != nil {
		return out, err
	}
	for i, r := range rows {
		p, ok := profiles[byNo[r.SubjectNo]]
		if !ok {
			return out, ErrInvariant
		}
		out.List = append(out.List, res.Entry{Rank: strconv.Itoa((page-1)*size + i + 1), Score: r.ScoreCoin, Quantity: r.Quantity, SubjectNo: r.SubjectNo, SubjectType: r.SubjectType, User: p})
	}
	out.Total = strconv.FormatInt(total, 10)
	out.Page = page
	out.PageSize = size
	out.RankType = "gift"
	out.Period = period
	out.PeriodKey = q.PeriodKey
	out.Timezone = s.zone()
	out.ScopeType = "anchor"
	out.ScopeNo = anchorNo
	out.GeneratedAt = s.now()
	out.DataAsOf = cp.DataAsOf
	// Empty lagCount means not calculated; do not claim that projection is current.
	return out, nil
}
