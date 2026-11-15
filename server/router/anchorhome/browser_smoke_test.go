package anchorhome

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	liveAPI "tb_live_module/api/v1/live"
	"tb_live_module/global"
	"tb_live_module/middleware"
	homeReq "tb_live_module/model/anchorhome/request"
	guardModel "tb_live_module/model/guard"
	"tb_live_module/model/live"
	rankModel "tb_live_module/model/ranking"
	"tb_live_module/model/social"
	socialRouter "tb_live_module/router/social"
	svc "tb_live_module/service/anchorhome"
	guardSvc "tb_live_module/service/guard"
	liveSvc "tb_live_module/service/live"
	"tb_live_module/service/ranking"
	socialSvc "tb_live_module/service/social"
	"tb_live_module/utils"
)

// TestAnchorHomeBrowserSmoke is opt-in and never opens a configured production DB.
// Its only listener is loopback; JWTs live in a 0600 temporary file and are deleted
// by testing cleanup. The browser uses real handlers/services/SQLite/media storage.
func TestAnchorHomeBrowserSmoke(t *testing.T) {
	if os.Getenv("H5_ANCHOR_HOME_BROWSER") != "1" {
		t.Skip("opt-in local browser integration")
	}
	h5 := os.Getenv("H5_SOURCE")
	if h5 == "" {
		h5 = "/Users/tabby/Desktop/IM/H5Player"
	}
	assets := os.Getenv("H5_BROWSER_ASSETS")
	if assets == "" {
		assets = "/tmp/h5-anchor-home-build"
	}
	_, err := os.Stat(filepath.Join(assets, "index.html"))
	require.NoError(t, err)
	r, owner, admin, album := albumHTTPFixture(t)
	db := album.DB
	models := []any{&live.LiveRoom{}, &live.LiveSession{}, &live.LiveCategory{}, &guardModel.Plan{}, &guardModel.Membership{}, &guardModel.Ledger{}, &rankModel.Checkpoint{}, &rankModel.ScoreBucket{}}
	models = append(models, social.Models()...)
	require.NoError(t, db.AutoMigrate(models...))
	require.NoError(t, db.Model(&live.LiveAnchor{}).Where("id=10").Updates(map[string]any{"nickname": "真实接口主播", "signature": "真实接口公开介绍", "category_id": 1, "level": 4, "cert_status": 2, "live_permission": 1}).Error)
	require.NoError(t, db.Model(&live.LiveAccount{}).Where("id=10").Update("nickname", "测试主播账号").Error)
	require.NoError(t, db.Create(&live.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: 11}, AccountNo: "1000011", Username: "browser_viewer", Nickname: "真实接口观众", Password: "unused-local", Status: 1, TokenVersion: 1}).Error)
	require.NoError(t, db.Create(&live.LiveCategory{GVA_MODEL: global.GVA_MODEL{ID: 1}, Code: "music", Name: "音乐", Status: 1}).Error)
	require.NoError(t, db.Create(&live.LiveCategory{GVA_MODEL: global.GVA_MODEL{ID: 2}, Code: "chat", Name: "聊天", Status: 1}).Error)
	require.NoError(t, db.Create(&live.LiveRoom{GVA_MODEL: global.GVA_MODEL{ID: 10}, RoomNo: "real_room", AnchorId: 10, Title: "真实接口直播间", Status: 1, Visibility: 1, LiveStatus: live.LiveRoomLiving, CurrentSessionId: 10}).Error)
	require.NoError(t, db.Create(&live.LiveSession{GVA_MODEL: global.GVA_MODEL{ID: 10}, SessionNo: "real_session", RoomId: 10, AnchorId: 10, Status: live.LiveSessionLiving}).Error)
	require.NoError(t, db.Create(&social.SocialUserCounter{UserID: 10, FollowingCount: 1, FansCount: 1}).Error)
	require.NoError(t, db.Create(&social.SocialUserCounter{UserID: 11, FollowingCount: 1, FansCount: 1}).Error)
	require.NoError(t, db.Create(&social.SocialFollow{UserID: 10, TargetUserID: 11}).Error)
	require.NoError(t, db.Create(&social.SocialFollow{UserID: 11, TargetUserID: 10}).Error)
	now := time.Now().UnixMilli()
	require.NoError(t, db.Create(&rankModel.Checkpoint{ConsumerName: "gift-revenue", Generation: 1, Timezone: "Asia/Kuala_Lumpur", DataAsOf: now}).Error)
	for _, period := range []string{"day", "month", "all"} {
		key, e := ranking.PeriodKey(period, now, "Asia/Kuala_Lumpur")
		require.NoError(t, e)
		require.NoError(t, db.Create(&rankModel.ScoreBucket{BucketKey: "browser_" + period, Generation: 1, RankType: "gift", PeriodType: period, PeriodKey: key, ScopeType: "anchor", ScopeNo: "http_anchor", SubjectType: "user", SubjectNo: "1000011", ScoreCoin: "9007199254740993", Quantity: "1", ScoreSort: strings.Repeat("0", 49) + "9007199254740993", Version: 1, UpdatedAt: now}).Error)
	}
	plan := guardModel.PlanSnapshot{PlanNo: "real_guard", Name: "接口守护", Level: 1, Priority: 1, BenefitConfig: `{"publicBenefits":["真实守护身份展示"]}`}
	require.NoError(t, db.Create(&guardModel.Plan{PlanNo: plan.PlanNo, Name: plan.Name, Level: 1, Priority: 1, BenefitConfig: plan.BenefitConfig, Status: "enabled", DefaultDurationDays: 30, Version: 1}).Error)
	projection, err := json.Marshal([]guardSvc.Segment{{Plan: plan, Start: now - 60000, End: now + 3600000}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&guardModel.Ledger{LedgerNo: "browser_ledger", MembershipNo: "browser_member", SourceNo: "browser_source", AfterSnapshot: string(projection)}).Error)
	require.NoError(t, db.Create(&guardModel.Membership{MembershipNo: "browser_member", UserID: 11, AnchorID: 10, Projection: string(projection), LedgerNo: "browser_ledger", ValidUntil: now + 3600000, Status: "active"}).Error)
	beforePublic := *svc.DefaultPublic
	svc.DefaultPublic.DB = db
	beforeSocialDB, beforeDirectory := socialSvc.Default.DB, socialSvc.Default.Directory
	socialSvc.Default.DB, socialSvc.Default.Directory = db, liveSvc.SocialDirectory{}
	rankReady, guardReady := global.RankingReady.Load(), global.GuardReady.Load()
	global.RankingReady.Store(true)
	global.GuardReady.Store(true)
	global.GVA_CONFIG.Gift.GuardEnabled = true
	t.Cleanup(func() {
		*svc.DefaultPublic = beforePublic
		socialSvc.Default.DB, socialSvc.Default.Directory = beforeSocialDB, beforeDirectory
		global.RankingReady.Store(rankReady)
		global.GuardReady.Store(guardReady)
	})
	RouterGroup{}.InitAnchorHomePublicRouter(r.Group("/v1/app"))
	(&socialRouter.SocialRouter{}).InitSocialRouter(r.Group("/v1/app"))
	private := r.Group("/v1/app/live", middleware.JWTAppAuth())
	private.GET("user/info", (&liveAPI.UserApi{}).Info)
	private.GET("anchor/info", (&liveAPI.AnchorApi{}).Info)
	im := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(40 + y/4), uint8(40 + x/4), 160, 255})
		}
	}
	var picture bytes.Buffer
	require.NoError(t, png.Encode(&picture, im))
	uploaded := responseData(t, uploadHTTP(t, r, owner, picture.Bytes(), false))
	firstPhoto := uploaded["photoNo"].(string)
	responseData(t, httpJSON(t, r, "/v1/admin/anchor-home/album/review", admin, true, homeReq.ReviewPhoto{AlbumMutation: homeReq.AlbumMutation{RequestID: "browser_approve", Version: 1}, PhotoNo: firstPhoto, Decision: "approve", Reason: "本地测试审核"}))
	viewer, _, err := utils.NewAppJWT().CreateToken(11, "browser_viewer", 1)
	require.NoError(t, err)
	tokenPath := filepath.Join(t.TempDir(), "browser-credentials.json")
	credentials, err := json.Marshal(map[string]string{"ownerToken": owner, "adminToken": admin, "viewerToken": viewer, "anchorNo": "http_anchor", "firstPhoto": firstPhoto})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenPath, credentials, 0600))
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", r))
	const prefix = "/app/v1/assets/h5/"
	mux.HandleFunc(prefix, func(w http.ResponseWriter, request *http.Request) {
		name := filepath.Join(assets, filepath.Clean("/"+strings.TrimPrefix(request.URL.Path, prefix)))
		if info, e := os.Stat(name); e != nil || info.IsDir() {
			name = filepath.Join(assets, "index.html")
		}
		http.ServeFile(w, request, name)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:4188")
	require.NoError(t, err)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "node", filepath.Join(h5, "tests/anchor-home-real-browser.mjs"))
	command.Dir = h5
	command.Env = append(os.Environ(), "H5_REAL_CREDENTIAL_FILE="+tokenPath)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "local browser failed: %s", output)
	t.Log(string(output))
}
