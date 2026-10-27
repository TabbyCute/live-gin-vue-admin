package initialize

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSwaggerPreservesExplicitVersionAndPrefixesLegacyAdmin(t *testing.T) {
	raw := normalizeSwaggerPaths(`{"basePath":"/api","paths":{"/base/login":{},"/live/room/list":{},"/v1/app/social/follow":{},"/v1/admin/social/health":{}}}`)
	var doc struct {
		BasePath string                 `json:"basePath"`
		Paths    map[string]interface{} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &doc))
	require.Equal(t, "/api", doc.BasePath)
	require.Contains(t, doc.Paths, "/v1/admin/base/login")
	require.Contains(t, doc.Paths, "/v1/admin/live/room/list")
	require.Contains(t, doc.Paths, "/v1/app/social/follow")
	require.Contains(t, doc.Paths, "/v1/admin/social/health")
}
