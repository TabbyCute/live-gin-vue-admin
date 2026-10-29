package live

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"tb_live_module/global"
	commonReq "tb_live_module/model/common/request"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	walletModel "tb_live_module/model/wallet"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}, &liveModel.LiveAccountNumberSequence{}, &liveModel.LiveAnchor{}, &walletModel.WalletAccount{}))
	previousDB := global.GVA_DB
	previousCode := global.LiveAccountSuperCode()
	global.GVA_DB = db
	global.SetLiveAccountSuperCode("123456")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.SetLiveAccountSuperCode(previousCode)
		_ = sqlDB.Close()
	})
}

func TestAccountRegisterAndLogin(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}

	registered, err := service.Register(liveReq.Register{
		Phone: " +60123456789 ", SmsCode: "123456",
		Password: "password123",
	})
	require.NoError(t, err)
	require.Equal(t, "u1145781", registered.Username)
	require.Equal(t, "用户1145781", registered.Nickname)
	require.Equal(t, "+60123456789", *registered.Phone)
	require.Positive(t, registered.PhoneVerifiedAt)
	require.NotEqual(t, "password123", registered.Password)
	require.Equal(t, "1145781", registered.AccountNo)
	require.Equal(t, uint64(1), registered.TokenVersion)
	next, err := service.Register(liveReq.Register{Phone: "+60123456780", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)
	require.Equal(t, "1145782", next.AccountNo)

	loggedIn, err := service.Login(liveReq.Login{Phone: " +60123456789 ", Password: "password123"})
	require.NoError(t, err)
	require.Equal(t, registered.ID, loggedIn.ID)
}

func TestAccountRegisterRejectsDuplicateAndInvalidInput(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}

	_, err := service.Register(liveReq.Register{Phone: "+60123456789", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)
	_, err = service.Register(liveReq.Register{Phone: " +60123456789 ", SmsCode: "123456", Password: "password456"})
	require.ErrorIs(t, err, ErrPhoneExists)

	_, err = service.Register(liveReq.Register{Phone: "13812345678", SmsCode: "123456", Password: "password123"})
	require.ErrorIs(t, err, ErrInvalidPhone)
	_, err = service.Register(liveReq.Register{Phone: "+60123456780", SmsCode: "123456", Password: "short"})
	require.ErrorIs(t, err, ErrInvalidPassword)
}

