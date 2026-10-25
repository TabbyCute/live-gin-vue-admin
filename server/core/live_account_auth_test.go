package core

import (
	"encoding/json"
	"os"
	"testing"

	"tb_live_module/global"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLiveAccountSuperCodeConfigurationAndSecrecy(t *testing.T) {
	previousConfig, previousCode := global.GVA_CONFIG.LiveAccountAuth, global.LiveAccountSuperCode()
	t.Cleanup(func() {
		global.GVA_CONFIG.LiveAccountAuth = previousConfig
		global.SetLiveAccountSuperCode(previousCode)
	})
	// t.Setenv负责恢复原环境，先显式取消以验证配置文件回退。
	t.Setenv("LIVE_ACCOUNT_SUPER_CODE", "")
	require.NoError(t, os.Unsetenv("LIVE_ACCOUNT_SUPER_CODE"))
	v := viper.New()
	v.Set("live-account-auth.super-code", "012345")
	require.NoError(t, v.UnmarshalKey("live-account-auth", &global.GVA_CONFIG.LiveAccountAuth))
	loadLiveAccountSuperCode(v)
	require.Equal(t, "012345", global.LiveAccountSuperCode())
	encoded, err := json.Marshal(global.GVA_CONFIG)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "012345")
	require.NotContains(t, string(encoded), "live-account-auth")
	t.Setenv("LIVE_ACCOUNT_SUPER_CODE", "654321")
	loadLiveAccountSuperCode(v)
	require.Equal(t, "654321", global.LiveAccountSuperCode())
	t.Setenv("LIVE_ACCOUNT_SUPER_CODE", "")
	loadLiveAccountSuperCode(v)
	require.Empty(t, global.LiveAccountSuperCode(), "显式空环境变量不能回退到文件超级验证码")
	require.NoError(t, os.Unsetenv("LIVE_ACCOUNT_SUPER_CODE"))
	loadLiveAccountSuperCode(viper.New())
	require.Empty(t, global.LiveAccountSuperCode(), "删除配置项时不能继续使用旧验证码")
}
