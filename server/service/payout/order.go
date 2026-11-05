package payout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"regexp"
	"strconv"
	em "tb_live_module/model/earning"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	"tb_live_module/model/system"
)

var (
	ErrOrderInvalid     = &Error{49201, "出款订单参数无效"}
	ErrOrderForbidden   = &Error{49202, "无打款中心操作权限"}
	ErrOrderUnavailable = &Error{49203, "打款中心暂不可用，请查询原提现单或使用原参数重试"}
	ErrOrderConflict    = &Error{49204, "提交请求号与原提现单不一致"}
	ErrOrderState       = &Error{49205, "提现、收款账户或账务核验未通过，不能提交打款中心"}
	ErrOrderNotFound    = &Error{49206, "出款订单不存在"}
)
var orderNoRE = regexp.MustCompile(`^po_[a-f0-9]{32}$`)
var withdrawalNoRE = regexp.MustCompile(`^wd_[a-f0-9]{32}$`)
var anchorNoRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

type Actor struct {
	ID          uint64
	AuthorityID uint
}

func (s *Service) authorizeOrder(tx *gorm.DB, a Actor, path, method string) error {
	return s.authorizePayout(tx, a, "order/"+path, method)
}
func (s *Service) authorizePayout(tx *gorm.DB, a Actor, path, method string) error {
	if e := s.authorizePayoutActor(tx, a); e != nil {
		return e
	}
	if system.IsSuperAdmin(a.ID) {
		return nil
	}
	var n int64
	if e := tx.Model(&em.AdminPermission{}).Where("authority_id=? AND path=? AND method=?", a.AuthorityID, "/payout/"+path, method).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return ErrOrderForbidden
	}
	return nil
}
func (s *Service) authorizePayoutActor(tx *gorm.DB, a Actor) error {
	if a.ID == 0 {
		return ErrOrderForbidden
	}
	var u system.SysUser
	if e := tx.Where("id=? AND enable=1", a.ID).Take(&u).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrOrderForbidden
		}
		return e
	}
	if system.IsSuperAdmin(a.ID) {
		return nil
	}
	var n int64
	if e := tx.Table("sys_user_authority m").Joins("JOIN sys_authorities r ON r.authority_id=m.sys_authority_authority_id AND r.deleted_at IS NULL").Where("m.sys_user_id=? AND m.sys_authority_authority_id=?", a.ID, a.AuthorityID).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return ErrOrderForbidden
	}
	return nil
}
func orderDigest(no string) string { return snapshotHash("payout.order.request.v1\x00" + no) }
func (s *Service) CreateOrder(ctx context.Context, a Actor, trace string, r req.CreateOrder) (res.Order, error) {
	var out res.Order
	if !requestRE.MatchString(r.RequestID) || !withdrawalNoRE.MatchString(r.WithdrawalNo) {
		return out, ErrOrderInvalid
	}
	if e := s.authorizeOrder(s.db(ctx), a, "create", "POST"); e != nil {
		return out, e
	}
	digest := orderDigest(r.WithdrawalNo)
	// Unique-key losers restart the entire transaction and resolve the winner.
	for attempt := 0; attempt < 3; attempt++ {
		e := s.transaction(ctx, func(tx *gorm.DB) error {
			if e := s.authorizeOrder(tx, a, "create", "POST"); e != nil {
				return e
			}
			var claim model.OrderRequest
			e := tx.Where("request_id=?", r.RequestID).Take(&claim).Error
			if e == nil {
				if claim.RequestHash != digest {
					return ErrOrderConflict
				}
				var old model.Order
				if e = tx.Where("payout_no=?", claim.PayoutNo).Take(&old).Error; e != nil {
					return e
				}
				if old.BusinessNo != r.WithdrawalNo {
					return ErrOrderConflict
				}
				if e = s.VerifyOrder(old); e != nil {
					return e
				}
				out = res.PublicOrder(old)
				return nil
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			var o model.Order
			e = tx.Where("business_type='anchor_withdrawal' AND business_no=?", r.WithdrawalNo).Take(&o).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				if s.FinancialGuard == nil || s.Crypto == nil {
					return ErrOrderUnavailable
				}
				w, proof, err := s.FinancialGuard(tx, r.WithdrawalNo)
				if err != nil {
					return err
				}
				// Finance lock is now held; repeat the existence check after any wait.
				e = tx.Where("business_type='anchor_withdrawal' AND business_no=?", r.WithdrawalNo).Take(&o).Error
				if errors.Is(e, gorm.ErrRecordNotFound) {
					if !NoRE.MatchString(w.PayeeRef) || w.PayeeVersion != "1" || w.Status != "approved" || w.ReconciliationStatus != "passed" {
						return ErrOrderState
					}
					safe, err := s.ResolveInTx(tx, w.AnchorNo, w.PayeeRef, w.Currency)
					if err != nil {
						if errors.Is(err, ErrState) {
							return ErrOrderState
						}
						return err
					}
					if strconv.FormatInt(safe.Version, 10) != w.PayeeVersion || safe.Method != w.PayeeMethod || safe.MaskedAccount != w.AccountMasked {
						return ErrOrderState
					}
					var binding model.PayeeAccount
					if e = tx.Where("payee_account_no=? AND owner_no=? AND owner_type='anchor'", w.PayeeRef, w.AnchorNo).Take(&binding).Error; e != nil {
						return e
					}
					now := s.now()
					o = model.Order{PayoutNo: number("po_"), RequestID: r.RequestID, RequestHash: digest, BusinessType: "anchor_withdrawal", BusinessNo: w.WithdrawalNo, AnchorID: w.AnchorID, AnchorNo: w.AnchorNo, ApplicantUserID: w.ApplicantUserID, Amount: w.Amount, Currency: w.Currency, PayeeAccountNo: w.PayeeRef, PayeeVersion: w.PayeeVersion, PayeeMethod: w.PayeeMethod, AccountName: w.AccountName, AccountMasked: w.AccountMasked, ReconciliationSnapshot: proof, Status: "ready", CreatedBy: a.ID, CreatedAt: now, UpdatedAt: now}
					if e = s.sealOrder(&o, binding); e != nil {
						return e
					}
					if e = tx.Create(&o).Error; e != nil {
						return e
					}
					if e = tx.Create(&model.StatusHistory{EventNo: number("psh_"), PayoutNo: o.PayoutNo, ToStatus: "ready", ActorID: a.ID, CreatedAt: now}).Error; e != nil {
						return e
					}
					b, err := json.Marshal(res.PublicOrder(o))
					if err != nil {
						return err
					}
					if e = tx.Create(&model.OrderAudit{EventNo: number("poa_"), PayoutNo: o.PayoutNo, Action: "order.create", ActorID: a.ID, TraceID: trace, Snapshot: string(b), CreatedAt: now}).Error; e != nil {
						return e
					}
				} else if e != nil {
					return e
				}
			} else if e != nil {
				return e
			}
			if e = s.VerifyOrder(o); e != nil {
				return e
			}
			if e = tx.Create(&model.OrderRequest{RequestID: r.RequestID, RequestHash: digest, PayoutNo: o.PayoutNo, CreatedBy: a.ID, CreatedAt: s.now()}).Error; e != nil {
				return e
			}
			out = res.PublicOrder(o)
			return nil
		})
		var me *driver.MySQLError
		if e == nil || !errors.As(e, &me) || me.Number != 1062 {
			return out, e
		}
	}
	return out, ErrOrderConflict
}

