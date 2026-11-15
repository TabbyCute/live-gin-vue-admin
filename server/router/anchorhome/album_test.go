package anchorhome

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/songzhibin97/gkit/cache/local_cache"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"tb_live_module/config"
	"tb_live_module/global"
	"tb_live_module/middleware"
	"tb_live_module/model/live"
	"tb_live_module/model/social"
	"tb_live_module/model/system"
	systemReq "tb_live_module/model/system/request"
	svc "tb_live_module/service/anchorhome"
	"tb_live_module/utils"
	"testing"
)

func albumHTTPFixture(t *testing.T) (*gin.Engine, string, string, *svc.AlbumService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http.db")), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&live.LiveAccount{}, &live.LiveAnchor{}, &social.SocialBlock{}, &system.SysUser{}, &system.SysAuthority{}, &system.SysUserAuthority{}))
	require.NoError(t, svc.MigrateAlbumSchema(db))
	require.NoError(t, db.Create(&live.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: 10}, AccountNo: "1000010", Username: "album_http", Password: "unused-local", Status: 1, TokenVersion: 1}).Error)
	require.NoError(t, db.Create(&live.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: 10}, UserId: 10, AnchorNo: "http_anchor", ApplyStatus: 2, Status: 1}).Error)
	require.NoError(t, db.Create(&system.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 1}, Username: "local_root", Enable: 1}).Error)
	s, err := svc.NewAlbumService(db, svc.AlbumOptions{StorageDir: filepath.Join(t.TempDir(), "media")})
	require.NoError(t, err)
	previousDB, previousLog, previousConfig, previousAlbum := global.GVA_DB, global.GVA_LOG, global.GVA_CONFIG, svc.DefaultAlbum
	previousCache := global.BlackCache
	global.BlackCache = local_cache.NewCache()
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	svc.DefaultAlbum = s
	global.GVA_CONFIG.JWT = config.JWT{SigningKey: "album-admin-isolated-signing-key", ExpiresTime: "1h", BufferTime: "1m", Issuer: "album-admin-test"}
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "album-app-isolated-signing-key", ExpiresTime: "1h", Issuer: "album-app-test"}
	t.Cleanup(func() {
		s.Close()
		global.GVA_DB = previousDB
		global.GVA_LOG = previousLog
		global.GVA_CONFIG = previousConfig
		svc.DefaultAlbum = previousAlbum
		global.BlackCache = previousCache
	})
	owner, _, err := utils.NewAppJWT().CreateToken(10, "album_http", 1)
	require.NoError(t, err)
	admin, err := utils.NewJWT().CreateToken(utils.NewJWT().CreateClaims(systemReq.BaseClaims{ID: 1, Username: "local_root", AuthorityId: 888}))
	require.NoError(t, err)
	r := gin.New()
	router := AlbumRouter{}
	router.InitAlbumOwnerRouter(r.Group("/v1/app"))
	router.InitAlbumAdminRouter(r.Group("/v1/admin"))
	router.InitAlbumPublicRouter(r.Group("/v1/app", middleware.OptionalJWTAppAuth()))
	return r, owner, admin, s
}
func httpJSON(t *testing.T, r *gin.Engine, path, token string, admin bool, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	if admin {
		request.Header.Set("x-token", token)
	} else {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	out := httptest.NewRecorder()
	r.ServeHTTP(out, request)
	return out
}
func responseData(t *testing.T, out *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, 200, out.Code)
	var body struct {
		Code int
		Msg  string
		Data map[string]any
	}
	require.NoError(t, json.Unmarshal(out.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code, body.Msg)
	return body.Data
}
func uploadHTTP(t *testing.T, r *gin.Engine, token string, picture []byte, extra bool) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	form := multipart.NewWriter(&b)
	require.NoError(t, form.WriteField("requestId", "http_upload_first"))
	require.NoError(t, form.WriteField("version", "0"))
	if extra {
		require.NoError(t, form.WriteField("version", "1"))
	}
	f, err := form.CreateFormFile("file", "../../untrusted.svg")
	require.NoError(t, err)
	_, err = f.Write(picture)
	require.NoError(t, err)
	require.NoError(t, form.Close())
	request := httptest.NewRequest(http.MethodPost, "/v1/app/anchor-home/album/upload", &b)
	request.Header.Set("Content-Type", form.FormDataContentType())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	out := httptest.NewRecorder()
	r.ServeHTTP(out, request)
	return out
}

