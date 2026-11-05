package gift

import (
	"github.com/gin-gonic/gin"
	"tb_live_module/model/common/response"
	earningModel "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	giftModel "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	giftResponse "tb_live_module/model/gift/response"
)

type API struct{}

var _ = response.Response{}
var _ = giftModel.Order{}
var _ = giftResponse.Page{}
var _ = earningModel.RevenueRule{}

// Categories
// @Tags Gift
// @Summary APP礼物分类列表
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/app/gift/category/list [get]
func (API) Categories(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := giftService.Catalog(c.Request.Context(), user, nil, "category", r)
	respond(c, out, e)
}

// Items
// @Tags Gift
// @Summary APP启用礼物列表，可按直播间筛选
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/app/gift/item/list [get]
func (API) Items(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := giftService.Catalog(c.Request.Context(), user, nil, "item", r)
	respond(c, out, e)
}

// Send
// @Tags Gift
// @Summary 送礼：服务端定价、永久幂等
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data body req.Send true "参数"
// @Success 200 {object} response.Response{data=giftResponse.AppOrder} "公开订单号可供客服核验；不返回内部财务字段"
// @Router /v1/app/gift/send [post]
func (API) Send(c *gin.Context) {
	var r req.Send
	if !bindBody(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := giftService.Send(c.Request.Context(), user, ip(c), trace(), r)
	respond(c, giftResponse.PublicOrder(out), e)
}

// Result
// @Tags Gift
// @Summary 本人送礼结果（订单号或幂等键）
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.AppOrder} "仅返回本人公开订单"
// @Router /v1/app/gift/result [get]
func (API) Result(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := giftService.Result(c.Request.Context(), user, r)
	respond(c, giftResponse.PublicOrder(out), e)
}

// History
// @Tags Gift
// @Summary 本人礼物消费记录
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/app/gift/order/list [get]
func (API) History(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	user, ok := appID(c)
	if !ok {
		return
	}
	out, e := giftService.Orders(c.Request.Context(), user, nil, r)
	respond(c, out, e)
}

// AdminCategories
// @Tags Gift
// @Summary 后台礼物分类
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/category/list [get]
func (API) AdminCategories(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.Catalog(c.Request.Context(), 0, &a, "category", r)
	respond(c, out, e)
}

// AdminItems
// @Tags Gift
// @Summary 后台礼物配置
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/item/list [get]
func (API) AdminItems(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.Catalog(c.Request.Context(), 0, &a, "item", r)
	respond(c, out, e)
}

// SaveCategory
// @Tags Gift
// @Summary 新增或修改分类，更新须传版本
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Category true "参数"
// @Success 200 {object} response.Response{data=giftModel.Category} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/category/save [post]
func (API) SaveCategory(c *gin.Context) {
	var r req.Category
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.SaveCategory(c.Request.Context(), a, r)
	respond(c, out, e)
}

// SaveItem
// @Tags Gift
// @Summary 新增或修改礼物，守护履约关闭
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Item true "参数"
// @Success 200 {object} response.Response{data=giftModel.Item} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/item/save [post]
func (API) SaveItem(c *gin.Context) {
	var r req.Item
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.SaveItem(c.Request.Context(), a, r)
	respond(c, out, e)
}

// Sort
// @Tags Gift
// @Summary 完整集合事务排序，须传目录版本
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Sort true "参数"
// @Success 200 {object} response.Response "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/sort [post]
func (API) Sort(c *gin.Context) {
	var r req.Sort
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	e := giftService.Sort(c.Request.Context(), a, r)
	respond(c, map[string]any{}, e)
}

// Delete
// @Tags Gift
// @Summary 删除未被使用的分类或礼物
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Delete true "参数"
// @Success 200 {object} response.Response "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/delete [post]
func (API) Delete(c *gin.Context) {
	var r req.Delete
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	e := giftService.Delete(c.Request.Context(), a, r)
	respond(c, map[string]any{}, e)
}

// Orders
// @Tags Gift
// @Summary 后台礼物消费记录
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/order/list [get]
func (API) Orders(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.Orders(c.Request.Context(), 0, &a, r)
	respond(c, out, e)
}

// Detail
// @Tags Gift
// @Summary 订单关联事实和一致性检查
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Detail} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/order/detail [get]
func (API) Detail(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.Detail(c.Request.Context(), a, r)
	respond(c, out, e)
}

// Refund
// @Tags Gift
// @Summary 授权后台部分或全部退款
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Refund true "参数"
// @Success 200 {object} response.Response{data=giftModel.Refund} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/refund [post]
func (API) Refund(c *gin.Context) {
	var r req.Refund
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.Refund(c.Request.Context(), a, r)
	respond(c, out, e)
}

// Ranking
// @Tags Gift
// @Summary 净消费排行和收入统计（UTC）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/ranking [get]
func (API) Ranking(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.Ranking(c.Request.Context(), a, r)
	respond(c, out, e)
}

// Rules
// @Tags Gift
// @Summary 分成规则版本列表
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data query req.Query true "参数"
// @Success 200 {object} response.Response{data=giftResponse.Page} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/rule/list [get]
func (API) Rules(c *gin.Context) {
	var r req.Query
	if !bindQuery(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.Rules(c.Request.Context(), a, r)
	respond(c, out, e)
}

// CreateRule
// @Tags Gift
// @Summary 提交新的分成规则版本
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body er.Rule true "参数"
// @Success 200 {object} response.Response{data=earningModel.RevenueRule} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/rule/create [post]
func (API) CreateRule(c *gin.Context) {
	var r er.Rule
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.CreateRule(c.Request.Context(), a, r)
	respond(c, out, e)
}

// ReviewRule
// @Tags Gift
// @Summary 复核分成规则（超级 root ID 1 可自审）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body er.Review true "参数"
// @Success 200 {object} response.Response{data=earningModel.RevenueRule} "统一响应，金额和大整数为十进制字符串"
// @Router /v1/admin/gift/rule/review [post]
func (API) ReviewRule(c *gin.Context) {
	var r er.Review
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.ReviewRule(c.Request.Context(), a, r)
	respond(c, out, e)
}

// TerminateRule
// @Tags GiftAdmin
// @Summary 申请提前终止分成规则，原规则和历史订单保持可追溯
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body er.TerminateRule true "规则编号、幂等键、终止原因"
// @Success 200 {object} response.Response{data=earningModel.RuleTermination}
// @Router /v1/admin/gift/rule/terminate [post]
func (API) TerminateRule(c *gin.Context) {
	var r er.TerminateRule
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.TerminateRule(c.Request.Context(), a, r)
	respond(c, out, e)
}

// ReviewTermination
// @Tags GiftAdmin
// @Summary 复核提前终止；超级 root ID 1 可自审，通过后立即停止匹配新订单
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body er.ReviewTermination true "终止申请编号、是否通过"
// @Success 200 {object} response.Response{data=earningModel.RuleTermination}
// @Router /v1/admin/gift/rule/terminate/review [post]
func (API) ReviewTermination(c *gin.Context) {
	var r er.ReviewTermination
	if !bindBody(c, &r) {
		return
	}
	a := actor(c)
	out, e := giftService.ReviewTermination(c.Request.Context(), a, r)
	respond(c, out, e)
}
