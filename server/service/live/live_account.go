package live

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"
	"tb_live_module/utils"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAccountNotFound               = errors.New("用户不存在")
	ErrInvalidCredentials            = errors.New("手机号或密码错误")
	ErrAccountDisabled               = errors.New("账号当前不可用")
	ErrInvalidSMSCode                = errors.New("验证码错误")
	ErrRegistrationUnavailable       = errors.New("注册暂不可用")
	ErrPhoneRequired                 = errors.New("手机号是唯一登录凭证，请更换手机号，不能清空或解绑")
	ErrInvalidPassword               = errors.New("密码长度必须为8到72字节")
	ErrInvalidProfile                = errors.New("用户资料不合法")
	ErrInvalidPhone                  = errors.New("手机号必须使用E.164格式")
	ErrPhoneExists                   = errors.New("手机号已被其他用户绑定")
	ErrAccountStatusReasonRequired   = errors.New("禁用、封禁或注销账号时必须填写原因")
	ErrAccountBanUntilInvalid        = errors.New("临时封禁截止时间必须晚于当前时间，永久封禁请传0")
	ErrAccountCancelledCannotRestore = errors.New("已注销账号不能通过普通状态接口恢复")
	phonePattern                     = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	smsCodePattern                   = regexp.MustCompile(`^[0-9]{6}$`)
	countryCodePattern               = regexp.MustCompile(`^[A-Za-z]{2}$`)
)

const accountAdminSelectColumns = "id, account_no, username, phone, phone_verified_at, nickname, avatar, signature, gender, birthday, country_code, region_code, city_code, language, real_name_status, real_name_verified_at, status, status_reason, ban_until, password_changed_at, last_login_at, register_source, register_channel_id, created_at, updated_at"

type AccountService struct{}

func (s *AccountService) Register(input liveReq.Register) (liveModel.LiveAccount, error) {
	return s.RegisterWithContext(context.Background(), input)
}

func (s *AccountService) RegisterWithContext(ctx context.Context, input liveReq.Register) (liveModel.LiveAccount, error) {
	if global.GVA_DB == nil {
		return liveModel.LiveAccount{}, errors.New("数据库未初始化")
	}
	phone, err := utils.NormalizeAccountPhone(input.Phone)
	if err != nil {
		return liveModel.LiveAccount{}, ErrInvalidPhone
	}
	if !validPassword(input.Password) {
		return liveModel.LiveAccount{}, ErrInvalidPassword
	}
	// 通用验证码仅放行注册。空值/非法配置不能意外允许空验证码注册。
	configuredCode := global.LiveAccountSuperCode()
	if !smsCodePattern.MatchString(configuredCode) {
		return liveModel.LiveAccount{}, ErrRegistrationUnavailable
	}
	expected, provided := sha256.Sum256([]byte(configuredCode)), sha256.Sum256([]byte(input.SmsCode))
	if !smsCodePattern.MatchString(input.SmsCode) || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
		return liveModel.LiveAccount{}, ErrInvalidSMSCode
	}
	if err = ctx.Err(); err != nil {
		return liveModel.LiveAccount{}, err
	}
	db := global.GVA_DB.WithContext(ctx)

	var existing liveModel.LiveAccount
	err = db.Unscoped().Select("id").Where("phone = ?", phone).First(&existing).Error
	if err == nil {
		return liveModel.LiveAccount{}, ErrPhoneExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return liveModel.LiveAccount{}, err
	}

	nickname := strings.TrimSpace(input.Nickname)
	if utf8.RuneCountInString(nickname) > 64 || utf8.RuneCountInString(strings.TrimSpace(input.Avatar)) > 500 {
		return liveModel.LiveAccount{}, fmt.Errorf("%w: 注册资料字段长度超出限制", ErrInvalidProfile)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return liveModel.LiveAccount{}, fmt.Errorf("生成密码哈希失败: %w", err)
	}
	account := liveModel.LiveAccount{
		Phone:           &phone,
		PhoneVerifiedAt: time.Now().UnixMilli(),
		Password:        string(passwordHash),
		Nickname:        nickname,
		Avatar:          strings.TrimSpace(input.Avatar),
		Status:          liveModel.LiveAccountStatusNormal,
		TokenVersion:    1,
		RegisterSource:  "app",
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		return createAccountWithNextNumber(tx, &account)
	})
	if err == nil {
		return account, nil
	}
	// 唯一索引负责兜住并发注册；统一返回稳定的业务错误。
	var duplicate liveModel.LiveAccount
	if lookupErr := db.Unscoped().Select("id").Where("phone = ?", phone).First(&duplicate).Error; lookupErr == nil {
		return liveModel.LiveAccount{}, ErrPhoneExists
	}
	return liveModel.LiveAccount{}, err
}

