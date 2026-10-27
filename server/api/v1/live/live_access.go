package live

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"
	liveService "tb_live_module/service/live"
	"time"
)

// Playback
// @Tags LiveRoomApp
// @Summary 为当前已登录观众签发30秒HLS播放凭证
// @Description 复用直播间权限入口。每个媒体请求必须同时携带APP Authorization和X-Playback-Token；只开放已验证网关的HLS，FLV/WebRTC受保护播放未开放。expiresAt为毫秒。
// @Security AppBearerAuth
// @Produce application/json
// @Param roomNo query string true "直播间公开编号，例如100001"
// @Success 200 {object} response.Response{data=liveService.PlaybackAuthorization}
// @Failure 401 {object} response.Response
// @Router /v1/app/live/room/playback [get]
func (a *RoomApi) Playback(c *gin.Context) {
	var r liveReq.LiveRoomPublicDetailReq
	if c.ShouldBindQuery(&r) != nil {
		response.FailWithMessage("参数错误", c)
		return
	}
	id, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("未登录", c)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	out, err := (&liveService.LiveAccessService{}).AuthorizePlayback(ctx, uint64(id), r.RoomNo)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.OkWithData(out, c)
}

// PlaybackCheck
// @Tags LiveMediaGateway
// @Summary 媒体网关逐请求鉴权适配（不返回业务JSON）
// @Description Nginx auth_request 专用HTTP状态契约：204允许，403拒绝。APP JWT仍须通过；复制URL不能代替Bearer。X-Media-Path必须是规范化媒体路径且仅支持HLS。
// @Security AppBearerAuth
// @Param X-Playback-Token header string true "30秒播放凭证"
// @Param X-Media-Path header string true "例如/tb_live/stream_name.m3u8"
// @Success 204 "允许媒体代理读取当前资源"
// @Failure 403 "拒绝或依赖不可用"
// @Router /v1/app/live/room/playback/check [get]
func (a *RoomApi) PlaybackCheck(c *gin.Context) {
	id, ok := middleware.GetAppAccountID(c)
	if !ok {
		c.Status(http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := (&liveService.LiveAccessService{}).ValidatePlayback(ctx, uint64(id), c.GetHeader("X-Playback-Token"), c.GetHeader("X-Media-Path")); err != nil {
		c.Status(http.StatusForbidden)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}
