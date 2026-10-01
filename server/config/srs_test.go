package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLiveAndSRSConfigUnmarshal(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(`
live:
  reconnect-window-seconds: 20
  push-token-seconds: 180
  publish-token-key: publish-token-secret
  end-scan-interval-seconds: 5
  end-scan-batch-size: 100
  end-retry-base-seconds: 5
  end-retry-max-seconds: 60
  srs:
    app: live
    http_apis: http://127.0.0.1:1985
    api-username: admin
    api-password: secret
    api-request-timeout-seconds: 3
    push-base-url: rtmp://127.0.0.1/live
    play-base-url: https://127.0.0.1/live
`)))

	var cfg Server
	require.NoError(t, v.Unmarshal(&cfg))
	require.Equal(t, 20, cfg.Live.ReconnectWindowSeconds)
	require.Equal(t, 180, cfg.Live.PushTokenSeconds)
	require.Equal(t, "publish-token-secret", cfg.Live.PublishTokenKey)
	require.Equal(t, 5, cfg.Live.EndScanIntervalSeconds)
	require.Equal(t, 100, cfg.Live.EndScanBatchSize)
	require.Equal(t, 5, cfg.Live.EndRetryBaseSeconds)
	require.Equal(t, 60, cfg.Live.EndRetryMaxSeconds)
	require.Equal(t, "live", cfg.Live.SRS.App)
	require.Equal(t, "http://127.0.0.1:1985", cfg.Live.SRS.HTTPAPIs)
	require.Equal(t, "admin", cfg.Live.SRS.APIUsername)
	require.Equal(t, "secret", cfg.Live.SRS.APIPassword)
	require.Equal(t, 3, cfg.Live.SRS.APIRequestTimeoutSeconds)
	require.Equal(t, "rtmp://127.0.0.1/live", cfg.Live.SRS.PushBaseURL)
	require.Equal(t, "https://127.0.0.1/live", cfg.Live.SRS.PlayBaseURL)
}

func TestEnvironmentConfigsUseNestedSRS(t *testing.T) {
	for _, filename := range []string{"config.dev.yaml", "config.prod.yaml", "config.docker.yaml"} {
		t.Run(filename, func(t *testing.T) {
			v := viper.New()
			v.SetConfigFile("../" + filename)
			require.NoError(t, v.ReadInConfig())
			require.True(t, v.InConfig("live.srs.app"))
			require.True(t, v.InConfig("live.push-token-seconds"))
			require.True(t, v.InConfig("live.publish-token-key"))
			require.True(t, v.InConfig("live.end-scan-interval-seconds"))
			require.True(t, v.InConfig("live.end-scan-batch-size"))
			require.True(t, v.InConfig("live.end-retry-base-seconds"))
			require.True(t, v.InConfig("live.end-retry-max-seconds"))
			require.True(t, v.InConfig("live.srs.http_apis"))
			require.True(t, v.InConfig("live.srs.api-username"))
			require.True(t, v.InConfig("live.srs.api-password"))
			require.True(t, v.InConfig("live.srs.api-request-timeout-seconds"))
			require.True(t, v.InConfig("live.srs.push-base-url"))
			require.True(t, v.InConfig("live.srs.play-base-url"))
			require.False(t, v.InConfig("live.srs.hook-token"))
		})
	}
}
