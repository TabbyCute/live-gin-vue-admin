package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestStandaloneMCPListenDefaultsAndExplicitConfig(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, want string
	}{
		{name: "omitted", yaml: "mcp: {}", want: "127.0.0.1"},
		{name: "empty", yaml: "mcp:\n  listen-host: ''", want: "127.0.0.1"},
		{name: "whitespace", yaml: "mcp:\n  listen-host: '  '", want: "127.0.0.1"},
		{name: "explicit_external", yaml: "mcp:\n  listen-host: 0.0.0.0", want: "0.0.0.0"},
		{name: "explicit_ipv6", yaml: "mcp:\n  listen-host: '::1'", want: "::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg standaloneConfig
			require.NoError(t, yaml.Unmarshal([]byte(tc.yaml), &cfg))
			applyStandaloneDefaults(filepath.Join(t.TempDir(), "mcp.yaml"), &cfg)
			require.Equal(t, tc.want, cfg.MCP.ListenHost)
			require.Equal(t, 8889, cfg.MCP.Addr)
		})
	}
}
