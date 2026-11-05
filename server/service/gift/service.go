package gift

import (
	"context"
	"database/sql"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"tb_live_module/global"
	earningModel "tb_live_module/model/earning"
	model "tb_live_module/model/gift"
	guardModel "tb_live_module/model/guard"
	"tb_live_module/model/system"
	"tb_live_module/service/earning"
	"tb_live_module/service/wallet"
	"time"
	"unicode"
)

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrInvalid     = &Error{48001, "参数无效"}
	ErrForbidden   = &Error{48002, "无礼物操作权限"}
	ErrUnavailable = &Error{48003, "服务暂不可用，请使用原幂等键查询或重试"}
	ErrConflict    = &Error{48004, "幂等参数或版本冲突，请刷新后重试"}
	ErrState       = &Error{48005, "礼物、分类、主播或直播场次当前不可用"}
	ErrNotFound    = &Error{48006, "记录不存在"}
	ErrRefund      = &Error{48007, "累计退款超过原订单"}
	ErrGuard       = &Error{48008, "守护当前不可用，请检查售卖开关、方案和有效期"}
	ErrInvariant   = &Error{48009, "礼物账务核查异常"}
	ErrSelfGift    = &Error{48012, "主播不能给自己送礼"}
)

type Actor struct {
	ID          uint64
	AuthorityID uint
	IP          string
}
type Wallet interface {
	DebitInTx(*gorm.DB, wallet.MovementInput) (wallet.MovementResult, error)
	RefundInTx(*gorm.DB, wallet.MovementInput) (wallet.MovementResult, error)
}

// LegacyGuardCompensator preserves the old compensation adapter for migration
// regression tests. Production fulfillment uses structured GuardService below.
type LegacyGuardCompensator interface {
	Compensate(context.Context, string, string, int64) error
}
type Service struct {
	Guard          GuardService
	GuardEnabled   bool
	EnrichEarnings func(*gorm.DB, []earningModel.Ledger) error
	DB             *gorm.DB
	Wallet         Wallet
	Now            func() time.Time
}

// GuardService performs fulfillment only after the gift transaction commits.
// PlanSnapshotInTx is a bounded local catalog read, never external fulfillment.
type GuardService interface {
	PlanSnapshotInTx(*gorm.DB, string) (guardModel.PlanSnapshot, error)
	Fulfill(context.Context, guardModel.GrantInput) error
	Revoke(context.Context, guardModel.CompensationInput) error
	RunTask(context.Context, string, string, func(context.Context) error) error
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
func (s *Service) wallet() Wallet {
	if s.Wallet != nil {
		return s.Wallet
	}
	return wallet.Default
}
func (s *Service) now() int64 {
	if s.Now != nil {
		return s.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func no(prefix string) string { return prefix + strings.ReplaceAll(uuid.NewString(), "-", "") }

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var numberPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

func integer(v string, zero bool) (int64, error) {
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n < 0 || (!zero && n == 0) || strconv.FormatInt(n, 10) != v {
		return 0, ErrInvalid
	}
	return n, nil
}
func textOK(v string, max int) bool {
	return len(v) > 0 && len(v) <= max && strings.TrimSpace(v) == v && strings.IndexFunc(v, unicode.IsControl) < 0
}
func urlOK(v string, optional bool) bool {
	if v == "" {
		return optional
	}
	u, e := url.Parse(v)
	return len(v) <= 500 && e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}
func multiply(a, b int64) (int64, error) {
	if a <= 0 || b <= 0 || a > math.MaxInt64/b {
		return 0, ErrInvalid
	}
	return a * b, nil
}
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		e := s.db(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var me *driver.MySQLError
		if attempt >= 3 || !errors.As(e, &me) || (me.Number != 1213 && me.Number != 1205) {
			return e
		}
		timer := time.NewTimer(time.Duration(10*(1<<attempt)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func lockControl(tx *gorm.DB, write bool) (model.Control, error) {
	var c model.Control
	strength := "SHARE"
	if write {
		strength = "UPDATE"
	}
	e := tx.Clauses(clause.Locking{Strength: strength}).First(&c, 1).Error
	return c, e
}
func bump(tx *gorm.DB) error {
	r := tx.Model(&model.Control{}).Where("id=1 AND version < ?", int64(math.MaxInt64)).Update("version", gorm.Expr("version + 1"))
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}

// Every admin entry validates the enabled account. Root bypasses role policies;
// other administrators require current membership and an exact operation grant.
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
	var member int64
	if e := tx.Table("sys_user_authority AS membership").Joins("JOIN sys_authorities AS role ON role.authority_id=membership.sys_authority_authority_id AND role.deleted_at IS NULL").Where("membership.sys_user_id=? AND membership.sys_authority_authority_id=?", a.ID, a.AuthorityID).Count(&member).Error; e != nil {
		return e
	}
	if member == 0 {
		return ErrForbidden
	}
	var count int64
	if e := tx.Model(&model.AdminPermission{}).Where("authority_id=? AND path=? AND method=?", a.AuthorityID, "/gift/"+path, method).Count(&count).Error; e != nil {
		return e
	}
	if count == 0 {
		return ErrForbidden
	}
	return nil
}
func audit(tx *gorm.DB, a Actor, object, action, snapshot string, now int64) error {
	return tx.Create(&model.Audit{ObjectNo: object, Action: action, OperatorID: a.ID, Snapshot: snapshot, ActorIP: a.IP, CreatedAt: now}).Error
}

var earnings = earning.Service{}

// CanReadRevenue is used by live admin DTO enrichment. Existing live read access
// does not implicitly grant access to gift financial summaries.
func (s *Service) CanReadRevenue(ctx context.Context, a Actor) bool {
	return s.authorize(s.db(ctx), a, "ranking", "GET") == nil
}
