package bankreconcile

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/url"
	"reflect"
	"tb_live_module/middleware"
	req "tb_live_module/model/bankreconcile/request"
	"tb_live_module/model/common/response"
	systemReq "tb_live_module/model/system/request"
	svc "tb_live_module/service/bankreconcile"
	payout "tb_live_module/service/payout"
	"unicode/utf8"
)

func actor(c *gin.Context) payout.Actor {
	v, _ := c.Get("claims")
	claims, ok := v.(*systemReq.CustomClaims)
	if !ok || claims == nil {
		return payout.Actor{}
	}
	return payout.Actor{ID: uint64(claims.BaseClaims.ID), AuthorityID: claims.AuthorityId}
}
func respond(c *gin.Context, v any, e error) {
	if e == nil {
		response.OkWithData(v, c)
		return
	}
	var b *payout.Error
	if !errors.As(e, &b) {
		b = payout.ErrOrderUnavailable
	}
	response.Result(b.Code, gin.H{}, b.Message, c)
}
func bind(c *gin.Context, out any) bool {
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" || c.Request.URL.RawQuery != "" {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	b, e := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if e != nil || len(b) > 1<<20 || !utf8.Valid(b) {
		respond(c, nil, svc.ErrLimit)
		return false
	}
	defer clear(b)
	allowed := map[string]bool{}
	t := reflect.TypeOf(out).Elem()
	for i := 0; i < t.NumField(); i++ {
		allowed[t.Field(i).Tag.Get("json")] = true
	}
	d := json.NewDecoder(bytes.NewReader(b))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		token, e = d.Token()
		key, ok := token.(string)
		if e != nil || !ok || !allowed[key] || seen[key] {
			respond(c, nil, svc.ErrInvalid)
			return false
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			respond(c, nil, svc.ErrInvalid)
			return false
		}
	}
	if _, e = d.Token(); e != nil {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	if _, e = d.Token(); e != io.EOF {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	if json.Unmarshal(b, out) != nil {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	return true
}

func query(c *gin.Context, allowed ...string) bool {
	if len(c.Request.URL.RawQuery) > 512 {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	values, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	keys := map[string]bool{}
	for _, key := range allowed {
		keys[key] = true
	}
	for key, v := range values {
		if !keys[key] || len(v) != 1 {
			respond(c, nil, svc.ErrInvalid)
			return false
		}
	}
	return true
}

// Import imports normalized bank assertions without changing payout or balances.
// @Tags BankReconciliation
// @Summary 导入银行账单声明（不自动付款或确认到账）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Import true "CSV≤512KiB，≤1000行；固定requestId"
// @Success 200 {object} response.Response{data=svc.Result}
// @Router /v1/admin/payout/bank-reconciliation/import [post]
func (API) Import(c *gin.Context) {
	var r req.Import
	if !bind(c, &r) {
		return
	}
	v, e := bankService.Import(c.Request.Context(), actor(c), middleware.RequestID(c), r)
	r.CSV = ""
	respond(c, v, e)
}

// Run records a two-way reconciliation snapshot; it never settles a payout.
// @Tags BankReconciliation
// @Summary 对指定账期运行双向核对
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Run true "批次和固定requestId"
// @Success 200 {object} response.Response{data=svc.Result}
// @Router /v1/admin/payout/bank-reconciliation/run [post]
func (API) Run(c *gin.Context) {
	var r req.Run
	if !bind(c, &r) {
		return
	}
	v, e := bankService.Run(c.Request.Context(), actor(c), middleware.RequestID(c), r)
	respond(c, v, e)
}

// Review only acknowledges a match or records investigation, never approves payment.
// @Tags BankReconciliation
// @Summary 独立复核核对记录或记录待调查差异
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body req.Review true "acknowledge仅用于matched；investigate用于调查"
// @Success 200 {object} response.Response{data=svc.Result}
// @Router /v1/admin/payout/bank-reconciliation/review [post]
func (API) Review(c *gin.Context) {
	var r req.Review
	if !bind(c, &r) {
		return
	}
	v, e := bankService.Review(c.Request.Context(), actor(c), middleware.RequestID(c), r)
	respond(c, v, e)
}

// List lists masked statement batches.
// @Tags BankReconciliation
// @Summary 分页查看脱敏导入批次
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页1—100"
// @Param channelAccountNo query string false "公司账户编号"
// @Success 200 {object} response.Response{data=svc.Page}
// @Router /v1/admin/payout/bank-reconciliation/list [get]
func (API) List(c *gin.Context) {
	if !query(c, "page", "pageSize", "channelAccountNo") {
		return
	}
	var r req.Search
	if c.ShouldBindQuery(&r) != nil {
		respond(c, nil, svc.ErrInvalid)
		return
	}
	v, e := bankService.List(c.Request.Context(), actor(c), r)
	respond(c, v, e)
}

// Detail returns safe lines and the most recent immutable reconciliation run.
// @Tags BankReconciliation
// @Summary 查看批次脱敏明细与最新核对结果
// @Security ApiKeyAuth
// @Produce json
// @Param batchNo query string true "导入批次编号"
// @Success 200 {object} response.Response{data=svc.Detail}
// @Router /v1/admin/payout/bank-reconciliation/detail [get]
func (API) Detail(c *gin.Context) {
	if !query(c, "batchNo") {
		return
	}
	v, e := bankService.Detail(c.Request.Context(), actor(c), c.Query("batchNo"))
	respond(c, v, e)
}