func createAccountWithNextNumber(tx *gorm.DB, account *liveModel.LiveAccount) error {
	// 冷启动首个并发注册也通过同一主键串行，不依赖进程内计数器。
	sequence := liveModel.LiveAccountNumberSequence{ID: 1, NextValue: liveModel.LiveAccountNumberStart}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&sequence).Error; err != nil {
		return err
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sequence, 1).Error
	if err != nil {
		return err
	}
	if sequence.NextValue < liveModel.LiveAccountNumberStart {
		sequence.NextValue = liveModel.LiveAccountNumberStart
	}

	for attempt := 0; attempt < 10000; attempt++ {
		if sequence.NextValue == math.MaxUint64 {
			return errors.New("客户端账号编号已耗尽")
		}
		candidate := strconv.FormatUint(sequence.NextValue, 10)
		sequence.NextValue++
		var count int64
		if err = tx.Unscoped().Model(&liveModel.LiveAccount{}).
			Where("account_no = ? OR username = ?", candidate, "u"+candidate).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err = tx.Model(&sequence).UpdateColumn("next_value", sequence.NextValue).Error; err != nil {
			return err
		}
		account.AccountNo = candidate
		account.Username = "u" + candidate
		if account.Nickname == "" {
			account.Nickname = "用户" + candidate
		}
		return tx.Create(account).Error
	}
	return errors.New("连续账号编号可用空间不足")
}

func (s *AccountService) Login(input liveReq.Login) (*liveModel.LiveAccount, error) {
	return s.LoginWithContext(context.Background(), input)
}

// 固定的非账号bcrypt哈希，未知手机号也执行一次校验，缩小账号枚举的耗时差异。
const missingAccountPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *AccountService) LoginWithContext(ctx context.Context, input liveReq.Login) (*liveModel.LiveAccount, error) {
	if global.GVA_DB == nil {
		return nil, errors.New("数据库未初始化")
	}
	phone, err := utils.NormalizeAccountPhone(input.Phone)
	if err != nil || !validPassword(input.Password) {
		return nil, ErrInvalidCredentials
	}
	db := global.GVA_DB.WithContext(ctx)
	var account liveModel.LiveAccount
	if err := db.Where("phone = ? AND phone_verified_at > 0", phone).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = bcrypt.CompareHashAndPassword([]byte(missingAccountPasswordHash), []byte(input.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if err := restoreExpiredAccountBan(db, &account); err != nil {
		return nil, err
	}
	if account.Status != liveModel.LiveAccountStatusNormal {
		return nil, ErrAccountDisabled
	}
	now := time.Now().UnixMilli()
	// bcrypt期间可能改号、改密或封号；不能用过期快照签发可用登录凭证。
	current := db.Model(&liveModel.LiveAccount{}).Where(
		"id = ? AND phone = ? AND phone_verified_at > 0 AND password = ? AND token_version = ? AND status = ?",
		account.ID, phone, account.Password, account.TokenVersion, liveModel.LiveAccountStatusNormal,
	)
	result := current.UpdateColumn("last_login_at", now)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL同一毫秒重复登录可能返回0行，重读条件区分未变化和凭证已变更。
		var count int64
		if err := current.Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, ErrInvalidCredentials
		}
	}
	account.LastLoginAt = now
	return &account, nil
}

