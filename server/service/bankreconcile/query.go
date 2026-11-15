package bankreconcile

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	model "tb_live_module/model/bankreconcile"
	req "tb_live_module/model/bankreconcile/request"
	payout "tb_live_module/service/payout"
)

type Page struct {
	List     []model.Batch `json:"list"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

func (s *Service) List(ctx context.Context, a payout.Actor, q req.Search) (Page, error) {
	out := Page{Page: q.Page, PageSize: q.PageSize, List: []model.Batch{}}
	if out.Page == 0 {
		out.Page = 1
	}
	if out.PageSize == 0 {
		out.PageSize = 20
	}
	if out.Page < 1 || out.Page > 10000 || out.PageSize < 1 || out.PageSize > 100 || q.Keyword != "" || (q.ChannelAccountNo != "" && !companyRE.MatchString(q.ChannelAccountNo)) {
		return out, ErrInvalid
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.payout().BankReconciliationAuthorize(tx, a, "list", "GET"); e != nil {
			return e
		}
		query := tx.Model(&model.Batch{})
		if q.ChannelAccountNo != "" {
			query = query.Where("channel_account_no=?", q.ChannelAccountNo)
		}
		if e := query.Count(&out.Total).Error; e != nil {
			return e
		}
		return query.Order("created_at DESC, batch_no DESC").Offset((out.Page - 1) * out.PageSize).Limit(out.PageSize).Find(&out.List).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}

type Detail struct {
	Batch   model.Batch    `json:"batch"`
	Lines   []model.Line   `json:"lines"`
	Run     *model.Run     `json:"run"`
	Checks  []model.Check  `json:"checks"`
	Reviews []model.Review `json:"reviews"`
}

// Detail is bounded to one batch's latest immutable run, not unbounded history.
func (s *Service) Detail(ctx context.Context, a payout.Actor, batchNo string) (Detail, error) {
	out := Detail{Lines: []model.Line{}, Checks: []model.Check{}, Reviews: []model.Review{}}
	if !noRE.MatchString(batchNo) {
		return out, ErrInvalid
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.payout().BankReconciliationAuthorize(tx, a, "detail", "GET"); e != nil {
			return e
		}
		if e := tx.Where("batch_no=?", batchNo).Take(&out.Batch).Error; e != nil {
			return ErrState
		}
		if e := tx.Table("payout_bank_line l").Select("l.*").Joins("JOIN payout_bank_batch_line b ON b.line_no=l.line_no").Where("b.batch_no=?", batchNo).Order("l.booked_at,l.line_no").Limit(MaxRows).Find(&out.Lines).Error; e != nil {
			return e
		}
		var runs []model.Run
		if e := tx.Where("batch_no=?", batchNo).Order("created_at DESC,run_no DESC").Limit(1).Find(&runs).Error; e != nil {
			return e
		}
		if len(runs) == 0 {
			return nil
		}
		out.Run = &runs[0]
		if e := tx.Where("run_no=?", out.Run.RunNo).Order("check_no").Limit(2 * MaxRows).Find(&out.Checks).Error; e != nil {
			return e
		}
		return tx.Table("payout_bank_review r").Select("r.*").Joins("JOIN payout_bank_check c ON c.check_no=r.check_no").Where("c.run_no=?", out.Run.RunNo).Limit(2 * MaxRows).Find(&out.Reviews).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
