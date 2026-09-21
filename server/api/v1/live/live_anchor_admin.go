package live

import (
	"tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"
	"tb_live_module/utils"

	"github.com/gin-gonic/gin"
)

type AnchorAdminApi struct{}

// Keep the response package imported for Swagger schema resolution.
var _ = liveRes.AnchorAdminDetailResp{}

// List
// @Tags        LiveAnchorAdmin
// @Summary     分页查询主播列表
// @Description 支持主播 ID/编号/用户 ID、昵称、类型、分类、公会、审核/认证/账号/权限/风控状态、来源、渠道和创建日期筛选。
// @Description 仅主播编号和昵称使用 LIKE，不使用 CONCAT 或跨字段全模糊查询；默认按 id DESC 排序。
// @Description 当前列表不关联查询用户或公会表，不产生 N+1 查询。
// @Security    ApiKeyAuth
// @Produce     application/json
// @Param       page                 query     int     true   "页码，从 1 开始"
// @Param       pageSize             query     int     true   "每页数量，最大 100"
// @Param       anchorId             query     uint    false  "主播 ID"
// @Param       anchorNo             query     string  false  "主播编号，模糊匹配"
// @Param       userId               query     uint64  false  "客户端用户 ID"
// @Param       nickname             query     string  false  "主播昵称，模糊匹配"
// @Param       anchorType           query     int     false  "主播类型：0普通 1官方 2内部运营"
// @Param       categoryId           query     uint64  false  "直播分类 ID"
// @Param       agencyId             query     uint64  false  "公会 ID"
// @Param       applyStatus          query     int     false  "申请状态：0未申请 1审核中 2通过 3拒绝"
// @Param       certStatus           query     int     false  "认证状态：0未认证 1认证中 2已认证 3失败"
// @Param       status               query     int     false  "账号状态：0禁用 1正常 2封禁 3注销"
// @Param       livePermission       query     int     false  "开播权限：0禁止 1允许"
// @Param       pkPermission         query     int     false  "PK 权限：0禁止 1允许"
// @Param       recommendPermission  query     int     false  "推荐准入权限：0禁止 1允许"
// @Param       withdrawPermission   query     int     false  "运营提现权限：0禁止 1允许"
// @Param       isSigned             query     int     false  "是否签约：0否 1是"
// @Param       isRecommended        query     int     false  "是否人工推荐：0否 1是"
// @Param       riskLevel            query     int     false  "风险等级：0正常 1低 2中 3高"
// @Param       source               query     string  false  "来源"
// @Param       channelId            query     uint64  false  "渠道 ID"
// @Param       createdAtStart       query     string  false  "创建日期起，YYYY-MM-DD"
// @Param       createdAtEnd         query     string  false  "创建日期止，YYYY-MM-DD（含当天）"
// @Success     200  {object}  response.Response{data=liveRes.AnchorAdminListResp}  "列表、总数和分页信息"
// @Failure     401  {object}  response.Response                                    "未登录或 token 无效"
// @Failure     200  {object}  response.Response                                    "参数错误、无 RBAC 权限或查询失败"
// @Router      /live/anchor/list [get]
func (a *AnchorAdminApi) List(c *gin.Context) {
	var req liveReq.AnchorAdminListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	if req.Page <= 0 || req.PageSize <= 0 {
		response.FailWithMessage("page 和 pageSize 必须大于 0", c)
		return
	}
	list, total, err := anchorService.GetAnchorList(&req)
	if err != nil {
		respondAnchorError(c, "获取主播列表失败", err)
		return
	}
	response.OkWithDetailed(response.PageResult{List: list, Total: total, Page: req.Page, PageSize: req.PageSize}, "获取成功", c)
}

