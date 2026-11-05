package httpkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"tb_live_module/internal/modulekit"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	systemReq "tb_live_module/model/system/request"
	"time"
	"unicode/utf8"
)

func Invalid(c *gin.Context) bool { Respond(c, nil, modulekit.Invalid); return false }
func Respond(c *gin.Context, data any, err error) {
	if err == nil {
		response.OkWithData(data, c)
		return
	}
	var business *modulekit.Error
	if !errors.As(err, &business) {
		business = modulekit.Unavailable
	}
	response.Result(business.Code, map[string]any{}, business.Message, c)
}
func Actor(c *gin.Context) modulekit.Actor {
	v, ok := c.Get("claims")
	if !ok {
		return modulekit.Actor{}
	}
	claims, ok := v.(*systemReq.CustomClaims)
	if !ok || claims == nil {
		return modulekit.Actor{}
	}
	ip, _, e := net.SplitHostPort(c.Request.RemoteAddr)
	if e != nil {
		ip = ""
	}
	return modulekit.Actor{ID: uint64(claims.BaseClaims.ID), AuthorityID: claims.AuthorityId, IP: ip}
}
func User(c *gin.Context) (uint64, bool) {
	id, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("请登录客户端账号", c)
	}
	return uint64(id), ok
}
func Body(c *gin.Context, out any) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		return Invalid(c)
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 || !utf8.Valid(body) {
		return Invalid(c)
	}
	// 拒绝重复JSON键和多值请求体，避免代理、审计与业务解释不一致。
	d := json.NewDecoder(bytes.NewReader(body))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return Invalid(c)
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return Invalid(c)
		}
		key, ok := token.(string)
		if !ok || seen[strings.ToLower(key)] {
			return Invalid(c)
		}
		seen[strings.ToLower(key)] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return Invalid(c)
		}
	}
	if _, err = d.Token(); err != nil {
		return Invalid(c)
	}
	if _, err = d.Token(); err != io.EOF {
		return Invalid(c)
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return Invalid(c)
	}
	return true
}
func Query(c *gin.Context, out any) bool {
	if len(c.Request.URL.RawQuery) > 2048 {
		return Invalid(c)
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
		return Invalid(c)
	}
	for key, values := range valuesMap {
		if !allowed[key] || len(values) != 1 {
			return Invalid(c)
		}
	}
	if c.ShouldBindQuery(out) != nil {
		return Invalid(c)
	}
	return true
}

func Budget(limit int, budget time.Duration) gin.HandlerFunc {
	// 容量在路由初始化时固定。无等待队列、无按IP增长的map；APP与后台独立。
	slots := make(chan struct{}, limit)
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(c.Request.Context(), budget)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		// 包括拒绝请求后的HTTP正文清理，也不能继承上传接口的长超时。
		controller := http.NewResponseController(c.Writer)
		deadline, _ := ctx.Deadline()
		_ = controller.SetReadDeadline(deadline)
		if c.Request.ContentLength > 64<<10 || len(c.Request.URL.RawQuery) > 2048 || len(c.GetHeader("Authorization")) > 8192 || len(c.GetHeader("x-token")) > 8192 {
			if c.Request.ProtoMajor == 1 {
				c.Header("Connection", "close")
			}
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"code": modulekit.Invalid.Code, "msg": modulekit.Invalid.Message, "data": gin.H{}})
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			if c.Request.ProtoMajor == 1 {
				c.Header("Connection", "close")
			}
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": modulekit.Unavailable.Code, "msg": modulekit.Unavailable.Message, "data": gin.H{}})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		// 不在Handler退出时清除：net/http随后还可能丢弃未读正文。
		// 清除会使超时请求再次无限阻塞；下次请求由HTTP server重置期限。
		c.Next()
	}
}
