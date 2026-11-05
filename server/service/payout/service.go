package payout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/mail"
	"regexp"
	"strings"
	"tb_live_module/global"
	earning "tb_live_module/model/earning"
	live "tb_live_module/model/live"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	"time"
	"unicode"
	"unicode/utf8"
)

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrInvalid     = &Error{49101, "收款账户参数无效"}
	ErrForbidden   = &Error{49102, "当前账号不能操作收款账户"}
	ErrUnavailable = &Error{49103, "收款账户服务暂不可用，请查询原请求结果或按原参数重试"}
	ErrConflict    = &Error{49104, "账户已绑定或请求参数冲突，请刷新本人账户列表"}
	ErrState       = &Error{49105, "收款账户不存在、已停用或不适用于当前提现"}
	ErrLimit       = &Error{49106, "收款账户数量已达上限，请联系运营"}
	ErrCrypto      = errors.New("payout cryptographic configuration or integrity failure")
)

const MaxAccounts = 100

var NoRE = regexp.MustCompile(`^pa_[a-f0-9]{32}$`)
var requestRE = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var currencyRE = regexp.MustCompile(`^[A-Z]{3,8}$`)
var bankRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,15}$`)
var cardRE = regexp.MustCompile(`^[0-9]{12,19}$`)
var wechatRE = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
var alipayNumberRE = regexp.MustCompile(`^\+?[0-9]{6,24}$`)
var appRE = regexp.MustCompile(`^wx[a-fA-F0-9]{16}$`)

type Service struct {
	DB            *gorm.DB
	Crypto        *Crypto
	EvidenceStore *EvidenceStore
	Now           func() time.Time
	// Installed by initialize after earning startup; caller owns the transaction.
	FinancialGuard    func(*gorm.DB, string) (earning.Withdrawal, string, error)
	FinancialLock     func(*gorm.DB, string) (earning.Withdrawal, error)
	FinancialCheck    func(*gorm.DB, string) error
	FinancialFinalize func(*gorm.DB, model.FinalizeInput) (model.TerminalEvent, error)
}

var Default = &Service{}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Default} }
func (s *Service) now() int64 {
	if s.Now != nil {
		return s.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func (s *Service) db(ctx context.Context) *gorm.DB {
	db := s.DB
	if db == nil {
		db = global.GVA_DB
	}
	return db.WithContext(ctx)
}
func number(prefix string) string { return prefix + strings.ReplaceAll(uuid.NewString(), "-", "") }
func clean(v string, min, max int) bool {
	return utf8.ValidString(v) && len(v) >= min && len(v) <= max && strings.TrimSpace(v) == v && strings.IndexFunc(v, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) < 0
}
func Supported(method string) bool {
	return method == "bank_card" || method == "alipay" || method == "wechat"
}

type payload struct {
	AccountName    string `json:"accountName"`
	Account        string `json:"account"`
	BankCode       string `json:"bankCode,omitempty"`
	IdentifierType string `json:"identifierType,omitempty"`
	AppID          string `json:"appId,omitempty"`
}

func validate(r req.CreatePayee) (payload, string, error) {
	v := payload{r.AccountName, r.Account, r.BankCode, r.IdentifierType, r.AppID}
	if !requestRE.MatchString(r.RequestID) || !currencyRE.MatchString(r.Currency) || r.Currency == "COIN" || !clean(r.AccountName, 1, 128) || !clean(r.Account, 6, 128) || utf8.RuneCountInString(r.Account) < 6 {
		return v, "", ErrInvalid
	}
	switch r.Method {
	case "bank_card":
		if !bankRE.MatchString(r.BankCode) || !cardRE.MatchString(r.Account) || r.IdentifierType != "" || r.AppID != "" {
			return v, "", ErrInvalid
		}
	case "alipay":
		// Exact channel identifier; no speculative case folding or provider validation.
		if r.BankCode != "" || r.IdentifierType != "" || r.AppID != "" || strings.IndexFunc(r.Account, unicode.IsSpace) >= 0 {
			return v, "", ErrInvalid
		}
		if strings.Contains(r.Account, "@") {
			address, e := mail.ParseAddress(r.Account)
			if e != nil || address.Address != r.Account {
				return v, "", ErrInvalid
			}
		} else if !alipayNumberRE.MatchString(r.Account) {
			return v, "", ErrInvalid
		}
	case "wechat":
		// An app-scoped OpenID is binding material, never proof of channel verification.
		if r.BankCode != "" || r.IdentifierType != "openid" || !appRE.MatchString(r.AppID) || !wechatRE.MatchString(r.Account) {
			return v, "", ErrInvalid
		}
	default:
		return v, "", ErrInvalid
	}
	runes := []rune(r.Account)
	return v, "****" + string(runes[len(runes)-4:]), nil
}
func (s *Service) fingerprint(owner, method, currency string, v payload) string {
	return s.Crypto.digest("payout.fingerprint.v1", []string{owner, method, currency, v.Account, v.AppID, v.IdentifierType})
}
func appOwner(tx *gorm.DB, user uint64) (live.LiveAnchor, error) {
	var a live.LiveAnchor
	if user == 0 {
		return a, ErrForbidden
	}
	var u live.LiveAccount
	if e := tx.Where("id=? AND status=1", user).Take(&u).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return a, ErrForbidden
		}
		return a, e
	}
	if e := tx.Where("user_id=? AND status=1 AND apply_status=2", user).Take(&a).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return a, ErrForbidden
		}
		return a, e
	}
	return a, nil
}

// LockOwner must run in the caller's transaction. Earning takes its finance lock
// first; payout never takes a finance lock, so the lock order has no reverse edge.
func LockOwner(tx *gorm.DB, owner string) error {
	row := model.OwnerLock{OwnerType: "anchor", OwnerNo: owner}
	if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
		return e
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_type='anchor' AND owner_no=?", owner).Take(&row).Error
}
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		e := s.db(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var me *driver.MySQLError
		if attempt >= 3 || !errors.As(e, &me) || (me.Number != 1213 && me.Number != 1205) {
			return e
		}
		timer := time.NewTimer(time.Duration(10<<attempt) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func audit(tx *gorm.DB, user uint64, trace, action string, p model.PayeeAccount, now int64) error {
	// Serialize only the same allowlisted DTO as APP responses, never a request/model.
	b, e := json.Marshal(res.Public(p))
	if e != nil {
		return e
	}
	return tx.Create(&model.Audit{EventNo: number("pae_"), PayeeAccountNo: p.PayeeAccountNo, OwnerNo: p.OwnerNo, ActorID: user, Action: action, TraceID: trace, Snapshot: string(b), CreatedAt: now}).Error
}
func (s *Service) Create(ctx context.Context, user uint64, trace string, r req.CreatePayee) (res.Payee, error) {
	var out res.Payee
	v, masked, e := validate(r)
	if e != nil {
		return out, e
	}
	if s.Crypto == nil {
		return out, ErrCrypto
	}
	e = s.transaction(ctx, func(tx *gorm.DB) error {
		a, e := appOwner(tx, user)
		if e != nil {
			return e
		}
		if e = LockOwner(tx, a.AnchorNo); e != nil {
			return e
		}
		current, e := appOwner(tx, user)
		if e != nil {
			return e
		}
		if current.AnchorNo != a.AnchorNo {
			return ErrForbidden
		}
		hash := s.Crypto.digest("payout.create.v1", struct {
			Owner   string
			Request req.CreatePayee
		}{a.AnchorNo, r})
		var p model.PayeeAccount
		e = tx.Where("owner_type='anchor' AND owner_no=? AND request_id=?", a.AnchorNo, r.RequestID).Take(&p).Error
		if e == nil {
			if p.RequestHash != hash {
				return ErrConflict
			}
			out = res.Public(p)
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var previous model.PayeeAccount
		e = tx.Where("owner_type='anchor' AND owner_no=?", a.AnchorNo).Order("id ASC").Take(&previous).Error
		if e == nil {
			if e = s.VerifyBinding(previous); e != nil {
				return e
			}
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		fp := s.fingerprint(a.AnchorNo, r.Method, r.Currency, v)
		var count int64
		if e = tx.Model(&model.PayeeAccount{}).Where("owner_type='anchor' AND owner_no=? AND method=? AND currency=? AND account_fingerprint=?", a.AnchorNo, r.Method, r.Currency, fp).Count(&count).Error; e != nil {
			return e
		}
		if count != 0 {
			return ErrConflict
		}
		if e = tx.Model(&model.PayeeAccount{}).Where("owner_type='anchor' AND owner_no=?", a.AnchorNo).Count(&count).Error; e != nil {
			return e
		}
		if count >= MaxAccounts {
			return ErrLimit
		}
		now := s.now()
		p = model.PayeeAccount{PayeeAccountNo: number("pa_"), OwnerType: "anchor", OwnerNo: a.AnchorNo, Method: r.Method, Currency: r.Currency, PayloadVersion: 1, MaskedAccount: masked, AccountFingerprint: fp, RequestID: r.RequestID, RequestHash: hash, Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now}
		if e = tx.Model(&model.PayeeAccount{}).Where("owner_type='anchor' AND owner_no=? AND currency=? AND default_slot=1", a.AnchorNo, r.Currency).Count(&count).Error; e != nil {
			return e
		}
		if count == 0 {
			slot := 1
			p.DefaultSlot = &slot
		}
		if e = s.Crypto.seal(&p, v); e != nil {
			return e
		}
		if e = tx.Create(&p).Error; e != nil {
			return e
		}
		if e = audit(tx, user, trace, "payee.create", p, now); e != nil {
			return e
		}
		out = res.Public(p)
		return nil
	})
	return out, e
}
func (s *Service) List(ctx context.Context, user uint64, q req.PayeeSearch) (res.Page, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	out := res.Page{Page: q.Page, PageSize: q.PageSize, List: []res.Payee{}}
	if q.Page < 1 || q.Page > 100 || q.PageSize < 1 || q.PageSize > 100 || (q.Currency != "" && !currencyRE.MatchString(q.Currency)) || (q.Status != "" && q.Status != "active" && q.Status != "disabled") {
		return out, ErrInvalid
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		a, e := appOwner(tx, user)
		if e != nil {
			return e
		}
		db := tx.Model(&model.PayeeAccount{}).Where("owner_type='anchor' AND owner_no=?", a.AnchorNo)
		if q.Currency != "" {
			db = db.Where("currency=?", q.Currency)
		}
		if q.Status != "" {
			db = db.Where("status=?", q.Status)
		}
		if e = db.Count(&out.Total).Error; e != nil {
			return e
		}
		var rows []model.PayeeAccount
		// Storage secrets are not fetched for lists.
		if e = db.Select("payee_account_no,method,currency,masked_account,status,version,default_slot,created_at,updated_at").Order("id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&rows).Error; e != nil {
			return e
		}
		for _, p := range rows {
			out.List = append(out.List, res.Public(p))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) Change(ctx context.Context, user uint64, trace, no string, disable bool) (res.Payee, error) {
	var out res.Payee
	if !NoRE.MatchString(no) {
		return out, ErrInvalid
	}
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		a, e := appOwner(tx, user)
		if e != nil {
			return e
		}
		if e = LockOwner(tx, a.AnchorNo); e != nil {
			return e
		}
		current, e := appOwner(tx, user)
		if e != nil {
			return e
		}
		if current.AnchorNo != a.AnchorNo {
			return ErrForbidden
		}
		var p model.PayeeAccount
		if e = tx.Where("owner_type='anchor' AND owner_no=? AND payee_account_no=?", a.AnchorNo, no).Take(&p).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrState
			}
			return e
		}
		if disable && p.Status == "disabled" || !disable && p.Status == "active" && p.DefaultSlot != nil {
			out = res.Public(p)
			return nil
		}
		if p.Status != "active" {
			return ErrState
		}
		now := s.now()
		if now < p.UpdatedAt {
			now = p.UpdatedAt
		}
		action := "payee.default"
		if disable {
			p.Status = "disabled"
			p.DefaultSlot = nil
			p.DisabledAt = now
			action = "payee.disable"
		} else {
			var old model.PayeeAccount
			e = tx.Where("owner_type='anchor' AND owner_no=? AND currency=? AND default_slot=1", a.AnchorNo, p.Currency).Take(&old).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if e == nil {
				old.DefaultSlot = nil
				old.UpdatedAt = now
				if e = tx.Model(&old).Updates(map[string]any{"default_slot": nil, "updated_at": now}).Error; e != nil {
					return e
				}
				if e = audit(tx, user, trace, "payee.default.clear", old, now); e != nil {
					return e
				}
			}
			slot := 1
			p.DefaultSlot = &slot
		}
		p.UpdatedAt = now
		if e = tx.Model(&p).Updates(map[string]any{"status": p.Status, "default_slot": p.DefaultSlot, "updated_at": now, "disabled_at": p.DisabledAt}).Error; e != nil {
			return e
		}
		if e = audit(tx, user, trace, action, p, now); e != nil {
			return e
		}
		out = res.Public(p)
		return nil
	})
	return out, e
}

// VerifyBinding authenticates ciphertext and its immutable metadata. No plaintext
// leaves this package. Default/disabled timestamps are intentionally not versions.
func (s *Service) VerifyBinding(p model.PayeeAccount) error {
	if p.PayloadVersion != 1 || p.Version != 1 || p.OwnerType != "anchor" || !NoRE.MatchString(p.PayeeAccountNo) || !Supported(p.Method) {
		return ErrState
	}
	v, e := s.Crypto.open(p)
	if e != nil {
		return e
	}
	_, masked, e := validate(req.CreatePayee{RequestID: p.RequestID, Method: p.Method, Currency: p.Currency, AccountName: v.AccountName, Account: v.Account, BankCode: v.BankCode, IdentifierType: v.IdentifierType, AppID: v.AppID})
	if e != nil || masked != p.MaskedAccount || s.fingerprint(p.OwnerNo, p.Method, p.Currency, v) != p.AccountFingerprint {
		return ErrCrypto
	}
	return nil
}
func (s *Service) ResolveInTx(tx *gorm.DB, owner, no, currency string) (res.Payee, error) {
	if !NoRE.MatchString(no) {
		return res.Payee{}, ErrState
	}
	if e := LockOwner(tx, owner); e != nil {
		return res.Payee{}, e
	}
	var p model.PayeeAccount
	if e := tx.Where("owner_type='anchor' AND owner_no=? AND payee_account_no=?", owner, no).Take(&p).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return res.Payee{}, ErrState
		}
		return res.Payee{}, e
	}
	if p.Status != "active" || p.Currency != currency {
		return res.Payee{}, ErrState
	}
	if e := s.VerifyBinding(p); e != nil {
		return res.Payee{}, e
	}
	return res.Public(p), nil
}
func (s *Service) Inspect(tx *gorm.DB, owner, no, currency, version, method, masked string) (res.Current, error) {
	out := res.Current{State: "missing"}
	if no == "payout_center_pending" {
		out.State = "legacy_placeholder"
		return out, nil
	}
	var p model.PayeeAccount
	if e := tx.Where("owner_type='anchor' AND owner_no=? AND payee_account_no=?", owner, no).Take(&p).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return out, nil
		}
		return out, e
	}
	safe := res.Public(p)
	out.Account = &safe
	out.State = p.Status
	out.MatchesSnapshot = version == "1" && p.Version == 1 && currency == p.Currency && method == p.Method && masked == p.MaskedAccount
	out.Usable = out.MatchesSnapshot && p.Status == "active" && s.VerifyBinding(p) == nil
	if p.Status == "active" && !out.Usable {
		out.State = "unverifiable"
	}
	return out, nil
}

// ValidateKeys catches missing historical decryption keys and accidental changes
// to the stable fingerprint key before accepting new bindings. One authenticated
// envelope per key ID is sufficient to check key configuration, not data health.
func (s *Service) ValidateKeys(db *gorm.DB) error {
	var ids []string
	if e := db.Model(&model.PayeeAccount{}).Distinct("encryption_key_id").Limit(17).Pluck("encryption_key_id", &ids).Error; e != nil {
		return e
	}
	if len(ids) > 16 {
		return ErrCrypto
	}
	for _, id := range ids {
		var p model.PayeeAccount
		if e := db.Where("encryption_key_id=?", id).Order("id ASC").Take(&p).Error; e != nil {
			return e
		}
		if e := s.VerifyBinding(p); e != nil {
			return e
		}
	}
	return nil
}
