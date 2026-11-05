package earning

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	res "tb_live_module/model/earning/response"
	pm "tb_live_module/model/payout"
	ps "tb_live_module/service/payout"
)

func pagination(q *req.FinanceQuery) error {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 || q.From < 0 || q.To < 0 || (q.To > 0 && q.From >= q.To) {
		return ErrInvalid
	}
	for _, v := range []string{q.No, q.AnchorNo, q.Currency, q.Status, q.SourceType} {
		if len(v) > 64 {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Service) List(ctx context.Context, user uint64, a *Actor, kind string, q req.FinanceQuery) (res.Page, error) {
	out := res.Page{}
	if e := pagination(&q); e != nil {
		return out, e
	}
	if q.PayeeAccountNo != "" && (a == nil || kind != "withdrawal" || !ps.NoRE.MatchString(q.PayeeAccountNo)) {
		return out, ErrInvalid
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var anchor uint64
		if a != nil {
			if e := s.authorize(tx, *a, kind+"/list", "GET"); e != nil {
				return e
			}
			if q.AnchorNo != "" {
				v, e := anchorByNo(tx, q.AnchorNo)
				if e != nil {
					return e
				}
				anchor = uint64(v.ID)
			}
		} else {
			if (kind != "settlement" && kind != "withdrawal") || q.AnchorNo != "" {
				return ErrForbidden
			}
			v, e := appAnchor(tx, user)
			if e != nil {
				return e
			}
			anchor = uint64(v.ID)
		}
		var rows any
		var dest any
		noColumn := ""
		amountColumn := "amount"
		switch kind {
		case "income":
			dest = &model.Income{}
			rows = &[]model.Income{}
			noColumn = "income_no"
		case "exchange":
			dest = &model.SettlementRule{}
			rows = &[]model.SettlementRule{}
			noColumn = "rule_no"
			amountColumn = "minor_units"
		case "settlement":
			dest = &model.Settlement{}
			rows = &[]model.Settlement{}
			noColumn = "settlement_no"
		case "withdrawal":
			dest = &model.Withdrawal{}
			rows = &[]model.Withdrawal{}
			noColumn = "withdrawal_no"
		case "account":
			dest = &model.Account{}
			rows = &[]model.Account{}
			amountColumn = "available"
		case "reconciliation":
			dest = &model.Reconciliation{}
			rows = &[]model.Reconciliation{}
			noColumn = "withdrawal_no"
			amountColumn = ""
		default:
			return ErrInvalid
		}
		db := tx.Model(dest)
		if q.PayeeAccountNo != "" {
			db = db.Where("payee_ref=?", q.PayeeAccountNo)
		}
		if anchor != 0 {
			if kind == "exchange" {
				return ErrInvalid
			}
			db = db.Where("anchor_id=?", anchor)
		}
		if q.No != "" {
			if noColumn == "" {
				return ErrInvalid
			}
			db = db.Where(noColumn+"=?", q.No)
		}
		if q.Currency != "" {
			if kind == "income" {
				return ErrInvalid
			}
			db = db.Where("currency=?", q.Currency)
		}
		if q.Status != "" {
			if kind == "account" {
				return ErrInvalid
			}
			if kind == "income" {
				switch q.Status {
				case "settled":
					db = db.Where("EXISTS (SELECT 1 FROM anchor_settlement_allocation a WHERE a.income_no=anchor_income_fact.income_no)")
				case "pending":
					db = db.Where("NOT EXISTS (SELECT 1 FROM anchor_settlement_allocation a WHERE a.income_no=anchor_income_fact.income_no)")
				default:
					return ErrInvalid
				}
			} else {
				db = db.Where("status=?", q.Status)
			}
		}
		if q.SourceType != "" {
			if kind != "income" {
				return ErrInvalid
			}
			db = db.Where("source_type=?", q.SourceType)
		}
		if q.From > 0 {
			db = db.Where("created_at>=?", q.From)
		}
		if q.To > 0 {
			db = db.Where("created_at<?", q.To)
		}
		ordering := "id DESC"
		if q.Sort != "" {
			col := ""
			switch q.Sort {
			case "createdAt":
				col = "created_at"
			case "amount":
				col = amountColumn
			}
			if col == "" {
				return ErrInvalid
			}
			dir := " ASC"
			if q.Desc {
				dir = " DESC"
			}
			ordering = col + dir + ", id ASC"
		}
		if e := db.Count(&out.Total).Error; e != nil {
			return e
		}
		if e := db.Order(ordering).Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(rows).Error; e != nil {
			return e
		}
		out.List = rows
		if a != nil && kind == "withdrawal" {
			if e := attachPayoutOrders(tx, *rows.(*[]model.Withdrawal)); e != nil {
				return e
			}
		}
		if a == nil {
			if kind == "settlement" {
				safe := []res.AppSettlement{}
				for _, v := range *rows.(*[]model.Settlement) {
					safe = append(safe, res.PublicSettlement(v))
				}
				out.List = safe
			} else {
				safe := []res.AppWithdrawal{}
				for _, v := range *rows.(*[]model.Withdrawal) {
					safe = append(safe, res.PublicWithdrawal(v))
				}
				out.List = safe
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) Overview(ctx context.Context, user uint64, a *Actor, q req.FinanceQuery) (res.FinanceOverview, error) {
	out := res.FinanceOverview{PendingCoin: "0", SettledCoin: "0", Balances: []res.Balance{}, WithdrawalEnabled: s.Payees != nil}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var id uint64
		if a != nil {
			if e := s.authorize(tx, *a, "overview", "GET"); e != nil {
				return e
			}
			if q.AnchorNo != "" {
				v, e := anchorByNo(tx, q.AnchorNo)
				if e != nil {
					return e
				}
				id = uint64(v.ID)
				out.AnchorNo = v.AnchorNo
			}
		} else {
			if q.AnchorNo != "" {
				return ErrForbidden
			}
			v, e := appAnchor(tx, user)
			if e != nil {
				return e
			}
			id = uint64(v.ID)
			out.AnchorNo = v.AnchorNo
		}
		income := tx.Model(&model.Income{})
		accounts := tx.Model(&model.Account{})
		if id != 0 {
			income = income.Where("anchor_id=?", id)
			accounts = accounts.Where("anchor_id=?", id)
		}
		if e := income.Session(&gorm.Session{}).Where("NOT EXISTS (SELECT 1 FROM anchor_settlement_allocation a WHERE a.income_no=anchor_income_fact.income_no)").Select("COALESCE(SUM(amount),0)").Scan(&out.PendingCoin).Error; e != nil {
			return e
		}
		if e := income.Session(&gorm.Session{}).Where("EXISTS (SELECT 1 FROM anchor_settlement_allocation a WHERE a.income_no=anchor_income_fact.income_no)").Select("COALESCE(SUM(amount),0)").Scan(&out.SettledCoin).Error; e != nil {
			return e
		}
		if id == 0 {
			return accounts.Select("currency, SUM(available) AS available, SUM(frozen) AS frozen, SUM(debt) AS debt, SUM(paid) AS paid").Group("currency").Scan(&out.Balances).Error
		}
		var rows []model.Account
		if e := accounts.Order("currency ASC").Find(&rows).Error; e != nil {
			return e
		}
		for _, v := range rows {
			out.Balances = append(out.Balances, res.PublicBalance(v))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) SettlementDetail(ctx context.Context, a Actor, no string) (res.SettlementDetail, error) {
	var out res.SettlementDetail
	if !noRE.MatchString(no) {
		return out, ErrInvalid
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "settlement/detail", "GET"); e != nil {
			return e
		}
		if e := tx.Where("settlement_no=?", no).Take(&out.Settlement).Error; e != nil {
			return ErrNotFound
		}
		if e := tx.Where("settlement_no=?", no).Order("income_no ASC").Limit(maxSettlementFacts + 1).Find(&out.Items).Error; e != nil {
			return e
		}
		if e := tx.Where("object_no=?", no).Order("id ASC").Find(&out.Audit).Error; e != nil {
			return e
		}
		e := s.verifySettlement(tx, out.Settlement, true)
		out.Consistent = e == nil
		if e != nil && !errors.Is(e, ErrInvariant) && !errors.Is(e, ErrSource) {
			return e
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) WithdrawalDetail(ctx context.Context, user uint64, a *Actor, no string) (any, error) {
	return s.withdrawalDetail(ctx, user, a, no, "")
}
func (s *Service) AppWithdrawalResult(ctx context.Context, user uint64, q req.AppWithdrawalResult) (any, error) {
	return s.withdrawalDetail(ctx, user, nil, q.No, q.RequestID)
}
func (s *Service) withdrawalDetail(ctx context.Context, user uint64, a *Actor, no, requestID string) (any, error) {
	if (no == "") == (requestID == "") || (no != "" && !noRE.MatchString(no)) || (requestID != "" && (!keyRE.MatchString(requestID) || a != nil)) {
		return nil, ErrInvalid
	}

	var out res.WithdrawalDetail
	var safe any
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		db := tx.Where("withdrawal_no=?", no)
		if requestID != "" {
			db = tx.Where("request_id=?", requestID)
		}
		if a != nil {
			if e := s.authorize(tx, *a, "withdrawal/detail", "GET"); e != nil {
				return e
			}
		} else {
			anchor, e := appAnchor(tx, user)
			if e != nil {
				return e
			}
			db = db.Where("anchor_id=?", anchor.ID)
		}
		if e := db.Take(&out.Withdrawal).Error; e != nil {
			return ErrNotFound
		}
		if a == nil {
			safe = res.PublicWithdrawal(out.Withdrawal)
			return nil
		}
		linked := []model.Withdrawal{out.Withdrawal}
		if e := attachPayoutOrders(tx, linked); e != nil {
			return e
		}
		out.Withdrawal = linked[0]
		out.CurrentPayee.State = "unavailable"
		if inspector, ok := s.Payees.(payeeInspector); ok {
			current, e := inspector.Inspect(tx, out.Withdrawal)
			if e != nil {
				return e
			}
			out.CurrentPayee = current
		}
		if e := tx.Where("withdrawal_no=?", no).Order("id DESC").Limit(100).Find(&out.Reconciliations).Error; e != nil {
			return e
		}
		if e := tx.Where("business_no=?", no).Order("id ASC").Find(&out.Ledger).Error; e != nil {
			return e
		}
		return tx.Where("object_no=?", no).Order("id ASC").Find(&out.Audit).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if a == nil {
		return safe, e
	}
	return out, e
}

func attachPayoutOrders(tx *gorm.DB, rows []model.Withdrawal) error {
	if len(rows) == 0 {
		return nil
	}
	nos := make([]string, 0, len(rows))
	for _, w := range rows {
		nos = append(nos, w.WithdrawalNo)
	}
	var orders []pm.Order
	if e := tx.Select("payout_no,business_no,status").Where("business_type='anchor_withdrawal' AND business_no IN ?", nos).Find(&orders).Error; e != nil {
		return e
	}
	byNo := map[string]pm.Order{}
	for _, o := range orders {
		byNo[o.BusinessNo] = o
	}
	for i := range rows {
		if o, ok := byNo[rows[i].WithdrawalNo]; ok {
			rows[i].PayoutNo = o.PayoutNo
			rows[i].PayoutStatus = o.Status
		}
	}
	return nil
}
func (s *Service) AccountLedger(ctx context.Context, a Actor, q req.FinanceQuery) (res.Page, error) {
	out := res.Page{}
	if e := pagination(&q); e != nil {
		return out, e
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "account/ledger", "GET"); e != nil {
			return e
		}
		anchor, e := anchorByNo(tx, q.AnchorNo)
		if e != nil {
			return e
		}
		if !currencyRE.MatchString(q.Currency) {
			return ErrInvalid
		}
		db := tx.Model(&model.AccountLedger{}).Where("anchor_id=? AND currency=?", anchor.ID, q.Currency)
		if e = db.Count(&out.Total).Error; e != nil {
			return e
		}
		var rows []model.AccountLedger
		e = db.Order("version DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&rows).Error
		out.List = rows
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) CheckAccount(ctx context.Context, a Actor, q req.FinanceQuery) (map[string]any, error) {
	out := map[string]any{"consistent": false}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "account/check", "GET"); e != nil {
			return e
		}
		anchor, e := anchorByNo(tx, q.AnchorNo)
		if e != nil {
			return e
		}
		account, e := loadAccount(tx, uint64(anchor.ID), anchor.AnchorNo, q.Currency, s.now(), false)
		if e != nil {
			return ErrNotFound
		}
		e = s.verifyAccount(tx, account, true)
		out["consistent"] = e == nil
		out["account"] = account
		if errors.Is(e, ErrInvariant) || errors.Is(e, ErrSource) || errors.Is(e, ErrLimit) {
			out["issue"] = e.Error()
			return nil
		}
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}

// MarkGiftSettlementStatus decorates response copies only; immutable source ledgers remain unchanged.
func MarkGiftSettlementStatus(tx *gorm.DB, rows []model.Ledger) error {
	if len(rows) == 0 {
		return nil
	}
	nos := make([]string, 0, len(rows))
	for _, r := range rows {
		nos = append(nos, r.EarningNo)
	}
	var settled []string
	if e := tx.Table("anchor_income_fact i").Joins("JOIN anchor_settlement_allocation a ON a.income_no=i.income_no").Where("i.source_type='gift' AND i.source_entry_no IN ?", nos).Pluck("i.source_entry_no", &settled).Error; e != nil {
		return e
	}
	have := map[string]bool{}
	for _, n := range settled {
		have[n] = true
	}
	for i := range rows {
		if have[rows[i].EarningNo] {
			rows[i].Status = "settled"
		}
	}
	return nil
}