func (s *AccountService) GetByID(accountID uint) (*liveModel.LiveAccount, error) {
	if global.GVA_DB == nil {
		return nil, errors.New("数据库未初始化")
	}
	var account liveModel.LiveAccount
	if err := global.GVA_DB.First(&account, accountID).Error; err != nil {
		return nil, normalizeAccountLookupError(err)
	}
	return &account, nil
}

func (s *AccountService) UpdateProfile(accountID uint, req liveReq.AccountProfileUpdateReq) error {
	updates, err := accountProfileUpdateValues(req)
	if err != nil {
		return err
	}
	result := global.GVA_DB.Model(&liveModel.LiveAccount{}).Where("id = ?", accountID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ensureAccountExists(global.GVA_DB, accountID)
	}
	return nil
}

func (s *AccountService) ChangePassword(accountID uint, req liveReq.AccountPasswordChangeReq) (*liveModel.LiveAccount, error) {
	if !validPassword(req.OldPassword) || !validPassword(req.NewPassword) {
		return nil, ErrInvalidPassword
	}
	if req.OldPassword == req.NewPassword {
		return nil, errors.New("新密码不能与旧密码相同")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("生成密码哈希失败: %w", err)
	}
	var updated liveModel.LiveAccount
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		account, lookupErr := getAccountByIDForUpdate(tx, accountID)
		if lookupErr != nil {
			return normalizeAccountLookupError(lookupErr)
		}
		if compareErr := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.OldPassword)); compareErr != nil {
			return ErrInvalidCredentials
		}
		now := time.Now().UnixMilli()
		if updateErr := tx.Model(&liveModel.LiveAccount{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
			"password": string(newHash), "password_changed_at": now,
			"token_version": gorm.Expr("token_version + 1"),
		}).Error; updateErr != nil {
			return updateErr
		}
		return tx.First(&updated, account.ID).Error
	})
	return &updated, err
}

func (s *AccountService) GetPublicProfile(accountNo string) (liveRes.AccountPublicProfileResp, error) {
	return s.GetViewerPublicProfile(context.Background(), 0, accountNo)
}
func (s *AccountService) GetViewerPublicProfile(ctx context.Context, viewer uint64, accountNo string) (liveRes.AccountPublicProfileResp, error) {
	var account liveModel.LiveAccount
	err := global.GVA_DB.WithContext(ctx).Select("id").Where("account_no = ? AND status = ?", strings.ToUpper(strings.TrimSpace(accountNo)), liveModel.LiveAccountStatusNormal).First(&account).Error
	if err != nil {
		return liveRes.AccountPublicProfileResp{}, normalizeAccountLookupError(err)
	}
	profiles, err := (SocialDirectory{}).Profiles(ctx, global.GVA_DB, viewer, []uint64{uint64(account.ID)})
	if err != nil {
		return liveRes.AccountPublicProfileResp{}, err
	}
	profile, ok := profiles[uint64(account.ID)]
	if !ok {
		return liveRes.AccountPublicProfileResp{}, ErrAccountNotFound
	}
	return profile, nil
}

func toAccountPublicProfile(account liveModel.LiveAccount) liveRes.AccountPublicProfileResp {
	return liveRes.AccountPublicProfileResp{
		AccountNo: account.AccountNo, Nickname: account.Nickname, Avatar: account.Avatar,
		Signature: account.Signature, Gender: account.Gender, CountryCode: account.CountryCode,
	}
}

