package wallet

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	res "tb_live_module/model/wallet/response"
)

func existingRequest(tx *gorm.DB, a Actor, key, digest string) (model.WalletAdjustmentOrder, bool, error) {
	var o model.WalletAdjustmentOrder
	err := tx.Where("created_by = ? AND request_id = ?", a.ID, key).Take(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return o, false, nil
	}
	if err != nil {
		return o, false, err
	}
	if o.RequestHash != digest {
		return o, true, ErrConflict
	}
	return o, true, nil
}

// validOrderRequest 用创建时的规范化请求重算摘要，作为数据库不可变触发器之外的
// 第二道保护。它也让SQLite单测和误关闭触发器的环境在入账前停止处理被篡改订单。
func validOrderRequest(o model.WalletAdjustmentOrder) bool {
	switch o.Kind {
	case model.KindGrant:
		canonical, err := normalizeExternalNo(o.ExternalOrderNo)
		if err != nil || canonical != o.ExternalOrderNo || o.SourceType != model.SourceManual {
			return false
		}
		r := req.CreateAdjustment{
			RequestID:       o.RequestID,
			SourceType:      o.SourceType,
			ExternalOrderNo: o.ExternalOrderNo,
			AccountNo:       o.AccountNo,
			CurrencyCode:    o.CurrencyCode,
			Amount:          strconv.FormatInt(o.Amount, 10),
			Reason:          o.Reason,
			EvidenceRef:     o.EvidenceRef,
		}
		return o.RequestHash == hash(struct {
			Kind    string
			Request req.CreateAdjustment
		}{model.KindGrant, r})
	case model.KindReverse:
		if o.SourceType != model.SourceReversal || o.ExternalOrderNo != o.OrderNo {
			return false
		}
		r := req.ReverseAdjustment{RequestID: o.RequestID, OrderNo: o.OriginalOrderNo, Reason: o.Reason, EvidenceRef: o.EvidenceRef}
		return o.RequestHash == hash(struct {
			Kind    string
			Request req.ReverseAdjustment
		}{model.KindReverse, r})
	default:
		return false
	}
}

func (s *Service) resolveRequest(ctx context.Context, a Actor, key, digest string) (model.WalletAdjustmentOrder, bool, error) {
	return existingRequest(s.db(ctx), a, key, digest)
}

func checkPending(tx *gorm.DB, a Actor) error {
	var ids []uint64
	if err := tx.Model(&model.WalletAdjustmentOrder{}).Where("created_by = ? AND status = ?", a.ID, model.StatusPending).Limit(100).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) >= 100 {
		return ErrLimit
	}
	return nil
}

func (s *Service) Create(ctx context.Context, a Actor, r req.CreateAdjustment) (res.Order, error) {
	cfg, err := s.options()
	if err != nil {
		return res.Order{}, err
	}
	r.CurrencyCode, err = currency(r.CurrencyCode)
	if err != nil {
		return res.Order{}, err
	}
	amount, err := parseAmount(r.Amount)
	if err != nil {
		return res.Order{}, err
	}
	if r.SourceType == "" {
		r.SourceType = model.SourceManual
	}
	if r.SourceType != model.SourceManual {
		return res.Order{}, ErrInvalid
	}
	r.ExternalOrderNo, err = normalizeExternalNo(r.ExternalOrderNo)
	if err != nil {
		return res.Order{}, err
	}
	if r.EvidenceRef == "" {
		return res.Order{}, ErrEvidence
	}
	if !requestKey.MatchString(r.RequestID) || !publicNo.MatchString(r.AccountNo) || !reasonOK(r.Reason) || !textOK(r.EvidenceRef, 1, 500) {
		return res.Order{}, ErrInvalid
	}
	digest := hash(struct {
		Kind    string
		Request req.CreateAdjustment
	}{model.KindGrant, r})
	var o model.WalletAdjustmentOrder
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "create", "UPDATE"); e != nil {
			return e
		}
		existing, found, e := existingRequest(tx, a, r.RequestID, digest)
		if e != nil {
			return e
		}
		if found {
			o = existing
			return nil
		}
		if amount > cfg.MaxGrantAmount {
			return ErrLimit
		}
		if e = checkPending(tx, a); e != nil {
			return e
		}
		user, e := findAccount(tx, r.AccountNo, 0, true, true)
		if e != nil {
			return e
		}
		now := s.now().UnixMilli()
		o = model.WalletAdjustmentOrder{OrderNo: newNo("wa_"), RequestID: r.RequestID, RequestHash: digest, SourceType: r.SourceType, ExternalOrderNo: r.ExternalOrderNo, Kind: model.KindGrant, AccountID: uint64(user.ID), AccountNo: user.AccountNo, CurrencyCode: r.CurrencyCode, Amount: amount, Reason: r.Reason, EvidenceRef: r.EvidenceRef, Status: model.StatusPending, CreatedBy: a.ID, CreatedIP: a.IP, CreatedAt: now, UpdatedAt: now}
		if e = tx.Create(&o).Error; e != nil {
			return e
		}
		return audit(tx, a, o.OrderNo, "create", r.Reason, now)
	})
	if duplicateKey(err) {
		if existing, found, e := s.resolveRequest(ctx, a, r.RequestID, digest); e != nil {
			return res.Order{}, e
		} else if found {
			return orderDTO(existing), nil
		}
		var duplicate model.WalletAdjustmentOrder
		e := s.db(ctx).Where("source_type = ? AND external_order_no = ?", r.SourceType, r.ExternalOrderNo).Take(&duplicate).Error
		if e == nil {
			return res.Order{}, ErrDuplicateSource
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return res.Order{}, e
		}
		return res.Order{}, ErrConflict
	}
	return orderDTO(o), err
}

