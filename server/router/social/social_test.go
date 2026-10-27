package social

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	systemReq "tb_live_module/model/system/request"
	"testing"
)

func TestSocialRoutesRejectAnonymousAndOnlyGetPost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	new(SocialRouter).InitSocialRouter(engine.Group("/api/v1/app"))
	removed := httptest.NewRecorder()
	engine.ServeHTTP(removed, httptest.NewRequest(http.MethodGet, "/api/v1/app/social/favorite/list", nil))
	require.Equal(t, http.StatusNotFound, removed.Code)
	for _, route := range engine.Routes() {
		require.Contains(t, []string{"GET", "POST"}, route.Method)
		r := httptest.NewRecorder()
		request := httptest.NewRequest(route.Method, route.Path, strings.NewReader(`{"accountNo":"10000001","userId":1}`))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(r, request)
		require.Equal(t, http.StatusUnauthorized, r.Code, route.Path)
	}
}
func TestSocialAdminAllRoutesRequireAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1/admin")
	group.Use(func(c *gin.Context) {
		c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{AuthorityId: 9528}})
		c.Next()
	})
	new(SocialAdminRouter).InitSocialAdminRouter(group)
	for _, route := range engine.Routes() {
		r := httptest.NewRecorder()
		engine.ServeHTTP(r, httptest.NewRequest(route.Method, route.Path, nil))
		require.Equal(t, http.StatusUnauthorized, r.Code, route.Path)
	}
}
