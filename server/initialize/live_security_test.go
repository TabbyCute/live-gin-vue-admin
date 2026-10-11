package initialize

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"tb_live_module/global"
	"tb_live_module/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const initializedSRSHookToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestResolveSRSHookTokenPrefersExplicitEnvironment(t *testing.T) {
	token, source, err := resolveSRSHookToken("configured-token", func(name string) (string, bool) {
		require.Equal(t, middleware.SRSHookTokenEnv, name)
		return "environment-token", true
	})
	require.NoError(t, err)
	require.Equal(t, "environment-token", token)
	require.Contains(t, source, middleware.SRSHookTokenEnv)
}

func TestResolveSRSHookTokenDoesNotFallbackFromEmptyEnvironment(t *testing.T) {
	_, _, err := resolveSRSHookToken("configured-token", func(string) (string, bool) {
		return "", true
	})
	require.Error(t, err)
}

func TestResolveSRSHookTokenUsesConfigOnlyWhenEnvironmentIsAbsent(t *testing.T) {
	token, source, err := resolveSRSHookToken("configured-token", func(string) (string, bool) {
		return "", false
	})
	require.NoError(t, err)
	require.Equal(t, "configured-token", token)
	require.Equal(t, "live.srs-hook-token", source)
}

func TestResolveSRSHookTokenFailsClosedWhenMissing(t *testing.T) {
	_, _, err := resolveSRSHookToken("", func(string) (string, bool) {
		return "", false
	})
	require.Error(t, err)
}

func TestInitializeLiveSecurityFreezesStartupToken(t *testing.T) {
	previousLive := global.GVA_CONFIG.Live
	liveSecurityOnce = sync.Once{}
	liveSecurityInitErr = nil
	srsHookAuthenticator = nil
	t.Cleanup(func() {
		global.GVA_CONFIG.Live = previousLive
		liveSecurityOnce = sync.Once{}
		liveSecurityInitErr = nil
		srsHookAuthenticator = nil
	})

	t.Setenv(middleware.SRSHookTokenEnv, initializedSRSHookToken)
	require.NoError(t, InitializeLiveSecurity())
	t.Setenv(middleware.SRSHookTokenEnv, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	require.NoError(t, InitializeLiveSecurity())

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/hook", requireSRSHookAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	accepted := httptest.NewRecorder()
	acceptedRequest := httptest.NewRequest(http.MethodPost, "/hook", nil)
	acceptedRequest.Header.Set(middleware.SRSHookTokenHeader, initializedSRSHookToken)
	router.ServeHTTP(accepted, acceptedRequest)
	require.Equal(t, http.StatusNoContent, accepted.Code)

	rejected := httptest.NewRecorder()
	rejectedRequest := httptest.NewRequest(http.MethodPost, "/hook", nil)
	rejectedRequest.Header.Set(middleware.SRSHookTokenHeader, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	router.ServeHTTP(rejected, rejectedRequest)
	require.Equal(t, http.StatusUnauthorized, rejected.Code)
}
