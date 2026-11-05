package wallet

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	res "tb_live_module/model/wallet/response"
)

func transactionDTO(t model.WalletTransaction, displays map[uint64]string) res.Transaction {
	display := t.ActorType + " [ " + t.ActorID + " ]"
	if t.ActorType == "user" {
		display = "用户 [ " + t.AccountNo + " ]"
	}
	if t.ActorType == "system" {
		display = "系统 [ " + t.ActorID + " ]"
	}
	if t.ActorType == "admin" {
		id, _ := strconv.ParseUint(t.ActorID, 10, 64)
		display = displays[id]
	}
	return res.Transaction{TransactionNo: t.TransactionNo, Kind: t.Kind, BusinessType: t.BusinessType, BusinessNo: t.BusinessNo, AccountNo: t.AccountNo, CurrencyCode: t.CurrencyCode, Amount: strconv.FormatInt(t.Amount, 10), RefundedAmount: strconv.FormatInt(t.RefundedAmount, 10), OriginalNo: t.OriginalNo, LedgerNo: t.LedgerNo, ActorType: t.ActorType, ActorID: t.ActorID, ActorDisplay: display, ActorIP: t.ActorIP, TraceID: t.TraceID, Reason: t.Reason, CreatedAt: t.CreatedAt}
}

// Trace只供已授权后台读取。No支持流水号、账务交易号和人工充值单号。
// 业务键查询必须带命名空间和动作，避免将不同产品的同号订单混淆。
func (s *Service) Trace(ctx context.Context, a Actor, r req.TraceQuery) (res.Trace, error) {
	result := res.Trace{Refunds: []res.Transaction{}, Audit: []res.Audit{}, Issues: []string{}, NextRefundID: "0"}
	if e := s.Authorize(ctx, a, "read"); e != nil {
		return result, e
	}
	var after uint64
	if r.AfterRefundID != "" {
		var e error
		after, e = strconv.ParseUint(r.AfterRefundID, 10, 64)
		if e != nil {
			return result, ErrInvalid
		}
	}
	if r.No != "" {
		if r.BusinessNo != "" || r.BusinessType != "" || r.Kind != "" {
			return result, ErrInvalid
		}
		if orderNo.MatchString(r.No) {
			d, e := s.Detail(ctx, a, r.No)
			result.Adjustment = &d
			result.Ledger = d.Ledger
			result.Audit = d.Audit
			result.Issues = d.Issues
			return result, e
		}
		if !transactionNo.MatchString(r.No) && !(strings.HasPrefix(r.No, "wl_") && len(r.No) == 35 && businessKey.MatchString(r.No)) {
			return result, ErrInvalid
		}
	} else if !businessNamespace.MatchString(r.BusinessType) || !businessKey.MatchString(r.BusinessNo) || (r.Kind != model.TransactionDebit && r.Kind != model.TransactionRefund) {
		return result, ErrInvalid
	}
	manualNo := ""
	e := s.snapshot(ctx, func(tx *gorm.DB) error {
		var t model.WalletTransaction
		q := tx.Where("kind = ? AND business_type = ? AND business_no = ?", r.Kind, r.BusinessType, r.BusinessNo)
		if strings.HasPrefix(r.No, "wl_") {
			var l model.WalletLedger
			if e := tx.Where("ledger_no = ?", r.No).Take(&l).Error; e != nil {
				return e
			}
			if l.BusinessType == model.BusinessGrant || l.BusinessType == model.BusinessReverse {
				manualNo = l.BusinessNo
				return nil
			}
			q = tx.Where("ledger_no = ?", r.No)
		} else if r.No != "" {
			q = tx.Where("transaction_no = ?", r.No)
		}
		if e := q.Take(&t).Error; e != nil {
			return e
		}
		var wallet model.WalletAccount
		if e := tx.Where("account_id = ? AND currency_code = ?", t.AccountID, t.CurrencyCode).Take(&wallet).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if _, e := validateTransaction(tx, t, wallet); e != nil {
			if !errors.Is(e, ErrInvariant) {
				return e
			}
			result.Issues = append(result.Issues, "transaction_integrity")
		}
		all := []model.WalletTransaction{t}
		original := t
		if t.Kind == model.TransactionRefund {
			original = model.WalletTransaction{}
			if e := tx.Where("transaction_no = ?", t.OriginalNo).Take(&original).Error; e != nil {
				if !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
				original.TransactionNo = t.OriginalNo
			}
			all = append(all, original)
		}
		var refunds []model.WalletTransaction
		if e := tx.Where("original_no = ? AND id > ?", original.TransactionNo, after).Order("id").Limit(51).Find(&refunds).Error; e != nil {
			return e
		}
		result.HasMore = len(refunds) > 50
		if result.HasMore {
			refunds = refunds[:50]
		}
		all = append(all, refunds...)
		ids := []uint64{}
		for _, v := range all {
			if v.ActorType == "admin" {
				id, _ := strconv.ParseUint(v.ActorID, 10, 64)
				ids = append(ids, id)
			}
		}
		displays, e := adminDisplays(tx, ids...)
		if e != nil {
			return e
		}
		d := transactionDTO(t, displays)
		result.Transaction = &d
		if t.Kind == model.TransactionRefund {
			d := transactionDTO(original, displays)
			result.Original = &d
		}
		for _, v := range refunds {
			result.Refunds = append(result.Refunds, transactionDTO(v, displays))
			result.NextRefundID = strconv.FormatUint(v.ID, 10)
		}
		var l model.WalletLedger
		e = tx.Where("ledger_no = ?", t.LedgerNo).Take(&l).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e == nil {
			d := ledgerDTO(l, t.CurrencyCode, true)
			d.OperatorDisplay = result.Transaction.ActorDisplay
			result.Ledger = &d
		}
		var logs []model.WalletAuditLog
		if e = tx.Where("object_no = ?", t.TransactionNo).Order("id").Limit(100).Find(&logs).Error; e != nil {
			return e
		}
		for _, log := range logs {
			result.Audit = append(result.Audit, res.Audit{Action: log.Action, OperatorID: strconv.FormatUint(log.OperatorID, 10), OperatorDisplay: result.Transaction.ActorDisplay, OperatorIP: log.OperatorIP, Reason: log.Reason, CreatedAt: log.CreatedAt})
		}
		return nil
	})
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	if e == nil && manualNo != "" {
		d, err := s.Detail(ctx, a, manualNo)
		result.Adjustment = &d
		result.Ledger = d.Ledger
		result.Audit = d.Audit
		result.Issues = d.Issues
		e = err
	}
	return result, e
}
