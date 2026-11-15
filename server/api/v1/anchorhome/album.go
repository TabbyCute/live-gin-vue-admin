package anchorhome

import (
	"errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"tb_live_module/global"
	"tb_live_module/internal/httpkit"
	"tb_live_module/internal/modulekit"
	"tb_live_module/middleware"
	req "tb_live_module/model/anchorhome/request"
	"tb_live_module/model/common/response"
	svc "tb_live_module/service/anchorhome"
)

type AlbumAPI struct{}

func albumRespond(c *gin.Context, out any, err error) {
	c.Header("Cache-Control", "no-store")
	if requestID := c.GetString("anchorHomeCommand"); requestID != "" && global.GVA_LOG != nil {
		outcome := "succeeded"
		if err != nil {
			outcome = svc.ErrorCode(err)
		}
		global.GVA_LOG.Info("anchor_home_command", zap.String("traceId", middleware.RequestID(c)), zap.String("requestId", requestID), zap.String("route", c.FullPath()), zap.String("outcome", outcome))
	}
	if err != nil {
		code := svc.ErrorCode(err)
		if global.GVA_LOG != nil {
			global.GVA_LOG.Warn("anchor_home_album_request_failed", zap.String("traceId", middleware.RequestID(c)), zap.String("route", c.FullPath()), zap.String("errorCode", code))
		}
		response.Result(response.ERROR, gin.H{"errorCode": code}, svc.PublicError(err), c)
		return
	}
	response.OkWithData(out, c)
}
func albumCommandTrace(c *gin.Context, requestID string) {
	// Only bounded validated IDs enter logs, never raw body/file/rejection text.
	if modulekit.RequestID.MatchString(requestID) {
		c.Set("anchorHomeCommand", requestID)
	}
}
func albumOwner(c *gin.Context) (uint64, bool) {
	id, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("请登录客户端账号", c)
	}
	return uint64(id), ok
}

// OwnerAlbum
// @Tags AnchorHome
// @Summary 主播自己的相册及排序版本（所有未删除审核状态）
// @Security AppBearerAuth
// @Produce json
// @Success 200 {object} response.Response
// @Router /v1/app/anchor-home/album [get]
func (AlbumAPI) OwnerAlbum(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		albumRespond(c, nil, svc.ErrInvalid)
		return
	}
	owner, ok := albumOwner(c)
	if !ok {
		return
	}
	out, err := svc.DefaultAlbum.OwnerAlbum(c.Request.Context(), owner)
	albumRespond(c, out, err)
}

