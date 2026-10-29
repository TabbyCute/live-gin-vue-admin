package wallet

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"
	common "tb_live_module/model/common/response"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	res "tb_live_module/model/wallet/response"
)

func (s *Service) Balance(ctx context.Context, accountID uint64, code string) (res.Balance, error) {
	if _, err := s.options(); err != nil {
		return res.Balance{}, err
	}
	code, err := currency(code)
	if err != nil {
		return res.Balance{}, err
	}
	if accountID == 0 {
		return res.Balance{}, ErrAccount
	}
	if _, err = findAccount(s.db(ctx), "", accountID, true, false); err != nil {
		return res.Balance{}, err
	}
	return s.balance(ctx, accountID, code)
}
func (s *Service) balance(ctx context.Context, id uint64, code string) (res.Balance, error) {
	var w model.WalletAccount
	err := s.db(ctx).Where("account_id = ? AND currency_code = ?", id, code).Take(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		w = model.WalletAccount{CurrencyCode: code, Status: model.AccountNormal}
		err = nil
	}
	return balanceDTO(w), err
}
func (s *Service) AdminBalance(ctx context.Context, a Actor, r req.AccountQuery) (res.Balance, error) {
	if err := s.Authorize(ctx, a, "read"); err != nil {
		return res.Balance{}, err
	}
	code, err := currency(r.CurrencyCode)
	if err != nil {
		return res.Balance{}, ErrInvalid
	}
	user, err := findAdminAccount(s.db(ctx), r.AccountNo, r.Phone, false, false)
	if err != nil {
		return res.Balance{}, err
	}
	balance, err := s.balance(ctx, uint64(user.ID), code)
	if err != nil {
		return res.Balance{}, err
	}
	balance.AccountNo = user.AccountNo
	if user.Phone != nil {
		balance.Phone = *user.Phone
	}
	return balance, nil
}
func (s *Service) SetStatus(ctx context.Context, a Actor, r req.AccountStatus) (res.Balance, error) {
	if _, err := s.options(); err != nil {
		return res.Balance{}, err
	}
	code, err := currency(r.CurrencyCode)
	if err != nil || !reasonOK(r.Reason) || (r.Status != model.AccountNormal && r.Status != model.AccountDisabled) {
		return res.Balance{}, ErrInvalid
	}
	var w model.WalletAccount
	var userAccount liveAccountIdentity
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "review", "SHARE"); e != nil {
			return e
		}
		user, e := findAdminAccount(tx, r.AccountNo, r.Phone, r.Status == model.AccountNormal, true)
		if e != nil {
			return e
		}
		userAccount.AccountNo = user.AccountNo
		if user.Phone != nil {
			userAccount.Phone = *user.Phone
		}
		w, e = lockWallet(tx, uint64(user.ID), code)
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrWalletNotFound
		}
		if e != nil {
			return e
		}
		if w.Status == r.Status {
			return nil
		}
		now := s.now().UnixMilli()
		if e = rowsOne(tx.Model(&model.WalletAccount{}).Where("id = ?", w.ID).Updates(map[string]any{"status": r.Status, "updated_at": now})); e != nil {
			return e
		}
		if e = audit(tx, a, fmt.Sprintf("wallet:%d", w.ID), "status_"+r.Status, r.Reason, now); e != nil {
			return e
		}
		w.Status = r.Status
		w.UpdatedAt = now
		return nil
	})
	balance := balanceDTO(w)
	balance.AccountNo = userAccount.AccountNo
	balance.Phone = userAccount.Phone
	return balance, err
}

type liveAccountIdentity struct {
	AccountNo string
	Phone     string
}

