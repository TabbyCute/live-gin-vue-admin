package payout

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
)

func revealPath(company bool, grant bool) string {
	p := "order/reveal-payee"
	if company {
		p = "channel-account/reveal"
	}
	if grant {
		p += "-grant"
	}
	return p
}
func (s *Service) revealToken(g model.RevealGrant) string {
	return s.Crypto.digest("payout.reveal.token.v1", []any{g.GrantNo, g.ActorID, g.ExpiresAt, g.TargetType, g.TargetNo})
}
func (s *Service) authorizeRevealTarget(tx *gorm.DB, a Actor, no string, company bool) (model.Order, model.ChannelAccount, error) {
	var o model.Order
	var c model.ChannelAccount
	if company {
		if !companyNoRE.MatchString(no) {
			return o, c, ErrOrderInvalid
		}
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_account_no=?", no).Take(&c).Error
		return o, c, e
	}
	var e error
	o, e = s.lockOrder(tx, no)
	if e != nil {
		return o, c, e
	}
	if o.Status != "processing" {
		return o, c, ErrOrderState
	}
	var at model.Attempt
	if e = tx.Where("payout_no=? AND status='processing' AND active_slot=1", o.PayoutNo).Take(&at).Error; e != nil {
		return o, c, e
	}
	if at.OperatorID != a.ID {
		return o, c, ErrOrderForbidden
	}
	return o, c, nil
}
func (s *Service) GrantReveal(ctx context.Context, a Actor, trace string, r req.RevealGrant, company bool) (res.RevealTicket, error) {
	var empty res.RevealTicket
	reason, e := safeNote(r.Reason)
	if e != nil {
		return empty, e
	}
	out, e := s.manualCommand(ctx, a, revealPath(company, true), r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		if e := s.authorizePayout(tx, a, revealPath(company, false), "POST"); e != nil {
			return res.ActionResult{}, e
		}
		o, _, e := s.authorizeRevealTarget(tx, a, r.TargetNo, company)
		if e != nil {
			return res.ActionResult{}, e
		}
		now := s.now()
		kind := "payee"
		if company {
			kind = "company"
		}
		g := model.RevealGrant{GrantNo: number("prg_"), TargetType: kind, TargetNo: r.TargetNo, ActorID: a.ID, Reason: reason, CreatedAt: now, ExpiresAt: now + 45000}
		g.TokenHash = snapshotHash(s.revealToken(g))
		if e = tx.Create(&g).Error; e != nil {
			return res.ActionResult{}, e
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, g.GrantNo, "payee.reveal_grant", map[string]any{"targetType": kind, "targetNo": r.TargetNo, "reason": reason, "expiresAt": g.ExpiresAt}, now); e != nil {
			return res.ActionResult{}, e
		}
		return res.ActionResult{GrantNo: g.GrantNo, ExpiresAt: g.ExpiresAt}, nil
	})
	if e != nil {
		return empty, e
	}
	var g model.RevealGrant
	if e = s.db(ctx).Where("grant_no=? AND actor_id=?", out.GrantNo, a.ID).Take(&g).Error; e != nil {
		return empty, e
	}
	if g.ConsumedAt != 0 || s.now() > g.ExpiresAt {
		return empty, ErrReveal
	}
	return res.RevealTicket{GrantNo: g.GrantNo, Token: s.revealToken(g), ExpiresAt: g.ExpiresAt}, nil
}
func (s *Service) ConsumeReveal(ctx context.Context, a Actor, trace string, r req.RevealConsume, company bool) (res.RevealedAccount, error) {
	var out res.RevealedAccount
	if !requestRE.MatchString(r.RequestID) || len(r.GrantNo) != 36 || len(r.Token) != 64 || s.Crypto == nil {
		return out, ErrReveal
	}
	path := revealPath(company, false)
	digest := s.Crypto.digest("payout.manual.request.v1", []any{a.ID, path, r})
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		out = res.RevealedAccount{}
		if e := s.authorizePayout(tx, a, path, "POST"); e != nil {
			return e
		}
		var old model.ManualRequest
		e := tx.Where("request_id=?", r.RequestID).Take(&old).Error
		if e == nil {
			if old.ActorID != a.ID || old.Action != path || old.RequestHash != digest {
				return ErrOrderConflict
			}
			return ErrReveal
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var g model.RevealGrant
		if e = tx.Where("grant_no=? AND actor_id=?", r.GrantNo, a.ID).Take(&g).Error; e != nil {
			return ErrReveal
		}
		kind := "payee"
		if company {
			kind = "company"
		}
		if g.TargetType != kind {
			return ErrReveal
		}
		o, c, e := s.authorizeRevealTarget(tx, a, g.TargetNo, company)
		if e != nil {
			return e
		}
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("grant_no=?", r.GrantNo).Take(&g).Error; e != nil {
			return e
		}
		now := s.now()
		tokenHash := snapshotHash(r.Token)
		if g.ActorID != a.ID || g.ConsumedAt != 0 || now > g.ExpiresAt || now < g.CreatedAt || subtle.ConstantTimeCompare([]byte(g.TokenHash), []byte(tokenHash)) != 1 {
			return ErrReveal
		}
		action := "payee.reveal"
		if company {
			account, e := s.openCompany(c)
			if e != nil {
				return e
			}
			out = res.RevealedAccount{TargetNo: c.ChannelAccountNo, AccountName: c.AccountName, Account: account, Method: "bank_card", Currency: c.Currency, InstitutionName: c.InstitutionName, ClearAfterSeconds: 30}
			action = "channel_account.reveal"
		} else {
			v, e := s.openOrder(o)
			if e != nil {
				return e
			}
			out = res.RevealedAccount{TargetNo: o.PayoutNo, AccountName: v.AccountName, Account: v.Account, Method: o.PayeeMethod, Currency: o.Currency, BankCode: v.BankCode, AppID: v.AppID, IdentifierType: v.IdentifierType, ClearAfterSeconds: 30}
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, g.GrantNo, action, map[string]any{"targetType": g.TargetType, "targetNo": g.TargetNo, "reason": g.Reason, "singleUse": true}, now); e != nil {
			return e
		}
		result := tx.Model(&model.RevealGrant{}).Where("grant_no=? AND consumed_at=0", g.GrantNo).Updates(map[string]any{"consumed_at": now, "consumed_request_id": r.RequestID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrReveal
		}
		b, _ := json.Marshal(res.ActionResult{GrantNo: g.GrantNo})
		return tx.Create(&model.ManualRequest{RequestID: r.RequestID, RequestHash: digest, ActorID: a.ID, Action: path, Response: string(b), CreatedAt: now}).Error
	})
	if e != nil {
		return res.RevealedAccount{}, e
	}
	return out, nil
}