// Reverse仅创建冲正申请；需另一管理员Approve后才全额反向记账。
func (s *Service) Reverse(ctx context.Context, a Actor, r req.ReverseAdjustment) (res.Order, error) {
	if _, err := s.options(); err != nil {
		return res.Order{}, err
	}
	if !requestKey.MatchString(r.RequestID) || !orderNo.MatchString(r.OrderNo) || !reasonOK(r.Reason) || !textOK(r.EvidenceRef, 0, 500) {
		return res.Order{}, ErrInvalid
	}
	digest := hash(struct {
		Kind    string
		Request req.ReverseAdjustment
	}{model.KindReverse, r})
	var o model.WalletAdjustmentOrder
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "create", "UPDATE"); e != nil {
			return e
		}
		existing, found, e := existingRequest(tx, a, r.RequestID, digest)
		if e != nil {
			return e
		}
		if found {
			o = existing
			return nil
		}
		if e = checkPending(tx, a); e != nil {
			return e
		}
		var original model.WalletAdjustmentOrder
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", r.OrderNo).Take(&original).Error; errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if e != nil {
			return e
		}
		if original.Kind != model.KindGrant || original.Status != model.StatusCredited || original.ReversalOrderNo != "" {
			return ErrConflict
		}
		now := s.now().UnixMilli()
		o = model.WalletAdjustmentOrder{OrderNo: newNo("wa_"), RequestID: r.RequestID, RequestHash: digest, Kind: model.KindReverse, AccountID: original.AccountID, AccountNo: original.AccountNo, CurrencyCode: original.CurrencyCode, Amount: original.Amount, Reason: r.Reason, EvidenceRef: r.EvidenceRef, Status: model.StatusPending, OriginalOrderNo: original.OrderNo, CreatedBy: a.ID, CreatedIP: a.IP, CreatedAt: now, UpdatedAt: now}
		o.SourceType, o.ExternalOrderNo = model.SourceReversal, o.OrderNo
		if e = tx.Create(&o).Error; e != nil {
			return e
		}
		if e = rowsOne(tx.Model(&model.WalletAdjustmentOrder{}).Where("id = ? AND reversal_order_no = '' AND status = ?", original.ID, model.StatusCredited).Updates(map[string]any{"reversal_order_no": o.OrderNo, "updated_at": now})); e != nil {
			return e
		}
		return audit(tx, a, o.OrderNo, "create_reverse", r.Reason, now)
	})
	// 并发同一幂等请求可能在原单锁处看到首个请求已经占位；事务结束后读取
	// 已提交事实并返回同一订单，而不是把安全重试误报为状态冲突。
	if err != nil && (duplicateKey(err) || errors.Is(err, ErrConflict)) {
		if existing, found, e := s.resolveRequest(ctx, a, r.RequestID, digest); e != nil {
			return res.Order{}, e
		} else if found {
			return orderDTO(existing), nil
		}
	}
	return orderDTO(o), err
}