func TestAlbumHTTPUploadReviewPreviewTakedown(t *testing.T) {
	r, owner, admin, s := albumHTTPFixture(t)
	ownerQuery := httptest.NewRequest("GET", "/v1/app/anchor-home/album?userId=99", nil)
	ownerQuery.Header.Set("Authorization", "Bearer "+owner)
	ownerReply := httptest.NewRecorder()
	r.ServeHTTP(ownerReply, ownerQuery)
	require.Contains(t, ownerReply.Body.String(), "invalid")
	// The real multipart exceeds the existing JSON-only 64KiB budget.
	im := image.NewRGBA(image.Rect(0, 0, 512, 512))
	random := rand.New(rand.NewPCG(11, 22))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(random.Uint32()), uint8(random.Uint32()), uint8(random.Uint32()), 255})
		}
	}
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, im))
	require.Greater(t, b.Len(), 64<<10)
	unauth := uploadHTTP(t, r, "", b.Bytes(), false)
	require.Equal(t, http.StatusUnauthorized, unauth.Code)
	invalid := uploadHTTP(t, r, owner, b.Bytes(), true)
	require.Contains(t, invalid.Body.String(), "invalid")
	data := responseData(t, uploadHTTP(t, r, owner, b.Bytes(), false))
	photo := data["photoNo"].(string)
	asset := "/v1/app/anchor-home/photo?photoNo=" + photo + "&variant=full"
	read := func(path, token, header string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", path, nil)
		if token != "" {
			request.Header.Set(header, token)
		}
		out := httptest.NewRecorder()
		r.ServeHTTP(out, request)
		return out
	}
	require.Equal(t, 404, read(asset, "", "").Code)
	preview := read("/v1/app/anchor-home/album/preview?photoNo="+photo+"&variant=thumb", "Bearer "+owner, "Authorization")
	require.Equal(t, 200, preview.Code)
	require.Equal(t, "image/jpeg", preview.Header().Get("Content-Type"))
	require.Contains(t, preview.Header().Get("Cache-Control"), "no-store")
	approve := map[string]any{"requestId": "http_review_approve", "version": 1, "photoNo": photo, "decision": "approve", "reason": "本地测试审核"}
	approved := responseData(t, httpJSON(t, r, "/v1/admin/anchor-home/album/review", admin, true, approve))
	require.Equal(t, float64(2), approved["photoVersion"])
	public := read(asset, "", "")
	require.Equal(t, 200, public.Code)
	require.Equal(t, "nosniff", public.Header().Get("X-Content-Type-Options"))
	require.Empty(t, public.Header().Get("ETag"))
	_, format, err := image.Decode(bytes.NewReader(public.Body.Bytes()))
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 400, read(asset+"&unexpected=%ZZ", "", "").Code)
	require.Equal(t, http.StatusUnauthorized, read(asset, "Bearer bad-token", "Authorization").Code)
	takedown := map[string]any{"requestId": "http_review_takedown", "version": 2, "photoNo": photo, "decision": "takedown", "reason": "审核后发现不适宜展示"}
	responseData(t, httpJSON(t, r, "/v1/admin/anchor-home/album/review", admin, true, takedown))
	request := httptest.NewRequest("GET", asset, nil)
	request.Header.Set("If-None-Match", "anything")
	out := httptest.NewRecorder()
	r.ServeHTTP(out, request)
	require.Equal(t, 404, out.Code)
	// Even a still-valid administrator JWT loses access when the database disables it.
	require.NoError(t, s.DB.Model(&system.SysUser{}).Where("id=1").Update("enable", 2).Error)
	require.Equal(t, 404, read("/v1/admin/anchor-home/album/preview?photoNo="+photo+"&variant=full", admin, "x-token").Code)
	oversized := httptest.NewRequest("POST", "/v1/app/anchor-home/album/upload", io.LimitReader(strings.NewReader("x"), 1))
	oversized.ContentLength = svc.MaxUploadBytes + (128 << 10)
	out = httptest.NewRecorder()
	r.ServeHTTP(out, oversized)
	require.Equal(t, 413, out.Code)
}
