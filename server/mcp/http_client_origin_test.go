package mcpTool

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"tb_live_module/global"
)

func TestMCPToolBackendRedirectDoesNotLeakAdminToken(t *testing.T) {
	old := global.GVA_CONFIG.MCP
	t.Cleanup(func() { global.GVA_CONFIG.MCP = old })
	global.GVA_CONFIG.MCP.AuthHeader = "X-Token"
	for _, targetKind := range []string{"other_host", "same_host_other_port"} {
		for _, status := range []int{301, 302, 303, 307, 308} {
			t.Run(fmt.Sprintf("%s/%d", targetKind, status), func(t *testing.T) {
				var attempts, leaked, original atomic.Int32
				foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts.Add(1)
					if r.Header.Get("X-Token") != "" {
						leaked.Add(1)
					}
					_, _ = w.Write([]byte(`{"code":0,"data":{},"msg":"ok"}`))
				}))
				defer foreign.Close()
				location := foreign.URL
				if targetKind == "other_host" {
					location = strings.Replace(location, "127.0.0.1", "localhost", 1)
				}
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					original.Add(1)
					require.Equal(t, "local-only-admin-token", r.Header.Get("X-Token"))
					http.Redirect(w, r, location, status)
				}))
				defer origin.Close()
				global.GVA_CONFIG.MCP.UpstreamBaseURL = origin.URL
				ctx := context.WithValue(context.Background(), authTokenContextKey, "local-only-admin-token")
				_, err := postUpstream[map[string]any](ctx, "/fixture", map[string]string{"value": "local"})
				require.ErrorContains(t, err, "重定向已拒绝")
				require.EqualValues(t, 1, original.Load())
				require.Zero(t, attempts.Load())
				require.Zero(t, leaked.Load())
			})
		}
	}
}

func TestMCPToolBackendNormalRequestRemainsCompatible(t *testing.T) {
	old := global.GVA_CONFIG.MCP
	t.Cleanup(func() { global.GVA_CONFIG.MCP = old })
	global.GVA_CONFIG.MCP.AuthHeader = "X-Token"
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/fixture", r.URL.Path)
		require.Equal(t, "local-only-admin-token", r.Header.Get("X-Token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"value":"works"},"msg":"ok"}`))
	}))
	defer origin.Close()
	global.GVA_CONFIG.MCP.UpstreamBaseURL = origin.URL
	ctx := context.WithValue(context.Background(), authTokenContextKey, "local-only-admin-token")
	result, err := getUpstream[map[string]string](ctx, "/fixture", nil)
	require.NoError(t, err)
	require.Equal(t, "works", result.Data["value"])
}

func TestMCPHealthDoesNotFollowCrossOriginLocation(t *testing.T) {
	old := global.GVA_CONFIG.MCP
	t.Cleanup(func() { global.GVA_CONFIG.MCP = old })
	var foreignRequests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { foreignRequests.Add(1); w.WriteHeader(200) }))
	defer foreign.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/health", r.URL.Path)
		http.Redirect(w, r, foreign.URL, 307)
	}))
	defer origin.Close()
	global.GVA_CONFIG.MCP.BaseURL = origin.URL + "/mcp"
	reachable, err := checkMCPHealth(context.Background())
	require.Error(t, err)
	require.False(t, reachable)
	require.Zero(t, foreignRequests.Load())
}