// 固定锁顺序：管理员 → 原入账单 → 冲正单 → 账号 → 钱包 → 每日额度。
func lockedOrder(tx *gorm.DB, no string) (model.WalletAdjustmentOrder, *model.WalletAdjustmentOrder, error) {
	var o model.WalletAdjustmentOrder
	if err := tx.Where("order_no = ?", no).Take(&o).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return o, nil, ErrNotFound
	} else if err != nil {
		return o, nil, err
	}
	var original *model.WalletAdjustmentOrder
	if o.Kind == model.KindReverse {
		original = &model.WalletAdjustmentOrder{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", o.OriginalOrderNo).Take(original).Error; err != nil {
			return o, nil, err
		}
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", o.ID).Take(&o).Error
	return o, original, err
}
func (s *Service) Approve(ctx context.Context, a Actor, r req.ReviewAdjustment) (res.Order, error) {
	return s.review(ctx, a, r, true)
}
func (s *Service) Reject(ctx context.Context, a Actor, r req.ReviewAdjustment) (res.Order, error) {
	return s.review(ctx, a, r, false)
}
func (s *Service) review(ctx context.Context, a Actor, r req.ReviewAdjustment, approve bool) (res.Order, error) {
	cfg, err := s.options()
	if err != nil {
		return res.Order{}, err
	}
	if !orderNo.MatchString(r.OrderNo) || !reasonOK(r.Reason) {
		return res.Order{}, ErrInvalid
	}
	var o model.WalletAdjustmentOrder
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "review", "SHARE"); e != nil {
			return e
		}
		locked, original, e := lockedOrder(tx, r.OrderNo)
		if e != nil {
			return e
		}
		o = locked
		if o.CreatedBy == a.ID && a.ID != topSuperAdminUserID {
			return ErrSelfReview
		}
		// 相同审核人、相同决定和原因重试返回当前事实，其他请求不能冒领成功。
		if approve && (o.Status == model.StatusCredited || o.Status == model.StatusReversed) && o.ApprovedBy == a.ID && o.ReviewReason == r.Reason {
			return validateReviewReplay(tx, o)
		}
		if !approve && o.Status == model.StatusRejected && o.RejectedBy == a.ID && o.ReviewReason == r.Reason {
			return validateReviewReplay(tx, o)
		}
		if o.Status != model.StatusPending {
			return ErrConflict
		}
		now := s.now().UnixMilli()
		if !approve {
			if original != nil {
				if original.ReversalOrderNo != o.OrderNo || original.Status != model.StatusCredited {
					return ErrConflict
				}
				if e = rowsOne(tx.Model(&model.WalletAdjustmentOrder{}).Where("id = ?", original.ID).Updates(map[string]any{"reversal_order_no": "", "updated_at": now})); e != nil {
					return e
				}
			}
			if e = rowsOne(tx.Model(&model.WalletAdjustmentOrder{}).Where("id = ? AND status = ?", o.ID, model.StatusPending).Updates(map[string]any{"status": model.StatusRejected, "rejected_by": a.ID, "rejected_at": now, "review_reason": r.Reason, "reviewed_ip": a.IP, "updated_at": now})); e != nil {
				return e
			}
			if e = audit(tx, a, o.OrderNo, "reject", r.Reason, now); e != nil {
				return e
			}
		} else {
			if o.Kind == model.KindGrant {
				if o.SourceType == model.SourceLegacy {
					return ErrLegacyOrder
				}
				if !validOrderRequest(o) {
					return ErrInvariant
				}
				if o.Amount > cfg.MaxGrantAmount {
					return ErrLimit
				}
				if o.EvidenceRef == "" {
					return ErrEvidence
				}
			} else if o.Kind != model.KindReverse || !validOrderRequest(o) || original == nil || original.Status != model.StatusCredited || original.ReversalOrderNo != o.OrderNo || original.AccountID != o.AccountID || original.AccountNo != o.AccountNo || original.Amount != o.Amount || original.CurrencyCode != o.CurrencyCode {
				return ErrConflict
			}
			var w model.WalletAccount
			if o.Kind == model.KindGrant {
				w, e = openWalletForCredit(tx, o.AccountID, o.AccountNo, o.CurrencyCode, a, o.Reason, now)
			} else {
				w, e = lockWallet(tx, o.AccountID, o.CurrencyCode)
			}
			if e != nil {
				return e
			}
			delta := o.Amount
			business := model.BusinessGrant
			originalLedger := ""
			if o.Kind == model.KindReverse {
				delta = -o.Amount
				business = model.BusinessReverse
				originalLedger = original.LedgerNo
				var l model.WalletLedger
				if e = tx.Where("ledger_no = ?", originalLedger).Take(&l).Error; e != nil {
					return e
				}
				if l.WalletAccountID != w.ID || l.ChangeAmount != o.Amount || l.BusinessType != model.BusinessGrant || l.BusinessNo != original.OrderNo {
					return ErrInvariant
				}
			} else {
				if e = s.consumeQuota(tx, o, a, now); e != nil {
					return e
				}
			}
			l, e := postLedger(tx, w, ledgerPosting{Delta: delta, BusinessType: business, BusinessNo: o.OrderNo, OriginalLedgerNo: originalLedger, OperatorID: a.ID, Reason: o.Reason, Now: now})
			if e != nil {
				return e
			}
			if original != nil {
				if e = rowsOne(tx.Model(&model.WalletAdjustmentOrder{}).Where("id = ? AND status = ?", original.ID, model.StatusCredited).Updates(map[string]any{"status": model.StatusReversed, "reversed_at": now, "updated_at": now})); e != nil {
					return e
				}
			}
			if e = rowsOne(tx.Model(&model.WalletAdjustmentOrder{}).Where("id = ? AND status = ?", o.ID, model.StatusPending).Updates(map[string]any{"status": model.StatusCredited, "approved_by": a.ID, "approved_at": now, "credited_at": now, "ledger_no": l.LedgerNo, "review_reason": r.Reason, "reviewed_ip": a.IP, "updated_at": now})); e != nil {
				return e
			}
			if e = audit(tx, a, o.OrderNo, "approve_and_credit", r.Reason, now); e != nil {
				return e
			}
		}
		return tx.Where("id = ?", o.ID).Take(&o).Error
	})
	return orderDTO(o), err
}

