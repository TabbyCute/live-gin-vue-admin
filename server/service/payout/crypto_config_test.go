package payout

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"tb_live_module/config"
	model "tb_live_module/model/payout"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestCryptoReadsYAMLDirectlyAndIgnoresEnvironment(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	fp := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32)))
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader("payout:\n  active-key-id: CaseSensitiveKey\n  encryption-keys: '{\"CaseSensitiveKey\":\""+key+"\"}'\n  fingerprint-key: '"+fp+"'\n")))
	var cfg config.Server
	require.NoError(t, v.Unmarshal(&cfg))
	// Even stale deployment environment variables cannot override YAML keys.
	t.Setenv("PAYOUT_ACTIVE_KEY_ID", "wrong_environment_key")
	t.Setenv("PAYOUT_ENCRYPTION_KEYS", "invalid environment JSON")
	t.Setenv("PAYOUT_FINGERPRINT_KEY", "invalid environment fingerprint")
	c, err := LoadCrypto(cfg.Payout)
	require.NoError(t, err)
	require.Equal(t, "CaseSensitiveKey", c.active)
	p := model.PayeeAccount{PayeeAccountNo: "yaml-config-test", PayloadVersion: 1}
	want := payload{Account: "isolated-configuration-test"}
	require.NoError(t, c.seal(&p, want))
	got, err := c.open(p)
	require.NoError(t, err)
	require.Equal(t, want, got)

	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	for _, secret := range []string{"payout", key, fp, "CaseSensitiveKey"} {
		require.NotContains(t, string(encoded), secret)
	}
	var submitted config.Server
	require.NoError(t, json.Unmarshal([]byte(`{"payout":{"active-key-id":"forged","encryption-keys":"forged","fingerprint-key":"forged"}}`), &submitted))
	require.Equal(t, config.Payout{}, submitted.Payout)

	for _, bad := range []config.Payout{
		{},
		{ActiveKeyID: "CaseSensitiveKey", EncryptionKeys: "not-json-private-material", FingerprintKey: fp},
		{ActiveKeyID: "missing", EncryptionKeys: cfg.Payout.EncryptionKeys, FingerprintKey: fp},
		{ActiveKeyID: cfg.Payout.ActiveKeyID, EncryptionKeys: cfg.Payout.EncryptionKeys, FingerprintKey: key},
	} {
		_, err := LoadCrypto(bad)
		require.ErrorIs(t, err, ErrCrypto)
		require.NotContains(t, err.Error(), key)
		require.NotContains(t, err.Error(), fp)
		require.NotContains(t, err.Error(), "not-json-private-material")
	}
}
