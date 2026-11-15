package eventdelivery

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"tb_live_module/model/common/response"
	systemReq "tb_live_module/model/system/request"
	svc "tb_live_module/service/eventdelivery"
	"unicode/utf8"
)

func actor(c *gin.Context) uint64 {
	v, _ := c.Get("claims")
	claims, ok := v.(*systemReq.CustomClaims)
	if !ok || claims == nil {
		return 0
	}
	return uint64(claims.BaseClaims.ID)
}
func respond(c *gin.Context, out any, err error) {
	c.Header("Cache-Control", "no-store")
	if err != nil {
		response.FailWithMessage(svc.PublicError(err), c)
		return
	}
	response.OkWithDetailed(out, "成功", c)
}
func queryOK(c *gin.Context, allowed ...string) bool {
	keys := map[string]bool{}
	for _, k := range allowed {
		keys[k] = true
	}
	if len(c.Request.URL.RawQuery) > 2048 {
		return false
	}
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return false
	}
	for k, v := range values {
		if !keys[k] || len(v) != 1 {
			return false
		}
	}
	return true
}

// List
// @Tags EventDelivery
// @Summary root分页查询可靠事件投递状态（未配置通道不会标记送达）
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码，最多1000"
// @Param pageSize query int false "每页1..100"
// @Param status query string false "pending|leased|blocked|delivered|dead"
// @Param eventId query string false "稳定事件ID"
// @Success 200 {object} response.Response{data=svc.Page}
// @Router /v1/admin/event-delivery/list [get]
func (API) List(c *gin.Context) {
	var q svc.Search
	if !queryOK(c, "page", "pageSize", "status", "eventId") || c.ShouldBindQuery(&q) != nil {
		respond(c, nil, svc.ErrInvalid)
		return
	}
	out, err := deliveryService.List(c.Request.Context(), actor(c), q)
	respond(c, out, err)
}

// Detail
// @Tags EventDelivery
// @Summary root查询事件白名单负载、投递尝试和重试审计
// @Security ApiKeyAuth
// @Produce json
// @Param id query string true "投递记录ID（十进制字符串）"
// @Success 200 {object} response.Response{data=svc.Detail}
// @Router /v1/admin/event-delivery/detail [get]
func (API) Detail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("id"), 10, 64)
	if err != nil || !queryOK(c, "id") {
		respond(c, nil, svc.ErrInvalid)
		return
	}
	out, err := deliveryService.Detail(c.Request.Context(), actor(c), id)
	respond(c, out, err)
}

// Retry
// @Tags EventDelivery
// @Summary root受控重试dead/blocked事件（固定requestId、版本、原因；保持事件幂等键）
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body svc.RetryRequest true "重试命令"
// @Success 200 {object} response.Response{data=object}
// @Router /v1/admin/event-delivery/retry [post]
func (API) Retry(c *gin.Context) {
	var r svc.RetryRequest
	if !retryBody(c, &r) {
		respond(c, nil, svc.ErrInvalid)
		return
	}
	out, err := deliveryService.Retry(c.Request.Context(), actor(c), r)
	respond(c, out, err)
}
func retryBody(c *gin.Context, r *svc.RetryRequest) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" || c.Request.URL.RawQuery != "" {
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	allowed := map[string]bool{"requestId": true, "deliveryId": true, "generation": true, "reason": true}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || seen[key] {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	if _, err = d.Token(); err != nil {
		return false
	}
	if _, err = d.Token(); err != io.EOF {
		return false
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(r) == nil
}
