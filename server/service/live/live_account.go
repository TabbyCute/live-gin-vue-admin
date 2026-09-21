package live

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrAccountExists      = errors.New("用户名已注册")
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrAccountDisabled    = errors.New("账号已被禁用")
	ErrInvalidUsername    = errors.New("用户名只能包含字母、数字和下划线，长度为3到32位")
	ErrInvalidPassword    = errors.New("密码长度必须为8到72字节")
	usernamePattern       = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)
)

type AccountService struct{}

func (s *AccountService) Register(input liveReq.Register) (liveModel.LiveAccount, error) {
	if global.GVA_DB == nil {
		return liveModel.LiveAccount{}, errors.New("数据库未初始化")
	}

	username := normalizeUsername(input.Username)
	if !usernamePattern.MatchString(username) {
		return liveModel.LiveAccount{}, ErrInvalidUsername
	}
	if passwordLength := len([]byte(input.Password)); passwordLength < 8 || passwordLength > 72 {
		return liveModel.LiveAccount{}, ErrInvalidPassword
	}

	var existing liveModel.LiveAccount
	err := global.GVA_DB.Select("id").Where("username = ?", username).First(&existing).Error
	if err == nil {
		return liveModel.LiveAccount{}, ErrAccountExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return liveModel.LiveAccount{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return liveModel.LiveAccount{}, fmt.Errorf("生成密码哈希失败: %w", err)
	}

	nickname := strings.TrimSpace(input.Nickname)
	if nickname == "" {
		nickname = username
	}
	account := liveModel.LiveAccount{
		Username: username,
		Password: string(passwordHash),
		Nickname: nickname,
		Avatar:   strings.TrimSpace(input.Avatar),
		Status:   liveModel.LiveAccountStatusNormal,
	}
	if err = global.GVA_DB.Create(&account).Error; err != nil {
		// 数据库唯一索引负责兜住并发注册；再次查询后统一返回业务错误。
		var duplicate liveModel.LiveAccount
		if lookupErr := global.GVA_DB.Select("id").Where("username = ?", username).First(&duplicate).Error; lookupErr == nil {
			return liveModel.LiveAccount{}, ErrAccountExists
		}
		return liveModel.LiveAccount{}, err
	}
	return account, nil
}

func (s *AccountService) Login(input liveReq.Login) (*liveModel.LiveAccount, error) {
	if global.GVA_DB == nil {
		return nil, errors.New("数据库未初始化")
	}

	username := normalizeUsername(input.Username)
	var account liveModel.LiveAccount
	if err := global.GVA_DB.Where("username = ?", username).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if account.Status != liveModel.LiveAccountStatusNormal {
		return nil, ErrAccountDisabled
	}
	return &account, nil
}

func (s *AccountService) GetByID(accountID uint) (*liveModel.LiveAccount, error) {
	if global.GVA_DB == nil {
		return nil, errors.New("数据库未初始化")
	}
	var account liveModel.LiveAccount
	if err := global.GVA_DB.First(&account, accountID).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
