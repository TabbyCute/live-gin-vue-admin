package live

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	systemReq "tb_live_module/model/system/request"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAccountAdminRoutesRequireTrustedAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAccount{}))
	previousDB := global.GVA_DB
	global.GVA_DB = db
	t.Cleanup(func() { global.GVA_DB = previousDB })

	requestList := func(authorityID uint) *httptest.ResponseRecorder {
		engine := gin.New()
		group := engine.Group("/")
		group.Use(func(c *gin.Context) {
			c.Set("claims", &systemReq.CustomClaims{BaseClaims: systemReq.BaseClaims{AuthorityId: authorityID}})
			c.Next()
		})
		new(AccountAdminRouter).InitAccountAdminRouter(group)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/live/account/list?page=1&pageSize=10", nil)
		engine.ServeHTTP(recorder, request)
		return recorder
	}

	require.Equal(t, http.StatusUnauthorized, requestList(9528).Code)
	require.Equal(t, http.StatusOK, requestList(888).Code)
}
