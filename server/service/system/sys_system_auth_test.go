package system

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"tb_live_module/config"
	"tb_live_module/global"
	"tb_live_module/model/system"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestSavingSystemConfigPreservesFileOnlyAccountAuth(t *testing.T) {
	previous := global.GVA_VP
	t.Cleanup(func() { global.GVA_VP = previous })
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(filepath.Join(t.TempDir(), "config.yaml"))
	v.Set("live-account-auth.super-code", "012345")
	v.Set("live-account-auth.trusted-proxies", []string{"127.0.0.1"})
	require.NoError(t, v.WriteConfigAs(v.ConfigFileUsed()))
	global.GVA_VP = v
	require.NoError(t, (&SystemConfigService{}).SetSystemConfig(system.System{Config: config.Server{}}))
	actual := viper.New()
	actual.SetConfigFile(v.ConfigFileUsed())
	require.NoError(t, actual.ReadInConfig())
	require.Equal(t, "012345", actual.GetString("live-account-auth.super-code"))
	require.Equal(t, []string{"127.0.0.1"}, actual.GetStringSlice("live-account-auth.trusted-proxies"))
}

func TestSystemConfigCannotReadOrReplacePayoutKeys(t *testing.T) {
	previous, previousConfig := global.GVA_VP, global.GVA_CONFIG
	t.Cleanup(func() { global.GVA_VP, global.GVA_CONFIG = previous, previousConfig })
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(filepath.Join(t.TempDir(), "config.yaml"))
	v.Set("payout.active-key-id", "originalKey")
	v.Set("payout.encryption-keys", `{"originalKey":"private-aes-fixture"}`)
	v.Set("payout.fingerprint-key", "private-hmac-fixture")
	require.NoError(t, v.WriteConfigAs(v.ConfigFileUsed()))
	require.NoError(t, v.Unmarshal(&global.GVA_CONFIG))
	global.GVA_VP = v
	service := &SystemConfigService{}
	view, err := service.GetSystemConfig()
	require.NoError(t, err)
	b, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(b), "payout")
	require.NotContains(t, string(b), "private-")
	for _, submitted := range []config.Payout{{}, {ActiveKeyID: "forged", EncryptionKeys: "forged", FingerprintKey: "forged"}} {
		require.NoError(t, service.SetSystemConfig(system.System{Config: config.Server{Payout: submitted}}))
		actual := viper.New()
		actual.SetConfigFile(v.ConfigFileUsed())
		require.NoError(t, actual.ReadInConfig())
		require.Equal(t, "originalKey", actual.GetString("payout.active-key-id"))
		require.Equal(t, `{"originalKey":"private-aes-fixture"}`, actual.GetString("payout.encryption-keys"))
		require.Equal(t, "private-hmac-fixture", actual.GetString("payout.fingerprint-key"))
	}
}
