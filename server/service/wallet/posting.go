package wallet

import (
	"math"

	"gorm.io/gorm"
	model "tb_live_module/model/wallet"
)

// ledgerPosting 只用于已经核实业务事实并持有钱包行锁的内部事务。
// 调用方负责订单/支付渠道鉴权、业务状态、限额与审计，并与记账一起提交。
// 不提供可由HTTP调用的任意增减余额接口；未来支付须先建立独立支付订单。
type ledgerPosting struct {
	Delta                                      int64
	BusinessType, BusinessNo, OriginalLedgerNo string
	OperatorID                                 uint64
	Reason                                     string
	Now                                        int64
}

func postLedger(tx *gorm.DB, w model.WalletAccount, p ledgerPosting) (model.WalletLedger, error) {
	var l model.WalletLedger
	if tx == nil || tx.Statement == nil {
		return l, ErrInvariant
	}
	if _, inTransaction := tx.Statement.ConnPool.(gorm.TxCommitter); !inTransaction {
		return l, ErrInvariant // 禁止意外传入普通DB，使余额更新先于流水独立提交。
	}
	if w.Status != model.AccountNormal {
		return l, ErrDisabled
	}
	if w.ID == 0 || w.FrozenBalance != 0 || w.AvailableBalance < 0 || w.Version < 0 || w.Version == math.MaxInt64 || p.Delta == 0 || p.Delta == math.MinInt64 || !textOK(p.BusinessNo, 1, 40) || !textOK(p.BusinessType, 1, 32) || !textOK(p.OriginalLedgerNo, 0, 40) || p.OperatorID == 0 || p.Now <= 0 || !reasonOK(p.Reason) {
		return l, ErrInvariant
	}
	if err := validateWalletTail(tx, w); err != nil {
		return l, err
	}
	if p.Delta > 0 && w.AvailableBalance > math.MaxInt64-p.Delta {
		return l, ErrLimit
	}
	if p.Delta < 0 && w.AvailableBalance < -p.Delta {
		return l, ErrInsufficient
	}
	// 即使未来调用者漏锁，版本/旧余额条件及两个唯一键仍阻止覆盖和重复记账。
	if err := rowsOne(tx.Model(&model.WalletAccount{}).Where("id = ? AND version = ? AND available_balance = ? AND frozen_balance = 0 AND status = ?", w.ID, w.Version, w.AvailableBalance, model.AccountNormal).
		Updates(map[string]any{"available_balance": w.AvailableBalance + p.Delta, "version": w.Version + 1, "updated_at": p.Now})); err != nil {
		return l, err
	}
	l = model.WalletLedger{LedgerNo: newNo("wl_"), WalletAccountID: w.ID, WalletVersion: w.Version + 1, ChangeAmount: p.Delta,
		BalanceBefore: w.AvailableBalance, BalanceAfter: w.AvailableBalance + p.Delta, BusinessType: p.BusinessType,
		BusinessNo: p.BusinessNo, OriginalLedgerNo: p.OriginalLedgerNo, OperatorID: p.OperatorID, Reason: p.Reason, CreatedAt: p.Now}
	if err := tx.Create(&l).Error; err != nil {
		return model.WalletLedger{}, err
	}
	return l, nil
}