// UploadPhoto
// @Tags AnchorHome
// @Summary 上传待审核的主页照片（8MiB、20MP，服务端重新编码）
// @Security AppBearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "JPEG/PNG/WebP"
// @Param requestId formData string true "8至64位幂等请求编号"
// @Param version formData int true "当前相册版本"
// @Success 200 {object} response.Response
// @Router /v1/app/anchor-home/album/upload [post]
func (AlbumAPI) UploadPhoto(c *gin.Context) {
	owner, ok := albumOwner(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, svc.MaxUploadBytes+(64<<10))
	if err := c.Request.ParseMultipartForm(svc.MaxUploadBytes + (64 << 10)); err != nil {
		albumRespond(c, nil, svc.ErrInvalid)
		return
	}
	if c.Request.MultipartForm == nil {
		albumRespond(c, nil, svc.ErrInvalid)
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	f := c.Request.MultipartForm
	if len(f.File) != 1 || len(f.File["file"]) != 1 || len(f.Value) != 2 || len(f.Value["requestId"]) != 1 || len(f.Value["version"]) != 1 || c.Request.URL.RawQuery != "" {
		albumRespond(c, nil, svc.ErrInvalid)
		return
	}
	version, err := strconv.ParseUint(f.Value["version"][0], 10, 64)
	if err != nil {
		albumRespond(c, nil, svc.ErrInvalid)
		return
	}
	file, err := f.File["file"][0].Open()
	if err != nil {
		albumRespond(c, nil, svc.ErrInvalid)
		return
	}
	defer file.Close()
	albumCommandTrace(c, f.Value["requestId"][0])
	out, err := svc.DefaultAlbum.Upload(c.Request.Context(), owner, req.AlbumMutation{RequestID: f.Value["requestId"][0], Version: version}, file)
	albumRespond(c, out, err)
}

// ReorderPhotos
// @Tags AnchorHome
// @Summary 按完整照片集合重排，第一张为公开封面候选
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data body req.ReorderPhotos true "固定requestId、相册version和完整photoNos"
// @Success 200 {object} response.Response
// @Router /v1/app/anchor-home/album/reorder [post]
func (AlbumAPI) ReorderPhotos(c *gin.Context) {
	owner, ok := albumOwner(c)
	if !ok {
		return
	}
	var r req.ReorderPhotos
	if !httpkit.Body(c, &r) {
		return
	}
	albumCommandTrace(c, r.RequestID)
	out, err := svc.DefaultAlbum.Reorder(c.Request.Context(), owner, r)
	albumRespond(c, out, err)
}

// DeletePhoto
// @Tags AnchorHome
// @Summary 删除自己的照片，立即撤销公开读取权限
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data body req.DeletePhoto true "固定requestId、相册version、photoNo"
// @Success 200 {object} response.Response
// @Router /v1/app/anchor-home/album/delete [post]
func (AlbumAPI) DeletePhoto(c *gin.Context) {
	owner, ok := albumOwner(c)
	if !ok {
		return
	}
	var r req.DeletePhoto
	if !httpkit.Body(c, &r) {
		return
	}
	albumCommandTrace(c, r.RequestID)
	out, err := svc.DefaultAlbum.Delete(c.Request.Context(), owner, r)
	albumRespond(c, out, err)
}

// AdminAlbumList
// @Tags AnchorHomeAdmin
// @Summary 当前有效root或888管理员分页查询照片审核队列
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.AlbumSearch true "page/pageSize/status/anchorNo"
// @Success 200 {object} response.Response
// @Router /v1/admin/anchor-home/album/list [post]
func (AlbumAPI) AdminAlbumList(c *gin.Context) {
	var r req.AlbumSearch
	if !httpkit.Body(c, &r) {
		return
	}
	out, err := svc.DefaultAlbum.AdminList(c.Request.Context(), httpkit.Actor(c).ID, r)
	albumRespond(c, out, err)
}

// ReviewPhoto
// @Tags AnchorHomeAdmin
// @Summary 审批/拒绝/下架照片，保存不可变审计记录
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.ReviewPhoto true "固定requestId、照片version；reject/takedown必须提供reason"
// @Success 200 {object} response.Response
// @Router /v1/admin/anchor-home/album/review [post]
func (AlbumAPI) ReviewPhoto(c *gin.Context) {
	var r req.ReviewPhoto
	if !httpkit.Body(c, &r) {
		return
	}
	albumCommandTrace(c, r.RequestID)
	out, err := svc.DefaultAlbum.Review(c.Request.Context(), httpkit.Actor(c).ID, r)
	albumRespond(c, out, err)
}

// PublicPhoto
// @Tags AnchorHome
// @Summary 读取当前公开照片，重新检查主播、账号和双向拉黑事实
// @Produce image/jpeg
// @Param photoNo query string true "照片公开编号"
// @Param variant query string true "full|thumb"
// @Success 200 {file} binary
// @Router /v1/app/anchor-home/photo [get]
func (AlbumAPI) PublicPhoto(c *gin.Context) {
	id, _ := middleware.GetAppAccountID(c)
	serveAlbumPhoto(c, uint64(id), "public")
}

// OwnerPhotoPreview
// @Tags AnchorHome
// @Summary 主播私有照片预览，客户端携带Bearer读取Blob
// @Security AppBearerAuth
// @Produce image/jpeg
// @Param photoNo query string true "照片公开编号"
// @Param variant query string true "full|thumb"
// @Success 200 {file} binary
// @Router /v1/app/anchor-home/album/preview [get]
func (AlbumAPI) OwnerPhotoPreview(c *gin.Context) {
	id, ok := albumOwner(c)
	if ok {
		serveAlbumPhoto(c, id, "owner")
	}
}

// AdminPhotoPreview
// @Tags AnchorHomeAdmin
// @Summary 当前有权限的管理员携带x-token读取照片Blob
// @Security ApiKeyAuth
// @Produce image/jpeg
// @Param photoNo query string true "照片公开编号"
// @Param variant query string true "full|thumb"
// @Success 200 {file} binary
// @Router /v1/admin/anchor-home/album/preview [get]
func (AlbumAPI) AdminPhotoPreview(c *gin.Context) { serveAlbumPhoto(c, httpkit.Actor(c).ID, "admin") }

func serveAlbumPhoto(c *gin.Context, actor uint64, kind string) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Vary", "Authorization, x-token")
	q, queryErr := url.ParseQuery(c.Request.URL.RawQuery)
	if queryErr != nil || len(q) != 2 || len(q["photoNo"]) != 1 || len(q["variant"]) != 1 || len(c.Request.URL.RawQuery) > 256 {
		c.Status(http.StatusBadRequest)
		return
	}
	f, size, err := svc.DefaultAlbum.ReadPhoto(c.Request.Context(), actor, kind, q.Get("photoNo"), q.Get("variant"))
	if err != nil {
		if errors.Is(err, svc.ErrNotFound) || errors.Is(err, svc.ErrForbidden) {
			c.Status(http.StatusNotFound)
		} else if errors.Is(err, svc.ErrInvalid) {
			c.Status(http.StatusBadRequest)
		} else {
			if global.GVA_LOG != nil {
				global.GVA_LOG.Warn("anchor_home_media_read_failed", zap.String("traceId", middleware.RequestID(c)), zap.String("route", c.FullPath()), zap.String("photoNo", q.Get("photoNo")), zap.String("errorCode", svc.ErrorCode(err)))
			}
			c.Status(http.StatusServiceUnavailable)
		}
		return
	}
	defer f.Close()
	c.Header("Content-Type", "image/jpeg")
	c.Header("Content-Disposition", "inline; filename=photo.jpg")
	c.Header("Content-Length", strconv.FormatInt(size, 10))
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, f) // no ServeFile/ETag/304: every request requires current authorization
}
