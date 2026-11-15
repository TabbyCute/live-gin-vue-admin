package anchorhome

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"tb_live_module/global"
	"tb_live_module/model/live"
	"testing"
)

func TestPublicHomeHTTPAuthenticationAndQueryBoundary(t *testing.T) {
	r, token, _, _ := albumHTTPFixture(t)
	(RouterGroup{}).InitAnchorHomePublicRouter(r.Group("/v1/app"))
	for _, tc := range []struct {
		path, bearer string
		status       int
		code         float64
	}{
		{"/v1/app/anchor-home/profile?anchorNo=http_anchor", "broken", 401, 7},
		{"/v1/app/anchor-home/settings", "", 401, 7},
		{"/v1/app/anchor-home/profile?anchorNo=a&anchorNo=b", "", 200, 49001},
		{"/v1/app/anchor-home/profile?anchorNo=http_anchor&userId=10", token, 200, 49001},
		{"/v1/app/anchor-home/profile?anchorNo=" + strings.Repeat("a", 3000), "", 400, 49001},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, tc.path)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		var out map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		require.Equal(t, tc.code, out["code"], w.Body.String())
	}
}

func TestSettingsHTTPRejectsPartialNullUnknownDuplicateAndAdminToken(t *testing.T) {
	r, owner, admin, s := albumHTTPFixture(t)
	require.NoError(t, s.DB.AutoMigrate(&live.LiveCategory{}))
	(RouterGroup{}).InitAnchorHomePublicRouter(r.Group("/v1/app"))
	for _, body := range []string{
		`{"version":"` + strings.Repeat("a", 64) + `","categoryId":0}`,
		`{"version":"` + strings.Repeat("a", 64) + `","categoryId":null,"signature":""}`,
		`{"version":"` + strings.Repeat("a", 64) + `","categoryId":0,"signature":null}`,
		`{"version":"a","categoryId":0,"signature":"a","signature":"b"}`,
		`{"version":"a","categoryId":0,"signature":"a","userId":11}`,
		`{"version":"a","categoryId":0,"signature":"a"}{}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/app/anchor-home/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+owner)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Contains(t, w.Body.String(), `"code":49001`)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/app/anchor-home/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-token", admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 401, w.Code)
	var a live.LiveAnchor
	require.NoError(t, global.GVA_DB.First(&a, 10).Error)
	require.Empty(t, a.Signature)
}
