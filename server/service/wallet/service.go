package wallet

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"tb_live_module/config"
	live "tb_live_module/model/live"
	"tb_live_module/model/system"
	model "tb_live_module/model/wallet"
	res "tb_live_module/model/wallet/response"
	"tb_live_module/utils"
)

type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string { return e.Message }

var (
	ErrInvalid         = &BusinessError{47001, "钱包参数无效，请检查整数金额、编号和原因"}
	ErrForbidden       = &BusinessError{47002, "无钱包操作权限或管理员已失效"}
	ErrAccount         = &BusinessError{47003, "账号不存在或当前不可入账"}
	ErrNotFound        = &BusinessError{47004, "入账单不存在"}
	ErrConflict        = &BusinessError{47005, "订单状态冲突或幂等键已用于其他请求"}
	ErrSelfReview      = &BusinessError{47006, "除顶级超级管理员外，申请人不能审核自己的订单"}
	ErrLimit           = &BusinessError{47007, "超过单笔、每日或待审核数量限制"}
	ErrEvidence        = &BusinessError{47008, "人工充值必须提供资金凭证引用"}
	ErrDisabled        = &BusinessError{47009, "钱包已停用"}
	ErrInsufficient    = &BusinessError{47010, "余额不足，不能执行冲正"}
	ErrInvariant       = &BusinessError{47011, "账务校验异常，请联系管理员核查"}
	ErrUnavailable     = &BusinessError{47012, "钱包服务暂不可用，请使用原请求编号重试"}
	ErrDuplicateSource = &BusinessError{47013, "该资金凭证已有申请，请按凭证号查询原单，禁止重复充值"}
	ErrWalletNotFound  = &BusinessError{47014, "钱包尚未开户，不能设置状态"}
	ErrLegacyOrder     = &BusinessError{47015, "历史申请缺少资金凭证标识，禁止入账，请核查后驳回"}
)

// topSuperAdminUserID 是系统初始化的固定顶级超级管理员账号。
// 该账号不受钱包角色列表和自审限制；启用状态、订单状态、额度和账务校验仍全部执行。
const topSuperAdminUserID uint64 = 1

type Actor struct {
	ID          uint64
	AuthorityID uint
	IP          string
}
type Service struct {
	DB     *gorm.DB
	Config config.Wallet
	Log    *zap.Logger
	Now    func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) options() (config.Wallet, error) {
	if s.DB == nil {
		return config.Wallet{}, ErrUnavailable
	}
	return s.Config.Normalize()
}
func (s *Service) db(ctx context.Context) *gorm.DB { return s.DB.WithContext(ctx) }