func (s *Service) Ledger(ctx context.Context, accountID uint64, r req.LedgerSearch) (res.LedgerList, error) {
	if _, err := s.options(); err != nil {
		return res.LedgerList{}, err
	}
	if accountID == 0 || r.AccountNo != "" || r.Phone != "" {
		return res.LedgerList{}, ErrInvalid
	}
	if _, err := findAccount(s.db(ctx), "", accountID, true, false); err != nil {
		return res.LedgerList{}, err
	}
	return s.ledger(ctx, accountID, r, false)
}
func (s *Service) AdminLedger(ctx context.Context, a Actor, r req.LedgerSearch) (res.LedgerList, error) {
	if err := s.Authorize(ctx, a, "read"); err != nil {
		return res.LedgerList{}, err
	}
	user, err := findAdminAccount(s.db(ctx), r.AccountNo, r.Phone, false, false)
	if err != nil {
		return res.LedgerList{}, err
	}
	return s.ledger(ctx, uint64(user.ID), r, true)
}
func (s *Service) ledger(ctx context.Context, id uint64, r req.LedgerSearch, admin bool) (res.LedgerList, error) {
	result := res.LedgerList{List: []res.Ledger{}}
	code, err := currency(r.CurrencyCode)
	if err != nil {
		return result, err
	}
	if r.Limit == 0 {
		r.Limit = 20
	}
	if r.Limit < 1 || r.Limit > 100 {
		return result, ErrInvalid
	}
	var w model.WalletAccount
	err = s.db(ctx).Where("account_id = ? AND currency_code = ?", id, code).Take(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if r.Cursor != "" {
			return result, ErrInvalid
		}
		return result, nil
	}
	if err != nil {
		return result, err
	}
	q := s.db(ctx).Where("wallet_account_id = ?", w.ID)
	if r.Cursor != "" {
		if len(r.Cursor) > 100 {
			return result, ErrInvalid
		}
		b, e := base64.RawURLEncoding.DecodeString(r.Cursor)
		if e != nil {
			return result, ErrInvalid
		}
		parts := strings.Split(string(b), ":")
		if len(parts) != 2 || parts[0] != strconv.FormatUint(w.ID, 10) {
			return result, ErrInvalid
		}
		before, e := strconv.ParseUint(parts[1], 10, 64)
		if e != nil || before == 0 {
			return result, ErrInvalid
		}
		q = q.Where("wallet_version < ?", before)
	}
	var rows []model.WalletLedger
	if err = q.Order("wallet_version DESC").Limit(r.Limit + 1).Find(&rows).Error; err != nil {
		return result, err
	}
	result.HasMore = len(rows) > r.Limit
	if result.HasMore {
		rows = rows[:r.Limit]
		result.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%d", w.ID, rows[len(rows)-1].WalletVersion)))
	}
	displays := map[uint64]string{}
	if admin {
		ids := make([]uint64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.OperatorID)
		}
		displays, err = adminDisplays(s.db(ctx), ids...)
		if err != nil {
			return result, err
		}
	}
	for _, l := range rows {
		dto := ledgerDTO(l, code, admin)
		if admin {
			dto.OperatorDisplay = displays[l.OperatorID]
		}
		result.List = append(result.List, dto)
	}
	return result, nil
}
func (s *Service) Orders(ctx context.Context, a Actor, r req.OrderSearch) (common.PageResult, error) {
	result := common.PageResult{List: []res.Order{}}
	if err := s.Authorize(ctx, a, "read"); err != nil {
		return result, err
	}
	if r.Page == 0 {
		r.Page = 1
	}
	if r.PageSize == 0 {
		r.PageSize = 20
	}
	if r.Keyword != "" || r.Page < 1 || r.PageSize < 1 || r.PageSize > 100 || r.Page > 10000/r.PageSize+1 {
		return result, ErrInvalid
	}
	q := s.db(ctx).Model(&model.WalletAdjustmentOrder{})
	if r.ExternalOrderNo != "" {
		if r.SourceType == "" {
			r.SourceType = model.SourceManual
		}
		no := r.ExternalOrderNo
		if r.SourceType == model.SourceManual {
			var err error
			no, err = normalizeExternalNo(no)
			if err != nil {
				return result, err
			}
		} else if !textOK(no, 1, 128) {
			return result, ErrInvalid
		}
		q = q.Where("external_order_no = ?", no)
	}
	if r.SourceType != "" {
		if r.SourceType != model.SourceManual && r.SourceType != model.SourceLegacy && r.SourceType != model.SourceReversal {
			return result, ErrInvalid
		}
		q = q.Where("source_type = ?", r.SourceType)
	}
	if r.AccountNo != "" || r.Phone != "" {
		user, err := findAdminAccount(s.db(ctx), r.AccountNo, r.Phone, false, false)
		if err != nil {
			return result, err
		}
		q = q.Where("account_id = ?", user.ID)
	}
	if r.OrderNo != "" {
		if !orderNo.MatchString(r.OrderNo) {
			return result, ErrInvalid
		}
		q = q.Where("order_no = ?", r.OrderNo)
	}
	if r.Status != "" {
		switch r.Status {
		case model.StatusPending, model.StatusCredited, model.StatusRejected, model.StatusReversed:
		default:
			return result, ErrInvalid
		}
		q = q.Where("status = ?", r.Status)
	}
	if r.Kind != "" {
		if r.Kind != model.KindGrant && r.Kind != model.KindReverse {
			return result, ErrInvalid
		}
		q = q.Where("kind = ?", r.Kind)
	}
	if err := q.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []model.WalletAdjustmentOrder
	if err := q.Order("id DESC").Limit(r.PageSize).Offset((r.Page - 1) * r.PageSize).Find(&rows).Error; err != nil {
		return result, err
	}
	ids := make([]uint64, 0, len(rows)*3)
	for _, order := range rows {
		ids = append(ids, order.CreatedBy, order.ApprovedBy, order.RejectedBy)
	}
	displays, err := adminDisplays(s.db(ctx), ids...)
	if err != nil {
		return result, err
	}
	list := make([]res.Order, 0, len(rows))
	for _, o := range rows {
		dto := orderDTO(o)
		dto.CreatedByDisplay = displays[o.CreatedBy]
		dto.ApprovedByDisplay = displays[o.ApprovedBy]
		dto.RejectedByDisplay = displays[o.RejectedBy]
		list = append(list, dto)
	}
	result.List = list
	result.Page = r.Page
	result.PageSize = r.PageSize
	return result, nil
}
func (s *Service) Detail(ctx context.Context, a Actor, no string) (res.OrderDetail, error) {
	result := res.OrderDetail{Audit: []res.Audit{}}
	if err := s.Authorize(ctx, a, "read"); err != nil {
		return result, err
	}
	if !orderNo.MatchString(no) {
		return result, ErrInvalid
	}
	// 同一只读快照中的订单、流水和审计一致，且不阻塞审核。
	err := s.snapshot(ctx, func(tx *gorm.DB) error {
		var o model.WalletAdjustmentOrder
		if e := tx.Where("order_no = ?", no).Take(&o).Error; errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if e != nil {
			return e
		}
		result.Order = orderDTO(o)
		operatorIDs := []uint64{o.CreatedBy, o.ApprovedBy, o.RejectedBy}
		var ledgerOperatorID uint64
		if o.LedgerNo != "" {
			var l model.WalletLedger
			if e := tx.Where("ledger_no = ?", o.LedgerNo).Take(&l).Error; e != nil {
				return e
			}
			dto := ledgerDTO(l, o.CurrencyCode, true)
			result.Ledger = &dto
			ledgerOperatorID = l.OperatorID
			operatorIDs = append(operatorIDs, l.OperatorID)
		}
		var logs []model.WalletAuditLog
		if e := tx.Where("object_no = ?", no).Order("id ASC").Limit(100).Find(&logs).Error; e != nil {
			return e
		}
		result.Audit = []res.Audit{}
		for _, log := range logs {
			operatorIDs = append(operatorIDs, log.OperatorID)
		}
		displays, e := adminDisplays(tx, operatorIDs...)
		if e != nil {
			return e
		}
		result.Order.CreatedByDisplay = displays[o.CreatedBy]
		result.Order.ApprovedByDisplay = displays[o.ApprovedBy]
		result.Order.RejectedByDisplay = displays[o.RejectedBy]
		if result.Ledger != nil {
			result.Ledger.OperatorDisplay = displays[ledgerOperatorID]
		}
		for _, log := range logs {
			result.Audit = append(result.Audit, res.Audit{Action: log.Action, OperatorID: strconv.FormatUint(log.OperatorID, 10), OperatorDisplay: displays[log.OperatorID], OperatorIP: log.OperatorIP, Reason: log.Reason, CreatedAt: log.CreatedAt})
		}
		return nil
	})
	return result, err
}

