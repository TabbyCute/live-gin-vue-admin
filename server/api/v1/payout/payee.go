package payout

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"mime"
	"net/url"
	"reflect"
	"strings"
	"tb_live_module/global"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	svc "tb_live_module/service/payout"
	"unicode/utf8"
)

func respond(c *gin.Context, data any, err error) {
	if err == nil {
		if o, ok := data.(res.ActionResult); ok && global.GVA_LOG != nil {
			global.GVA_LOG.Info("payout manual operation completed", zap.String("path", c.FullPath()), zap.String("traceId", c.GetString("payoutTrace")), zap.String("payoutNo", o.PayoutNo), zap.String("attemptNo", o.AttemptNo), zap.String("status", o.Status), zap.String("reconciliationNo", o.ReconciliationNo))
		}
		if o, ok := data.(res.Order); ok && c.Request.Method == "POST" && global.GVA_LOG != nil {
			global.GVA_LOG.Info("payout order ready", zap.String("traceId", c.GetString("payoutTrace")), zap.String("payoutNo", o.PayoutNo), zap.String("withdrawalNo", o.BusinessNo), zap.String("payeeAccountNo", o.PayeeAccountNo), zap.String("status", o.Status))
		}
		if p, ok := data.(res.Payee); ok && c.Request.Method == "POST" && global.GVA_LOG != nil {
			global.GVA_LOG.Info("payout account operation completed", zap.String("path", c.FullPath()), zap.String("traceId", c.GetString("payoutTrace")), zap.String("payeeAccountNo", p.PayeeAccountNo), zap.String("status", p.Status))
		}
		response.OkWithData(data, c)
		return
	}
	var business *svc.Error
	if !errors.As(err, &business) {
		business = svc.ErrUnavailable
		if strings.Contains(c.FullPath(), "/payout/") && !strings.Contains(c.FullPath(), "/payout/payee/") {
			business = svc.ErrOrderUnavailable
		}
	}
	if global.GVA_LOG != nil {
		class := "business"
		if business == svc.ErrUnavailable || business == svc.ErrOrderUnavailable {
			class = "storage"
		}
		if errors.Is(err, svc.ErrCrypto) {
			class = "crypto_integrity"
		}
		// Do not attach errors, request bodies, query strings, or storage models.
		global.GVA_LOG.Warn("payout request failed", zap.String("path", c.FullPath()), zap.String("traceId", c.GetString("payoutTrace")), zap.String("class", class), zap.Int("code", business.Code))
	}
	response.Result(business.Code, gin.H{}, business.Message, c)
}
func identity(c *gin.Context) (uint64, bool) {
	id, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("请登录客户端账号", c)
	}
	trace := "pt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	c.Set("payoutTrace", trace)
	c.Header("X-Request-ID", trace)
	exposed := c.Writer.Header().Get("Access-Control-Expose-Headers")
	if exposed != "" {
		exposed += ", "
	}
	c.Header("Access-Control-Expose-Headers", exposed+"X-Request-ID")
	return uint64(id), ok
}
func invalid(c *gin.Context) bool {
	if strings.Contains(c.FullPath(), "/payout/") && !strings.Contains(c.FullPath(), "/payout/payee/") {
		respond(c, nil, svc.ErrOrderInvalid)
	} else {
		respond(c, nil, svc.ErrInvalid)
	}
	return false
}
func bindBody(c *gin.Context, out any) bool {
	return bindBodySized(c, out, 8192)
}
func bindBodySized(c *gin.Context, out any, limit int64) bool {
	typeName, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || typeName != "application/json" {
		return invalid(c)
	}
	if c.Request.URL.RawQuery != "" {
		return invalid(c)
	}
	b, e := io.ReadAll(io.LimitReader(c.Request.Body, limit+1))
	if e != nil || int64(len(b)) > limit || !utf8.Valid(b) {
		return invalid(c)
	}
	allowed := map[string]bool{}
	t := reflect.TypeOf(out).Elem()
	for i := 0; i < t.NumField(); i++ {
		allowed[t.Field(i).Tag.Get("json")] = true
	}
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return invalid(c)
	}
	seen := map[string]bool{}
	for d.More() {
		tok, e = d.Token()
		if e != nil {
			return invalid(c)
		}
		key, ok := tok.(string)
		if !ok || !allowed[key] || seen[key] {
			return invalid(c)
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return invalid(c)
		}
	}
	if _, e = d.Token(); e != nil {
		return invalid(c)
	}
	if _, e = d.Token(); e != io.EOF {
		return invalid(c)
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return invalid(c)
	}
	return true
}
func bindQuery(c *gin.Context, q *req.PayeeSearch) bool {
	if len(c.Request.URL.RawQuery) > 512 {
		return invalid(c)
	}
	values, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil {
		return invalid(c)
	}
	for k, v := range values {
		if len(v) != 1 || (k != "page" && k != "pageSize" && k != "currency" && k != "status") {
			return invalid(c)
		}
	}
	if c.ShouldBindQuery(q) != nil {
		return invalid(c)
	}
	return true
}

// List lists only the current APP anchor's masked bindings.
// @Tags PayoutApp
// @Summary 本人脱敏收款账户（绑定不代表渠道认证）
// @Security AppBearerAuth
// @Produce json
// @Param page query int false "页码，默认1"
// @Param pageSize query int false "每页1—100，默认20"
// @Param currency query string false "币种"
// @Param status query string false "active/disabled"
// @Success 200 {object} response.Response{data=res.Page}
// @Router /v1/app/payout/payee/list [get]
func (API) List(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	var q req.PayeeSearch
	if !bindQuery(c, &q) {
		return
	}
	out, e := payoutService.List(c.Request.Context(), id, q)
	respond(c, out, e)
}

// Create creates an encrypted, unverified binding. Sensitive fields are write-only.
// @Tags PayoutApp
// @Summary 新增本人收款账户；同requestId须保持原参数
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data body req.CreatePayee true "加密保存的绑定资料；禁止提交ownerNo、anchorNo、认证或版本"
// @Success 200 {object} response.Response{data=res.Payee}
// @Router /v1/app/payout/payee/create [post]
func (API) Create(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	var r req.CreatePayee
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.Create(c.Request.Context(), id, c.GetString("payoutTrace"), r)
	respond(c, out, e)
}

// SetDefault changes only the current anchor's default selection.
// @Tags PayoutApp
// @Summary 设置本人同币种唯一默认收款账户
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data body req.PayeeAction true "内部收款账户编号"
// @Success 200 {object} response.Response{data=res.Payee}
// @Router /v1/app/payout/payee/set-default [post]
func (API) SetDefault(c *gin.Context) { change(c, false) }

// Disable prevents new withdrawals without rewriting historical snapshots.
// @Tags PayoutApp
// @Summary 停用本人收款账户（不可恢复或删除历史快照）
// @Security AppBearerAuth
// @Accept json
// @Produce json
// @Param data body req.PayeeAction true "内部收款账户编号"
// @Success 200 {object} response.Response{data=res.Payee}
// @Router /v1/app/payout/payee/disable [post]
func (API) Disable(c *gin.Context) { change(c, true) }
func change(c *gin.Context, disable bool) {
	id, ok := identity(c)
	if !ok {
		return
	}
	var r req.PayeeAction
	if !bindBody(c, &r) {
		return
	}
	out, e := payoutService.Change(c.Request.Context(), id, c.GetString("payoutTrace"), r.PayeeAccountNo, disable)
	respond(c, out, e)
}
