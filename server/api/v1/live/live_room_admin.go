package live

import (
	"tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"

	"github.com/gin-gonic/gin"
)

type RoomAdminApi struct{}

var _ = liveRes.LiveRoomAdminListResp{}
var _ = liveRes.LiveSessionAdminListResp{}

// RoomList
// @Tags LiveRoomAdmin
// @Summary 分页查询直播间
// @Security ApiKeyAuth
// @Produce application/json
// @Param page query int true "页码"
// @Param pageSize query int true "每页数量"
// @Param roomNo query string false "直播间编号"
// @Param anchorNo query string false "主播编号"
// @Param title query string false "标题"
// @Param categoryId query uint64 false "分类ID"
// @Param status query int false "房间状态：0禁用 1正常 2关闭"
// @Param liveStatus query int false "直播状态：0未开播 1准备中 2直播中 3结束中"
// @Success 200 {object} response.Response{data=liveRes.LiveRoomAdminListResp}
// @Router /live/room/list [get]
func (a *RoomAdminApi) RoomList(c *gin.Context) {
	var req liveReq.LiveRoomAdminListReq
	if err := c.ShouldBindQuery(&req); err != nil || req.Page <= 0 || req.PageSize <= 0 {
		response.FailWithMessage("查询参数错误", c)
		return
	}
	list, total, err := roomService.GetAdminRoomList(req)
	if err != nil {
		respondLiveRoomError(c, "获取直播间列表失败", err)
		return
	}
	response.OkWithDetailed(response.PageResult{List: list, Total: total, Page: req.Page, PageSize: req.PageSize}, "获取成功", c)
}

// RoomDetail
// @Tags LiveRoomAdmin
// @Summary 获取直播间后台详情
// @Security ApiKeyAuth
// @Produce application/json
// @Param roomId query uint true "直播间内部ID"
// @Success 200 {object} response.Response{data=liveRes.LiveRoomAdminItem}
// @Router /live/room/detail [get]
func (a *RoomAdminApi) RoomDetail(c *gin.Context) {
	var req liveReq.LiveRoomAdminDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	result, err := roomService.GetAdminRoomDetail(req.RoomID)
	if err != nil {
		respondLiveRoomError(c, "获取直播间详情失败", err)
		return
	}
	response.OkWithDetailed(result, "获取成功", c)
}

// UpdateRoom
// @Tags LiveRoomAdmin
// @Summary 后台修改直播间编号和资料
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveRoomAdminUpdateReq true "直播间资料"
// @Success 200 {object} response.Response
// @Router /live/room/update [post]
func (a *RoomAdminApi) UpdateRoom(c *gin.Context) {
	var req liveReq.LiveRoomAdminUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("修改参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.UpdateAdminRoom(req); err != nil {
		respondLiveRoomError(c, "修改直播间失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateRoomStatus
// @Tags LiveRoomAdmin
// @Summary 修改直播间状态
// @Description 禁用必须填写原因；活动直播间被禁用或关闭时，当前场次会进入结束中。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveRoomAdminStatusReq true "状态和原因"
// @Success 200 {object} response.Response
// @Router /live/room/status/update [post]
func (a *RoomAdminApi) UpdateRoomStatus(c *gin.Context) {
	var req liveReq.LiveRoomAdminStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("状态参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.UpdateAdminRoomStatus(req); err != nil {
		respondLiveRoomError(c, "修改直播间状态失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// SessionList
// @Tags LiveSessionAdmin
// @Summary 分页查询直播场次
// @Security ApiKeyAuth
// @Produce application/json
// @Param page query int true "页码"
// @Param pageSize query int true "每页数量"
// @Param sessionNo query string false "场次编号"
// @Param roomNo query string false "直播间编号"
// @Param anchorNo query string false "主播编号"
// @Param categoryId query uint64 false "分类ID"
// @Param status query int false "场次状态"
// @Param startedAtStart query int64 false "开播时间起，毫秒时间戳"
// @Param startedAtEnd query int64 false "开播时间止，毫秒时间戳"
// @Success 200 {object} response.Response{data=liveRes.LiveSessionAdminListResp}
// @Router /live/session/list [get]
func (a *RoomAdminApi) SessionList(c *gin.Context) {
	var req liveReq.LiveSessionAdminListReq
	if err := c.ShouldBindQuery(&req); err != nil || req.Page <= 0 || req.PageSize <= 0 {
		response.FailWithMessage("查询参数错误", c)
		return
	}
	list, total, err := roomService.GetAdminSessionList(req)
	if err != nil {
		respondLiveRoomError(c, "获取直播场次列表失败", err)
		return
	}
	response.OkWithDetailed(response.PageResult{List: list, Total: total, Page: req.Page, PageSize: req.PageSize}, "获取成功", c)
}

// SessionDetail
// @Tags LiveSessionAdmin
// @Summary 获取直播场次后台详情
// @Security ApiKeyAuth
// @Produce application/json
// @Param sessionId query uint true "场次内部ID"
// @Success 200 {object} response.Response{data=liveRes.LiveSessionAdminItem}
// @Router /live/session/detail [get]
func (a *RoomAdminApi) SessionDetail(c *gin.Context) {
	var req liveReq.LiveSessionAdminDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	result, err := roomService.GetAdminSessionDetail(req.SessionID)
	if err != nil {
		respondLiveRoomError(c, "获取直播场次详情失败", err)
		return
	}
	response.OkWithDetailed(result, "获取成功", c)
}

// EndSession
// @Tags LiveSessionAdmin
// @Summary 后台强制结束直播场次
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.LiveSessionAdminEndReq true "场次ID和原因"
// @Success 200 {object} response.Response
// @Router /live/session/end [post]
func (a *RoomAdminApi) EndSession(c *gin.Context) {
	var req liveReq.LiveSessionAdminEndReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("结束参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.RequestAdminEnd(req.SessionID, req.Reason); err != nil {
		respondLiveRoomError(c, "强制结束直播失败", err)
		return
	}
	response.OkWithMessage("直播正在结束", c)
}