// 每批查询使用同一个MVCC快照，避免普通RC多次SELECT读到不同提交状态，
// 同时不持有资金行锁；对账和详情不能让充值等待读操作结束。
func (s *Service) snapshot(ctx context.Context, fn func(*gorm.DB) error) error {
	return s.db(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

// 仅在数据库明确回滚的死锁/锁超时重试。断网或Commit结果不明不能盲目重放。
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		err := s.db(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var me *driver.MySQLError
		if attempt >= 3 || !errors.As(err, &me) || (me.Number != 1213 && me.Number != 1205) {
			return err
		}
		if s.Log != nil {
			s.Log.Warn("wallet transaction retry", zap.Uint16("mysqlCode", me.Number), zap.Int("attempt", attempt+1))
		}
		timer := time.NewTimer(time.Duration((1<<attempt)*10+rand.IntN(15)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

var publicNo = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
var requestKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var orderNo = regexp.MustCompile(`^wa_[a-f0-9]{32}$`)
var positiveInteger = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
var externalNo = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._:/-]{0,127}$`)

// 人工凭证号统一大写ASCII，避免大小写、Unicode和空白造成意外重复。
func normalizeExternalNo(v string) (string, error) {
	for i := range v {
		if v[i] >= 128 {
			return "", ErrInvalid
		}
	}
	v = strings.ToUpper(v)
	if !externalNo.MatchString(v) {
		return "", ErrInvalid
	}
	return v, nil
}

func duplicateKey(err error) bool {
	var me *driver.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062
	}
	var se interface{ Code() int }
	return errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &se) && (se.Code() == 2067 || se.Code() == 1555))
}

func currency(v string) (string, error) {
	if v == "" {
		v = model.CurrencyCOIN
	}
	if v != model.CurrencyCOIN {
		return "", ErrInvalid
	}
	return v, nil
}
func reasonOK(v string) bool { return textOK(v, 3, 500) }
func textOK(v string, min, max int) bool {
	if !utf8.ValidString(v) || strings.TrimSpace(v) != v || utf8.RuneCountInString(v) < min || utf8.RuneCountInString(v) > max {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func parseAmount(v string) (int64, error) {
	if !positiveInteger.MatchString(v) {
		return 0, ErrInvalid
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, ErrInvalid
	}
	return n, nil
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func newNo(prefix string) string { return prefix + strings.ReplaceAll(uuid.NewString(), "-", "") }
func contains(ids []uint, id uint) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// 不信任JWT内长期缓存的权限；核验当前管理员行和当前角色归属。写事务锁住该行直到提交。
// 固定ID=1是顶级超级管理员，不受钱包角色列表限制，但仍必须存在、启用且来自合法IP。
func (s *Service) authorize(db *gorm.DB, a Actor, action, lock string) error {
	cfg, err := s.options()
	if err != nil {
		return err
	}
	if a.ID == 0 || (a.ID != topSuperAdminUserID && a.AuthorityID == 0) || net.ParseIP(a.IP) == nil {
		return ErrForbidden
	}
	allowed := cfg.ReadAuthorities
	if action == "create" {
		allowed = cfg.CreateAuthorities
	} else if action == "review" {
		allowed = cfg.ReviewAuthorities
	} else if action == "read" {
		allowed = append(append(append([]uint{}, allowed...), cfg.CreateAuthorities...), cfg.ReviewAuthorities...)
	} else {
		return ErrForbidden
	}
	if a.ID != topSuperAdminUserID && !contains(allowed, a.AuthorityID) {
		return ErrForbidden
	}
	q := db.Model(&system.SysUser{}).Select("id", "enable", "authority_id").Where("id = ?", a.ID)
	if lock != "" {
		q = q.Clauses(clause.Locking{Strength: lock})
	}
	var u system.SysUser
	if err = q.Take(&u).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrForbidden
	} else if err != nil {
		return err
	}
	if u.Enable != 1 {
		return ErrForbidden
	}
	if a.ID == topSuperAdminUserID {
		return nil
	}
	if u.AuthorityId != a.AuthorityID {
		var membership system.SysUserAuthority
		q = db.Where("sys_user_id = ? AND sys_authority_authority_id = ?", a.ID, a.AuthorityID)
		if lock != "" {
			q = q.Clauses(clause.Locking{Strength: "SHARE"})
		}
		if err = q.Take(&membership).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrForbidden
		} else if err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Authorize(ctx context.Context, a Actor, action string) error {
	if _, err := s.options(); err != nil {
		return err
	}
	return s.authorize(s.db(ctx), a, action, "")
}

func findAccount(db *gorm.DB, no string, id uint64, active bool, lock bool) (live.LiveAccount, error) {
	var a live.LiveAccount
	q := db.Unscoped().Select("id", "account_no", "phone", "status", "deleted_at")
	if no != "" {
		q = q.Where("account_no = ?", no)
	} else {
		q = q.Where("id = ?", id)
	}
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if err := q.Take(&a).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return a, ErrAccount
	} else if err != nil {
		return a, err
	}
	if active && (a.DeletedAt.Valid || a.Status != live.LiveAccountStatusNormal) {
		return a, ErrAccount
	}
	return a, nil
}

// findAdminAccount 允许后台使用公开账号、E.164手机号或两者交叉确认定位用户。
// 手机号只参与精确查询，账务事实仍只保存稳定的内部用户ID和公开账号快照。
func findAdminAccount(db *gorm.DB, accountNo, phone string, active bool, lock bool) (live.LiveAccount, error) {
	accountNo = strings.TrimSpace(accountNo)
	phone = strings.TrimSpace(phone)
	if accountNo == "" && phone == "" {
		return live.LiveAccount{}, ErrInvalid
	}
	if accountNo != "" && !publicNo.MatchString(accountNo) {
		return live.LiveAccount{}, ErrInvalid
	}
	if phone != "" {
		var err error
		phone, err = utils.NormalizeAccountPhone(phone)
		if err != nil {
			return live.LiveAccount{}, ErrInvalid
		}
	}
	var a live.LiveAccount
	q := db.Unscoped().Select("id", "account_no", "phone", "status", "deleted_at")
	if accountNo != "" {
		q = q.Where("account_no = ?", accountNo)
	}
	if phone != "" {
		q = q.Where("phone = ?", phone)
	}
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if err := q.Take(&a).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return a, ErrAccount
	} else if err != nil {
		return a, err
	}
	if active && (a.DeletedAt.Valid || a.Status != live.LiveAccountStatusNormal) {
		return a, ErrAccount
	}
	return a, nil
}
func lockWallet(tx *gorm.DB, accountID uint64, code string) (model.WalletAccount, error) {
	var w model.WalletAccount
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("account_id = ? AND currency_code = ?", accountID, code).Take(&w).Error
	return w, err
}

// 只允许已验证充值业务的事务调用；用户共享锁阻止开户期间注销/删除。
// 普通INSERT让唯一键仲裁并发开户，不依赖clientFoundRows的upsert行数语义。
func openWalletForCredit(tx *gorm.DB, accountID uint64, accountNo, code string, a Actor, reason string, now int64) (model.WalletAccount, error) {
	user, err := findAccount(tx, "", accountID, true, true)
	if err != nil {
		return model.WalletAccount{}, err
	}
	if user.AccountNo != accountNo {
		return model.WalletAccount{}, ErrInvariant
	}
	w, err := lockWallet(tx, accountID, code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		w = model.WalletAccount{AccountID: accountID, CurrencyCode: code, Status: model.AccountNormal, CreatedAt: now, UpdatedAt: now}
		err = tx.Create(&w).Error
		if duplicateKey(err) {
			return lockWallet(tx, accountID, code)
		}
		if err != nil {
			return w, err
		}
		err = audit(tx, a, fmt.Sprintf("wallet:%d", w.ID), "wallet_open", reason, now)
	}
	return w, err
}
func audit(tx *gorm.DB, a Actor, object, action, reason string, now int64) error {
	return tx.Create(&model.WalletAuditLog{ObjectNo: object, Action: action, OperatorID: a.ID, OperatorIP: a.IP, Reason: reason, CreatedAt: now}).Error
}
func balanceDTO(w model.WalletAccount) res.Balance {
	return res.Balance{CurrencyCode: w.CurrencyCode, AvailableBalance: strconv.FormatInt(w.AvailableBalance, 10), FrozenBalance: strconv.FormatInt(w.FrozenBalance, 10), Version: strconv.FormatInt(w.Version, 10), Status: w.Status, UpdatedAt: w.UpdatedAt}
}
func ledgerDTO(l model.WalletLedger, code string, admin bool) res.Ledger {
	r := res.Ledger{LedgerNo: l.LedgerNo, CurrencyCode: code, WalletVersion: strconv.FormatInt(l.WalletVersion, 10), ChangeAmount: strconv.FormatInt(l.ChangeAmount, 10), BalanceBefore: strconv.FormatInt(l.BalanceBefore, 10), BalanceAfter: strconv.FormatInt(l.BalanceAfter, 10), BusinessType: l.BusinessType, BusinessNo: l.BusinessNo, OriginalLedgerNo: l.OriginalLedgerNo, CreatedAt: l.CreatedAt}
	if admin {
		r.OperatorID = strconv.FormatUint(l.OperatorID, 10)
		r.Reason = l.Reason
	}
	return r
}
func optionalID(v uint64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatUint(v, 10)
}
func orderDTO(o model.WalletAdjustmentOrder) res.Order {
	return res.Order{OrderNo: o.OrderNo, RequestID: o.RequestID, SourceType: o.SourceType, ExternalOrderNo: o.ExternalOrderNo, Kind: o.Kind, AccountNo: o.AccountNo, CurrencyCode: o.CurrencyCode, Amount: strconv.FormatInt(o.Amount, 10), Reason: o.Reason, EvidenceRef: o.EvidenceRef, Status: o.Status, OriginalOrderNo: o.OriginalOrderNo, ReversalOrderNo: o.ReversalOrderNo, LedgerNo: o.LedgerNo, CreatedBy: strconv.FormatUint(o.CreatedBy, 10), ApprovedBy: optionalID(o.ApprovedBy), RejectedBy: optionalID(o.RejectedBy), ReviewReason: o.ReviewReason, CreatedIP: o.CreatedIP, ReviewedIP: o.ReviewedIP, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt, ApprovedAt: o.ApprovedAt, CreditedAt: o.CreditedAt, RejectedAt: o.RejectedAt, ReversedAt: o.ReversedAt}
}

func adminDisplay(nickname string, id uint64) string {
	if id == 0 {
		return ""
	}
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		nickname = "管理员"
	}
	return fmt.Sprintf("%s [ %d ]", nickname, id)
}

// adminDisplays 一次批量读取当前及已软删除管理员，避免流水/订单列表产生N+1查询。
// 若历史管理员已被物理删除，仍返回带ID的明确占位，不能把审计主体显示为空。
func adminDisplays(db *gorm.DB, ids ...uint64) (map[uint64]string, error) {
	unique := make(map[uint64]struct{}, len(ids))
	list := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := unique[id]; exists {
			continue
		}
		unique[id] = struct{}{}
		list = append(list, id)
	}
	displays := make(map[uint64]string, len(list))
	if len(list) == 0 {
		return displays, nil
	}
	var users []struct {
		ID       uint64 `gorm:"column:id"`
		NickName string `gorm:"column:nick_name"`
	}
	if err := db.Unscoped().Table("sys_users").Select("id", "nick_name").Where("id IN ?", list).Scan(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		displays[user.ID] = adminDisplay(user.NickName, user.ID)
	}
	for _, id := range list {
		if displays[id] == "" {
			displays[id] = adminDisplay("", id)
		}
	}
	return displays, nil
}
func rowsOne(r *gorm.DB) error {
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}

// 每次余额变动前核对最后一条已提交流水，账面余额被旁路篡改时立即停止入账。
func validateWalletTail(tx *gorm.DB, w model.WalletAccount) error {
	var tail model.WalletLedger
	err := tx.Where("wallet_account_id = ?", w.ID).Order("wallet_version DESC").Take(&tail).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if w.Version == 0 && w.AvailableBalance == 0 {
			return nil
		}
		return ErrInvariant
	}
	if err != nil {
		return err
	}
	if tail.WalletVersion != w.Version || tail.BalanceAfter != w.AvailableBalance {
		return ErrInvariant
	}
	return nil
}
