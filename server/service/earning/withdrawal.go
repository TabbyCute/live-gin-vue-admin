package earning

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math/big"
	"strconv"
	"strings"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	ps "tb_live_module/service/payout"
)

type ReconciliationSnapshot struct {
	Opening             string   `json:"opening"`
	SettlementIncome    string   `json:"settlementIncome"`
	NegativeAdjustments string   `json:"negativeAdjustments"`
	ManualAdjustments   string   `json:"manualAdjustments"`
	Paid                string   `json:"paid"`
	Available           string   `json:"available"`
	Frozen              string   `json:"frozen"`
	Debt                string   `json:"debt"`
	Version             string   `json:"version"`
	RequestedAmount     string   `json:"requestedAmount"`
	AnchorHealthy       bool     `json:"anchorHealthy"`
	PayeeChanged        bool     `json:"payeeChanged"`
	Payee               *Payee   `json:"payee,omitempty"`
	Consistent          bool     `json:"consistent"`
	Issues              []string `json:"issues"`
	CheckedAt           int64    `json:"checkedAt"`
}

func pendingReversal(tx *gorm.DB, id uint64) (bool, error) {
	var f model.Income
	e := unallocated(tx, id).Where("entry_type='reversal'").Limit(1).Find(&f).Error
	return f.ID != 0, e
}
func (s *Service) payee(tx *gorm.DB, anchor, ref, currency string) (Payee, error) {
	if s.Payees == nil {
		return Payee{}, ErrDisabled
	}
	p, e := s.Payees.ResolveInTx(tx, anchor, ref, currency)
	if e != nil {
		if errors.Is(e, ps.ErrState) {
			return Payee{}, ErrState
		}
		if errors.Is(e, ps.ErrCrypto) {
			return Payee{}, fmt.Errorf("%w: %w", ErrUnavailable, ps.ErrCrypto)
		}
		return Payee{}, ErrUnavailable
	}
	if p.AnchorNo != anchor || p.Reference != ref || p.Currency != currency || !clean(p.Reference, 128) || !clean(p.Version, 64) || !ps.Supported(p.Method) || !clean(p.Name, 128) || !clean(p.Masked, 128) || !strings.Contains(p.Masked, "*") || p.CheckedAt > s.now() || s.now()-p.CheckedAt > 30000 || p.ChangedAt < 0 || p.ChangedAt > p.CheckedAt || len(p.RiskFlags) > 100 {
		return Payee{}, ErrState
	}
	for _, flag := range p.RiskFlags {
		if !clean(flag, 256) {
			return Payee{}, ErrState
		}
	}
	if !p.RiskChecked || len(p.RiskFlags) > 0 {
		return p, ErrState
	}

	return p, nil
}
func (s *Service) ApplyWithdrawal(ctx context.Context, user uint64, r req.ApplyWithdrawal) (model.Withdrawal, error) {
	var out model.Withdrawal
	amount, e := integer(r.Amount, false)
	if e != nil || !keyRE.MatchString(r.RequestID) || !currencyRE.MatchString(r.Currency) || !ps.NoRE.MatchString(r.PayeeAccountNo) {
		return out, ErrInvalid
	}
	anchor, e := appAnchor(s.db(ctx), user)
	if e != nil {
		return out, e
	}
	digest := hash(struct {
		R    req.ApplyWithdrawal
		User uint64
	}{r, user})
	e = s.db(ctx).Where("anchor_id=? AND request_id=?", anchor.ID, r.RequestID).Take(&out).Error
	if e == nil {
		if out.RequestHash != digest {
			return out, ErrConflict
		}
		e = s.transaction(ctx, func(tx *gorm.DB) error {
			if e := lockAnchor(tx, uint64(anchor.ID)); e != nil {
				return e
			}
			current, err := appAnchor(tx, user)
			if err != nil {
				return err
			}
			if current.ID != anchor.ID || current.AnchorNo != anchor.AnchorNo {
				return ErrConflict
			}
			account, e := loadAccount(tx, uint64(anchor.ID), anchor.AnchorNo, out.Currency, s.now(), false)
			if e != nil {
				return e
			}
			return s.verifyAccount(tx, account, true)
		})
		return out, e
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	e = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := lockRules(tx, false); e != nil {
			return e
		}
		if e := lockAnchor(tx, uint64(anchor.ID)); e != nil {
			return e
		}
		current, e := appAnchor(tx, user)
		if e != nil {
			return e
		}
		if current.ID != anchor.ID || current.AnchorNo != anchor.AnchorNo {
			return ErrConflict
		}
		if !healthyAnchor(tx, current) {
			return ErrState
		}
		e = tx.Where("anchor_id=? AND request_id=?", anchor.ID, r.RequestID).Take(&out).Error
		if e == nil {
			if out.RequestHash != digest {
				return ErrConflict
			}
			account, e := loadAccount(tx, uint64(anchor.ID), anchor.AnchorNo, out.Currency, s.now(), false)
			if e != nil {
				return e
			}
			return s.verifyAccount(tx, account, true)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		payee, e := s.payee(tx, anchor.AnchorNo, r.PayeeAccountNo, r.Currency)
		if e != nil {
			return e
		}
		// Re-read ownership after every trusted dependency; no reassignment may
		// make the previous owner's balance spendable.
		latest, e := appAnchor(tx, user)
		if e != nil {
			return e
		}
		if latest.ID != anchor.ID || latest.AnchorNo != anchor.AnchorNo {
			return ErrConflict
		}
		var n int64
		if e = tx.Model(&model.SettlementRule{}).Where("status='active' AND currency=? AND effective_from<=? AND effective_to>?", r.Currency, s.now(), s.now()).Count(&n).Error; e != nil {
			return e
		}
		if n != 1 {
			return ErrExchange
		}
		if payee.CheckedAt > s.now() || s.now()-payee.CheckedAt > 30000 {
			return ErrState
		}
		account, e := loadAccount(tx, uint64(anchor.ID), anchor.AnchorNo, r.Currency, s.now(), false)
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrInsufficient
		}
		if e != nil {
			return e
		}
		if e = s.verifyAccount(tx, account, true); e != nil {
			return e
		}
		blocked, e := pendingReversal(tx, uint64(anchor.ID))
		if e != nil {
			return e
		}
		if blocked || account.Debt > 0 {
			return ErrInsufficient
		}
		out = model.Withdrawal{ApplicantUserID: user, WithdrawalNo: number("wd_"), RequestID: r.RequestID, RequestHash: digest, AnchorID: uint64(anchor.ID), AnchorNo: anchor.AnchorNo, Amount: amount, Currency: r.Currency, PayeeRef: payee.Reference, PayeeVersion: payee.Version, PayeeMethod: payee.Method, AccountName: payee.Name, AccountMasked: payee.Masked, Status: "pending_review", ReconciliationStatus: "pending", CreatedAt: s.now()}
		if e = post(tx, &account, "withdrawal_freeze", out.WithdrawalNo, amount, s.now()); e != nil {
			return e
		}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		return tx.Create(&model.FinanceAudit{ObjectNo: out.WithdrawalNo, Action: "withdrawal.create", ActorID: user, ActorType: "app", Snapshot: encode(out), CreatedAt: s.now()}).Error
	})
	return out, e
}
func (s *Service) makeReconciliation(tx *gorm.DB, a Actor, w model.Withdrawal, p *Payee) (model.Reconciliation, error) {
	record, e := s.buildReconciliation(tx, a, w, p)
	if e != nil {
		return record, e
	}
	return record, tx.Create(&record).Error
}
func (s *Service) buildReconciliation(tx *gorm.DB, a Actor, w model.Withdrawal, p *Payee) (model.Reconciliation, error) {
	snapshot := ReconciliationSnapshot{Opening: "0", SettlementIncome: "0", NegativeAdjustments: "0", ManualAdjustments: "0", Paid: "0", RequestedAmount: strconv.FormatInt(w.Amount, 10), Issues: []string{}, CheckedAt: s.now()}
	account, e := loadAccount(tx, w.AnchorID, w.AnchorNo, w.Currency, s.now(), false)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		snapshot.Issues = append(snapshot.Issues, "可提现账户记录缺失")
		account = model.Account{AnchorID: w.AnchorID, AnchorNo: w.AnchorNo, Currency: w.Currency}
	} else if e != nil {
		return model.Reconciliation{}, e
	}

	snapshot.Available = strconv.FormatInt(account.Available, 10)
	snapshot.Frozen = strconv.FormatInt(account.Frozen, 10)
	snapshot.Debt = strconv.FormatInt(account.Debt, 10)
	snapshot.Paid = strconv.FormatInt(account.Paid, 10)
	snapshot.Version = strconv.FormatInt(account.Version, 10)
	if e = s.verifyAccount(tx, account, true); e != nil {
		if !errors.Is(e, ErrInvariant) && !errors.Is(e, ErrSource) && !errors.Is(e, ErrLimit) {
			return model.Reconciliation{}, e
		}
		snapshot.Issues = append(snapshot.Issues, e.Error())
	}
	var st []model.Settlement
	if e = tx.Where("anchor_id=? AND currency=? AND status='approved'", w.AnchorID, w.Currency).Limit(maxAuditFacts + 1).Find(&st).Error; e != nil {
		return model.Reconciliation{}, e
	}
	if len(st) > maxAuditFacts {
		return model.Reconciliation{}, ErrLimit
	}
	credits, negative := new(big.Int), new(big.Int)
	for _, v := range st {
		if v.Amount >= 0 {
			credits.Add(credits, big.NewInt(v.Amount))
		} else {
			negative.Add(negative, big.NewInt(v.Amount))
		}
	}
	snapshot.SettlementIncome = credits.String()
	snapshot.NegativeAdjustments = negative.String()

	anchor, e := anchorByNo(tx, w.AnchorNo)
	if e != nil {
		return model.Reconciliation{}, e
	}
	snapshot.AnchorHealthy = healthyAnchor(tx, anchor)
	if !snapshot.AnchorHealthy {
		snapshot.Issues = append(snapshot.Issues, "主播状态、运营提现权限或风控状态不通过")
	}
	pending, e := pendingReversal(tx, w.AnchorID)
	if e != nil {
		return model.Reconciliation{}, e
	}
	if pending || account.Debt > 0 {
		snapshot.Issues = append(snapshot.Issues, "存在未结算退款冲减或待抵扣欠额")
	}
	snapshot.Payee = p
	snapshot.PayeeChanged = p == nil || p.Reference != w.PayeeRef || p.Version != w.PayeeVersion || p.Name != w.AccountName || p.Masked != w.AccountMasked || p.Method != w.PayeeMethod || p.ChangedAt > w.CreatedAt
	if p == nil {
		snapshot.Issues = append(snapshot.Issues, "可信收款账户及风险核验不可用")
	} else if snapshot.PayeeChanged {
		snapshot.Issues = append(snapshot.Issues, "收款账户与申请快照不一致，请驳回后重新申请")
	} else if s.now()-p.CheckedAt > 30000 || p.CheckedAt > s.now() {
		snapshot.Issues = append(snapshot.Issues, "收款账户核验已过期")
	}
	if p != nil {
		if !p.RiskChecked {
			snapshot.Issues = append(snapshot.Issues, "风控核验未完成")
		}
		snapshot.Issues = append(snapshot.Issues, p.RiskFlags...)
	}
	snapshot.Consistent = len(snapshot.Issues) == 0
	status := "abnormal"
	if snapshot.Consistent {
		status = "passed"
	}
	record := model.Reconciliation{ReconciliationNo: number("rc_"), WithdrawalNo: w.WithdrawalNo, AnchorID: w.AnchorID, Currency: w.Currency, Snapshot: encode(snapshot), Status: status, CreatedBy: a.ID, CreatedAt: s.now()}
	record.SnapshotHash = hash(record.Snapshot)
	return record, nil
}
func (s *Service) ReconcileWithdrawal(ctx context.Context, a Actor, r req.Reconcile) (model.Reconciliation, error) {
	var proof model.Reconciliation
	if !noRE.MatchString(r.WithdrawalNo) {
		return proof, ErrInvalid
	}
	if e := s.authorize(s.db(ctx), a, "reconciliation/run", "POST"); e != nil {
		return proof, e
	}
	var w model.Withdrawal
	if e := s.db(ctx).Where("withdrawal_no=?", r.WithdrawalNo).Take(&w).Error; e != nil {
		return proof, ErrNotFound
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "reconciliation/run", "POST"); e != nil {
			return e
		}
		if e := lockAnchor(tx, w.AnchorID); e != nil {
			return e
		}
		if e := tx.Where("withdrawal_no=?", w.WithdrawalNo).Take(&w).Error; e != nil {
			return e
		}
		var e error
		var payee *Payee
		if p, err := s.payee(tx, w.AnchorNo, w.PayeeRef, w.Currency); err == nil || (errors.Is(err, ErrState) && p.Reference != "") {
			payee = &p
		}
		proof, e = s.makeReconciliation(tx, a, w, payee)
		if e != nil {
			return e
		}
		if w.Status == "pending_review" {
			return tx.Model(&model.Withdrawal{}).Where("id=? AND status='pending_review'", w.ID).Updates(map[string]any{"reconciliation_status": proof.Status, "reconciliation_no": proof.ReconciliationNo}).Error
		}
		return nil
	})
	return proof, e
}
func (s *Service) ReviewWithdrawal(ctx context.Context, a Actor, r req.FinanceReview) (model.Withdrawal, error) {
	var out model.Withdrawal
	if !noRE.MatchString(r.No) || !clean(r.Reason, 500) {
		return out, ErrInvalid
	}
	if e := s.authorize(s.db(ctx), a, "withdrawal/review", "POST"); e != nil {
		return out, e
	}
	if e := s.db(ctx).Where("withdrawal_no=?", r.No).Take(&out).Error; e != nil {
		return out, ErrNotFound
	}
	blocked := false
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		blocked = false
		if e := s.authorize(tx, a, "withdrawal/review", "POST"); e != nil {
			return e
		}
		if e := lockAnchor(tx, out.AnchorID); e != nil {
			return e
		}
		if e := tx.Where("withdrawal_no=?", r.No).Take(&out).Error; e != nil {
			return e
		}
		want := "rejected"
		if r.Approve {
			want = "approved"
		}
		account, e := loadAccount(tx, out.AnchorID, out.AnchorNo, out.Currency, s.now(), false)
		if e != nil && !(errors.Is(e, gorm.ErrRecordNotFound) && r.Approve && out.Status == "pending_review") {
			return ErrInvariant
		}

		if out.Status != "pending_review" {
			if out.Status == want && out.ReviewedBy == a.ID && out.ReviewReason == r.Reason {
				return s.verifyAccount(tx, account, true)
			}
			return ErrConflict
		}
		if r.Approve {
			var payee *Payee
			if p, err := s.payee(tx, out.AnchorNo, out.PayeeRef, out.Currency); err == nil || (errors.Is(err, ErrState) && p.Reference != "") {
				payee = &p
			}
			proof, e := s.makeReconciliation(tx, a, out, payee)
			if e != nil {
				return e
			}
			out.ReconciliationNo = proof.ReconciliationNo
			out.ReconciliationStatus = proof.Status
			if e = tx.Model(&model.Withdrawal{}).Where("id=? AND status='pending_review'", out.ID).Updates(map[string]any{"reconciliation_no": proof.ReconciliationNo, "reconciliation_status": proof.Status}).Error; e != nil {
				return e
			}
			if proof.Status != "passed" {
				blocked = true
				return nil
			}
		} else {
			if e = s.verifyAccount(tx, account, false); e != nil {
				return e
			}
			if e = post(tx, &account, "withdrawal_release", out.WithdrawalNo, out.Amount, s.now()); e != nil {
				return e
			}
		}
		reviewedAt := s.now()
		change := tx.Model(&model.Withdrawal{}).Where("id=? AND status='pending_review'", out.ID).Updates(map[string]any{"status": want, "reviewed_by": a.ID, "reviewed_at": reviewedAt, "review_reason": r.Reason})
		if change.Error != nil {
			return change.Error
		}
		if change.RowsAffected != 1 {
			return ErrConflict
		}
		out.Status = want
		out.ReviewedBy = a.ID
		out.ReviewedAt = reviewedAt
		out.ReviewReason = r.Reason
		return audit(tx, a, out.WithdrawalNo, "withdrawal.review", encode(out), s.now())
	})
	if e == nil && blocked {
		e = ErrInvariant
	}
	return out, e
}
