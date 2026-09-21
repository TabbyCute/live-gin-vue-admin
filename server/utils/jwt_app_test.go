package utils

import (
	"testing"

	"tb_live_module/config"
	"tb_live_module/global"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func setupAppJWTConfig(t *testing.T) {
	t.Helper()
	previousJWT := global.GVA_CONFIG.JWT
	previousAppJWT := global.GVA_CONFIG.JWTApp
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "admin-signing-key-for-tests"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{
		SigningKey:  "app-signing-key-for-tests",
		ExpiresTime: "1h",
		Issuer:      "live-app-tests",
	}
	t.Cleanup(func() {
		global.GVA_CONFIG.JWT = previousJWT
		global.GVA_CONFIG.JWTApp = previousAppJWT
	})
}

func TestAppJWTCreateAndParse(t *testing.T) {
	setupAppJWTConfig(t)

	token, createdClaims, err := NewAppJWT().CreateToken(12, "demo")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, uint(12), createdClaims.AccountID)

	parsedClaims, err := NewAppJWT().ParseToken(token)
	require.NoError(t, err)
	require.Equal(t, uint(12), parsedClaims.AccountID)
	require.Equal(t, "demo", parsedClaims.Username)
}

func TestAppJWTRejectsAdminKeyReuse(t *testing.T) {
	setupAppJWTConfig(t)
	global.GVA_CONFIG.JWTApp.SigningKey = global.GVA_CONFIG.JWT.SigningKey

	_, _, err := NewAppJWT().CreateToken(12, "demo")
	require.ErrorIs(t, err, ErrAppJWTSigningKeyNotSplit)
}

func TestAppAndAdminTokensAreNotInterchangeable(t *testing.T) {
	setupAppJWTConfig(t)

	adminToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "1"}).SignedString([]byte(global.GVA_CONFIG.JWT.SigningKey))
	require.NoError(t, err)
	_, err = NewAppJWT().ParseToken(adminToken)
	require.ErrorIs(t, err, ErrAppTokenInvalid)

	appToken, _, err := NewAppJWT().CreateToken(12, "demo")
	require.NoError(t, err)
	_, err = NewJWT().ParseToken(appToken)
	require.Error(t, err)
}
