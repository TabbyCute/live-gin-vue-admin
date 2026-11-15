package initialize

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterPublicHTML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPublicHTML(router)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, publicHTMLRoute+"/html-5.html", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	require.Contains(t, recorder.Body.String(), "SRS 推流、断流与主播重连测试")
}

func TestRegisterPublicHTMLDoesNotListDirectory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPublicHTML(router)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, publicHTMLRoute+"/", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.False(t, strings.Contains(recorder.Body.String(), "play.html"))
}

func TestRegisterPublicAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminFiles := fstest.MapFS{
		"index.html":    {Data: []byte("<html><title>Admin</title></html>")},
		"assets/app.js": {Data: []byte("console.log('admin')")},
	}
	registerPublicAdmin(router, http.FS(adminFiles))

	tests := []struct {
		name             string
		target           string
		wantContent      string
		wantCacheControl string
	}{
		{name: "path without trailing slash", target: publicAdminRoute, wantContent: "<title>Admin</title>", wantCacheControl: "no-cache"},
		{name: "path with trailing slash", target: publicAdminRoute + "/", wantContent: "<title>Admin</title>", wantCacheControl: "no-cache"},
		{name: "bundled asset", target: publicAdminRoute + "/assets/app.js", wantContent: "console.log('admin')", wantCacheControl: "public, max-age=31536000, immutable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.target, nil)
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), tt.wantContent)
			require.Equal(t, tt.wantCacheControl, recorder.Header().Get("Cache-Control"))
		})
	}

	directoryRecorder := httptest.NewRecorder()
	directoryRequest := httptest.NewRequest(http.MethodGet, publicAdminRoute+"/assets/", nil)
	router.ServeHTTP(directoryRecorder, directoryRequest)
	require.Equal(t, http.StatusNotFound, directoryRecorder.Code)

}

func TestRegisterPublicH5(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPublicAdmin(router, http.FS(fstest.MapFS{
		"index.html": {Data: []byte("<title>Admin</title>")},
	}))
	registerPublicH5(router, http.FS(fstest.MapFS{
		"index.html":         {Data: []byte("<title>H5</title>")},
		"assets/js/app.js":   {Data: []byte("console.log('h5')")},
		"cdn_scripts/hls.js": {Data: []byte("window.Hls = {}")},
		".env":               {Data: []byte("private config")},
	}))

	for _, tt := range []struct {
		name, method, target string
		status               int
		content, cache       string
	}{
		{"home", http.MethodGet, publicH5Route, 200, "<title>H5</title>", "no-cache"},
		{"trailing slash", http.MethodGet, publicH5Route + "/", 200, "<title>H5</title>", "no-cache"},
		{"entry", http.MethodGet, publicH5Route + "/index.html", 200, "<title>H5</title>", "no-cache"},
		{"refresh page", http.MethodGet, publicH5Route + "/home", 200, "<title>H5</title>", "no-cache"},
		{"refresh room", http.MethodGet, publicH5Route + "/live/room/1145781?from=share", 200, "<title>H5</title>", "no-cache"},
		{"head page", http.MethodHead, publicH5Route + "/login", 200, "", "no-cache"},
		{"asset", http.MethodGet, publicH5Route + "/assets/js/app.js", 200, "console.log('h5')", "public, max-age=31536000, immutable"},
		{"unhashed script", http.MethodGet, publicH5Route + "/cdn_scripts/hls.js", 200, "window.Hls = {}", "no-cache"},
		{"missing asset", http.MethodGet, publicH5Route + "/assets/js/missing.js", 404, "", ""},
		{"asset path without extension", http.MethodGet, publicH5Route + "/assets/missing", 404, "", ""},
		{"missing script", http.MethodGet, publicH5Route + "/cdn_scripts/missing", 404, "", ""},
		{"directory listing", http.MethodGet, publicH5Route + "/assets/", 404, "", ""},
		{"hidden file", http.MethodGet, publicH5Route + "/.env", 404, "", ""},
		{"traversal", http.MethodGet, publicH5Route + "/../admin/index.html", 404, "", ""},
		{"encoded traversal", http.MethodGet, publicH5Route + "/%2e%2e/admin/index.html", 404, "", ""},
		{"admin still isolated", http.MethodGet, publicAdminRoute, 200, "<title>Admin</title>", "no-cache"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.target, nil))
			require.Equal(t, tt.status, recorder.Code)
			require.Equal(t, tt.content, recorder.Body.String())
			require.Equal(t, tt.cache, recorder.Header().Get("Cache-Control"))
		})
	}
}

func TestRegisterPublicH5WithoutBuild(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPublicH5(router, http.FS(fstest.MapFS{}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, publicH5Route+"/login", nil))
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
