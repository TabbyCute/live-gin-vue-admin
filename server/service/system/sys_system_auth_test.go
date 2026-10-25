package system

import (
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
