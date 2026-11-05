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
	socialReq "tb_live_module/model/social/request"
	liveService "tb_live_module/service/live"
	service "tb_live_module/service/social"
	"testing"
	"time"
)

func TestRequestParsingLimitsAndNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, body        string
		unknownLength, ok bool
	}{
		{"valid", `{"accountNo":"target"}`, false, true},
		{"unknown_actor_ignored", `{"accountNo":"target","userId":99}`, false, true},
		{"second_object", `{"accountNo":"target"}{"accountNo":"other"}`, false, false},
		{"trailing_garbage", `{"accountNo":"target"}broken`, false, false},
		{"trailing_oversize_known", `{"accountNo":"target"}` + strings.Repeat(" ", 33<<10), false, false},
		{"trailing_oversize_chunked", `{"accountNo":"target"}` + strings.Repeat(" ", 33<<10), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			called := false
			engine.POST("/test", RequestBudget(), func(c *gin.Context) {
				deadline, ok := c.Request.Context().Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
				var r socialReq.Target
				if bind(c, &r) {
					called = true
					respond(c, r, nil)
				}
			})
			r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tc.body))
			if tc.unknownLength {
				r.ContentLength = -1
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			require.Equal(t, tc.ok, called)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			if !tc.ok {
				require.Contains(t, w.Body.String(), `"code":46001`)
				require.NotContains(t, w.Body.String(), "invalid character")
			}
		})
	}
}

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
