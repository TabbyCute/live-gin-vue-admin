package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWalletConfigurationDefaultsAndValidation(t *testing.T) {
	c, e := (Wallet{}).Normalize()
	require.NoError(t, e)
	require.EqualValues(t, 1000000, c.MaxGrantAmount)
	require.Equal(t, 64, c.AppMaxInflight)
	require.Equal(t, 16, c.AdminMaxInflight)
	require.Equal(t, []uint{888}, c.ReviewAuthorities)
	for _, v := range []Wallet{{AppMaxInflight: -1}, {AdminMaxInflight: 4097}, {AppMaxInflight: 4097}, {AdminMaxInflight: -1}, {MaxGrantAmount: -1}, {AccountDailyLimit: -1}, {OperatorDailyLimit: -1}, {EvidenceThreshold: -1}, {MaxGrantAmount: 100, AccountDailyLimit: 1}, {MaxGrantAmount: 100, OperatorDailyLimit: 1}, {ReviewAuthorities: []uint{0}}} {
		_, e = v.Normalize()
		require.Error(t, e)
	}
	roles := []uint{888}
	c, e = (Wallet{ReviewAuthorities: roles}).Normalize()
	require.NoError(t, e)
	roles[0] = 1
	require.Equal(t, uint(888), c.ReviewAuthorities[0])
}
