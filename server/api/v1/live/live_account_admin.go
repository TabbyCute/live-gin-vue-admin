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

type AccountAdminApi struct{}

var _ = liveRes.AccountAdminListResp{}

// List
// @Tags LiveAccountAdmin
// @Summary 分页查询客户端用户
// @Security ApiKeyAuth
// @Produce application/json
// @Param page query int true "页码"
// @Param pageSize query int true "每页数量，最大100"
// @Param accountNo query string false "对外账号编号前缀"
// @Param username query string false "内部用户名前缀"
// @Param phone query string false "E.164完整手机号，精确查询"
// @Param nickname query string false "昵称"
// @Param status query int false "账号状态：1正常 2禁用 3封禁 4注销"
// @Param realNameStatus query int false "实名状态：0未认证 1审核中 2已认证 3未通过 4过期或撤销"
// @Param registerSource query string false "注册来源"
// @Param registerChannelId query int false "注册渠道ID"
// @Param createdAtStart query string false "注册日期起，YYYY-MM-DD"
// @Param createdAtEnd query string false "注册日期止，YYYY-MM-DD（含当天）"
// @Success 200 {object} response.Response{data=liveRes.AccountAdminListResp} "获取成功"
// @Failure 401 {object} response.Response "未登录或token无效"
// @Failure 200 {object} response.Response "参数错误或查询失败"
// @Router /live/account/list [get]
func (a *AccountAdminApi) List(c *gin.Context) {
	var req liveReq.AccountAdminListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	if req.Page <= 0 || req.PageSize <= 0 {
		response.FailWithMessage("page和pageSize必须大于0", c)
		return
	}
	list, total, err := accountService.GetAdminList(&req)
	if err != nil {
		respondAccountAdminError(c, "获取用户列表失败", err)
		return
	}
	response.OkWithDetailed(response.PageResult{List: list, Total: total, Page: req.Page, PageSize: req.PageSize}, "获取成功", c)
}

// Detail
// @Tags LiveAccountAdmin
// @Summary 获取客户端用户后台详情
// @Security ApiKeyAuth
// @Produce application/json
// @Param accountId query int true "用户ID"
// @Success 200 {object} response.Response{data=liveRes.AccountAdminDetailResp} "获取成功"
// @Failure 401 {object} response.Response "未登录或token无效"
// @Failure 200 {object} response.Response "参数错误或用户不存在"
// @Router /live/account/detail [get]
func (a *AccountAdminApi) Detail(c *gin.Context) {
	var req liveReq.AccountAdminDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	detail, err := accountService.GetAdminDetail(req.AccountID)
	if err != nil {
		respondAccountAdminError(c, "获取用户详情失败", err)
		return
	}
	response.OkWithDetailed(detail, "获取成功", c)
}

// UpdateProfile
// @Tags LiveAccountAdmin
// @Summary 后台修改客户端用户公开资料
// @Description phone不传表示保持不变，提供时必须为非空E.164格式；后台换号直接视为已验证并使既有客户端JWT失效。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.AccountAdminProfileUpdateReq true "用户ID和资料白名单"
// @Success 200 {object} response.Response "修改成功"
// @Failure 401 {object} response.Response "仅高级管理员可操作"
// @Router /live/account/profile/update [post]
func (a *AccountAdminApi) UpdateProfile(c *gin.Context) {
	var req liveReq.AccountAdminProfileUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("资料参数错误: "+err.Error(), c)
		return
	}
	if err := accountService.AdminUpdateProfile(req); err != nil {
		respondAccountAdminError(c, "修改用户资料失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateStatus
// @Tags LiveAccountAdmin
// @Summary 修改客户端用户状态
// @Description 非正常状态必须填写原因；封禁截止为0表示永久封禁。操作会立即使既有客户端JWT失效。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.AccountAdminStatusUpdateReq true "用户状态"
// @Success 200 {object} response.Response "修改成功"
// @Failure 401 {object} response.Response "仅高级管理员可操作"
// @Router /live/account/status/update [post]
func (a *AccountAdminApi) UpdateStatus(c *gin.Context) {
	var req liveReq.AccountAdminStatusUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("状态参数错误: "+err.Error(), c)
		return
	}
	if err := accountService.AdminUpdateStatus(req); err != nil {
		respondAccountAdminError(c, "修改用户状态失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UnbindPhone
// @Tags LiveAccountAdmin
// @Summary 手机号解绑已禁用
// @Description 纯手机号登录下禁止解绑，请通过修改资料更换手机号。
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.AccountAdminActionReq true "用户ID"
// @Failure 200 {object} response.Response "纯手机号登录下禁止解绑"
// @Failure 401 {object} response.Response "仅高级管理员可操作"
// @Router /live/account/phone/unbind [post]
func (a *AccountAdminApi) UnbindPhone(c *gin.Context) {
	var req liveReq.AccountAdminActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("解绑参数错误: "+err.Error(), c)
		return
	}
	respondAccountAdminError(c, "解绑手机号失败", accountService.AdminUnbindPhone(req.AccountID))
}

// LogoutAll
// @Tags LiveAccountAdmin
// @Summary 强制客户端用户全部登录失效
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.AccountAdminActionReq true "用户ID"
// @Success 200 {object} response.Response "操作成功"
// @Failure 401 {object} response.Response "仅高级管理员可操作"
// @Router /live/account/logout-all [post]
func (a *AccountAdminApi) LogoutAll(c *gin.Context) {
	var req liveReq.AccountAdminActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("操作参数错误: "+err.Error(), c)
		return
	}
	if err := accountService.AdminLogoutAll(req.AccountID); err != nil {
		respondAccountAdminError(c, "强制下线失败", err)
		return
	}
	response.OkWithMessage("全部客户端登录已失效", c)
}

func respondAccountAdminError(c *gin.Context, fallback string, err error) {
	switch {
	case errors.Is(err, liveService.ErrAccountNotFound),
		errors.Is(err, liveService.ErrPhoneRequired),
		errors.Is(err, liveService.ErrInvalidPhone),
		errors.Is(err, liveService.ErrPhoneExists),
		errors.Is(err, liveService.ErrInvalidProfile),
		errors.Is(err, liveService.ErrAccountStatusReasonRequired),
		errors.Is(err, liveService.ErrAccountBanUntilInvalid),
		errors.Is(err, liveService.ErrAccountCancelledCannotRestore):
		response.FailWithMessage(err.Error(), c)
	default:
		global.GVA_LOG.Error(fallback, zap.Error(err))
		response.FailWithMessage(fallback, c)
	}
}
