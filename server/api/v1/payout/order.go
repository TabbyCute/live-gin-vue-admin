package payout

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/url"
	"reflect"
	"strings"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	systemReq "tb_live_module/model/system/request"
	svc "tb_live_module/service/payout"
)

// Named API contracts keep Swagger aliases unambiguous across finance modules.
type orderEnvelope = response.Response
type orderResult = res.Order
type orderListResult = res.OrderPage
type orderDetailResult = res.OrderDetail

func orderActor(c *gin.Context) svc.Actor {
	trace := "pt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	c.Set("payoutTrace", trace)
	c.Header("X-Request-ID", trace)
	exposed := c.Writer.Header().Get("Access-Control-Expose-Headers")
	if exposed != "" {
		exposed += ", "
	}
	c.Header("Access-Control-Expose-Headers", exposed+"X-Request-ID")
	v, _ := c.Get("claims")
	claims, ok := v.(*systemReq.CustomClaims)
	if !ok || claims == nil {
		return svc.Actor{}
	}
	return svc.Actor{ID: uint64(claims.BaseClaims.ID), AuthorityID: claims.AuthorityId}
}
func orderQuery(c *gin.Context, out any) bool {
	allowed := map[string]bool{}
	var collect func(reflect.Type)
	collect = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				collect(f.Type)
			} else if tag := f.Tag.Get("form"); tag != "" {
				allowed[tag] = true
			}
		}
	}
	collect(reflect.TypeOf(out).Elem())
	values, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil {
		return invalid(c)
	}
	for k, v := range values {
		if !allowed[k] || len(v) != 1 {
			return invalid(c)
		}
	}
	if c.ShouldBindQuery(out) != nil {
		return invalid(c)
	}
	return true
}

// CreateOrder creates a ready instruction only, without moving funds.
// @Tags PayoutAdmin
// @Summary 提交打款中心（只创建待出款订单，不实际打款）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.CreateOrder true "固定requestId与withdrawalNo"
// @Success 200 {object} orderEnvelope{data=orderResult}
// @Router /v1/admin/payout/order/create [post]
func (API) CreateOrder(c *gin.Context) {
	a := orderActor(c)
	var r req.CreateOrder
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.CreateOrder(c.Request.Context(), a, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// OrderList returns only safe, masked order fields.
// @Tags PayoutAdmin
// @Summary 出款订单分页筛选
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码，默认1，最多10000"
// @Param pageSize query int false "每页1—100，默认20"
// @Param payoutNo query string false "出款单号"
// @Param withdrawalNo query string false "提现单号"
// @Param anchorNo query string false "主播编号"
// @Param currency query string false "币种"
// @Param payeeMethod query string false "bank_card/alipay/wechat"
// @Param status query string false "ready/processing/pending_confirmation/succeeded/failed/unknown/cancelled"
// @Param channelAccountNo query string false "公司出款账户编号，匹配历史尝试"
// @Param operatorId query string false "操作人ID字符串"
// @Param reviewerId query string false "复核人ID字符串"
// @Param paymentReference query string false "银行流水号，须同时指定公司账户编号"
// @Param from query int false "创建时间起（毫秒，包含）"
// @Param to query int false "创建时间止（毫秒，不包含）"
// @Success 200 {object} orderEnvelope{data=orderListResult}
// @Router /v1/admin/payout/order/list [get]
func (API) OrderList(c *gin.Context) {
	a := orderActor(c)
	var q req.OrderSearch
	if !orderQuery(c, &q) {
		return
	}
	out, e := payoutService.ListOrders(c.Request.Context(), a, q)
	respond(c, out, e)
}

// OrderDetail returns immutable masked evidence and a fresh consistency result.
// @Tags PayoutAdmin
// @Summary 只读出款详情、状态历史、审计及账务核验
// @Security ApiKeyAuth
// @Produce json
// @Param payoutNo query string true "出款单号"
// @Success 200 {object} orderEnvelope{data=orderDetailResult}
// @Router /v1/admin/payout/order/detail [get]
func (API) OrderDetail(c *gin.Context) {
	a := orderActor(c)
	var q req.OrderDetail
	if !orderQuery(c, &q) {
		return
	}
	out, e := payoutService.OrderDetail(c.Request.Context(), a, q.PayoutNo)
	respond(c, out, e)
}
