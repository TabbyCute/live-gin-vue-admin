package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	systemReq "tb_live_module/model/system/request"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequireAuthorityRejectsOrdinaryAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	reached := false
	router.POST("/dangerous", func(c *gin.Context) {
		c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{AuthorityId: 9528}})
	}, RequireAuthority(888), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dangerous", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.False(t, reached)
	require.Contains(t, recorder.Body.String(), "权限不足")
}

func TestRequireAuthorityAllowsTrustedAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	reached := false
	router.POST("/dangerous", func(c *gin.Context) {
		c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{AuthorityId: 888}})
	}, RequireAuthority(888), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dangerous", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.True(t, reached)
}
