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
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveSessionPrepareReq true "本场标题、封面、分类和可见范围"
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
	result, err := roomService.PrepareSession(userID, req)
	if err != nil {
		respondLiveRoomError(c, "创建直播场次失败", err)
		return
	}
	response.OkWithDetailed(result, "场次已准备", c)
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
// @Description 准备中的场次会取消；直播中的场次进入结束中，由后台任务幂等结算后变为已结束。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveSessionEndReq true "场次对外编号"
// @Success 200 {object} response.Response
// @Router /v1/app/live/room/session/end [post]
func (a *RoomApi) End(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	var req liveReq.LiveSessionEndReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("结束参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.RequestOwnerEnd(userID, req.SessionNo); err != nil {
		respondLiveRoomError(c, "结束直播失败", err)
		return
	}
	response.OkWithMessage("直播正在结束", c)
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
// @Summary 获取正在直播的公开直播间
// @Description 只返回正常、公开、正在直播且主播状态正常的房间，不暴露任何内部主键。
// @Produce application/json
// @Param roomNo query string true "直播间对外编号"
// @Success 200 {object} response.Response{data=liveRes.LiveRoomPublicItem}
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
		liveService.ErrLiveRoomActiveSessionExists, liveService.ErrLiveRoomStatusReasonRequired,
		liveService.ErrLiveSessionNotFound, liveService.ErrLiveSessionStateInvalid,
		liveService.ErrLivePublishTokenInvalid, liveService.ErrLiveHookUnauthorized,
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
