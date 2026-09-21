package live

import (
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

func TestAnchorPublicDetailUsesAnchorNoAndHidesPrimaryKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAnchor{}))

	previousDB := global.GVA_DB
	previousLog := global.GVA_LOG
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	t.Cleanup(func() {
		global.GVA_DB = previousDB
		global.GVA_LOG = previousLog
	})

	anchor := liveModel.LiveAnchor{
		UserId: 31, AnchorNo: "7100001", Nickname: "Public",
		ApplyStatus: liveModel.AnchorApplyStatusApproved,
		Status:      liveModel.AnchorStatusNormal,
	}
	require.NoError(t, db.Create(&anchor).Error)

	router := gin.New()
	router.GET("/detail", (&AnchorApi{}).Detail)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/detail?anchorNo=7100001", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var body commonRes.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, commonRes.SUCCESS, body.Code)
	data, ok := body.Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, anchor.AnchorNo, data["anchorNo"])
	require.NotContains(t, data, "id")
	require.NotContains(t, data, "anchorId")

	legacyRecorder := httptest.NewRecorder()
	router.ServeHTTP(legacyRecorder, httptest.NewRequest(http.MethodGet, "/detail?anchorId=1", nil))
	require.NoError(t, json.Unmarshal(legacyRecorder.Body.Bytes(), &body))
	require.Equal(t, commonRes.ERROR, body.Code, "客户端不应继续接受数据库主键查询")
}
