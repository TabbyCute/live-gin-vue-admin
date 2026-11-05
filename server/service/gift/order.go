package gift

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	model "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	guardModel "tb_live_module/model/guard"
	live "tb_live_module/model/live"
	"tb_live_module/service/earning"
	livesvc "tb_live_module/service/live"
	"tb_live_module/service/wallet"
)

// Lock order: sender account UPDATE -> existing order -> catalog control SHARE ->
// anchor SHARE -> room SHARE -> session SHARE -> wallet's internal account/wallet locks.
// Sender lock serializes same-user idempotency without Redis; different senders remain concurrent.
func lockSender(tx *gorm.DB, id uint64) (live.LiveAccount, error) {
	var u live.LiveAccount
	e := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, id).Error
	return u, e
}
func validRoom(tx *gorm.DB, user uint64, roomNo string) (live.LiveRoom, live.LiveAnchor, live.LiveSession, error) {
	var room live.LiveRoom
	var anchor live.LiveAnchor
	var session live.LiveSession
	e := tx.Where("room_no=?", roomNo).Take(&room).Error
	if e != nil {
		return room, anchor, session, ErrState
	}
	e = tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&anchor, room.AnchorId).Error
	if e != nil {
		return room, anchor, session, ErrState
	}
	e = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("room_no=?", roomNo).Take(&room).Error
	if e != nil || room.AnchorId != anchor.ID || room.CurrentSessionId == 0 {
		return room, anchor, session, ErrState
	}
	e = tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&session, room.CurrentSessionId).Error
	if e != nil {
		return room, anchor, session, ErrState
	}
	// Reuse live/social visibility, bilateral blocks, certification and active-account checks in this transaction.
	access, e := (&livesvc.LiveAccessService{}).BatchCheckLiveAccess(tx.Statement.Context, tx, user, []string{roomNo}, true)
	if e != nil {
		return room, anchor, session, e
	}
	if len(access) != 1 {
		return room, anchor, session, ErrState
	}
	return room, anchor, session, nil
}
func (s *Service) Send(ctx context.Context, user uint64, ip, trace string, r req.Send) (model.Order, error) {
	var result model.Order
	q, e := integer(r.Quantity, false)
	if e != nil || q > 10000 || user == 0 || !keyPattern.MatchString(r.RequestID) || !numberPattern.MatchString(r.RoomNo) || !numberPattern.MatchString(r.GiftNo) {
		return result, ErrInvalid
	}
	e = s.transaction(ctx, func(tx *gorm.DB) error {
		u, e := lockSender(tx, user)
		if e != nil {
			return e
		}
		var existing model.Order
		e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("sender_user_id=? AND request_id=?", user, r.RequestID).Take(&existing).Error
		if e == nil {
			if existing.RoomNo != r.RoomNo || existing.GiftNo != r.GiftNo || existing.Quantity != q {
				return ErrConflict
			}
			result = existing
			return validateOrderFacts(tx, existing)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if u.Status != live.LiveAccountStatusNormal || u.DeletedAt.Valid {
			return ErrState
		}
		if _, e = lockControl(tx, false); e != nil {
			return e
		}
		room, anchor, session, e := validRoom(tx, user, r.RoomNo)
		if e != nil {
			return e
		}
		if anchor.UserId == user {
			return ErrSelfGift
		}
		var item model.Item
		if e = tx.Where("gift_no=? AND status='enabled'", r.GiftNo).Take(&item).Error; e != nil {
			return ErrState
		}
		var category model.Category
		if e = tx.Where("category_no=? AND status='enabled'", item.CategoryNo).Take(&category).Error; e != nil {
			return ErrState
		}
		if q > item.MaxQuantityPerOrder {
			return ErrInvalid
		}
		var guardSnapshot *guardModel.BenefitSnapshot
		if item.BenefitType == "anchor_guard" && item.BenefitStatus == "enabled" {
			if !s.GuardEnabled || s.Guard == nil || s.now() < item.EffectiveFrom || s.now() >= item.EffectiveTo {
				return ErrGuard
			}
			plan, e := s.Guard.PlanSnapshotInTx(tx, item.GuardPlanNo)
			if e != nil {
				return ErrGuard
			}
			days, e := multiply(item.GuardDurationDays, q)
			if e != nil || days > 36500000 {
				return ErrInvalid
			}
			guardSnapshot = &guardModel.BenefitSnapshot{BenefitType: "anchor_guard", GuardPlanNo: plan.PlanNo, GuardDurationDays: strconv.FormatInt(item.GuardDurationDays, 10), TotalDurationDays: strconv.FormatInt(days, 10), Status: "enabled", Plan: plan}
		}
		amount, e := multiply(item.CoinPrice, q)
		if e != nil {
			return e
		}
		now := s.now()
		rule, e := earnings.SelectInTx(tx, anchor.AnchorNo, item.GiftNo, category.CategoryNo, now)
		if e != nil {
			return e
		}
		benefit, _ := json.Marshal(map[string]any{"benefitType": item.BenefitType, "guardPlanNo": item.GuardPlanNo, "guardDurationDays": strconv.FormatInt(item.GuardDurationDays, 10), "status": item.BenefitStatus, "effectiveFrom": item.EffectiveFrom, "effectiveTo": item.EffectiveTo})
		if guardSnapshot != nil {
			benefit, _ = json.Marshal(guardSnapshot)
		}
		result = model.Order{OrderNo: no("go_"), RequestID: r.RequestID, SenderUserID: user, SenderAccountNo: u.AccountNo, AnchorID: uint64(anchor.ID), AnchorNo: anchor.AnchorNo, RoomID: uint64(room.ID), RoomNo: room.RoomNo, SessionID: uint64(session.ID), SessionNo: session.SessionNo, CategoryID: category.ID, CategoryNo: category.CategoryNo, CategoryName: category.Name, GiftID: item.ID, GiftNo: item.GiftNo, GiftName: item.Name, IconURL: item.IconURL, AnimationURL: item.AnimationURL, AnimationType: item.AnimationType, UnitPrice: item.CoinPrice, Quantity: q, GrossCoinAmount: amount, RevenueRuleNo: rule.RuleNo, AnchorShareBPS: rule.AnchorShareBPS, EstimatedAnchorEarningCoin: earning.Share(amount, rule.AnchorShareBPS), BenefitSnapshot: string(benefit), Status: "succeeded", ActorIP: ip, TraceID: trace, CreatedAt: now}
		result.EstimatedPlatformIncomeCoin = amount - result.EstimatedAnchorEarningCoin
		receipt, e := s.wallet().DebitInTx(tx, wallet.MovementInput{AccountID: user, Amount: strconv.FormatInt(amount, 10), BusinessType: "gift", BusinessNo: result.OrderNo, ActorType: "user", ActorID: strconv.FormatUint(user, 10), ActorIP: ip, TraceID: trace, Reason: "直播礼物消费"})
		if e != nil {
			return e
		}
		result.WalletTransactionNo = receipt.TransactionNo
		result.WalletLedgerNo = receipt.LedgerNo
		if e = tx.Create(&result).Error; e != nil {
			return e
		}
		if e = tx.Create(&model.Revenue{RevenueNo: no("gr_"), GiftOrderNo: result.OrderNo, BusinessNo: result.OrderNo, EntryType: "gift", GrossCoinAmount: amount, Quantity: q, CreatedAt: now}).Error; e != nil {
			return e
		}
		if _, e = earnings.AppendInTx(tx, result.AnchorID, result.OrderNo, result.OrderNo, rule.RuleNo, rule.AnchorShareBPS, result.EstimatedAnchorEarningCoin, amount, now, false); e != nil {
			return e
		}
		// A disabled configured rule is snapshotted but is never promised or granted.
		if guardSnapshot != nil {
			if e = tx.Create(&model.BenefitGrant{GrantNo: no("gg_"), GiftOrderNo: result.OrderNo, BenefitType: "anchor_guard", BenefitSnapshot: string(benefit), ReceiverUserID: user, AnchorID: result.AnchorID, Status: "pending", CreatedAt: now, UpdatedAt: now}).Error; e != nil {
				return e
			}
		}
		payload, _ := json.Marshal(map[string]any{"eventId": result.OrderNo, "orderNo": result.OrderNo, "roomNo": room.RoomNo, "sessionNo": session.SessionNo, "sender": map[string]string{"accountNo": u.AccountNo, "nickname": u.Nickname, "avatar": u.Avatar}, "gift": map[string]string{"giftNo": item.GiftNo, "name": item.Name, "iconUrl": item.IconURL, "animationUrl": item.AnimationURL, "animationType": item.AnimationType}, "quantity": r.Quantity})
		return tx.Create(&model.Event{EventID: result.OrderNo, EventType: "gift.sent", GiftOrderNo: result.OrderNo, RoomNo: room.RoomNo, SessionNo: session.SessionNo, Payload: string(payload), DeliveryStatus: "pending", CreatedAt: now}).Error
	})
	if e != nil {
		return model.Order{}, e
	}
	return result, nil
}
func (s *Service) Refund(ctx context.Context, a Actor, r req.Refund) (model.Refund, error) {
	var result model.Refund
	q, e := integer(r.Quantity, false)
	if e != nil || q > 10000 || !keyPattern.MatchString(r.RequestID) || !numberPattern.MatchString(r.OrderNo) || !textOK(r.Reason, 500) {
		return result, ErrInvalid
	}
	e = s.transaction(ctx, func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "refund", "POST"); e != nil {
			return e
		}
		var order model.Order
		if e := tx.Where("order_no=?", r.OrderNo).Take(&order).Error; e != nil {
			return ErrNotFound
		}
		if _, e := lockSender(tx, order.SenderUserID); e != nil {
			return e
		}
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no=?", r.OrderNo).Take(&order).Error; e != nil {
			return e
		}
		e := tx.Where("request_id=?", r.RequestID).Take(&result).Error
		if e == nil {
			if result.GiftOrderNo != r.OrderNo || result.RefundQuantity != q || result.Reason != r.Reason || result.OperatorID != a.ID {
				return ErrConflict
			}
			return validateOrderFacts(tx, order)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e := validateOrderFacts(tx, order); e != nil {
			return e
		}
		amount, e := multiply(order.UnitPrice, q)
		if e != nil {
			return e
		}
		if amount > order.GrossCoinAmount-order.RefundedCoinAmount {
			return ErrRefund
		}
		now := s.now()
		result = model.Refund{RefundNo: no("gf_"), RequestID: r.RequestID, GiftOrderNo: order.OrderNo, RefundQuantity: q, RefundCoinAmount: amount, Reason: r.Reason, OperatorID: a.ID, BenefitCompensationStatus: "not_required", Status: "succeeded", CreatedAt: now}
		receipt, e := s.wallet().RefundInTx(tx, wallet.MovementInput{AccountID: order.SenderUserID, Amount: strconv.FormatInt(amount, 10), BusinessType: "gift", BusinessNo: result.RefundNo, OriginalNo: order.WalletTransactionNo, ActorType: "admin", ActorID: strconv.FormatUint(a.ID, 10), ActorIP: a.IP, TraceID: no("gt_"), Reason: r.Reason})
		if e != nil {
			return e
		}
		result.WalletRefundTransactionNo = receipt.TransactionNo
		result.WalletRefundLedgerNo = receipt.LedgerNo
		// Cumulative rounding: split refunds reverse exactly the original earning on full refund.
		reversal := earning.Share(order.GrossCoinAmount-order.RefundedCoinAmount, order.AnchorShareBPS) - earning.Share(order.GrossCoinAmount-order.RefundedCoinAmount-amount, order.AnchorShareBPS)
		result.EarningReversalNo, e = earnings.AppendInTx(tx, order.AnchorID, order.OrderNo, result.RefundNo, order.RevenueRuleNo, order.AnchorShareBPS, -reversal, -amount, now, true)
		if e != nil {
			return e
		}
		if e = tx.Create(&model.Revenue{RevenueNo: no("gr_"), GiftOrderNo: order.OrderNo, BusinessNo: result.RefundNo, EntryType: "refund", GrossCoinAmount: -amount, Quantity: -q, CreatedAt: now}).Error; e != nil {
			return e
		}
		var grants []model.BenefitGrant
		if e = tx.Where("gift_order_no=?", order.OrderNo).Find(&grants).Error; e != nil {
			return e
		}
		for _, g := range grants {
			if g.ReceiverUserID != order.SenderUserID || g.AnchorID != order.AnchorID || g.BenefitType != "anchor_guard" {
				return ErrInvariant
			}
			var snapshot struct {
				GuardDurationDays string `json:"guardDurationDays"`
			}
			if json.Unmarshal([]byte(g.BenefitSnapshot), &snapshot) != nil {
				return ErrInvariant
			}
			days, e := integer(snapshot.GuardDurationDays, false)
			if e != nil {
				return ErrInvariant
			}
			duration, e := multiply(days, q)
			if e != nil {
				return e
			}
			if e = tx.Create(&model.BenefitCompensation{CompensationNo: no("gc_"), RefundNo: result.RefundNo, GiftOrderNo: order.OrderNo, ReceiverUserID: order.SenderUserID, AnchorID: order.AnchorID, DurationDays: duration, Status: "pending", CreatedAt: now}).Error; e != nil {
				return e
			}
			if e = tx.Model(&model.BenefitGrant{}).Where("id=?", g.ID).Updates(map[string]any{"status": "compensating", "updated_at": now}).Error; e != nil {
				return e
			}
			result.BenefitCompensationStatus = "pending"
		}
		if e = tx.Create(&result).Error; e != nil {
			return e
		}
		status := "partial_refund"
		if order.RefundedCoinAmount+amount == order.GrossCoinAmount {
			status = "refunded"
		}
		if e = tx.Model(&model.Order{}).Where("id=?", order.ID).Updates(map[string]any{"refunded_coin_amount": order.RefundedCoinAmount + amount, "status": status}).Error; e != nil {
			return e
		}
		return audit(tx, a, result.RefundNo, "refund", r.Reason, now)
	})
	if e != nil {
		return model.Refund{}, e
	}
	return result, nil
}