func (s *AccountService) GetAdminList(req *liveReq.AccountAdminListReq) ([]liveRes.AccountAdminListItemResp, int64, error) {
	if global.GVA_DB == nil {
		return nil, 0, errors.New("数据库未初始化")
	}
	db := global.GVA_DB.Model(&liveModel.LiveAccount{})
	if value := strings.ToUpper(strings.TrimSpace(req.AccountNo)); value != "" {
		db = db.Where("account_no LIKE ?", value+"%")
	}
	if value := normalizeUsername(req.Username); value != "" {
		db = db.Where("username LIKE ?", value+"%")
	}
	if value := strings.TrimSpace(req.Phone); value != "" {
		if !phonePattern.MatchString(value) {
			return nil, 0, ErrInvalidPhone
		}
		db = db.Where("phone = ?", value)
	}
	if value := strings.TrimSpace(req.Nickname); value != "" {
		db = db.Where("nickname LIKE ?", "%"+value+"%")
	}
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}
	if req.RealNameStatus != nil {
		db = db.Where("real_name_status = ?", *req.RealNameStatus)
	}
	if value := strings.TrimSpace(req.RegisterSource); value != "" {
		db = db.Where("register_source = ?", value)
	}
	if req.RegisterChannelID > 0 {
		db = db.Where("register_channel_id = ?", req.RegisterChannelID)
	}
	var err error
	db, err = applyCreatedAtRange(db, req.CreatedAtStart, req.CreatedAtEnd)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err = db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var accounts []liveModel.LiveAccount
	if err = db.Select(accountAdminSelectColumns).Scopes(req.PageInfo.Paginate()).Order("id DESC").Find(&accounts).Error; err != nil {
		return nil, 0, err
	}
	list := make([]liveRes.AccountAdminListItemResp, 0, len(accounts))
	for i := range accounts {
		list = append(list, toAccountAdminListItem(accounts[i]))
	}
	return list, total, nil
}

func (s *AccountService) GetAdminDetail(accountID uint) (liveRes.AccountAdminDetailResp, error) {
	var account liveModel.LiveAccount
	err := global.GVA_DB.Select(accountAdminSelectColumns).First(&account, accountID).Error
	if err != nil {
		return liveRes.AccountAdminDetailResp{}, normalizeAccountLookupError(err)
	}
	return toAccountAdminDetail(account), nil
}

func (s *AccountService) AdminUpdateProfile(req liveReq.AccountAdminProfileUpdateReq) error {
	updates, err := accountProfileUpdateValues(req.AccountProfileUpdateReq)
	if err != nil {
		return err
	}
	phoneForDuplicateCheck := ""
	if req.Phone != nil {
		if strings.TrimSpace(*req.Phone) == "" {
			return ErrPhoneRequired
		}
		phoneForDuplicateCheck, err = utils.NormalizeAccountPhone(*req.Phone)
		if err != nil {
			return ErrInvalidPhone
		}
	}
	err = global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		account, lookupErr := getAccountByIDForUpdate(tx, req.AccountID)
		if lookupErr != nil {
			return normalizeAccountLookupError(lookupErr)
		}
		if req.Phone != nil {
			phone := phoneForDuplicateCheck
			currentPhone := ""
			if account.Phone != nil {
				currentPhone = *account.Phone
			}
			phoneChanged := currentPhone != phone || account.PhoneVerifiedAt == 0
			if phoneChanged {
				var duplicate liveModel.LiveAccount
				findErr := tx.Unscoped().Select("id").Where("phone = ? AND id <> ?", phone, account.ID).First(&duplicate).Error
				if findErr == nil {
					return ErrPhoneExists
				}
				if !errors.Is(findErr, gorm.ErrRecordNotFound) {
					return findErr
				}
				updates["phone"], updates["phone_verified_at"] = phone, time.Now().UnixMilli()
				updates["token_version"] = gorm.Expr("token_version + 1")
			}
		}
		return tx.Model(&liveModel.LiveAccount{}).Where("id = ?", account.ID).Updates(updates).Error
	})
	if err != nil && phoneForDuplicateCheck != "" && !errors.Is(err, ErrPhoneExists) {
		var duplicate liveModel.LiveAccount
		if lookupErr := global.GVA_DB.Unscoped().Select("id").
			Where("phone = ? AND id <> ?", phoneForDuplicateCheck, req.AccountID).First(&duplicate).Error; lookupErr == nil {
			return ErrPhoneExists
		}
	}
	return err
}

