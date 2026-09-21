package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
)

func TestCategoryAdminListUsesSharedAdminAuthenticationBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:category-admin-router?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveCategory{}))

	originalDB := global.GVA_DB
	global.GVA_DB = db
	t.Cleanup(func() {
		global.GVA_DB = originalDB
	})

	engine := gin.New()
	new(CategoryAdminRouter).InitCategoryAdminRouter(engine.Group("/"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/live/category/list?page=1&pageSize=10", nil)
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.NotEqual(t, "权限不足", body.Msg)
}
