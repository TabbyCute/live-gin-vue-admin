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