func TestAccountRegisterSkipsExistingNumericAccountNumber(t *testing.T) {
	setupAccountTestDB(t)
	require.NoError(t, global.GVA_DB.Create(&liveModel.LiveAccount{
		AccountNo: "1145781", Username: "legacy", Password: "hash", Nickname: "Legacy",
		Status: liveModel.LiveAccountStatusNormal, TokenVersion: 1,
	}).Error)

	registered, err := (&AccountService{}).Register(liveReq.Register{Phone: "+60123456789", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)
	require.Equal(t, "1145782", registered.AccountNo)
}

func TestAccountLoginRejectsInvalidCredentialsAndDisabledAccount(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}
	account, err := service.Register(liveReq.Register{Phone: "+60123456789", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)

	_, err = service.Login(liveReq.Login{Phone: "+60123456789", Password: "wrong-password"})
	require.ErrorIs(t, err, ErrInvalidCredentials)

	require.NoError(t, global.GVA_DB.Model(&account).Update("status", liveModel.LiveAccountStatusDisabled).Error)
	_, err = service.Login(liveReq.Login{Phone: "+60123456789", Password: "password123"})
	require.True(t, errors.Is(err, ErrAccountDisabled))
}

func TestAccountProfilePasswordAndAdminSecurityActions(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}
	account, err := service.Register(liveReq.Register{Phone: "+60123456788", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)

	birthday := "2000-01-02"
	profile := liveReq.AccountProfileUpdateReq{
		Nickname: "安全用户", Signature: "hello", Gender: 1, Birthday: &birthday,
		CountryCode: "my", RegionCode: "10", CityCode: "101", Language: "zh-CN",
	}
	require.NoError(t, service.UpdateProfile(account.ID, profile))
	// MySQL对未发生变化的UPDATE可能返回0行，重复提交仍应成功。
	require.NoError(t, service.UpdateProfile(account.ID, profile))

	updated, err := service.ChangePassword(account.ID, liveReq.AccountPasswordChangeReq{
		OldPassword: "password123", NewPassword: "new-password456",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.TokenVersion)
	_, err = service.Login(liveReq.Login{Phone: *account.Phone, Password: "password123"})
	require.ErrorIs(t, err, ErrInvalidCredentials)
	_, err = service.Login(liveReq.Login{Phone: *account.Phone, Password: "new-password456"})
	require.NoError(t, err)

	phone := "+60123456789"
	require.NoError(t, service.AdminUpdateProfile(liveReq.AccountAdminProfileUpdateReq{
		AccountID: account.ID, Phone: &phone, AccountProfileUpdateReq: profile,
	}))
	detail, err := service.GetAdminDetail(account.ID)
	require.NoError(t, err)
	require.Equal(t, phone, detail.Phone)
	require.Positive(t, detail.PhoneVerifiedAt)
	var phoneUpdated liveModel.LiveAccount
	require.NoError(t, global.GVA_DB.First(&phoneUpdated, account.ID).Error)
	require.Equal(t, uint64(3), phoneUpdated.TokenVersion)

	other, err := service.Register(liveReq.Register{Phone: "+60123456780", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)
	require.ErrorIs(t, service.AdminUpdateProfile(liveReq.AccountAdminProfileUpdateReq{
		AccountID: other.ID, Phone: &phone,
		AccountProfileUpdateReq: liveReq.AccountProfileUpdateReq{Nickname: "Other"},
	}), ErrPhoneExists)

	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveAccount{}).Where("id = ?", account.ID).
		Update("real_name_status", liveModel.LiveAccountRealNameVerified).Error)
	list, total, err := service.GetAdminList(&liveReq.AccountAdminListReq{
		PageInfo: commonPage(1, 20), Phone: phone,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, list, 1)
	require.Equal(t, phone, list[0].Phone)
	require.Contains(t, list[0].PhoneMasked, "****")
	require.Equal(t, "0", list[0].WalletAvailableBalance)
	require.False(t, list[0].WalletOpened)
	require.NoError(t, global.GVA_DB.Create(&walletModel.WalletAccount{AccountID: uint64(account.ID), CurrencyCode: walletModel.CurrencyCOIN, AvailableBalance: 123456, Status: walletModel.AccountNormal}).Error)
	list, _, err = service.GetAdminList(&liveReq.AccountAdminListReq{PageInfo: commonPage(1, 20), AccountNo: account.AccountNo})
	require.NoError(t, err)
	require.Equal(t, "123456", list[0].WalletAvailableBalance)
	require.Equal(t, walletModel.AccountNormal, list[0].WalletStatus)
	require.True(t, list[0].WalletOpened)

	banUntil := time.Now().Add(time.Hour).UnixMilli()
	status := liveModel.LiveAccountStatusBanned
	require.NoError(t, service.AdminUpdateStatus(liveReq.AccountAdminStatusUpdateReq{
		AccountID: account.ID, Status: &status, StatusReason: "risk control", BanUntil: banUntil,
	}))
	var banned liveModel.LiveAccount
	require.NoError(t, global.GVA_DB.First(&banned, account.ID).Error)
	require.Equal(t, liveModel.LiveAccountStatusBanned, banned.Status)
	require.Equal(t, uint64(4), banned.TokenVersion)
	_, err = service.Login(liveReq.Login{Phone: phone, Password: "new-password456"})
	require.ErrorIs(t, err, ErrAccountDisabled)

	// 到期的临时封禁应在下一次鉴权或登录时自动恢复。
	require.NoError(t, global.GVA_DB.Model(&banned).Update("ban_until", time.Now().Add(-time.Minute).UnixMilli()).Error)
	_, err = service.Login(liveReq.Login{Phone: phone, Password: "new-password456"})
	require.NoError(t, err)
}

func commonPage(page, pageSize int) commonReq.PageInfo {
	return commonReq.PageInfo{Page: page, PageSize: pageSize}
}

func TestRegistrationSuperCodeFailsClosed(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}
	for _, code := range []string{"", "000000", "12345", "1234567", " 123456", "１２３４５６"} {
		_, err := service.Register(liveReq.Register{Phone: "+60123456789", SmsCode: code, Password: "password123"})
		require.ErrorIs(t, err, ErrInvalidSMSCode)
	}
	for _, configured := range []string{"", "12345", " 123456", "secret"} {
		global.SetLiveAccountSuperCode(configured)
		_, err := service.Register(liveReq.Register{Phone: "+60123456789", SmsCode: configured, Password: "password123"})
		require.ErrorIs(t, err, ErrRegistrationUnavailable)
	}
	global.SetLiveAccountSuperCode("012345")
	account, err := service.Register(liveReq.Register{Phone: "+60123456789", SmsCode: "012345", Password: "password123"})
	require.NoError(t, err)
	require.Equal(t, "1145781", account.AccountNo, "校验失败不得占用账号编号")
	_, err = service.Login(liveReq.Login{Phone: *account.Phone, Password: "012345"})
	require.ErrorIs(t, err, ErrInvalidCredentials, "超级验证码不得用于登录")
}

func TestRegistrationRejectsDeletedPhoneAndInvalidBoundaries(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}
	input := liveReq.Register{Phone: "+60123456789", SmsCode: "123456", Password: "password123"}
	account, err := service.Register(input)
	require.NoError(t, err)
	require.NoError(t, global.GVA_DB.Delete(&account).Error)
	_, err = service.Register(input)
	require.ErrorIs(t, err, ErrPhoneExists)
	input.Phone = "+60123456780"
	input.Password = strings.Repeat("密", 25)
	_, err = service.Register(input)
	require.ErrorIs(t, err, ErrInvalidPassword, "bcrypt上限是字节数，不是字符数")
	input.Password = "password123"
	input.Nickname = strings.Repeat("名", 65)
	_, err = service.Register(input)
	require.ErrorIs(t, err, ErrInvalidProfile)
	input.Nickname = ""
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.RegisterWithContext(ctx, input)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPhoneOnlyLoginAndAdminPhoneReplacement(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}
	account, err := service.Register(liveReq.Register{Phone: "+60123456789", SmsCode: "123456", Password: "password123"})
	require.NoError(t, err)
	for _, identifier := range []string{account.Username, account.AccountNo, "+60123456780"} {
		_, err = service.Login(liveReq.Login{Phone: identifier, Password: "password123"})
		require.ErrorIs(t, err, ErrInvalidCredentials)
	}
	for _, value := range []string{"", " ", "+60123456780"} {
		err = service.AdminUpdateProfile(liveReq.AccountAdminProfileUpdateReq{
			AccountID: account.ID, Phone: &value, AccountProfileUpdateReq: liveReq.AccountProfileUpdateReq{Nickname: "用户"},
		})
		if strings.TrimSpace(value) == "" {
			require.ErrorIs(t, err, ErrPhoneRequired)
		} else {
			require.NoError(t, err)
		}
	}
	require.ErrorIs(t, service.AdminUnbindPhone(account.ID), ErrPhoneRequired)
	_, err = service.Login(liveReq.Login{Phone: *account.Phone, Password: "password123"})
	require.ErrorIs(t, err, ErrInvalidCredentials)
	loggedIn, err := service.Login(liveReq.Login{Phone: "+60123456780", Password: "password123"})
	require.NoError(t, err)
	require.Equal(t, uint64(2), loggedIn.TokenVersion)
	require.NoError(t, global.GVA_DB.Model(&account).UpdateColumn("phone_verified_at", 0).Error)
	_, err = service.Login(liveReq.Login{Phone: "+60123456780", Password: "password123"})
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestConcurrentRegistrationUsesUniqueNumbersAndPhones(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}
	type result struct {
		account liveModel.LiveAccount
		err     error
	}
	run := func(phones []string) []result {
		results := make([]result, len(phones))
		var wg sync.WaitGroup
		for index, phone := range phones {
			wg.Add(1)
			go func(index int, phone string) {
				defer wg.Done()
				results[index].account, results[index].err = service.Register(liveReq.Register{Phone: phone, SmsCode: "123456", Password: "password123"})
			}(index, phone)
		}
		wg.Wait()
		return results
	}
	phones := make([]string, 6)
	for i := range phones {
		phones[i] = fmt.Sprintf("+6012345678%d", i)
	}
	seen := make(map[string]bool)
	for _, result := range run(phones) {
		require.NoError(t, result.err)
		require.False(t, seen[result.account.AccountNo])
		seen[result.account.AccountNo] = true
	}
	successes := 0
	for _, result := range run([]string{"+60123456789", "+60123456789", "+60123456789"}) {
		if result.err == nil {
			successes++
		} else {
			require.ErrorIs(t, result.err, ErrPhoneExists)
		}
	}
	require.Equal(t, 1, successes)
	var sequence liveModel.LiveAccountNumberSequence
	require.NoError(t, global.GVA_DB.First(&sequence, 1).Error)
	require.Equal(t, uint64(1145788), sequence.NextValue, "失败的重复注册事务不能消耗编号")
}

func TestLoginRejectsCredentialsChangedAfterLookup(t *testing.T) {
	for _, change := range []map[string]interface{}{
		{"phone": "+60123456780"}, {"password": "replaced-hash"},
		{"token_version": 2}, {"status": liveModel.LiveAccountStatusDisabled},
	} {
		t.Run(fmt.Sprint(change), func(t *testing.T) {
			setupAccountTestDB(t)
			service := AccountService{}
			account, err := service.Register(liveReq.Register{Phone: "+60123456789", SmsCode: "123456", Password: "password123"})
			require.NoError(t, err)
			changed := false
			require.NoError(t, global.GVA_DB.Callback().Query().After("gorm:query").Register("test:credentials-change", func(tx *gorm.DB) {
				if !changed && tx.Statement.Table == "live_account" {
					changed = true
					require.NoError(t, global.GVA_DB.Model(&account).Updates(change).Error)
				}
			}))
			_, err = service.Login(liveReq.Login{Phone: "+60123456789", Password: "password123"})
			require.ErrorIs(t, err, ErrInvalidCredentials)
		})
	}
}
