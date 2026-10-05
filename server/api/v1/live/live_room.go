package live

import (
	"errors"

	"tb_live_module/global"
	"tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"
	liveService "tb_live_module/service/live"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type RoomApi struct{}

var _ = liveRes.LiveRoomOwnerInfoResp{}

// Info
// @Tags LiveRoomApp
// @Summary 获取当前主播的直播间
// @Description 身份从客户端 JWT 获取；审核通过时系统通常已自动创建直播间，存量异常数据没有房间时返回 hasRoom=false。响应不包含数据库主键和推流密钥哈希。
// @Security AppBearerAuth
// @Produce application/json
// @Success 200 {object} response.Response{data=liveRes.LiveRoomOwnerInfoResp}
// @Router /v1/app/live/room/info [get]
func (a *RoomApi) Info(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	result, err := roomService.GetOwnerRoom(userID)
	if err != nil {
		respondLiveRoomError(c, "获取直播间失败", err)
		return
	}
	response.OkWithDetailed(result, "获取成功", c)
}

// Save
// @Tags LiveRoomApp
// @Summary 修改当前主播的直播间资料
// @Description 每名主播只有一个直播间；房间通常在审核通过时创建，首次保存仍会为存量异常数据补建，对外只返回 roomNo。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveRoomSaveReq true "直播间资料"
// @Success 200 {object} response.Response{data=liveRes.LiveRoomInfo}
// @Router /v1/app/live/room/save [post]
func (a *RoomApi) Save(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	var req liveReq.LiveRoomSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("直播间参数错误: "+err.Error(), c)
		return
	}
	result, err := roomService.SaveOwnerRoom(userID, req)
	if err != nil {
		respondLiveRoomError(c, "保存直播间失败", err)
		return
	}
	response.OkWithDetailed(result, "保存成功", c)
}

