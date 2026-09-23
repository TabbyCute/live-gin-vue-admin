package live

import (
	"tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"
	liveService "tb_live_module/service/live"

	"github.com/gin-gonic/gin"
)

type HookApi struct{}

func authorizeLiveHook(c *gin.Context) bool {
	if err := liveService.ValidateLiveHookToken(c.GetHeader("X-Live-Hook-Token")); err != nil {
		response.FailWithMessage(err.Error(), c)
		return false
	}
	return true
}

// Publish
// @Tags LiveHook
// @Summary SRS发布流回调
// @Description 必须携带 X-Live-Hook-Token；param 中必须包含 prepare 接口签发的 token。
// @Accept application/json
// @Produce application/json
// @Param X-Live-Hook-Token header string true "SRS回调密钥"
// @Param data body liveReq.SRSHookReq true "SRS回调"
// @Success 200 {object} response.Response
// @Router /v1/app/live/hook/publish [post]
func (a *HookApi) Publish(c *gin.Context) {
	if !authorizeLiveHook(c) {
		return
	}
	var req liveReq.SRSHookReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("回调参数错误: "+err.Error(), c)
		return
	}
	result, err := roomService.OnPublish(req.Stream, req.Param)
	if err != nil {
		respondLiveRoomError(c, "处理发布流回调失败", err)
		return
	}
	response.OkWithDetailed(result, "回调处理成功", c)
}

// Unpublish
// @Tags LiveHook
// @Summary SRS停止发布流回调
// @Description 不直接结束场次，而是记录断流并开启可配置的重连窗口。
// @Accept application/json
// @Produce application/json
// @Param X-Live-Hook-Token header string true "SRS回调密钥"
// @Param data body liveReq.SRSHookReq true "SRS回调"
// @Success 200 {object} response.Response
// @Router /v1/app/live/hook/unpublish [post]
func (a *HookApi) Unpublish(c *gin.Context) {
	if !authorizeLiveHook(c) {
		return
	}
	var req liveReq.SRSHookReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("回调参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.OnUnpublish(req.Stream); err != nil {
		respondLiveRoomError(c, "处理断流回调失败", err)
		return
	}
	response.OkWithMessage("回调处理成功", c)
}

// UpdateStats
// @Tags LiveHook
// @Summary 写入直播场次实时统计快照
// @Description 供可信统计服务写入；礼物流水仍应由独立账本保存，本接口只更新场次汇总。
// @Accept application/json
// @Produce application/json
// @Param X-Live-Hook-Token header string true "内部回调密钥"
// @Param data body liveReq.LiveSessionStatsReq true "场次统计"
// @Success 200 {object} response.Response
// @Router /v1/app/live/hook/session/stats [post]
func (a *HookApi) UpdateStats(c *gin.Context) {
	if !authorizeLiveHook(c) {
		return
	}
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
