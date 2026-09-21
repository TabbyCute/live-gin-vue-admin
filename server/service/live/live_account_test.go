package live

import (
	"errors"
	"testing"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}))
	previousDB := global.GVA_DB
	global.GVA_DB = db
	t.Cleanup(func() { global.GVA_DB = previousDB })
}

func TestAccountRegisterAndLogin(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}

	registered, err := service.Register(liveReq.Register{
		Username: " Demo_User ",
		Password: "password123",
	})
	require.NoError(t, err)
	require.Equal(t, "demo_user", registered.Username)
	require.Equal(t, "demo_user", registered.Nickname)
	require.NotEqual(t, "password123", registered.Password)

	loggedIn, err := service.Login(liveReq.Login{Username: "DEMO_USER", Password: "password123"})
	require.NoError(t, err)
	require.Equal(t, registered.ID, loggedIn.ID)
}

func TestAccountRegisterRejectsDuplicateAndInvalidInput(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}

	_, err := service.Register(liveReq.Register{Username: "demo", Password: "password123"})
	require.NoError(t, err)
	_, err = service.Register(liveReq.Register{Username: "DEMO", Password: "password456"})
	require.ErrorIs(t, err, ErrAccountExists)

	_, err = service.Register(liveReq.Register{Username: "bad-name", Password: "password123"})
	require.ErrorIs(t, err, ErrInvalidUsername)
	_, err = service.Register(liveReq.Register{Username: "valid_name", Password: "short"})
	require.ErrorIs(t, err, ErrInvalidPassword)
}

func TestAccountLoginRejectsInvalidCredentialsAndDisabledAccount(t *testing.T) {
	setupAccountTestDB(t)
	service := AccountService{}
	account, err := service.Register(liveReq.Register{Username: "demo", Password: "password123"})
	require.NoError(t, err)

	_, err = service.Login(liveReq.Login{Username: "demo", Password: "wrong-password"})
	require.ErrorIs(t, err, ErrInvalidCredentials)

	require.NoError(t, global.GVA_DB.Model(&account).Update("status", liveModel.LiveAccountStatusDisabled).Error)
	_, err = service.Login(liveReq.Login{Username: "demo", Password: "password123"})
	require.True(t, errors.Is(err, ErrAccountDisabled))
}
