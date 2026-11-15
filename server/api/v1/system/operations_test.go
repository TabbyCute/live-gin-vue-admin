package system

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	req "tb_live_module/model/system/request"
	"testing"
)

func TestOperationsRejectsMissingClaimsAndUnexpectedInput(t *testing.T) {
	for _, tc := range []struct {
		actor  uint
		path   string
		status int
	}{{0, "/ops", 403}, {2, "/ops", 403}, {1, "/ops?secret=ignored", 400}} {
		r := gin.New()
		r.GET("/ops", func(c *gin.Context) {
			if tc.actor > 0 {
				c.Set("claims", &req.CustomClaims{BaseClaims: req.BaseClaims{ID: tc.actor}})
			}
		}, (&SystemApi{}).Operations)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		require.Equal(t, tc.status, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}
