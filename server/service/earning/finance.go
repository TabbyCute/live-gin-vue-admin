package earning

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"tb_live_module/global"
	model "tb_live_module/model/earning"
	live "tb_live_module/model/live"
	"tb_live_module/model/system"
	"time"
	"unicode"
)

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrInvalid      = &Error{49001, "参数无效"}
	ErrForbidden    = &Error{49002, "无收益操作权限"}
	ErrUnavailable  = &Error{49003, "收益服务暂不可用，请使用原幂等键查询或重试"}
	ErrConflict     = &Error{49004, "幂等参数、结算占用或复核状态冲突"}
	ErrState        = &Error{49005, "主播或账户状态不允许该操作"}
	ErrNotFound     = &Error{49006, "记录不存在"}
	ErrInsufficient = &Error{49007, "可提现余额不足或存在待处理负向调整"}
	ErrDisabled     = &Error{49008, "提现服务初始化异常，请稍后重试"}
	ErrInvariant    = &Error{49009, "自动对账异常，禁止审核通过"}
	ErrExchange     = &Error{49010, "无有效结算兑换规则或规则冲突"}
	ErrSource       = &Error{49011, "收益来源尚未接入可信核验"}
	ErrLimit        = &Error{49012, "财务核验数据量超限，请联系财务或技术处理，禁止跳过核验"}
)

type Actor struct {
	ID          uint64
	AuthorityID uint
	IP          string
}

// Providers are immutable startup dependencies. ResolveInTx must use the caller's
// transaction and must never call remote payment APIs while financial locks are held.
type Payee struct {
	AnchorNo         string   `json:"anchorNo"`
	Reference        string   `json:"reference"`
	Version          string   `json:"version"`
	Currency         string   `json:"currency"`
	Method           string   `json:"method"`
	Name             string   `json:"name"`
	Masked           string   `json:"masked"`
	IdentityVerified bool     `json:"identityVerified"`
	RiskChecked      bool     `json:"riskChecked"`
	RiskFlags        []string `json:"riskFlags"`
	ChangedAt        int64    `json:"changedAt"`
	CheckedAt        int64    `json:"checkedAt"`
}
type PayoutPayeeProvider interface {
	ResolveInTx(*gorm.DB, string, string, string) (Payee, error)
}

// SourceVerifier verifies immutable source entries, payment/refund links and fulfillment.
// gift, bounty, guard, fan_club and charge can register separate adapters; unregistered sources fail closed.
type SourceVerifier func(*gorm.DB, []model.Income) error
type Service struct {
	DB      *gorm.DB
	Now     func() time.Time
	Payees  PayoutPayeeProvider
	Sources map[string]SourceVerifier
}

var Default = &Service{}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Default} }
func (s *Service) db(ctx context.Context) *gorm.DB {
	db := s.DB
	if db == nil {
		db = global.GVA_DB
	}
	return db.WithContext(ctx)
}
func (s *Service) now() int64 {
	if s.Now != nil {
		return s.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func number(prefix string) string { return prefix + strings.ReplaceAll(uuid.NewString(), "-", "") }
func encode(v any) string         { b, _ := json.Marshal(v); return string(b) }
func hash(v any) string           { h := sha256.Sum256([]byte(encode(v))); return hex.EncodeToString(h[:]) }

var keyRE = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var noRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
var currencyRE = regexp.MustCompile(`^[A-Z]{3,8}$`)
var sourceRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func clean(v string, n int) bool {
	return len(v) > 0 && len(v) <= n && strings.TrimSpace(v) == v && strings.IndexFunc(v, unicode.IsControl) < 0
}
func integer(v string, zero bool) (int64, error) {
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n < 0 || (!zero && n == 0) || strconv.FormatInt(n, 10) != v {
		return 0, ErrInvalid
	}
	return n, nil
}
func add(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrInvalid
	}
	return a + b, nil
}

// Convert computes a standalone nonnegative conversion; settlements use ConvertWithRemainder.
func Convert(amount, coin, minor int64) (int64, error) {
	if amount < 0 || coin <= 0 || minor <= 0 {
		return 0, ErrInvalid
	}
	n := new(big.Int).Mul(big.NewInt(amount), big.NewInt(minor))
	n.Quo(n, big.NewInt(coin))
	if !n.IsInt64() {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	for i := 0; ; i++ {
		e := s.db(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var me *driver.MySQLError
		if i >= 3 || !errors.As(e, &me) || (me.Number != 1213 && me.Number != 1205) {
			return e
		}
		timer := time.NewTimer(time.Duration(10<<i) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func lockAnchor(tx *gorm.DB, id uint64) error {
	if id == 0 {
		return ErrInvalid
	}
	row := model.IncomeLock{AnchorID: id}
	if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
		return e
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("anchor_id=?", id).Take(&row).Error
}
func lockRules(tx *gorm.DB, write bool) error {
	mode := "SHARE"
	if write {
		mode = "UPDATE"
	}
	return tx.Clauses(clause.Locking{Strength: mode}).First(&model.FinanceControl{}, 1).Error
}
func (s *Service) authorize(tx *gorm.DB, a Actor, path, method string) error {
	if a.ID == 0 {
		return ErrForbidden
	}
	var user system.SysUser
	if e := tx.Where("id=? AND enable=1", a.ID).Take(&user).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrForbidden
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
		return ErrForbidden
	}
	if e := tx.Model(&model.AdminPermission{}).Where("authority_id=? AND path=? AND method=?", a.AuthorityID, "/earning/"+path, method).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return ErrForbidden
	}
	return nil
}
func audit(tx *gorm.DB, a Actor, no, action, snapshot string, now int64) error {
	return tx.Create(&model.FinanceAudit{ObjectNo: no, Action: action, ActorID: a.ID, ActorType: "admin", ActorIP: a.IP, Snapshot: snapshot, CreatedAt: now}).Error
}
func appAnchor(tx *gorm.DB, user uint64) (live.LiveAnchor, error) {
	var a live.LiveAnchor
	var u live.LiveAccount
	if user == 0 {
		return a, ErrForbidden
	}
	if e := tx.Where("id=? AND status=1", user).Take(&u).Error; e != nil {
		return a, ErrForbidden
	}
	if e := tx.Where("user_id=?", user).Take(&a).Error; e != nil {
		return a, ErrNotFound
	}
	return a, nil
}
func anchorByNo(tx *gorm.DB, no string) (live.LiveAnchor, error) {
	var a live.LiveAnchor
	if !noRE.MatchString(no) {
		return a, ErrInvalid
	}
	if e := tx.Where("anchor_no=?", no).Take(&a).Error; e != nil {
		return a, ErrNotFound
	}
	return a, nil
}
func healthyAnchor(tx *gorm.DB, a live.LiveAnchor) bool {
	if a.Status != 1 || a.ApplyStatus != 2 || a.WithdrawPermission != 1 || a.RiskLevel != 0 {
		return false
	}
	var n int64
	return tx.Model(&live.LiveAccount{}).Where("id=? AND status=1", a.UserId).Count(&n).Error == nil && n == 1
}
func (s *Service) verifySources(tx *gorm.DB, facts []model.Income) error {
	grouped := map[string][]model.Income{}
	for _, f := range facts {
		grouped[f.SourceType] = append(grouped[f.SourceType], f)
	}
	for kind, rows := range grouped {
		v := s.Sources[kind]
		if v == nil {
			return ErrSource
		}
		if e := v(tx, rows); e != nil {
			return e
		}
	}
	return nil
}
