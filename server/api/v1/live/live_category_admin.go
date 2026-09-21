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

type CategoryAdminApi struct{}

var _ = liveRes.LiveCategoryAdminListResp{}

// List
// @Tags        LiveCategoryAdmin
// @Summary     分页查询直播分类
// @Description 支持按编码/名称关键字、父分类和状态筛选，默认按 sort DESC、id ASC 排序。
// @Description 父分类名称批量查询，不产生 N+1 查询。
// @Security    ApiKeyAuth
// @Produce     application/json
// @Param       page      query     int    true   "页码，从 1 开始"
// @Param       pageSize  query     int    true   "每页数量，最大 100"
// @Param       keyword   query     string false  "分类编码或名称，模糊匹配"
// @Param       parentId  query     uint   false  "父分类 ID；0 表示只查询顶级分类"
// @Param       status    query     int    false  "状态：0停用 1启用"
// @Success     200  {object}  response.Response{data=liveRes.LiveCategoryAdminListResp}  "获取成功"
// @Failure     401  {object}  response.Response                                             "未登录或 token 无效"
// @Failure     200  {object}  response.Response                                             "参数错误、无权限或查询失败"
// @Router      /live/category/list [get]
func (a *CategoryAdminApi) List(c *gin.Context) {
	var req liveReq.LiveCategoryAdminListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	if req.Page <= 0 || req.PageSize <= 0 {
		response.FailWithMessage("page 和 pageSize 必须大于 0", c)
		return
	}
	list, total, err := categoryService.GetAdminCategoryList(&req)
	if err != nil {
		respondLiveCategoryError(c, "获取直播分类列表失败", err)
		return
	}
	response.OkWithDetailed(response.PageResult{
		List: list, Total: total, Page: req.Page, PageSize: req.PageSize,
	}, "获取成功", c)
}

// Tree
// @Tags        LiveCategoryAdmin
// @Summary     获取后台直播分类树
// @Description 返回全部未删除分类，包括停用分类，供父分类选择器和分类管理页面使用。
// @Security    ApiKeyAuth
// @Produce     application/json
// @Success     200  {object}  response.Response{data=[]liveRes.LiveCategoryAdminTreeItem}  "获取成功"
// @Failure     401  {object}  response.Response                                              "未登录或 token 无效"
// @Failure     200  {object}  response.Response                                              "无权限或查询失败"
// @Router      /live/category/tree [get]
func (a *CategoryAdminApi) Tree(c *gin.Context) {
	result, err := categoryService.GetAdminCategoryTree()
	if err != nil {
		respondLiveCategoryError(c, "获取直播分类树失败", err)
		return
	}
	response.OkWithDetailed(result, "获取成功", c)
}

// Create
// @Tags        LiveCategoryAdmin
// @Summary     创建直播分类
// @Description code 会转为小写且创建后不可修改；同一父分类下名称不能重复。
// @Description 启用分类只能放在全部处于启用状态的父级链路下。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.LiveCategoryCreateReq                         true  "分类资料"
// @Success     200   {object}  response.Response{data=liveRes.LiveCategoryAdminItem}  "创建成功"
// @Failure     401   {object}  response.Response                                     "未登录或 token 无效"
// @Failure     200   {object}  response.Response                                     "参数错误、编码/名称重复或父分类非法"
// @Router      /live/category/create [post]
func (a *CategoryAdminApi) Create(c *gin.Context) {
	var req liveReq.LiveCategoryCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("创建参数错误: "+err.Error(), c)
		return
	}
	result, err := categoryService.CreateCategory(req)
	if err != nil {
		respondLiveCategoryError(c, "创建直播分类失败", err)
		return
	}
	response.OkWithDetailed(result, "创建成功", c)
}

// Update
// @Tags        LiveCategoryAdmin
// @Summary     修改直播分类资料
// @Description 允许修改父分类、名称、图标和排序；稳定编码 code 不允许修改。
// @Description 修改父级时会检查父分类存在性、启用状态和循环引用。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.LiveCategoryUpdateReq                         true  "分类 ID 和资料"
// @Success     200   {object}  response.Response{data=liveRes.LiveCategoryAdminItem}  "修改成功"
// @Failure     401   {object}  response.Response                                     "未登录或 token 无效"
// @Failure     200   {object}  response.Response                                     "参数错误、同级名称重复或分类层级非法"
// @Router      /live/category/update [post]
func (a *CategoryAdminApi) Update(c *gin.Context) {
	var req liveReq.LiveCategoryUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("修改参数错误: "+err.Error(), c)
		return
	}
	result, err := categoryService.UpdateCategory(req)
	if err != nil {
		respondLiveCategoryError(c, "修改直播分类失败", err)
		return
	}
	response.OkWithDetailed(result, "修改成功", c)
}

// UpdateStatus
// @Tags        LiveCategoryAdmin
// @Summary     启用或停用直播分类
// @Description 停用父分类会在同一事务中级联停用全部子孙分类；启用子分类要求所有上级分类已启用。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.LiveCategoryStatusUpdateReq  true  "分类 ID 和状态"
// @Success     200   {object}  response.Response                     "修改成功"
// @Failure     401   {object}  response.Response                     "未登录或 token 无效"
// @Failure     200   {object}  response.Response                     "参数错误、分类不存在或上级未启用"
// @Router      /live/category/status/update [post]
func (a *CategoryAdminApi) UpdateStatus(c *gin.Context) {
	var req liveReq.LiveCategoryStatusUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("状态参数错误: "+err.Error(), c)
		return
	}
	if err := categoryService.UpdateCategoryStatus(req); err != nil {
		respondLiveCategoryError(c, "修改直播分类状态失败", err)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// Delete
// @Tags        LiveCategoryAdmin
// @Summary     删除直播分类
// @Description 仅允许软删除已停用、没有子分类且未被 live_anchor.category_id 引用的分类。
// @Security    ApiKeyAuth
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.LiveCategoryDeleteReq  true  "分类 ID"
// @Success     200   {object}  response.Response               "删除成功"
// @Failure     401   {object}  response.Response               "未登录或 token 无效"
// @Failure     200   {object}  response.Response               "参数错误、分类启用中、有子分类或正在使用"
// @Router      /live/category/delete [post]
func (a *CategoryAdminApi) Delete(c *gin.Context) {
	var req liveReq.LiveCategoryDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("删除参数错误: "+err.Error(), c)
		return
	}
	if err := categoryService.DeleteCategory(req.ID); err != nil {
		respondLiveCategoryError(c, "删除直播分类失败", err)
		return
	}
	response.OkWithMessage("删除成功", c)
}

func respondLiveCategoryError(c *gin.Context, operation string, err error) {
	if isLiveCategoryBusinessError(err) {
		response.FailWithMessage(err.Error(), c)
		return
	}
	global.GVA_LOG.Error(operation, zap.Error(err))
	response.FailWithMessage(operation, c)
}

func isLiveCategoryBusinessError(err error) bool {
	known := []error{
		liveService.ErrLiveCategoryNotFound, liveService.ErrLiveCategoryCodeInvalid,
		liveService.ErrLiveCategoryCodeExists, liveService.ErrLiveCategoryNameEmpty,
		liveService.ErrLiveCategoryNameExists, liveService.ErrLiveCategoryStatusInvalid,
		liveService.ErrLiveCategoryParentInvalid,
		liveService.ErrLiveCategoryParentDisabled, liveService.ErrLiveCategoryCycle,
		liveService.ErrLiveCategoryHasChildren, liveService.ErrLiveCategoryInUse,
		liveService.ErrLiveCategoryDeleteEnabled,
	}
	for _, target := range known {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
