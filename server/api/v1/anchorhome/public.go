package anchorhome

import (
	"errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"tb_live_module/global"
	"tb_live_module/internal/httpkit"
	"tb_live_module/internal/modulekit"
	"tb_live_module/internal/telemetry"
	"tb_live_module/middleware"
	req "tb_live_module/model/anchorhome/request"
	homeRes "tb_live_module/model/anchorhome/response"
	"tb_live_module/model/common/response"
	guardRes "tb_live_module/model/guard/response"
	rankRes "tb_live_module/model/ranking/response"
	svc "tb_live_module/service/anchorhome"
)

var _ homeRes.Profile
var _ guardRes.PublicMembers
var _ rankRes.Board

func publicViewer(c *gin.Context) uint64 { id, _ := middleware.GetAppAccountID(c); return uint64(id) }
func respondPublic(c *gin.Context, out any, err error) {
	c.Header("Cache-Control", "no-store")
	if err == nil {
		response.OkWithData(out, c)
		return
	}
	var business *modulekit.Error
	if !errors.As(err, &business) {
		switch {
		case errors.Is(err, svc.ErrInvalid):
			business = modulekit.Invalid
		case errors.Is(err, svc.ErrForbidden):
			business = modulekit.Forbidden
		case errors.Is(err, svc.ErrNotFound):
			business = modulekit.NotFound
		default:
			business = modulekit.Unavailable
		}
	}
	if global.GVA_LOG != nil && (business == modulekit.Unavailable || business == modulekit.Invariant) {
		global.GVA_LOG.Warn("anchor home request failed", zap.String("traceId", middleware.RequestID(c)), zap.String("route", c.FullPath()), zap.Int("code", business.Code), zap.String("errorKind", telemetry.ErrorKind(err)))
	}
	response.Result(business.Code, map[string]any{}, business.Message, c)
}

// Profile
// @Tags AnchorHome
// @Summary 主播公开主页（游客可读，提供Bearer时必须有效）
// @Description 仅返回已审核相册和公开字段；直播入口遵循双向拉黑和房间权限。所有读取使用同一只读快照。
// @Produce json
// @Param anchorNo query string true "主播公开编号"
// @Success 200 {object} response.Response{data=homeRes.Profile}
// @Router /v1/app/anchor-home/profile [get]
func (PublicAPI) Profile(c *gin.Context) {
	var q req.ProfileQuery
	if !httpkit.Query(c, &q) {
		return
	}
	out, err := publicService.Profile(c.Request.Context(), publicViewer(c), q.AnchorNo)
	respondPublic(c, out, err)
}

// Supporters
// @Tags AnchorHome
// @Summary 主播贡献榜，来自退款抵减后的礼物投影
// @Description day/month/all使用业务时区；dataAsOf展示投影时间，lagCount空表示公开请求不计算全站投影积压。
// @Produce json
// @Param anchorNo query string true "主播公开编号"
// @Param period query string false "day|month|all"
// @Param page query int false "1..100"
// @Param pageSize query int false "1..50"
// @Success 200 {object} response.Response{data=rankRes.Board}
// @Router /v1/app/anchor-home/supporters [get]
func (PublicAPI) Supporters(c *gin.Context) {
	var q req.SupportersQuery
	if !httpkit.Query(c, &q) {
		return
	}
	out, err := publicService.Supporters(c.Request.Context(), publicViewer(c), q)
	respondPublic(c, out, err)
}

// Guards
// @Tags AnchorHome
// @Summary 主播当前有效守护公开列表，不返回购买与到期明细
// @Produce json
// @Param anchorNo query string true "主播公开编号"
// @Param page query int false "1..100"
// @Param pageSize query int false "1..50"
// @Success 200 {object} response.Response{data=guardRes.PublicMembers}
// @Router /v1/app/anchor-home/guards [get]
func (PublicAPI) Guards(c *gin.Context) {
	var q req.GuardsQuery
	if !httpkit.Query(c, &q) {
		return
	}
	out, err := publicService.Guards(c.Request.Context(), publicViewer(c), q)
	respondPublic(c, out, err)
}

// Settings
// @Tags AnchorHome
// @Security AppBearerAuth
// @Summary 本人主播简介与内容分类编辑快照
// @Produce json
// @Success 200 {object} response.Response{data=homeRes.Settings}
// @Router /v1/app/anchor-home/settings [get]
func (PublicAPI) Settings(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		httpkit.Invalid(c)
		return
	}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, err := publicService.Settings(c.Request.Context(), user)
	respondPublic(c, out, err)
}

// SaveSettings
// @Tags AnchorHome
// @Security AppBearerAuth
// @Summary 仅修改本人主播简介与分类，版本冲突拒绝覆盖
// @Accept json
// @Produce json
// @Param data body req.SaveSettings true "完整的窄字段命令，version来自GET settings"
// @Success 200 {object} response.Response{data=homeRes.Settings}
// @Router /v1/app/anchor-home/settings [post]
func (PublicAPI) SaveSettings(c *gin.Context) {
	var input struct {
		Signature  *string `json:"signature"`
		CategoryID *uint64 `json:"categoryId"`
		Version    *string `json:"version"`
	}
	if c.Request.URL.RawQuery != "" {
		httpkit.Invalid(c)
		return
	}
	if !httpkit.Body(c, &input) {
		return
	}
	if input.Signature == nil || input.CategoryID == nil || input.Version == nil {
		httpkit.Invalid(c)
		return
	}
	r := req.SaveSettings{Signature: *input.Signature, CategoryID: *input.CategoryID, Version: *input.Version}
	user, ok := httpkit.User(c)
	if !ok {
		return
	}
	out, err := publicService.SaveSettings(c.Request.Context(), user, r)
	if err == nil && global.GVA_LOG != nil {
		global.GVA_LOG.Info("anchor_home_settings_saved", zap.String("traceId", middleware.RequestID(c)), zap.Uint64("ownerId", user), zap.String("version", out.Version))
	}
	respondPublic(c, out, err)
}