func (s *AccountService) AdminUpdateStatus(req liveReq.AccountAdminStatusUpdateReq) error {
	if req.Status == nil {
		return errors.New("账号状态不能为空")
	}
	status := *req.Status
	reason := strings.TrimSpace(req.StatusReason)
	now := time.Now().UnixMilli()
	if status != liveModel.LiveAccountStatusNormal && reason == "" {
		return ErrAccountStatusReasonRequired
	}
	if status == liveModel.LiveAccountStatusBanned && req.BanUntil != 0 && req.BanUntil <= now {
		return ErrAccountBanUntilInvalid
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		account, err := getAccountByIDForUpdate(tx, req.AccountID)
		if err != nil {
			return normalizeAccountLookupError(err)
		}
		if account.Status == liveModel.LiveAccountStatusCancelled && status != liveModel.LiveAccountStatusCancelled {
			return ErrAccountCancelledCannotRestore
		}
		updates := map[string]interface{}{"status": status, "token_version": gorm.Expr("token_version + 1")}
		switch status {
		case liveModel.LiveAccountStatusNormal:
			updates["status_reason"], updates["ban_until"] = "", int64(0)
		case liveModel.LiveAccountStatusDisabled:
			updates["status_reason"], updates["ban_until"] = reason, int64(0)
		case liveModel.LiveAccountStatusBanned:
			updates["status_reason"], updates["ban_until"] = reason, req.BanUntil
		case liveModel.LiveAccountStatusCancelled:
			updates["status_reason"], updates["ban_until"] = reason, int64(0)
		default:
			return errors.New("无效的账号状态")
		}
		if status != liveModel.LiveAccountStatusNormal {
			var anchor liveModel.LiveAnchor
			if findErr := tx.Select("id").Where("user_id = ?", uint64(account.ID)).First(&anchor).Error; findErr == nil {
				if err = endActiveSessionForAnchor(tx, anchor.ID, reason); err != nil {
					return err
				}
			} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return findErr
			}
		}
		return tx.Model(&liveModel.LiveAccount{}).Where("id = ?", account.ID).Updates(updates).Error
	})
}

func (s *AccountService) AdminUnbindPhone(_ uint) error {
	return ErrPhoneRequired
}

func (s *AccountService) AdminLogoutAll(accountID uint) error {
	result := global.GVA_DB.Model(&liveModel.LiveAccount{}).Where("id = ?", accountID).
		UpdateColumn("token_version", gorm.Expr("token_version + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ensureAccountExists(global.GVA_DB, accountID)
	}
	return nil
}

func getAccountByIDForUpdate(db *gorm.DB, accountID uint) (*liveModel.LiveAccount, error) {
	var account liveModel.LiveAccount
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, accountID).Error
	return &account, err
}

func normalizeAccountLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAccountNotFound
	}
	return err
}

func ensureAccountExists(db *gorm.DB, accountID uint) error {
	var exists bool
	if err := db.Model(&liveModel.LiveAccount{}).Select("count(*) > 0").Where("id = ?", accountID).Find(&exists).Error; err != nil {
		return err
	}
	if !exists {
		return ErrAccountNotFound
	}
	return nil
}

