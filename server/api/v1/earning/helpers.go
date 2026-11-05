package earning

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"mime"
	"net"
	"net/url"
	"reflect"
	"strings"
	"tb_live_module/global"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	systemReq "tb_live_module/model/system/request"
	svc "tb_live_module/service/earning"
	payout "tb_live_module/service/payout"
	"unicode/utf8"
)

func invalid(c *gin.Context) bool { respond(c, nil, svc.ErrInvalid); return false }
func respond(c *gin.Context, data any, err error) {
	requestTrace := c.GetString("earningTrace")
	if requestTrace == "" {
		requestTrace = trace()
		c.Set("earningTrace", requestTrace)
	}
	c.Header("X-Request-ID", requestTrace)
	exposed := c.Writer.Header().Get("Access-Control-Expose-Headers")
	if exposed != "" {
		exposed += ", "
	}
	c.Header("Access-Control-Expose-Headers", exposed+"X-Request-ID")
	if err == nil {
		response.OkWithData(data, c)
		return
	}
	var b *svc.Error
	switch {
	case errors.As(err, &b):
		if errors.Is(err, payout.ErrCrypto) && global.GVA_LOG != nil {
			global.GVA_LOG.Error("earning payee integrity failed", zap.String("path", c.FullPath()), zap.String("traceId", requestTrace))
		}
		if errors.Is(err, svc.ErrInvariant) && global.GVA_LOG != nil {
			global.GVA_LOG.Error("earning invariant violated", zap.String("path", c.FullPath()), zap.String("traceId", requestTrace))
		}
		response.Result(b.Code, map[string]any{}, b.Message, c)
	default:
		if global.GVA_LOG != nil {
			global.GVA_LOG.Error("earning request failed", zap.String("path", c.FullPath()), zap.String("traceId", requestTrace))
		}
		response.Result(svc.ErrUnavailable.Code, map[string]any{}, svc.ErrUnavailable.Message, c)
	}
}
func ip(c *gin.Context) string {
	v, _, e := net.SplitHostPort(c.Request.RemoteAddr)
	if e != nil {
		return c.Request.RemoteAddr
	}
	return v
}
func actor(c *gin.Context) svc.Actor {
	v, ok := c.Get("claims")
	if !ok {
		return svc.Actor{}
	}
	claims, ok := v.(*systemReq.CustomClaims)
	if !ok || claims == nil {
		return svc.Actor{}
	}
	return svc.Actor{ID: uint64(claims.BaseClaims.ID), AuthorityID: claims.AuthorityId, IP: ip(c)}
}
func appID(c *gin.Context) (uint64, bool) {
	id, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("请登录客户端账号", c)
	}
	return uint64(id), ok
}
func trace() string { return "gt_" + strings.ReplaceAll(uuid.NewString(), "-", "") }
func bindBody(c *gin.Context, out any) bool {
	if c.Request.URL.RawQuery != "" {
		return invalid(c)
	}
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		return invalid(c)
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 || !utf8.Valid(body) {
		return invalid(c)
	}
	// 拒绝重复JSON键和多值请求体，避免代理、审计与业务解释不一致。
	d := json.NewDecoder(bytes.NewReader(body))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return invalid(c)
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	t := reflect.TypeOf(out).Elem()
	for i := 0; i < t.NumField(); i++ {
		allowed[strings.Split(t.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return invalid(c)
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || seen[strings.ToLower(key)] {
			return invalid(c)
		}
		seen[strings.ToLower(key)] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return invalid(c)
		}
	}
	if reflect.TypeOf(out).Elem().Name() == "FinanceReview" && !seen["approve"] {
		return invalid(c)
	}
	if _, err = d.Token(); err != nil {
		return invalid(c)
	}
	if _, err = d.Token(); err != io.EOF {
		return invalid(c)
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return invalid(c)
	}
	return true
}
func bindQuery(c *gin.Context, out any) bool {
	if len(c.Request.URL.RawQuery) > 2048 {
		return invalid(c)
	}
	allowed := map[string]bool{}
	var collect func(reflect.Type)
	collect = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				collect(f.Type)
			} else if key := f.Tag.Get("form"); key != "" {
				allowed[strings.Split(key, ",")[0]] = true
			}
		}
	}
	collect(reflect.TypeOf(out).Elem())
	valuesMap, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return invalid(c)
	}
	for key, values := range valuesMap {
		if !allowed[key] || len(values) != 1 {
			return invalid(c)
		}
	}
	if c.ShouldBindQuery(out) != nil {
		return invalid(c)
	}
	return true
}
