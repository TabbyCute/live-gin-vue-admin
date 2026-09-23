package live

import (
	"errors"

	"tb_live_module/global"
	"tb_live_module/middleware"
	commonRes "tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"
	liveService "tb_live_module/service/live"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AnchorApi struct{}

// Keep the response package imported for Swagger schema resolution.
var _ = liveRes.MyAnchorInfoResp{}

// Info
// @Tags        LiveAnchorApp
// @Summary     获取当前用户的主播身份
// @Description 从客户端 JWT 获取当前账号 ID，并实时查询主播记录；客户端不能传 userId。
// @Description 尚未创建主播记录时返回 isAnchor=false、anchor=null，不把 record not found 当成接口错误。
// @Description 响应使用独立 DTO，不返回审核管理员、风控、后台备注、来源 ID 或扩展字段。
// @Security    AppBearerAuth
// @Produce     application/json
// @Success     200  {object}  commonRes.Response{data=liveRes.MyAnchorInfoResp}  "获取成功"
// @Failure     401  {object}  commonRes.Response                                "客户端 token 缺失、无效或账号禁用"
// @Failure     200  {object}  commonRes.Response                                "查询失败"
// @Router      /v1/app/live/anchor/info [get]
func (a *AnchorApi) Info(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	result, err := anchorService.GetMyAnchorInfo(userID)
	if err != nil {
		respondAnchorError(c, "获取主播信息失败", err)
		return
	}
	commonRes.OkWithDetailed(result, "获取成功", c)
}

// Apply
// @Tags        LiveAnchorApp
// @Summary     申请成为主播
// @Description 当前客户端账号首次申请时创建唯一主播记录；主播编号按“渠道号 + 六位序列”生成。
// @Description 审核中和已通过的申请不能重复提交；被拒后复用原记录和主播编号，清空上一轮审核结果后重新进入审核中。
// @Description userId 始终取自客户端 JWT；channelId 只在首次申请时写入，重新申请不能改写来源渠道。
// @Description categoryId=0 表示未分类；非 0 时必须来自当前启用的直播分类。
// @Security    AppBearerAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorApplyReq                                  true  "主播申请资料，不包含 userId"
// @Success     200   {object}  commonRes.Response{data=liveRes.AnchorApplyStatusResp}  "申请已提交"
// @Failure     401   {object}  commonRes.Response                                       "客户端 token 缺失、无效或账号禁用"
// @Failure     200   {object}  commonRes.Response                                       "参数错误、正在审核或已经是主播"
// @Router      /v1/app/live/anchor/apply [post]
func (a *AnchorApi) Apply(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	var req liveReq.AnchorApplyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		commonRes.FailWithMessage("申请参数错误: "+err.Error(), c)
		return
	}
	if _, err := anchorService.ApplyAnchor(userID, req); err != nil {
		respondAnchorError(c, "提交主播申请失败", err)
		return
	}
	result, err := anchorService.GetApplyStatus(userID)
	if err != nil {
		respondAnchorError(c, "读取申请状态失败", err)
		return
	}
	commonRes.OkWithDetailed(result, "主播申请已提交", c)
}

// ApplyStatus
// @Tags        LiveAnchorApp
// @Summary     获取当前用户的主播申请状态
// @Description userId 只从客户端 JWT 获取。未创建主播记录时返回 isApplied=false、applyStatus=0。
// @Security    AppBearerAuth
// @Produce     application/json
// @Success     200  {object}  commonRes.Response{data=liveRes.AnchorApplyStatusResp}  "获取成功"
// @Failure     401  {object}  commonRes.Response                                       "客户端 token 缺失、无效或账号禁用"
// @Failure     200  {object}  commonRes.Response                                       "查询失败"
// @Router      /v1/app/live/anchor/apply/status [get]
func (a *AnchorApi) ApplyStatus(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	result, err := anchorService.GetApplyStatus(userID)
	if err != nil {
		respondAnchorError(c, "获取申请状态失败", err)
		return
	}
	commonRes.OkWithDetailed(result, "获取成功", c)
}

// UpdateProfile
// @Tags        LiveAnchorApp
// @Summary     修改当前主播公开资料
// @Description 只允许修改昵称、头像、封面、签名、性别、生日、地区、语言、分类和标签。
// @Description userId 从 JWT 获取；请求中的空字符串、0、空标签和 null 生日均按合法零值显式更新。
// @Description categoryId=0 表示清空分类；非 0 时必须来自当前启用的直播分类。
// @Description 本接口不能修改主播编号、审核、状态、权限、认证、运营、统计、来源、风控和后台备注字段。
// @Security    AppBearerAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorProfileUpdateReq  true  "公开资料白名单"
// @Success     200   {object}  commonRes.Response               "修改成功"
// @Failure     401   {object}  commonRes.Response               "客户端 token 缺失、无效或账号禁用"
// @Failure     200   {object}  commonRes.Response               "参数错误、主播不存在或修改失败"
// @Router      /v1/app/live/anchor/profile/update [post]
func (a *AnchorApi) UpdateProfile(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	var req liveReq.AnchorProfileUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		commonRes.FailWithMessage("资料参数错误: "+err.Error(), c)
		return
	}
	if err := anchorService.UpdateMyAnchorProfile(userID, req); err != nil {
		respondAnchorError(c, "修改主播资料失败", err)
		return
	}
	commonRes.OkWithMessage("修改成功", c)
}