// Detail
// @Tags        LiveAnchorAdmin
// @Summary     获取主播后台完整详情
// @Description 根据 anchorId 返回资料、审核、认证、状态、权限、运营、统计、来源、风控、备注和扩展信息。
// @Description 使用独立后台 Response DTO，不直接返回 GORM Model，避免模型新增字段后意外泄露。
// @Security    ApiKeyAuth
// @Produce     application/json
// @Param       anchorId  query     uint                                                     true  "主播 ID"
// @Success     200       {object}  response.Response{data=liveRes.AnchorAdminDetailResp}  "获取成功"
// @Failure     401       {object}  response.Response                                         "未登录或 token 无效"
// @Failure     200       {object}  response.Response                                         "参数错误、无 RBAC 权限或主播不存在"
// @Router      /live/anchor/detail [get]
func (a *AnchorAdminApi) Detail(c *gin.Context) {
	var req liveReq.AnchorAdminDetailReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	result, err := anchorService.GetAnchorAdminDetail(req.AnchorId)
	if err != nil {
		respondAnchorError(c, "获取主播详情失败", err)
		return
	}
	response.OkWithDetailed(result, "获取成功", c)
}

// Audit
// @Tags        LiveAnchorAdmin
// @Summary     审核主播申请
// @Description 只允许审核中的申请流转到通过(2)或拒绝(3)，不能重复审核或把已通过申请改为拒绝。
// @Description 拒绝时 rejectReason 必填并关闭直播/PK 权限；通过时清空拒绝原因，但第一版审核与业务权限分离，不自动开启直播、PK、提现权限。
// @Description auditUserId 从当前管理后台 JWT 获取，审核时间使用毫秒时间戳。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorAuditReq  true  "主播 ID、目标申请状态和拒绝原因"
// @Success     200   {object}  response.Response         "审核成功"
// @Failure     401   {object}  response.Response         "未登录或 token 无效"
// @Failure     200   {object}  response.Response         "参数错误、无 RBAC 权限、状态流转非法或主播不存在"
// @Router      /live/anchor/audit [post]
func (a *AnchorAdminApi) Audit(c *gin.Context) {
	var req liveReq.AnchorAuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("审核参数错误: "+err.Error(), c)
		return
	}
	if err := anchorService.AuditAnchor(uint64(utils.GetUserID(c)), req); err != nil {
		respondAnchorError(c, "审核主播失败", err)
		return
	}
	response.OkWithMessage("审核成功", c)
}