// UpdateOwnerStatus
// @Tags LiveRoomApp
// @Summary 主播关闭或重新开启直播间
// @Description 主播只允许在正常(1)和关闭(2)之间切换；活动场次存在时不能关闭。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveRoomOwnerStatusReq true "目标状态"
// @Success 200 {object} response.Response
// @Router /v1/app/live/room/status/update [post]
func (a *RoomApi) UpdateOwnerStatus(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	var req liveReq.LiveRoomOwnerStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("状态参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.UpdateOwnerRoomStatus(userID, *req.Status); err != nil {
		respondLiveRoomError(c, "修改直播间状态失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// Prepare
// @Tags LiveRoomApp
// @Summary 创建一场待推流的直播场次
// @Description 实时校验主播开播资格和直播间状态；返回一次性推流凭证，数据库仅保存其哈希。
// @Description 同一直播间在准备中、直播中、结束中最多只能有一个场次，由数据库唯一约束兜底。
// @Description 建议客户端为一次准备操作生成 Idempotency-Key，并在网络超时重试时复用；服务端会返回完全相同的推流地址。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveSessionPrepareReq true "本场标题、封面、分类和可见范围"
// @Param Idempotency-Key header string false "8到128位幂等键；同一次操作重试必须保持不变"
// @Success 200 {object} response.Response{data=liveRes.LiveSessionPrepareResp}
// @Router /v1/app/live/room/session/prepare [post]
func (a *RoomApi) Prepare(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	var req liveReq.LiveSessionPrepareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("开播参数错误: "+err.Error(), c)
		return
	}
	req.IdempotencyKey = c.GetHeader("Idempotency-Key")
	result, err := roomService.PrepareSession(userID, req)
	if err != nil {
		respondLiveRoomError(c, "创建直播场次失败", err)
		return
	}
	response.OkWithDetailed(result, "场次已准备", c)
}

// RefreshPushURL
// @Tags LiveRoomApp
// @Summary 为尚未开始推流的准备场次重新签发推流地址
// @Description 仅允许当前主播自己的 Preparing 场次，且尚未收到 SRS on_publish；不会延长 prepareDeadlineAt，旧地址立即失效。
// @Description Idempotency-Key 必填；相同键重试会返回完全相同的推流地址。每场累计签发次数受 live.push-url-refresh-limit 限制。
// @Security AppBearerAuth
// @Produce application/json
// @Param Idempotency-Key header string true "8到128位幂等键；同一次刷新重试必须保持不变"
// @Success 200 {object} response.Response{data=liveRes.LiveSessionPrepareResp}
// @Router /v1/app/live/room/session/push-url/refresh [post]
func (a *RoomApi) RefreshPushURL(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	result, err := roomService.RefreshOwnerPushURL(userID, c.GetHeader("Idempotency-Key"))
	if err != nil {
		respondLiveRoomError(c, "重新签发推流地址失败", err)
		return
	}
	response.OkWithDetailed(result, "推流地址已签发", c)
}

// Current
// @Tags LiveRoomApp
// @Summary 获取当前活动直播场次
// @Security AppBearerAuth
// @Produce application/json
// @Success 200 {object} response.Response{data=liveRes.LiveSessionInfo}
// @Router /v1/app/live/room/session/current [get]
func (a *RoomApi) Current(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	result, err := roomService.GetOwnerCurrentSession(userID)
	if err != nil {
		respondLiveRoomError(c, "获取当前场次失败", err)
		return
	}
	response.OkWithDetailed(result, "获取成功", c)
}

// End
// @Tags LiveRoomApp
// @Summary 主播结束直播场次
// @Description 根据 APP JWT 结束当前主播的活动场次，无需请求体。准备中的场次直接取消；直播中的场次先进入结束中并同步尝试SRS断流，成功后立即结算，失败由定时任务重试。接口具有幂等语义。
// @Security AppBearerAuth
// @Produce application/json
// @Success 200 {object} response.Response
// @Router /v1/app/live/room/session/end [post]
func (a *RoomApi) End(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	if err := roomService.RequestOwnerEnd(userID); err != nil {
		respondLiveRoomError(c, "结束直播失败", err)
		return
	}
	response.OkWithMessage("直播结束请求已处理", c)
}

// History
// @Tags LiveRoomApp
// @Summary 分页获取当前主播的直播场次历史
// @Security AppBearerAuth
// @Produce application/json
// @Param page query int true "页码"
// @Param pageSize query int true "每页数量"
// @Param status query int false "场次状态：0准备中 1直播中 2结束中 3已结束 4已取消 5失败"
// @Success 200 {object} response.Response{data=liveRes.LiveSessionHistoryResp}
// @Router /v1/app/live/room/session/history [get]
func (a *RoomApi) History(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	var req liveReq.LiveSessionHistoryReq
	if err := c.ShouldBindQuery(&req); err != nil || req.Page <= 0 || req.PageSize <= 0 {
		response.FailWithMessage("分页参数错误", c)
		return
	}
	list, total, err := roomService.GetOwnerSessionHistory(userID, req)
	if err != nil {
		respondLiveRoomError(c, "获取场次历史失败", err)
		return
	}
	response.OkWithDetailed(response.PageResult{List: list, Total: total, Page: req.Page, PageSize: req.PageSize}, "获取成功", c)
}

// PublicDetail
// @Tags LiveRoomApp
// @Summary 按房间号获取公开直播间
// @Description 未开播也可以查询；只返回房间正常、公开且主播审核通过并状态正常的房间，不暴露内部主键和服务端控制字段。latestSession 在有活动场次时返回当前场次，否则返回最近一次已结束直播；从未完成过直播时返回空对象 {}。
// @Produce application/json
// @Param roomNo query string true "直播间对外编号"
// @Success 200 {object} response.Response{data=liveRes.LiveRoomPublicItemV2}
// @Router /v1/app/live/room/detail [get]
func (a *RoomApi) PublicDetail(c *gin.Context) {
	var req liveReq.LiveRoomPublicDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	result, err := roomService.GetPublicRoom(req.RoomNo)
	if err != nil {
		respondLiveRoomError(c, "获取直播间失败", err)
		return
	}
	response.OkWithDetailed(result, "获取成功", c)
}

// PublicList
// @Tags LiveRoomApp
// @Summary 分页获取公开直播列表
// @Produce application/json
// @Param page query int true "页码"
// @Param pageSize query int true "每页数量"
// @Param categoryId query uint64 false "分类ID"
// @Success 200 {object} response.Response{data=liveRes.LiveRoomPublicListResp}
// @Router /v1/app/live/room/list [get]
func (a *RoomApi) PublicList(c *gin.Context) {
	var req liveReq.LiveRoomPublicListReq
	if err := c.ShouldBindQuery(&req); err != nil || req.Page <= 0 || req.PageSize <= 0 {
		response.FailWithMessage("分页参数错误", c)
		return
	}
	list, total, err := roomService.GetPublicRoomList(req)
	if err != nil {
		respondLiveRoomError(c, "获取直播列表失败", err)
		return
	}
	response.OkWithDetailed(response.PageResult{List: list, Total: total, Page: req.Page, PageSize: req.PageSize}, "获取成功", c)
}

func respondLiveRoomError(c *gin.Context, operation string, err error) {
	known := []error{
		liveService.ErrLiveRoomNotFound, liveService.ErrLiveRoomNoRequired, liveService.ErrLiveRoomNoExists,
		liveService.ErrLiveRoomUnavailable,
		liveService.ErrLiveRoomActiveSessionExists, liveService.ErrLiveRoomActiveSessionNotFound,
		liveService.ErrLiveRoomStatusReasonRequired,
		liveService.ErrLiveSessionNotFound, liveService.ErrLiveSessionStateInvalid,
		liveService.ErrLivePublishTokenInvalid, liveService.ErrLivePublishTokenConfigInvalid,
		liveService.ErrLiveIdempotencyKeyInvalid, liveService.ErrLiveIdempotencyUnavailable,
		liveService.ErrLiveIdempotencyConflict,
		liveService.ErrLivePushURLRefreshLimit,
		liveService.ErrLiveManualConfirmationTooEarly,
		liveService.ErrAnchorNotFound, liveService.ErrLiveCategoryNotFound,
		liveService.ErrLiveCategoryParentDisabled,
	}
	for _, target := range known {
		if errors.Is(err, target) {
			response.FailWithMessage(err.Error(), c)
			return
		}
	}
	global.GVA_LOG.Error(operation, zap.Error(err))
	response.FailWithMessage(operation, c)
}
