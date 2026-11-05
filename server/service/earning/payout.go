package earning

import (
	"gorm.io/gorm"
	"strconv"
	model "tb_live_module/model/earning"
	res "tb_live_module/model/payout/response"
	svc "tb_live_module/service/payout"
	"time"
)

const SafePayeeName = "收款人资料已加密"

type DatabasePayeeProvider struct{ Accounts *svc.Service }

func (p DatabasePayeeProvider) ResolveInTx(tx *gorm.DB, anchor, no, currency string) (Payee, error) {
	if p.Accounts == nil {
		return Payee{}, ErrUnavailable
	}
	a, e := p.Accounts.ResolveInTx(tx, anchor, no, currency)
	if e != nil {
		return Payee{}, e
	}
	return Payee{AnchorNo: anchor, Reference: a.PayeeAccountNo, Version: strconv.FormatInt(a.Version, 10), Currency: a.Currency, Method: a.Method, Name: SafePayeeName, Masked: a.MaskedAccount, RiskChecked: true, RiskFlags: []string{}, ChangedAt: a.CreatedAt, CheckedAt: time.Now().UnixMilli()}, nil
}

type payeeInspector interface {
	Inspect(*gorm.DB, model.Withdrawal) (res.Current, error)
}

func (p DatabasePayeeProvider) Inspect(tx *gorm.DB, w model.Withdrawal) (res.Current, error) {
	if p.Accounts == nil {
		return res.Current{State: "unavailable"}, nil
	}
	out, e := p.Accounts.Inspect(tx, w.AnchorNo, w.PayeeRef, w.Currency, w.PayeeVersion, w.PayeeMethod, w.AccountMasked)
	if w.AccountName != SafePayeeName {
		out.MatchesSnapshot = false
		out.Usable = false
	}
	return out, e
}