const orderSafeColumns = "payout_no,business_type,business_no,anchor_no,amount,currency,payee_account_no,payee_version,payee_method,account_masked,payee_snapshot_hash,status,created_by,created_at,updated_at"

func (s *Service) ListOrders(ctx context.Context, a Actor, q req.OrderSearch) (res.OrderPage, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	out := res.OrderPage{Page: q.Page, PageSize: q.PageSize, List: []res.Order{}}
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 || q.From < 0 || q.To < 0 || (q.To > 0 && q.From >= q.To) || (q.PayoutNo != "" && !orderNoRE.MatchString(q.PayoutNo)) || (q.WithdrawalNo != "" && !withdrawalNoRE.MatchString(q.WithdrawalNo)) || (q.AnchorNo != "" && !anchorNoRE.MatchString(q.AnchorNo)) || (q.Currency != "" && !currencyRE.MatchString(q.Currency)) || (q.PayeeMethod != "" && !Supported(q.PayeeMethod)) || (q.Status != "" && !ValidOrderStatus(q.Status)) {
		return out, ErrOrderInvalid
	}
	if e := validateAttemptFilters(q); e != nil {
		return out, e
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorizeOrder(tx, a, "list", "GET"); e != nil {
			return e
		}
		db := tx.Model(&model.Order{})
		if q.ChannelAccountNo != "" || q.OperatorID != "" || q.ReviewerID != "" || q.PaymentReference != "" {
			db = filterOrderAttempts(db, tx, s.Crypto, q)
		}
		for _, f := range []struct{ col, value string }{{"payout_no", q.PayoutNo}, {"business_no", q.WithdrawalNo}, {"anchor_no", q.AnchorNo}, {"currency", q.Currency}, {"payee_method", q.PayeeMethod}, {"status", q.Status}} {
			if f.value != "" {
				db = db.Where(f.col+"=?", f.value)
			}
		}
		if q.From > 0 {
			db = db.Where("created_at>=?", q.From)
		}
		if q.To > 0 {
			db = db.Where("created_at<?", q.To)
		}
		if e := db.Count(&out.Total).Error; e != nil {
			return e
		}
		var rows []model.Order
		if e := db.Select(orderSafeColumns).Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error; e != nil {
			return e
		}
		for _, o := range rows {
			out.List = append(out.List, res.PublicOrder(o))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) OrderDetail(ctx context.Context, a Actor, no string) (res.OrderDetail, error) {
	var out res.OrderDetail
	if !orderNoRE.MatchString(no) {
		return out, ErrOrderInvalid
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorizeOrder(tx, a, "detail", "GET"); e != nil {
			return e
		}
		o, e := s.lockOrder(tx, no)
		if e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return e
		}
		if e := s.VerifyOrder(o); e != nil {
			return e
		}
		out.Order = res.PublicOrder(o)
		out.CreationReconciliation = o.ReconciliationSnapshot
		if e := tx.Where("withdrawal_no=?", o.BusinessNo).Take(&out.Withdrawal).Error; e != nil {
			return e
		}
		out.Check, e = s.checkOrderInTx(tx, o, s.now(), false)
		if e != nil {
			return e
		}
		out.Consistent = out.Check.Consistent
		out.CheckedAt = s.now()
		out.CurrentPayee, e = s.Inspect(tx, o.AnchorNo, o.PayeeAccountNo, o.Currency, o.PayeeVersion, o.PayeeMethod, o.AccountMasked)
		if e != nil {
			return e
		}
		if e = tx.Where("payout_no=?", no).Order("sequence ASC").Limit(512).Find(&out.History).Error; e != nil {
			return e
		}
		return s.manualDetailInTx(tx, a, o, &out)
	})
	return out, e
}