// Detail
// @Tags        LiveAnchorApp
// @Summary     获取主播公共主页资料
// @Description 根据唯一对外编号 anchorNo 查询已审核通过且状态正常的主播，只返回公开资料。
// @Description 客户端请求和响应均不暴露 LiveAnchor 数据库主键；同时不返回完整生日、userId、审核信息、封禁原因、提现权限、风控、备注、来源 ID 或扩展字段。
// @Produce     application/json
// @Param       anchorNo  query     string                                                    true  "主播对外编号"
// @Success     200       {object}  commonRes.Response{data=liveRes.AnchorPublicDetailResp}  "获取成功"
// @Failure     200       {object}  commonRes.Response                                        "参数错误、主播不存在或不可公开"
// @Router      /v1/app/live/anchor/detail [get]
func (a *AnchorApi) Detail(c *gin.Context) {
	var req liveReq.AnchorPublicDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		commonRes.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	result, err := anchorService.GetAnchorPublicDetail(req.AnchorNo)
	if err != nil {
		respondAnchorError(c, "获取主播公开资料失败", err)
		return
	}
	commonRes.OkWithDetailed(result, "获取成功", c)
}

// CheckLive
// @Tags        LiveAnchorApp
// @Summary     检查当前主播开播资格
// @Description 实时检查主播存在、申请通过、账号状态、封禁、风险等级、直播权限和当前认证策略。
// @Description 高风险主播禁止开播；中风险第一版不扩大限制；认证第一版不是开播硬条件。
// @Description 临时封禁已到期但 status 尚未恢复时仍拒绝，本 GET 接口不会隐式修改数据库。
// @Security    AppBearerAuth
// @Produce     application/json
// @Success     200  {object}  commonRes.Response{data=liveRes.AnchorPermissionCheckResp}  "返回 allow、业务 code、说明和封禁截止时间"
// @Failure     401  {object}  commonRes.Response                                             "客户端 token 缺失、无效或账号禁用"
// @Failure     200  {object}  commonRes.Response                                             "检查失败"
// @Router      /v1/app/live/anchor/live/check [get]
func (a *AnchorApi) CheckLive(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	result, err := anchorService.CheckLivePermission(userID)
	if err != nil {
		respondAnchorError(c, "检查开播资格失败", err)
		return
	}
	commonRes.OkWithData(result, c)
}

// CheckPk
// @Tags        LiveAnchorApp
// @Summary     检查当前主播 PK 资格
// @Description PK 资格严格建立在完整开播资格之上；通过直播检查后再判断 pkPermission。
// @Description 本接口只判断资格，不创建直播间、LiveKit room 或 PK session。
// @Security    AppBearerAuth
// @Produce     application/json
// @Success     200  {object}  commonRes.Response{data=liveRes.AnchorPermissionCheckResp}  "返回 allow 和具体业务原因"
// @Failure     401  {object}  commonRes.Response                                             "客户端 token 缺失、无效或账号禁用"
// @Failure     200  {object}  commonRes.Response                                             "检查失败"
// @Router      /v1/app/live/anchor/pk/check [get]
func (a *AnchorApi) CheckPk(c *gin.Context) {
	userID, ok := appUserID(c)
	if !ok {
		return
	}
	result, err := anchorService.CheckPkPermission(userID)
	if err != nil {
		respondAnchorError(c, "检查 PK 资格失败", err)
		return
	}
	commonRes.OkWithData(result, c)
}

func appUserID(c *gin.Context) (uint64, bool) {
	accountID, ok := middleware.GetAppAccountID(c)
	if !ok {
		commonRes.NoAuth("未获取到客户端登录信息", c)
		return 0, false
	}
	return uint64(accountID), true
}

func respondAnchorError(c *gin.Context, operation string, err error) {
	if isAnchorBusinessError(err) {
		commonRes.FailWithMessage(err.Error(), c)
		return
	}
	global.GVA_LOG.Error(operation, zap.Error(err))
	commonRes.FailWithMessage(operation, c)
}

func isAnchorBusinessError(err error) bool {
	known := []error{
		liveService.ErrAnchorNotFound, liveService.ErrAnchorAlreadyExists,
		liveService.ErrAnchorApplyPending, liveService.ErrAnchorAlreadyApproved,
		liveService.ErrInvalidApplyStatus, liveService.ErrRejectReasonRequired,
		liveService.ErrStatusReasonRequired, liveService.ErrInvalidBanUntil,
		liveService.ErrCancelledCannotRestore, liveService.ErrInvalidPermission,
		liveService.ErrRecommendPermissionClosed, liveService.ErrInvalidCertState,
		liveService.ErrInvalidBirthday, liveService.ErrInvalidDateRange,
		liveService.ErrAnchorNoTooLong, liveService.ErrInvalidChannel,
		liveService.ErrLiveCategoryNotFound, liveService.ErrLiveRoomNoExists,
	}
	for _, target := range known {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