// UpdateStatus
// @Tags        LiveAnchorAdmin
// @Summary     修改主播账号状态
// @Description 状态包括禁用(0)、正常(1)、封禁(2)、注销(3)。禁用和封禁必须填写原因。
// @Description 封禁时 banUntil=0 表示永久封禁，未来毫秒时间戳表示临时封禁；恢复正常会清空原因和 banUntil。
// @Description 注销只改变状态、不删除数据，且注销后不能通过本接口恢复。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorStatusUpdateReq  true  "账号状态、原因和封禁截止时间"
// @Success     200   {object}  response.Response               "修改成功"
// @Failure     401   {object}  response.Response               "未登录或 token 无效"
// @Failure     200   {object}  response.Response               "参数错误、无 RBAC 权限、非法状态或主播不存在"
// @Router      /live/anchor/status/update [post]
func (a *AnchorAdminApi) UpdateStatus(c *gin.Context) {
	var req liveReq.AnchorStatusUpdateReq
	if !bindAnchorAdminJSON(c, &req, "状态参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorStatus(req); err != nil {
		respondAnchorError(c, "修改主播状态失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdatePermission
// @Tags        LiveAnchorAdmin
// @Summary     修改主播功能权限
// @Description 修改指定主播的直播、PK、推荐准入和运营提现权限，四个字段必须完整传入且只允许 0/1。
// @Description livePermission=0 时不得开启 pkPermission；接口校验失败而不会偷偷联动修改。
// @Description recommendPermission 表示是否允许进入推荐系统，与 isRecommended 人工推荐状态不同。
// @Description 本接口不修改主播状态、审核状态、推荐运营属性或风控状态。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorPermissionUpdateReq  true  "四项主播权限"
// @Success     200   {object}  response.Response                   "修改成功"
// @Failure     401   {object}  response.Response                   "未登录或 token 无效"
// @Failure     200   {object}  response.Response                   "参数错误、无 RBAC 权限、权限冲突或主播不存在"
// @Router      /live/anchor/permission/update [post]
func (a *AnchorAdminApi) UpdatePermission(c *gin.Context) {
	var req liveReq.AnchorPermissionUpdateReq
	if !bindAnchorAdminJSON(c, &req, "权限参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorPermission(req); err != nil {
		respondAnchorError(c, "修改主播权限失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateProfile
// @Tags        LiveAnchorAdmin
// @Summary     后台修改主播资料
// @Description 只允许修改公开资料以及 anchorType；level 第一版不开放人工调整。
// @Description 空字符串、0、空标签和 null 生日均按合法零值显式更新，不使用 Save 或 struct Updates。
// @Description categoryId=0 表示清空分类；非 0 时必须来自当前启用的直播分类。
// @Description 审核、认证、状态、权限、推荐、签约、公会、风控、统计和来源均由独立接口或系统维护。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorAdminProfileUpdateReq  true  "后台资料白名单"
// @Success     200   {object}  response.Response                    "修改成功"
// @Failure     401   {object}  response.Response                    "未登录或 token 无效"
// @Failure     200   {object}  response.Response                    "参数错误、无 RBAC 权限或主播不存在"
// @Router      /live/anchor/profile/update [post]
func (a *AnchorAdminApi) UpdateProfile(c *gin.Context) {
	var req liveReq.AnchorAdminProfileUpdateReq
	if !bindAnchorAdminJSON(c, &req, "资料参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorProfile(req); err != nil {
		respondAnchorError(c, "修改主播资料失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateRecommend
// @Tags        LiveAnchorAdmin
// @Summary     修改主播运营推荐属性
// @Description 只修改人工推荐、新人期截止毫秒时间戳、人工排序和推荐基础权重。
// @Description 本接口不修改 recommendPermission；当推荐准入权限为 0 时，不能设置 isRecommended=1。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorRecommendUpdateReq  true  "运营推荐属性"
// @Success     200   {object}  response.Response                  "修改成功"
// @Failure     401   {object}  response.Response                  "未登录或 token 无效"
// @Failure     200   {object}  response.Response                  "参数错误、无 RBAC 权限、推荐权限关闭或主播不存在"
// @Router      /live/anchor/recommend/update [post]
func (a *AnchorAdminApi) UpdateRecommend(c *gin.Context) {
	var req liveReq.AnchorRecommendUpdateReq
	if !bindAnchorAdminJSON(c, &req, "推荐参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorRecommend(req); err != nil {
		respondAnchorError(c, "修改推荐属性失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateSigned
// @Tags        LiveAnchorAdmin
// @Summary     修改主播签约状态
// @Description 只修改 isSigned；签约状态不等同于 anchorType、agencyId 或认证状态，不做任何联动。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorSignedUpdateReq  true  "主播 ID 和签约状态"
// @Success     200   {object}  response.Response               "修改成功"
// @Failure     401   {object}  response.Response               "未登录或 token 无效"
// @Failure     200   {object}  response.Response               "参数错误、无 RBAC 权限或主播不存在"
// @Router      /live/anchor/signed/update [post]
func (a *AnchorAdminApi) UpdateSigned(c *gin.Context) {
	var req liveReq.AnchorSignedUpdateReq
	if !bindAnchorAdminJSON(c, &req, "签约参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorSigned(req); err != nil {
		respondAnchorError(c, "修改签约状态失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateAgency
// @Tags        LiveAnchorAdmin
// @Summary     修改主播公会归属
// @Description agencyId=0 表示移出公会并清空加入时间；加入或更换公会会把加入时间更新为当前毫秒时间戳。
// @Description 当前代码库尚无 Agency 模型，本接口不创建虚假公会表或外键；接入公会模块后需补充存在性和状态校验。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorAgencyUpdateReq  true  "主播 ID 和目标公会 ID"
// @Success     200   {object}  response.Response               "修改成功"
// @Failure     401   {object}  response.Response               "未登录或 token 无效"
// @Failure     200   {object}  response.Response               "参数错误、无 RBAC 权限或主播不存在"
// @Router      /live/anchor/agency/update [post]
func (a *AnchorAdminApi) UpdateAgency(c *gin.Context) {
	var req liveReq.AnchorAgencyUpdateReq
	if !bindAnchorAdminJSON(c, &req, "公会参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorAgency(req); err != nil {
		respondAnchorError(c, "修改公会归属失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateRisk
// @Tags        LiveAnchorAdmin
// @Summary     修改主播风险等级
// @Description 只修改 riskLevel（0正常、1低、2中、3高），不联动状态或任何权限字段。
// @Description 实际能否开播由 live/check 实时综合判断；第一版高风险禁止开播，中风险不扩大限制。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorRiskUpdateReq  true  "主播 ID 和风险等级"
// @Success     200   {object}  response.Response             "修改成功"
// @Failure     401   {object}  response.Response             "未登录或 token 无效"
// @Failure     200   {object}  response.Response             "参数错误、无 RBAC 权限或主播不存在"
// @Router      /live/anchor/risk/update [post]
func (a *AnchorAdminApi) UpdateRisk(c *gin.Context) {
	var req liveReq.AnchorRiskUpdateReq
	if !bindAnchorAdminJSON(c, &req, "风控参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorRisk(req); err != nil {
		respondAnchorError(c, "修改风险等级失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateRemark
// @Tags        LiveAnchorAdmin
// @Summary     修改主播后台备注
// @Description 只修改后台可见的 remark，按 UTF-8 字符数限制为 500；APP 的任何 Response DTO 均不包含该字段。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorRemarkUpdateReq  true  "主播 ID 和后台备注"
// @Success     200   {object}  response.Response               "修改成功"
// @Failure     401   {object}  response.Response               "未登录或 token 无效"
// @Failure     200   {object}  response.Response               "参数错误、无 RBAC 权限或主播不存在"
// @Router      /live/anchor/remark/update [post]
func (a *AnchorAdminApi) UpdateRemark(c *gin.Context) {
	var req liveReq.AnchorRemarkUpdateReq
	if !bindAnchorAdminJSON(c, &req, "备注参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorRemark(req); err != nil {
		respondAnchorError(c, "修改主播备注失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// UpdateCert
// @Tags        LiveAnchorAdmin
// @Summary     修改主播认证状态
// @Description 只修改认证状态、类型和展示名称；证件号、证件图片等认证资料不存入 LiveAnchor。
// @Description 未认证(0)要求 certType=0 且 certName 为空；认证中(1)/已认证(2)要求类型非 0；已认证还要求展示名称非空。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.AnchorCertUpdateReq  true  "主播认证状态、类型和展示名"
// @Success     200   {object}  response.Response             "修改成功"
// @Failure     401   {object}  response.Response             "未登录或 token 无效"
// @Failure     200   {object}  response.Response             "参数错误、无 RBAC 权限、认证状态不一致或主播不存在"
// @Router      /live/anchor/cert/update [post]
func (a *AnchorAdminApi) UpdateCert(c *gin.Context) {
	var req liveReq.AnchorCertUpdateReq
	if !bindAnchorAdminJSON(c, &req, "认证参数错误") {
		return
	}
	if err := anchorService.UpdateAnchorCert(req); err != nil {
		respondAnchorError(c, "修改主播认证失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

func bindAnchorAdminJSON(c *gin.Context, target interface{}, prefix string) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		response.FailWithMessage(prefix+": "+err.Error(), c)
		return false
	}
	return true
}