// Check 在每批只读快照内核对余额链、业务单和审核人，不做自动修复。
func (s *Service) Check(ctx context.Context, a Actor, r req.CheckSearch) (res.Check, error) {
	result := res.Check{AccountNo: r.AccountNo, Issues: []string{}, Consistent: true, NextVersion: "0", SnapshotVersion: "0", NextOrderID: "0"}
	if err := s.Authorize(ctx, a, "read"); err != nil {
		return result, err
	}
	code, err := currency(r.CurrencyCode)
	if err != nil {
		return result, ErrInvalid
	}
	result.CurrencyCode = code
	if r.Limit == 0 {
		r.Limit = 100
	}
	if r.Limit < 1 || r.Limit > 1000 {
		return result, ErrInvalid
	}
	var after int64
	if r.AfterVersion != "" {
		after, err = strconv.ParseInt(r.AfterVersion, 10, 64)
		if err != nil || after < 0 {
			return result, ErrInvalid
		}
	}
	var afterOrder uint64
	if r.AfterOrderID != "" {
		afterOrder, err = strconv.ParseUint(r.AfterOrderID, 10, 64)
		if err != nil {
			return result, ErrInvalid
		}
	}
	user, err := findAdminAccount(s.db(ctx), r.AccountNo, r.Phone, false, false)
	if err != nil {
		return result, err
	}
	result.AccountNo = user.AccountNo
	err = s.snapshot(ctx, func(tx *gorm.DB) error {
		result.Issues = []string{}
		result.Checked = 0
		result.HasMore = false
		result.Consistent = true
		issue := func(v string) { result.Consistent = false; result.Issues = append(result.Issues, v) }
		var w model.WalletAccount
		// 首次SELECT建立RR快照；即使另一事务随后开户，此批次的订单/流水也
		// 仍处于开户前状态，无需锁父账号或钱包来规避首次开户竞争。
		e := tx.Where("account_id = ? AND currency_code = ?", user.ID, code).Take(&w).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if after != 0 {
				return ErrInvalid
			}
			return checkOrders(tx, uint64(user.ID), w.ID, code, afterOrder, r.Limit, &result)
		}
		if e != nil {
			return e
		}
		if after > w.Version {
			return ErrInvalid
		}
		result.SnapshotVersion = strconv.FormatInt(w.Version, 10)
		if w.FrozenBalance != 0 || w.AvailableBalance < 0 {
			issue("wallet_balance_invalid")
		}
		before := int64(0)
		if after > 0 {
			var prev model.WalletLedger
			if e = tx.Where("wallet_account_id = ? AND wallet_version = ?", w.ID, after).Take(&prev).Error; errors.Is(e, gorm.ErrRecordNotFound) {
				issue("previous_ledger_missing")
			} else if e != nil {
				return e
			} else {
				before = prev.BalanceAfter
			}
		}
		var rows []model.WalletLedger
		if e = tx.Where("wallet_account_id = ? AND wallet_version > ?", w.ID, after).Order("wallet_version ASC").Limit(r.Limit + 1).Find(&rows).Error; e != nil {
			return e
		}
		result.HasMore = len(rows) > r.Limit
		if result.HasMore {
			rows = rows[:r.Limit]
		}
		nos := make([]string, 0, len(rows))
		for _, l := range rows {
			nos = append(nos, l.BusinessNo)
		}
		orders := map[string]model.WalletAdjustmentOrder{}
		if len(nos) > 0 {
			var os []model.WalletAdjustmentOrder
			if e = tx.Where("order_no IN ?", nos).Find(&os).Error; e != nil {
				return e
			}
			for _, o := range os {
				orders[o.OrderNo] = o
			}
		}
		version := after
		for _, l := range rows {
			result.Checked++
			version++
			if l.WalletVersion != version || l.BalanceBefore != before {
				issue("ledger_chain:" + l.LedgerNo)
			}
			if l.ChangeAmount == 0 || l.BalanceBefore < 0 || l.BalanceAfter < 0 || (l.ChangeAmount > 0 && l.BalanceBefore > math.MaxInt64-l.ChangeAmount) || (l.ChangeAmount < 0 && (l.ChangeAmount == math.MinInt64 || l.BalanceBefore < -l.ChangeAmount)) || l.BalanceAfter != l.BalanceBefore+l.ChangeAmount {
				issue("ledger_amount:" + l.LedgerNo)
			}
			o, ok := orders[l.BusinessNo]
			expected := o.Amount
			business := model.BusinessGrant
			if o.Kind == model.KindReverse {
				expected = -o.Amount
				business = model.BusinessReverse
			}
			if !ok || o.AccountID != w.AccountID || o.CurrencyCode != code || o.LedgerNo != l.LedgerNo || (o.Status != model.StatusCredited && o.Status != model.StatusReversed) || o.ApprovedBy == 0 || (o.CreatedBy == o.ApprovedBy && o.ApprovedBy != topSuperAdminUserID) || o.ApprovedBy != l.OperatorID || expected != l.ChangeAmount || business != l.BusinessType {
				issue("ledger_order:" + l.LedgerNo)
			}
			before = l.BalanceAfter
			version = l.WalletVersion
		}
		result.NextVersion = strconv.FormatInt(version, 10)
		if !result.HasMore && (version != w.Version || before != w.AvailableBalance) {
			issue("wallet_tail_mismatch")
		}
		return checkOrders(tx, uint64(user.ID), w.ID, code, afterOrder, r.Limit, &result)
	})
	return result, err
}