func (s *Service) consumeQuota(tx *gorm.DB, o model.WalletAdjustmentOrder, a Actor, now int64) error {
	cfg, err := s.options()
	if err != nil {
		return err
	}
	type subject struct {
		kind  string
		id    uint64
		limit int64
	}
	subjects := []subject{{"account", o.AccountID, cfg.AccountDailyLimit}, {"operator", o.CreatedBy, cfg.OperatorDailyLimit}, {"operator", a.ID, cfg.OperatorDailyLimit}}
	sort.Slice(subjects, func(i, j int) bool {
		if subjects[i].kind == subjects[j].kind {
			return subjects[i].id < subjects[j].id
		}
		return subjects[i].kind < subjects[j].kind
	})
	day := time.UnixMilli(now).UTC().Format("2006-01-02")
	for i, sub := range subjects {
		// ID=1自审时创建人与复核人是同一个额度主体，只累计一次。
		if i > 0 && sub.kind == subjects[i-1].kind && sub.id == subjects[i-1].id {
			continue
		}
		var row model.WalletDailyQuota
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("day = ? AND subject_type = ? AND subject_id = ? AND currency_code = ?", day, sub.kind, sub.id, o.CurrencyCode)
		e := q.Take(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			row = model.WalletDailyQuota{Day: day, SubjectType: sub.kind, SubjectID: sub.id, CurrencyCode: o.CurrencyCode, UpdatedAt: now}
			if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
				return e
			}
			row = model.WalletDailyQuota{}
			e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("day = ? AND subject_type = ? AND subject_id = ? AND currency_code = ?", day, sub.kind, sub.id, o.CurrencyCode).Take(&row).Error
		}
		if e != nil {
			return e
		}
		if row.Amount < 0 || row.Amount > sub.limit-o.Amount {
			return ErrLimit
		}
		if e = rowsOne(tx.Model(&model.WalletDailyQuota{}).Where("id = ? AND amount = ?", row.ID, row.Amount).Updates(map[string]any{"amount": row.Amount + o.Amount, "updated_at": now})); e != nil {
			return e
		}
	}
	return nil
}
