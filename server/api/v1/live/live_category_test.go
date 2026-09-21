package live

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tb_live_module/global"
	commonRes "tb_live_module/model/common/response"
	liveModel "tb_live_module/model/live"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestLiveCategoryPublicTreeAndAdminCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveCategory{}, &liveModel.LiveAnchor{}))

	previousDB := global.GVA_DB
	previousLog := global.GVA_LOG
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.GVA_LOG = previousLog
	})

	adminRouter := gin.New()
	adminRouter.POST("/create", (&CategoryAdminApi{}).Create)
	createBody := []byte(`{"parentId":0,"code":"Music","name":"音乐","icon":"","sort":100,"status":1}`)
	createRecorder := httptest.NewRecorder()
	adminRouter.ServeHTTP(createRecorder, httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(createBody)))
	require.Equal(t, http.StatusOK, createRecorder.Code)
	var responseBody commonRes.Response
	require.NoError(t, json.Unmarshal(createRecorder.Body.Bytes(), &responseBody))
	require.Equal(t, commonRes.SUCCESS, responseBody.Code)
	created, ok := responseBody.Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "music", created["code"], "分类编码应由后端统一转为小写")
	rootID := uint(created["id"].(float64))

	child := liveModel.LiveCategory{ParentId: rootID, Code: "singing", Name: "唱歌", Status: liveModel.LiveCategoryStatusEnabled}
	require.NoError(t, db.Create(&child).Error)
	disabled := liveModel.LiveCategory{Code: "hidden", Name: "隐藏", Status: liveModel.LiveCategoryStatusEnabled}
	require.NoError(t, db.Create(&disabled).Error)
	require.NoError(t, db.Model(&disabled).Update("status", liveModel.LiveCategoryStatusDisabled).Error)

	publicRouter := gin.New()
	publicRouter.GET("/list", (&CategoryApi{}).List)
	listRecorder := httptest.NewRecorder()
	publicRouter.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/list", nil))
	require.Equal(t, http.StatusOK, listRecorder.Code)
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &responseBody))
	require.Equal(t, commonRes.SUCCESS, responseBody.Code)
	list, ok := responseBody.Data.([]interface{})
	require.True(t, ok)
	require.Len(t, list, 1, "公开分类树不得包含停用分类")
	root := list[0].(map[string]interface{})
	require.Equal(t, "music", root["code"])
	require.NotContains(t, root, "status")
	require.NotContains(t, root, "createdAt")
	require.Len(t, root["children"].([]interface{}), 1)
}