// 独立扫描业务单，能够发现“已入账订单丢流水”或待审单提前产生流水，而不仅是余额合计不符。
func checkOrders(tx *gorm.DB, accountID uint64, walletID uint64, code string, after uint64, limit int, result *res.Check) error {
	result.NextOrderID = strconv.FormatUint(after, 10)
	result.CheckedOrders = 0
	var orders []model.WalletAdjustmentOrder
	if err := tx.Where("account_id = ? AND currency_code = ? AND id > ?", accountID, code, after).Order("id ASC").Limit(limit + 1).Find(&orders).Error; err != nil {
		return err
	}
	if len(orders) > limit {
		result.HasMore = true
		orders = orders[:limit]
	}
	if len(orders) == 0 {
		return nil
	}
	nos := make([]string, 0, len(orders))
	originalNos := []string{}
	for _, o := range orders {
		nos = append(nos, o.OrderNo)
		if o.Kind == model.KindReverse && o.Status == model.StatusCredited {
			originalNos = append(originalNos, o.OriginalOrderNo)
		}
		if o.Kind == model.KindGrant && o.Status == model.StatusReversed {
			originalNos = append(originalNos, o.ReversalOrderNo)
		}
	}
	var ledgers []model.WalletLedger
	if err := tx.Where("business_type IN ? AND business_no IN ?", []string{model.BusinessGrant, model.BusinessReverse}, nos).Find(&ledgers).Error; err != nil {
		return err
	}
	byOrder := map[string][]model.WalletLedger{}
	for _, l := range ledgers {
		byOrder[l.BusinessNo] = append(byOrder[l.BusinessNo], l)
	}
	// 每个订单应有1条创建审计、终态再有1条审核审计。限制读取量；重复审计
	// 也属于异常，不能因为异常行过多把批次放大为无界查询。
	var auditRows []model.WalletAuditLog
	if err := tx.Where("object_no IN ?", nos).Order("id").Limit(2*len(orders) + 1).Find(&auditRows).Error; err != nil {
		return err
	}
	audits := map[string][]model.WalletAuditLog{}
	for _, log := range auditRows {
		audits[log.ObjectNo] = append(audits[log.ObjectNo], log)
	}
	originals := map[string]model.WalletAdjustmentOrder{}
	if len(originalNos) > 0 {
		var rows []model.WalletAdjustmentOrder
		if err := tx.Where("order_no IN ?", originalNos).Find(&rows).Error; err != nil {
			return err
		}
		for _, o := range rows {
			originals[o.OrderNo] = o
		}
	}
	for _, o := range orders {
		result.CheckedOrders++
		result.NextOrderID = strconv.FormatUint(o.ID, 10)
		if !validOrderAudit(o, audits[o.OrderNo]) {
			result.Consistent = false
			result.Issues = append(result.Issues, "order_audit:"+o.OrderNo)
		}
		if o.SourceType == model.SourceLegacy {
			if o.Status != model.StatusRejected {
				result.Consistent = false
				result.Issues = append(result.Issues, "legacy_source_unverified:"+o.OrderNo)
			}
		} else if !validOrderRequest(o) {
			result.Consistent = false
			result.Issues = append(result.Issues, "order_request_hash:"+o.OrderNo)
		}
		valid := true
		ls := byOrder[o.OrderNo]
		if o.Status == model.StatusCredited || o.Status == model.StatusReversed {
			valid = len(ls) == 1 && o.LedgerNo != "" && o.ApprovedBy > 0 && (o.CreatedBy != o.ApprovedBy || o.ApprovedBy == topSuperAdminUserID)
			if valid {
				valid = validOrderLedger(o, ls[0], walletID)
			}
			if o.Kind == model.KindGrant && o.Status == model.StatusReversed {
				reverse, ok := originals[o.ReversalOrderNo]
				valid = valid && ok && reverse.Kind == model.KindReverse && reverse.Status == model.StatusCredited && reverse.OriginalOrderNo == o.OrderNo && reverse.AccountID == o.AccountID && reverse.CurrencyCode == o.CurrencyCode && reverse.Amount == o.Amount
			}
			if o.Kind == model.KindReverse {
				original, ok := originals[o.OriginalOrderNo]
				valid = valid && ok && original.Kind == model.KindGrant && original.Status == model.StatusReversed && original.ReversalOrderNo == o.OrderNo && original.Amount == o.Amount && original.AccountID == o.AccountID && original.CurrencyCode == o.CurrencyCode
				if len(ls) == 1 {
					valid = valid && ls[0].OriginalLedgerNo == original.LedgerNo
				}
			}
		} else {
			valid = len(ls) == 0 && o.LedgerNo == "" && o.ApprovedBy == 0
		}
		if !valid {
			result.Consistent = false
			result.Issues = append(result.Issues, "order_ledger:"+o.OrderNo)
		}
	}
	return nil
}
