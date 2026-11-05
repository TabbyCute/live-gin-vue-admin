package payout

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"regexp"
	"strings"
	em "tb_live_module/model/earning"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
)

var companyAccountRE = regexp.MustCompile(`^[A-Z0-9]{6,34}$`)

func maskCompany(v string) string { return "****" + v[len(v)-4:] }
func (s *Service) CreateChannelAccount(ctx context.Context, a Actor, trace string, r req.CreateChannelAccount) (res.ActionResult, error) {
	var empty res.ActionResult
	if r.ChannelCode != "manual_bank" || !clean(r.AccountName, 1, 128) || !clean(r.InstitutionName, 1, 128) || !companyAccountRE.MatchString(r.Account) || !currencyRE.MatchString(r.Currency) || r.Currency == "COIN" {
		return empty, ErrOrderInvalid
	}
	single, e := money(r.SingleLimit)
	if e != nil {
		return empty, e
	}
	daily, e := money(r.DailyLimit)
	if e != nil || daily < single {
		return empty, ErrOrderInvalid
	}
	return s.manualCommand(ctx, a, "channel-account/create", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&em.FinanceControl{}, 1).Error; e != nil {
			return empty, e
		}
		var channel model.Channel
		if e := tx.Where("channel_code='manual_bank' AND channel_type='manual'").Take(&channel).Error; e != nil {
			return empty, e
		}
		var count int64
		if e := tx.Model(&model.ChannelAccount{}).Count(&count).Error; e != nil {
			return empty, e
		}
		if count >= 1000 {
			return empty, ErrChannelLimit
		}
		now := s.now()
		account := model.ChannelAccount{ChannelAccountNo: number("pca_"), ChannelCode: r.ChannelCode, AccountName: r.AccountName, InstitutionName: r.InstitutionName, Currency: r.Currency, MaskedAccount: maskCompany(r.Account), Status: "active", SingleLimit: single, DailyLimit: daily, CreatedBy: a.ID, CreatedAt: now, UpdatedAt: now, Version: 1}
		account.AccountFingerprint = s.Crypto.digest("payout.company.fingerprint.v1", []string{account.ChannelCode, account.Currency, r.Account})
		if e := s.sealCompany(&account, r.Account); e != nil {
			return empty, e
		}
		no, e := auditManual(tx, a, trace, "", account.ChannelAccountNo, "channel_account.create", res.PublicChannelAccount(account), now)
		if e != nil {
			return empty, e
		}
		account.LastAuditNo = no
		if e = tx.Create(&account).Error; e != nil {
			return empty, e
		}
		return res.ActionResult{ChannelAccountNo: account.ChannelAccountNo, Status: account.Status}, nil
	})
}
func (s *Service) ChangeChannelAccount(ctx context.Context, a Actor, trace string, r req.ChannelAccountAction, enable bool) (res.ActionResult, error) {
	var empty res.ActionResult
	reason, e := safeNote(r.Reason)
	if e != nil || !companyNoRE.MatchString(r.ChannelAccountNo) {
		return empty, ErrOrderInvalid
	}
	op, status := "disable", "disabled"
	if enable {
		op, status = "enable", "active"
	}
	return s.manualCommand(ctx, a, "channel-account/"+op, r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		var row model.ChannelAccount
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_account_no=?", r.ChannelAccountNo).Take(&row).Error; e != nil {
			return empty, e
		}
		if _, e := s.openCompany(row); e != nil {
			return empty, e
		}
		if row.Status == status {
			return empty, ErrOrderState
		}
		now := s.now()
		disabled := now
		if enable {
			disabled = 0
		}
		no, e := auditManual(tx, a, trace, "", row.ChannelAccountNo, "channel_account."+op, map[string]any{"channelAccountNo": row.ChannelAccountNo, "status": status, "reason": reason, "version": row.Version + 1}, now)
		if e != nil {
			return empty, e
		}
		result := tx.Model(&model.ChannelAccount{}).Where("id=? AND version=?", row.ID, row.Version).Updates(map[string]any{"status": status, "disabled_at": disabled, "updated_at": now, "version": row.Version + 1, "last_audit_no": no})
		if result.Error != nil {
			return empty, result.Error
		}
		if result.RowsAffected != 1 {
			return empty, ErrOrderConflict
		}
		return res.ActionResult{ChannelAccountNo: row.ChannelAccountNo, Status: status}, nil
	})
}
func (s *Service) ListChannelAccounts(ctx context.Context, a Actor, q req.ChannelAccountSearch) (res.ChannelAccountPage, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	out := res.ChannelAccountPage{Page: q.Page, PageSize: q.PageSize, List: []res.ChannelAccount{}}
	if q.Page < 1 || q.Page > 100 || q.PageSize < 1 || q.PageSize > 100 || (q.Currency != "" && !currencyRE.MatchString(q.Currency)) || (q.Status != "" && q.Status != "active" && q.Status != "disabled") {
		return out, ErrOrderInvalid
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorizePayout(tx, a, "channel-account/list", "GET"); e != nil {
			return e
		}
		db := tx.Model(&model.ChannelAccount{})
		if q.Currency != "" {
			db = db.Where("currency=?", q.Currency)
		}
		if q.Status != "" {
			db = db.Where("status=?", q.Status)
		}
		if e := db.Count(&out.Total).Error; e != nil {
			return e
		}
		var rows []model.ChannelAccount
		cols := strings.Join([]string{"channel_account_no", "channel_code", "account_name", "institution_name", "currency", "masked_account", "status", "single_limit", "daily_limit", "created_by", "created_at", "updated_at", "disabled_at"}, ",")
		if e := db.Select(cols).Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error; e != nil {
			return e
		}
		for _, row := range rows {
			out.List = append(out.List, res.PublicChannelAccount(row))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
