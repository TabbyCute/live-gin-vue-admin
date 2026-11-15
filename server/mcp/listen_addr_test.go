package mcpTool

import (
	"testing"

	"github.com/stretchr/testify/require"
	"tb_live_module/config"
	"tb_live_module/global"
)

func TestMCPListenAddressDefaultsToLoopback(t *testing.T) {
	old := global.GVA_CONFIG.MCP
	t.Cleanup(func() { global.GVA_CONFIG.MCP = old })
	for _, tc := range []struct {
		name, host, want string
		port             int
	}{
		{name: "omitted", want: "127.0.0.1:8889"},
		{name: "whitespace", host: " \t", want: "127.0.0.1:8889"},
		{name: "negative_port", port: -1, want: "127.0.0.1:8889"},
		{name: "explicit_external", host: "0.0.0.0", port: 9999, want: "0.0.0.0:9999"},
		{name: "explicit_address", host: " 127.0.0.2 ", port: 9999, want: "127.0.0.2:9999"},
		{name: "ipv6_loopback", host: "::1", port: 9999, want: "[::1]:9999"},
		{name: "ipv6_external", host: "::", port: 9999, want: "[::]:9999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global.GVA_CONFIG.MCP = config.MCP{ListenHost: tc.host, Addr: tc.port}
			require.Equal(t, tc.want, ResolveMCPListenAddr())
		})
	}
}

func TestMCPHealthFallbackRemainsLocalAfterListenChange(t *testing.T) {
	old := global.GVA_CONFIG.MCP
	t.Cleanup(func() { global.GVA_CONFIG.MCP = old })
	global.GVA_CONFIG.MCP = config.MCP{ListenHost: "0.0.0.0", BaseURL: "%invalid", Addr: 9999}
	require.Equal(t, "http://127.0.0.1:9999/health", ResolveMCPHealthURL())
	global.GVA_CONFIG.MCP = config.MCP{ListenHost: "::1", BaseURL: "http://[::1]:9999/mcp?value=ignored#fragment", Addr: 9999}
	require.Equal(t, "http://[::1]:9999/health", ResolveMCPHealthURL())
}