func restoreExpiredAccountBan(db *gorm.DB, account *liveModel.LiveAccount) error {
	now := time.Now().UnixMilli()
	if account.Status != liveModel.LiveAccountStatusBanned || account.BanUntil == 0 || account.BanUntil > now {
		return nil
	}
	result := db.Model(&liveModel.LiveAccount{}).
		Where("id = ? AND status = ? AND ban_until > 0 AND ban_until <= ?", account.ID, liveModel.LiveAccountStatusBanned, now).
		Updates(map[string]interface{}{"status": liveModel.LiveAccountStatusNormal, "status_reason": "", "ban_until": int64(0)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		account.Status, account.StatusReason, account.BanUntil = liveModel.LiveAccountStatusNormal, "", 0
	}
	return nil
}

func validateAccountProfile(req liveReq.AccountProfileUpdateReq) error {
	if strings.TrimSpace(req.Nickname) == "" || utf8.RuneCountInString(req.Nickname) > 64 {
		return fmt.Errorf("%w: 昵称不能为空且不能超过64个字符", ErrInvalidProfile)
	}
	if utf8.RuneCountInString(req.Avatar) > 500 || utf8.RuneCountInString(req.Signature) > 255 ||
		utf8.RuneCountInString(req.RegionCode) > 32 || utf8.RuneCountInString(req.CityCode) > 32 || utf8.RuneCountInString(req.Language) > 16 {
		return fmt.Errorf("%w: 用户资料字段长度超出限制", ErrInvalidProfile)
	}
	country := strings.TrimSpace(req.CountryCode)
	if country != "" && !countryCodePattern.MatchString(country) {
		return fmt.Errorf("%w: 国家或地区代码必须是2位字母", ErrInvalidProfile)
	}
	if req.Gender > 3 {
		return fmt.Errorf("%w: 无效的性别", ErrInvalidProfile)
	}
	return nil
}

func accountProfileUpdateValues(req liveReq.AccountProfileUpdateReq) (map[string]interface{}, error) {
	if err := validateAccountProfile(req); err != nil {
		return nil, err
	}
	birthday, err := parseBirthday(req.Birthday)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProfile, err)
	}
	return map[string]interface{}{
		"nickname": strings.TrimSpace(req.Nickname), "avatar": strings.TrimSpace(req.Avatar),
		"signature": strings.TrimSpace(req.Signature), "gender": req.Gender, "birthday": birthday,
		"country_code": strings.ToUpper(strings.TrimSpace(req.CountryCode)),
		"region_code":  strings.TrimSpace(req.RegionCode), "city_code": strings.TrimSpace(req.CityCode),
		"language": strings.TrimSpace(req.Language),
	}, nil
}

func validPassword(password string) bool {
	length := len([]byte(password))
	return length >= 8 && length <= 72
}

func normalizeUsername(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

func maskPhone(phone *string) string {
	if phone == nil || *phone == "" {
		return ""
	}
	value := []rune(*phone)
	if len(value) <= 7 {
		return "***"
	}
	return string(value[:3]) + "****" + string(value[len(value)-4:])
}

// MaskAccountPhone 仅返回适合普通账号和后台列表展示的脱敏手机号。
func MaskAccountPhone(phone *string) string { return maskPhone(phone) }

// FormatAccountBirthday 返回公开资料使用的日期格式。
func FormatAccountBirthday(value *time.Time) string { return formatBirthday(value) }

func toAccountAdminListItem(account liveModel.LiveAccount) liveRes.AccountAdminListItemResp {
	return liveRes.AccountAdminListItemResp{
		ID: account.ID, AccountNo: account.AccountNo, Username: account.Username,
		PhoneMasked: maskPhone(account.Phone), PhoneVerified: account.Phone != nil && account.PhoneVerifiedAt > 0,
		Nickname: account.Nickname, Avatar: account.Avatar, Status: account.Status, BanUntil: account.BanUntil,
		RealNameStatus: account.RealNameStatus, RegisterSource: account.RegisterSource,
		RegisterChannelID: account.RegisterChannelID, LastLoginAt: account.LastLoginAt, CreatedAt: account.CreatedAt,
	}
}

func toAccountAdminDetail(account liveModel.LiveAccount) liveRes.AccountAdminDetailResp {
	phone := ""
	if account.Phone != nil {
		phone = *account.Phone
	}
	return liveRes.AccountAdminDetailResp{
		ID: account.ID, AccountNo: account.AccountNo, Username: account.Username,
		Phone: phone, PhoneMasked: maskPhone(account.Phone), PhoneVerifiedAt: account.PhoneVerifiedAt,
		Nickname: account.Nickname, Avatar: account.Avatar, Signature: account.Signature, Gender: account.Gender,
		Birthday: formatBirthday(account.Birthday), CountryCode: account.CountryCode, RegionCode: account.RegionCode,
		CityCode: account.CityCode, Language: account.Language, RealNameStatus: account.RealNameStatus,
		RealNameVerifiedAt: account.RealNameVerifiedAt, Status: account.Status, StatusReason: account.StatusReason,
		BanUntil: account.BanUntil, PasswordChangedAt: account.PasswordChangedAt, LastLoginAt: account.LastLoginAt,
		RegisterSource: account.RegisterSource, RegisterChannelID: account.RegisterChannelID,
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
	}
}
