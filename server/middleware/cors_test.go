package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tb_live_module/config"
	"tb_live_module/global"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCorsByRulesAllowsServerRequestsWithoutOrigin(t *testing.T) {
	restoreCorsConfig(t)
	global.GVA_CONFIG.Cors = config.CORS{Mode: "strict-whitelist"}
	called := false
	router := gin.New()
	router.Use(CorsByRules())
	router.POST("/hook", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hook", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, called)
	require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
}

func TestCorsByRulesPreflightAllowsRequiredIdempotencyHeader(t *testing.T) {
	restoreCorsConfig(t)
	global.GVA_CONFIG.Cors = config.CORS{
		Mode: "strict-whitelist",
		Whitelist: []config.CORSWhitelist{{
			AllowOrigin: "https://app.example.com", AllowHeaders: "Content-Type, Authorization",
			AllowMethods: "POST, OPTIONS", AllowCredentials: true,
		}},
	}
	called := false
	router := gin.New()
	router.Use(CorsByRules())
	router.POST("/prepare", func(c *gin.Context) { called = true })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/prepare", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "authorization,idempotency-key")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.False(t, called)
	require.Equal(t, "https://app.example.com", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")), "idempotency-key")
	require.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
	require.ElementsMatch(t,
		[]string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"},
		recorder.Header().Values("Vary"),
	)
}

func TestCorsByRulesRejectsUnknownBrowserOrigin(t *testing.T) {
	restoreCorsConfig(t)
	global.GVA_CONFIG.Cors = config.CORS{
		Mode:      "strict-whitelist",
		Whitelist: []config.CORSWhitelist{{AllowOrigin: "https://app.example.com"}},
	}
	called := false
	router := gin.New()
	router.Use(CorsByRules())
	router.POST("/prepare", func(c *gin.Context) { called = true })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/prepare", nil)
	request.Header.Set("Origin", "https://evil.example.com")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.False(t, called)
}

func TestAllowAllCorsIncludesIdempotencyHeader(t *testing.T) {
	restoreCorsConfig(t)
	global.GVA_CONFIG.Cors = config.CORS{Mode: "allow-all"}
	router := gin.New()
	router.Use(CorsByRules())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/prepare", nil)
	request.Header.Set("Origin", "http://localhost:8080")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Contains(t, strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")), "idempotency-key")
}

func restoreCorsConfig(t *testing.T) {
	t.Helper()
	previous := global.GVA_CONFIG.Cors
	t.Cleanup(func() { global.GVA_CONFIG.Cors = previous })
}
