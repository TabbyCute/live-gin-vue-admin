package wallet

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/wallet"
)

// MovementInput 是可信业务服务的内部契约，禁止直接绑定HTTP请求。
// AccountID来自服务端身份/业务订单，Amount来自服务端核价；BusinessNo是持久化业务单号。
// 相同(kind,businessType,businessNo)永久只能记账一次。IP/TraceID重试可变化，保留首次值。
type MovementInput struct {
	AccountID                                                  uint64
	CurrencyCode, Amount, BusinessType, BusinessNo, OriginalNo string
	ActorType, ActorID, ActorIP, TraceID, Reason               string
}

type MovementResult struct {
	TransactionNo string `json:"transactionNo"`
	LedgerNo      string `json:"ledgerNo"`
	Amount        string `json:"amount"`
	BalanceAfter  string `json:"balanceAfter"`
	WalletVersion string `json:"walletVersion"`
}

var businessNamespace = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
var businessKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}$`)
var transactionNo = regexp.MustCompile(`^wt_[a-f0-9]{32}$`)

// 限制一笔消费的退款拆分数量，使核查累计额和追踪有明确资源上限。
const maxRefundParts = 1000

func movementHash(r MovementInput, kind string) string {
	r.ActorIP, r.TraceID = "", ""
	return hash(struct {
		Kind    string
		Request MovementInput
	}{kind, r})
}
func movementInput(t model.WalletTransaction) MovementInput {
	return MovementInput{AccountID: t.AccountID, CurrencyCode: t.CurrencyCode, Amount: strconv.FormatInt(t.Amount, 10), BusinessType: t.BusinessType, BusinessNo: t.BusinessNo, OriginalNo: t.OriginalNo, ActorType: t.ActorType, ActorID: t.ActorID, ActorIP: t.ActorIP, TraceID: t.TraceID, Reason: t.Reason}
}
func validateMovement(r MovementInput, kind string) (MovementInput, int64, error) {
	code, e := currency(r.CurrencyCode)
	if e != nil {
		return r, 0, e
	}
	r.CurrencyCode = code
	amount, e := parseAmount(r.Amount)
	if e != nil || r.AccountID == 0 || !businessNamespace.MatchString(r.BusinessType) || !businessKey.MatchString(r.BusinessNo) || !reasonOK(r.Reason) || !textOK(r.TraceID, 1, 64) || net.ParseIP(r.ActorIP) == nil {
		return r, 0, ErrInvalid
	}
	if kind == model.TransactionDebit && r.OriginalNo != "" || kind == model.TransactionRefund && !transactionNo.MatchString(r.OriginalNo) {
		return r, 0, ErrInvalid
	}
	switch r.ActorType {
	case "user", "admin":
		id, e := strconv.ParseUint(r.ActorID, 10, 64)
		if e != nil || id == 0 || strconv.FormatUint(id, 10) != r.ActorID || (r.ActorType == "user" && id != r.AccountID) {
			return r, 0, ErrInvalid
		}
	case "system":
		if !businessNamespace.MatchString(r.ActorID) {
			return r, 0, ErrInvalid
		}
	default:
		return r, 0, ErrInvalid
	}
	return r, amount, nil
}

func (s *Service) Debit(ctx context.Context, r MovementInput) (MovementResult, error) {
	return s.move(ctx, r, model.TransactionDebit)
}
func (s *Service) Refund(ctx context.Context, r MovementInput) (MovementResult, error) {
	return s.move(ctx, r, model.TransactionRefund)
}
func (s *Service) move(ctx context.Context, r MovementInput, kind string) (MovementResult, error) {
	if _, e := s.options(); e != nil {
		return MovementResult{}, e
	}
	var result MovementResult
	e := s.transaction(ctx, func(tx *gorm.DB) error { var err error; result, err = s.movementInTx(tx, r, kind); return err })
	if e != nil {
		return MovementResult{}, e
	}
	return result, nil
}

// InTx不会提交、不会自行重试。调用者必须将业务订单/权益与记账一起提交；
// 任何错误必须回滚整个事务，不得忽略后继续提交。返回结果只在外层Commit成功后有效。
func (s *Service) DebitInTx(tx *gorm.DB, r MovementInput) (MovementResult, error) {
	return s.movementInTx(tx, r, model.TransactionDebit)
}
func (s *Service) RefundInTx(tx *gorm.DB, r MovementInput) (MovementResult, error) {
	return s.movementInTx(tx, r, model.TransactionRefund)
}

func (s *Service) movementInTx(tx *gorm.DB, r MovementInput, kind string) (MovementResult, error) {
	var zero MovementResult
	if tx == nil || tx.Statement == nil {
		return zero, ErrInvariant
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return zero, ErrInvariant
	}
	// 外层业务事务可能已在REPEATABLE READ中建立旧快照。所有账务查询必须
	// 使用当前读，否则钱包锁虽取得最新余额，幂等/原单/尾流水仍可能读到旧值。
	// Session使每次查询克隆条件；钱包锁会将SHARE覆盖成UPDATE。
	tx = tx.Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
	r, amount, err := validateMovement(r, kind)
	if err != nil {
		return zero, err
	}
	// 统一锁顺序：用户共享锁→钱包排他锁→本钱包交易。所有写入均由钱包锁串行。
	// 退款允许注销/禁用用户收回原消费款，但不会为不存在的钱包自动开户。
	user, err := findAccount(tx, "", r.AccountID, false, true)
	if err != nil {
		return zero, err
	}
	w, err := lockWallet(tx, r.AccountID, r.CurrencyCode)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return zero, ErrInsufficient
	}
	if err != nil {
		return zero, err
	}
	var existing model.WalletTransaction
	err = tx.Where("kind = ? AND business_type = ? AND business_no = ?", kind, r.BusinessType, r.BusinessNo).Take(&existing).Error
	if err == nil {
		if existing.RequestHash != movementHash(r, kind) || existing.AccountID != r.AccountID {
			return zero, ErrConflict
		}
		l, e := validateTransaction(tx, existing, w)
		if e != nil {
			return zero, e
		}
		return movementResult(existing, l), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return zero, err
	}
	if kind == model.TransactionDebit {
		if _, err = findAccount(tx, "", r.AccountID, true, false); err != nil {
			return zero, err
		}
	}
	var original model.WalletTransaction
	originalLedger := ""
	if kind == model.TransactionRefund {
		err = tx.Where("transaction_no = ?", r.OriginalNo).Take(&original).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return zero, ErrNotFound
		}
		if err != nil {
			return zero, err
		}
		if original.Kind != model.TransactionDebit || original.AccountID != r.AccountID || original.CurrencyCode != r.CurrencyCode || original.BusinessType != r.BusinessType {
			return zero, ErrConflict
		}
		if _, err = validateTransaction(tx, original, w); err != nil {
			return zero, err
		}
		if amount > original.Amount-original.RefundedAmount {
			return zero, ErrRefundLimit
		}
		var parts int64
		if err = tx.Model(&model.WalletTransaction{}).Where("original_no = ?", original.TransactionNo).Count(&parts).Error; err != nil {
			return zero, err
		}
		if parts >= maxRefundParts {
			return zero, ErrLimit
		}
		originalLedger = original.LedgerNo
	}
	now := s.now().UnixMilli()
	t := model.WalletTransaction{TransactionNo: newNo("wt_"), Kind: kind, BusinessType: r.BusinessType, BusinessNo: r.BusinessNo, AccountID: r.AccountID, AccountNo: user.AccountNo, CurrencyCode: r.CurrencyCode, Amount: amount, OriginalNo: r.OriginalNo, RequestHash: movementHash(r, kind), ActorType: r.ActorType, ActorID: r.ActorID, ActorIP: r.ActorIP, TraceID: r.TraceID, Reason: r.Reason, CreatedAt: now}
	delta, business := -amount, model.BusinessDebit
	if kind == model.TransactionRefund {
		delta, business = amount, model.BusinessRefund
	}
	operator := uint64(0)
	if r.ActorType == "admin" {
		operator, _ = strconv.ParseUint(r.ActorID, 10, 64)
	}
	l, err := postLedger(tx, w, ledgerPosting{Delta: delta, BusinessType: business, BusinessNo: t.TransactionNo, OriginalLedgerNo: originalLedger, OperatorID: operator, Reason: r.Reason, Now: now})
	if err != nil {
		return zero, err
	}
	t.LedgerNo = l.LedgerNo
	if err = tx.Create(&t).Error; err != nil {
		if duplicateKey(err) {
			return zero, ErrConflict
		}
		return zero, err
	}
	if kind == model.TransactionRefund {
		err = rowsOne(tx.Model(&model.WalletTransaction{}).Where("id = ? AND refunded_amount = ?", original.ID, original.RefundedAmount).Update("refunded_amount", original.RefundedAmount+amount))
		if err != nil {
			return zero, err
		}
	}
	if err = audit(tx, Actor{ID: operator, IP: r.ActorIP}, t.TransactionNo, kind, r.Reason, now); err != nil {
		return zero, err
	}
	return movementResult(t, l), nil
}
func movementResult(t model.WalletTransaction, l model.WalletLedger) MovementResult {
	return MovementResult{TransactionNo: t.TransactionNo, LedgerNo: l.LedgerNo, Amount: strconv.FormatInt(t.Amount, 10), BalanceAfter: strconv.FormatInt(l.BalanceAfter, 10), WalletVersion: strconv.FormatInt(l.WalletVersion, 10)}
}

func validTransactionLedger(t model.WalletTransaction, l model.WalletLedger, w model.WalletAccount) bool {
	r, _, e := validateMovement(movementInput(t), t.Kind)
	if e != nil || (t.Kind != model.TransactionDebit && t.Kind != model.TransactionRefund) || t.RequestHash != movementHash(r, t.Kind) || t.AccountID != w.AccountID || t.CurrencyCode != w.CurrencyCode || t.RefundedAmount < 0 || t.RefundedAmount > t.Amount {
		return false
	}
	delta, business := -t.Amount, model.BusinessDebit
	if t.Kind == model.TransactionRefund {
		delta, business = t.Amount, model.BusinessRefund
		if t.RefundedAmount != 0 {
			return false
		}
	}
	operator := uint64(0)
	if t.ActorType == "admin" {
		operator, _ = strconv.ParseUint(t.ActorID, 10, 64)
	}
	return l.LedgerNo == t.LedgerNo && l.WalletAccountID == w.ID && l.BusinessNo == t.TransactionNo && l.BusinessType == business && l.ChangeAmount == delta && l.OperatorID == operator && l.CreatedAt == t.CreatedAt && l.Reason == t.Reason && l.WalletVersion > 0 && l.WalletVersion <= w.Version && l.BalanceBefore >= 0 && l.BalanceAfter >= 0 && ((delta > 0 && l.BalanceAfter >= delta && l.BalanceAfter-delta == l.BalanceBefore) || (delta < 0 && l.BalanceBefore >= -delta && l.BalanceBefore+delta == l.BalanceAfter))
}
func validateTransaction(tx *gorm.DB, t model.WalletTransaction, w model.WalletAccount) (model.WalletLedger, error) {
	var l model.WalletLedger
	if e := tx.Where("ledger_no = ?", t.LedgerNo).Take(&l).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return l, ErrInvariant
		}
		return l, e
	}
	if !validTransactionLedger(t, l, w) {
		return l, ErrInvariant
	}
	var logs []model.WalletAuditLog
	if e := tx.Where("object_no = ?", t.TransactionNo).Limit(2).Find(&logs).Error; e != nil {
		return l, e
	}
	if len(logs) != 1 || logs[0].Action != t.Kind || logs[0].OperatorID != l.OperatorID || logs[0].OperatorIP != t.ActorIP || logs[0].CreatedAt != t.CreatedAt || logs[0].Reason != t.Reason {
		return l, ErrInvariant
	}
	if t.Kind == model.TransactionDebit {
		var total int64
		if e := tx.Model(&model.WalletTransaction{}).Where("original_no = ? AND kind = ?", t.TransactionNo, model.TransactionRefund).Select("COALESCE(SUM(amount),0)").Scan(&total).Error; e != nil {
			return l, e
		}
		if total != t.RefundedAmount || l.OriginalLedgerNo != "" {
			return l, ErrInvariant
		}
	} else {
		var original model.WalletTransaction
		if e := tx.Where("transaction_no = ?", t.OriginalNo).Take(&original).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return l, ErrInvariant
			}
			return l, e
		}
		if original.Kind != model.TransactionDebit || original.AccountID != t.AccountID || original.CurrencyCode != t.CurrencyCode || original.BusinessType != t.BusinessType || original.LedgerNo != l.OriginalLedgerNo || original.RefundedAmount < t.Amount {
			return l, ErrInvariant
		}
		parent, e := validateTransaction(tx, original, w)
		if e != nil {
			return l, e
		}
		if parent.WalletVersion >= l.WalletVersion {
			return l, ErrInvariant
		}
	}
	return l, nil
}
