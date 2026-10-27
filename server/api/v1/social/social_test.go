package social

import (
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	model "tb_live_module/model/social"
	liveService "tb_live_module/service/live"
	service "tb_live_module/service/social"
	"testing"
)

func TestActorCannotBeSpoofedInJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	require.NoError(t, db.AutoMigrate(append(model.Models(), &liveModel.LiveAccount{}, &liveModel.LiveAnchor{})...))
	require.NoError(t, db.Create(&liveModel.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: 1}, AccountNo: "actor", Username: "actor", Password: "hash", Status: 1}).Error)
	require.NoError(t, db.Create(&liveModel.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: 2}, AccountNo: "target", Username: "target", Password: "hash", Status: 1}).Error)
	previous := socialService
	socialService = &service.Service{DB: db, Directory: liveService.SocialDirectory{}}
	defer func() { socialService = previous }()
	engine := gin.New()
	engine.POST("/follow", func(c *gin.Context) { c.Set("app-claims", &liveReq.AppClaims{AccountID: 1}); c.Next() }, (&SocialApi{}).Follow)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/follow", strings.NewReader(`{"accountNo":"target","userId":2,"actorId":2}`))
	r.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, r)
	require.Contains(t, w.Body.String(), `"code":0`)
	var relation model.SocialFollow
	require.NoError(t, db.First(&relation).Error)
	require.Equal(t, uint64(1), relation.UserID)
	require.Equal(t, uint64(2), relation.TargetUserID)
}
