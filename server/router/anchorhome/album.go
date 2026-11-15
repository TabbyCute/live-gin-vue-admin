package anchorhome

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"tb_live_module/global"
	"tb_live_module/internal/httpkit"
	"tb_live_module/middleware"
	"time"
)

type AlbumRouter struct{}

// Authentication and rate limits belong to this route, so a new registration
// cannot accidentally downgrade supplied invalid credentials to a guest.
func (AlbumRouter) InitAlbumPublicRouter(public *gin.RouterGroup) {
	r := public.Group("anchor-home").Use(httpkit.Budget(32, 5*time.Second), middleware.AnchorHomeReadLimit("media", global.GVA_CONFIG.AnchorHome.MediaReadPerMinute), middleware.OptionalJWTAppAuth())
	a := albumAPI
	r.GET("photo", a.PublicPhoto)
}
func (AlbumRouter) InitAlbumOwnerRouter(app *gin.RouterGroup) {
	r := app.Group("anchor-home").Use(albumOwnerBudget(), middleware.JWTAppAuth(), httpkit.UserRateLimit())
	a := albumAPI
	r.GET("album", a.OwnerAlbum)
	r.POST("album/upload", a.UploadPhoto)
	r.POST("album/reorder", a.ReorderPhotos)
	r.POST("album/delete", a.DeletePhoto)
	r.GET("album/preview", a.OwnerPhotoPreview)
}

// Upload routes cannot pass through the repository's 64KiB JSON budget.
func albumOwnerBudget() gin.HandlerFunc {
	slots := make(chan struct{}, 4)
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		maxBody := int64(64 << 10)
		if c.Request.Method == http.MethodPost && strings.HasSuffix(c.FullPath(), "/album/upload") {
			maxBody = (8 << 20) + (64 << 10)
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(15 * time.Second))
		if c.Request.ContentLength > maxBody || len(c.Request.URL.RawQuery) > 2048 || len(c.GetHeader("Authorization")) > 8192 {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"code": 7, "data": gin.H{"errorCode": "invalid"}, "msg": "请求体过大"})
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 7, "data": gin.H{"errorCode": "busy"}, "msg": "图片处理繁忙，请稍后重试"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
		c.Next()
	}
}
func (AlbumRouter) InitAlbumAdminRouter(admin *gin.RouterGroup) {
	r := admin.Group("anchor-home/album").Use(httpkit.Budget(16, 10*time.Second), middleware.JWTAuth())
	a := albumAPI
	r.POST("list", a.AdminAlbumList)
	r.POST("review", a.ReviewPhoto)
	r.GET("preview", a.AdminPhotoPreview)
}
