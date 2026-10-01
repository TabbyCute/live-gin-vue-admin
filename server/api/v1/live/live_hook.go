package live

import (
	"tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"

	"github.com/gin-gonic/gin"
)

type HookApi struct{}

// Publish
// @Tags LiveHook
// @Summary SRS发布流回调
// @Description 接口来源由部署层 IP 白名单保护；param 中必须包含 prepare 接口签发的 pt，服务端会解密并校验完整载荷。
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.SRSHookReq true "SRS回调"
// @Success 200 {object} response.Response
// @Router /v1/app/live/hook/publish [post]
func (a *HookApi) Publish(c *gin.Context) {
	var req liveReq.SRSHookReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("回调参数错误: "+err.Error(), c)
		return
	}
	if req.Action != "on_publish" {
		response.FailWithMessage("回调 action 必须为 on_publish", c)
		return
	}
	result, err := roomService.OnPublish(req)
	if err != nil {
		respondLiveRoomError(c, "处理发布流回调失败", err)
		return
	}
	response.OkWithDetailed(result, "回调处理成功", c)
}

// Unpublish
// @Tags LiveHook
// @Summary SRS停止发布流回调
// @Description 接口来源由部署层 IP 白名单保护；不直接结束场次，而是记录断流并开启可配置的重连窗口。
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.SRSHookReq true "SRS回调"
// @Success 200 {object} response.Response
// @Router /v1/app/live/hook/unpublish [post]
func (a *HookApi) Unpublish(c *gin.Context) {
	var req liveReq.SRSHookReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("回调参数错误: "+err.Error(), c)
		return
	}
	if req.Action != "on_unpublish" {
		response.FailWithMessage("回调 action 必须为 on_unpublish", c)
		return
	}
	if err := roomService.OnUnpublish(req); err != nil {
		respondLiveRoomError(c, "处理断流回调失败", err)
		return
	}
	response.OkWithMessage("回调处理成功", c)
}

// UpdateStats
// @Tags LiveHook
// @Summary 写入直播场次实时统计快照
// @Description 由部署层 IP 白名单保护，供可信统计服务写入；礼物流水仍应由独立账本保存，本接口只更新场次汇总。
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveSessionStatsReq true "场次统计"
// @Success 200 {object} response.Response
// @Router /v1/app/live/hook/session/stats [post]
func (a *HookApi) UpdateStats(c *gin.Context) {
	var req liveReq.LiveSessionStatsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("统计参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.UpdateSessionStats(req); err != nil {
		respondLiveRoomError(c, "更新场次统计失败", err)
		return
	}
	response.OkWithMessage("更新成功", c)
}
