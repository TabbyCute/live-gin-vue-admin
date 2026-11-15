package live

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"
	svc "tb_live_module/service/live"
)

var _ = liveRes.PresenceSession{}

type PresenceApi struct{}

func presenceReply(c *gin.Context, v any, e error) {
	if e == nil {
		response.OkWithData(v, c)
		return
	}
	var b *svc.PresenceError
	if c.Request.ProtoMajor == 1 && c.Request.ContentLength != 0 {
		c.Header("Connection", "close")
	}
	if !errors.As(e, &b) {
		b = svc.ErrPresenceUnavailable
	}
	status := http.StatusBadRequest
	switch b.Code {
	case 49502, 49503:
		status = http.StatusForbidden
	case 49505, 49507:
		status = http.StatusTooManyRequests
		c.Header("Retry-After", "5")
	case 49506, 49508:
		status = http.StatusServiceUnavailable
		c.Header("Retry-After", "5")
	}
	c.JSON(status, response.Response{Code: b.Code, Data: struct{}{}, Msg: b.Message})
}

// PresenceBudget is BEFORE authentication. Body/query/header/parallel work are bounded.
func PresenceBudget() gin.HandlerFunc {
	guard := middleware.NewAccountAuthGuard()
	slots := make(chan struct{}, 256)
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		controller := http.NewResponseController(c.Writer)
		_ = controller.SetReadDeadline(time.Now().Add(3 * time.Second))
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		if len(c.Request.Header.Values("Authorization")) > 1 || len(c.Request.Header.Values("X-Presence-Guest")) > 1 || c.Request.ContentLength > 4096 || len(c.Request.URL.RawQuery) > 1024 || len(c.GetHeader("Authorization")) > 8192 || len(c.GetHeader("X-Presence-Guest")) > 2048 {
			presenceReply(c, nil, svc.ErrPresenceInvalid)
			c.Abort()
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			presenceReply(c, nil, svc.ErrPresenceLimited)
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		ip := guard.PresenceClientIP(c)
		c.Set("presence-ip", svc.DefaultPresence.IPKey(ip))
		if e := svc.DefaultPresence.Rate(ctx, "ip", ip, 3000); e != nil {
			presenceReply(c, nil, e)
			c.Abort()
			return
		}
		c.Next()
	}
}

// Strict flat-object JSON rejects duplicate/unknown/case-alias keys, null fields and
// trailing data. No request bodies or bearer credentials enter operation logs.
func presenceBind(c *gin.Context, v any) bool {
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(3 * time.Second))
	raw, e := io.ReadAll(c.Request.Body)
	if e != nil || len(raw) > 4096 {
		presenceReply(c, nil, svc.ErrPresenceInvalid)
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, e := dec.Token()
	if e != nil || token != json.Delim('{') {
		presenceReply(c, nil, svc.ErrPresenceInvalid)
		return false
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		k, err := dec.Token()
		name, ok := k.(string)
		if err != nil || !ok {
			e = svc.ErrPresenceInvalid
			break
		}
		if _, exists := fields[name]; exists {
			e = svc.ErrPresenceInvalid
			break
		}
		var value json.RawMessage
		if err = dec.Decode(&value); err != nil || bytes.Equal(value, []byte("null")) {
			e = svc.ErrPresenceInvalid
			break
		}
		fields[name] = value
	}
	if e == nil {
		_, e = dec.Token()
	}
	if e == nil {
		var extra any
		if dec.Decode(&extra) != io.EOF {
			e = svc.ErrPresenceInvalid
		}
	}
	// Derive the exact permitted JSON names from the DTO, including bool false values.
	allowedRaw, _ := json.Marshal(v)
	allowed := map[string]json.RawMessage{}
	_ = json.Unmarshal(allowedRaw, &allowed)
	for name := range fields {
		if _, ok := allowed[name]; !ok {
			e = svc.ErrPresenceInvalid
		}
	}
	if len(fields) != len(allowed) {
		e = svc.ErrPresenceInvalid
	}
	if e == nil {
		e = json.Unmarshal(raw, v)
	}
	if e != nil {
		presenceReply(c, nil, svc.ErrPresenceInvalid)
		return false
	}
	return true
}
func presenceIdentity(c *gin.Context) (svc.PresenceIdentity, bool) {
	var i svc.PresenceIdentity
	if claims, ok := middleware.GetAppClaims(c); ok {
		i.Actor = uint64(claims.AccountID)
		i.TokenVersion = claims.TokenVersion
	} else {
		var e error
		i, e = svc.DefaultPresence.GuestIdentity(c.GetHeader("X-Presence-Guest"))
		if e != nil {
			presenceReply(c, nil, e)
			return i, false
		}
	}
	scope := "request"
	limit := 600
	if strings.HasSuffix(c.FullPath(), "/open") {
		scope = "open"
		limit = 20
	}
	subject := i.Visitor
	if i.Actor > 0 {
		subject = stringMustJSON(i.Actor)
	}
	if e := svc.DefaultPresence.Rate(c.Request.Context(), scope, subject, limit); e != nil {
		presenceReply(c, nil, e)
		return i, false
	}
	return i, true
}
func stringMustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func presenceQuery(c *gin.Context) (req.PresenceQuery, bool) {
	var q req.PresenceQuery
	values, e := url.ParseQuery(c.Request.URL.RawQuery)
	for k, vs := range values {
		if len(vs) != 1 || (k != "roomNo" && k != "cursor" && k != "limit") {
			e = svc.ErrPresenceInvalid
		}
	}
	if e == nil {
		e = c.ShouldBindQuery(&q)
	}
	if e != nil {
		presenceReply(c, nil, svc.ErrPresenceInvalid)
		return q, false
	}
	return q, true
}

// Open
// @Tags LivePresence
// @Summary 观众进房，会员或签名游客，requestId幂等
// @Description 数量为十进制字符串；错误49501参数，49502过期，49503权限，49504序号，49505限流，49506依赖，49507背压，49508状态异常。无WebSocket。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param X-Presence-Guest header string false "游客签名凭证；与APP JWT身份分离"
// @Param data body req.PresenceOpen true "请求"
// @Success 200 {object} response.Response{data=liveRes.PresenceSession}
// @Router /v1/app/live/presence/open [post]
func (a *PresenceApi) Open(c *gin.Context) {
	var r req.PresenceOpen
	if !presenceBind(c, &r) {
		return
	}
	i, ok := presenceIdentity(c)
	if !ok {
		return
	}
	v, e := presenceService.Open(c.Request.Context(), i, c.GetString("presence-ip"), r)
	presenceReply(c, v, e)
}

// Heartbeat
// @Tags LivePresence
// @Summary 观众心跳，服务端时间与递增sequence
// @Description 数量为十进制字符串；错误49501参数，49502过期，49503权限，49504序号，49505限流，49506依赖，49507背压，49508状态异常。无WebSocket。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param X-Presence-Guest header string false "游客签名凭证；与APP JWT身份分离"
// @Param data body req.PresenceHeartbeat true "请求"
// @Success 200 {object} response.Response{data=liveRes.PresenceSession}
// @Router /v1/app/live/presence/heartbeat [post]
func (a *PresenceApi) Heartbeat(c *gin.Context) {
	var r req.PresenceHeartbeat
	if !presenceBind(c, &r) {
		return
	}
	i, ok := presenceIdentity(c)
	if !ok {
		return
	}
	v, e := presenceService.Heartbeat(c.Request.Context(), i, r)
	presenceReply(c, v, e)
}

// Leave
// @Tags LivePresence
// @Summary 幂等退房，浏览器关闭时最佳努力
// @Description 数量为十进制字符串；错误49501参数，49502过期，49503权限，49504序号，49505限流，49506依赖，49507背压，49508状态异常。无WebSocket。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param X-Presence-Guest header string false "游客签名凭证；与APP JWT身份分离"
// @Param data body req.PresenceLeave true "请求"
// @Success 200 {object} response.Response{data=liveRes.PresenceSession}
// @Router /v1/app/live/presence/leave [post]
func (a *PresenceApi) Leave(c *gin.Context) {
	var r req.PresenceLeave
	if !presenceBind(c, &r) {
		return
	}
	i, ok := presenceIdentity(c)
	if !ok {
		return
	}
	v, e := presenceService.Leave(c.Request.Context(), i, r)
	presenceReply(c, v, e)
}

// Like
// @Tags LivePresence
// @Summary 登录观众批量点赞，原子幂等与限流
// @Description 数量为十进制字符串；错误49501参数，49502过期，49503权限，49504序号，49505限流，49506依赖，49507背压，49508状态异常。无WebSocket。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param X-Presence-Guest header string false "游客签名凭证；与APP JWT身份分离"
// @Param data body req.PresenceLike true "请求"
// @Success 200 {object} response.Response{data=liveRes.PresenceSession}
// @Router /v1/app/live/room/like [post]
func (a *PresenceApi) Like(c *gin.Context) {
	var r req.PresenceLike
	if !presenceBind(c, &r) {
		return
	}
	i, ok := presenceIdentity(c)
	if !ok {
		return
	}
	v, e := presenceService.Like(c.Request.Context(), i, r)
	presenceReply(c, v, e)
}

// Guest
// @Tags LivePresence
// @Summary 签发24小时游客身份，仅可访问公开直播；不代表物理去重人数
// @Produce application/json
// @Success 200 {object} response.Response{data=liveRes.PresenceGuest}
// @Router /v1/app/live/presence/guest [post]
func (a *PresenceApi) Guest(c *gin.Context) {
	var body struct{}
	if !presenceBind(c, &body) {
		return
	}
	if e := presenceService.Rate(c.Request.Context(), "guest", c.GetString("presence-ip"), 20); e != nil {
		presenceReply(c, nil, e)
		return
	}
	v, e := presenceService.NewGuest()
	presenceReply(c, v, e)
}

// Stats
// @Tags LivePresence
// @Summary 查询近实时游客与会员在线统计
// @Security AppBearerAuth
// @Produce application/json
// @Param roomNo query string true "房间编号"
// @Param cursor query string false "不透明游标，更新中的列表允许最终一致"
// @Param limit query int false "每页1至100，默认20"
// @Param X-Presence-Guest header string false "签名游客凭证"
// @Success 200 {object} response.Response{data=liveRes.PresenceStats}
// @Router /v1/app/live/presence/stats [get]
func (a *PresenceApi) Stats(c *gin.Context) {
	q, ok := presenceQuery(c)
	if !ok {
		return
	}
	i, ok := presenceIdentity(c)
	if !ok {
		return
	}
	v, e := presenceService.Stats(c.Request.Context(), i, q.RoomNo)
	presenceReply(c, v, e)
}

// Viewers
// @Tags LivePresence
// @Summary 当前在线会员公开资料游标列表
// @Security AppBearerAuth
// @Produce application/json
// @Param roomNo query string true "房间编号"
// @Param cursor query string false "不透明游标，更新中的列表允许最终一致"
// @Param limit query int false "每页1至100，默认20"
// @Param X-Presence-Guest header string false "签名游客凭证"
// @Success 200 {object} response.Response{data=liveRes.PresenceViewers}
// @Router /v1/app/live/presence/viewers [get]
func (a *PresenceApi) Viewers(c *gin.Context) {
	q, ok := presenceQuery(c)
	if !ok {
		return
	}
	i, ok := presenceIdentity(c)
	if !ok {
		return
	}
	v, e := presenceService.Viewers(c.Request.Context(), i, q, false)
	presenceReply(c, v, e)
}

// ActiveViewers
// @Tags LivePresence
// @Summary 当前会员最近活动列表，不含礼物金额
// @Security AppBearerAuth
// @Produce application/json
// @Param roomNo query string true "房间编号"
// @Param cursor query string false "不透明游标，更新中的列表允许最终一致"
// @Param limit query int false "每页1至100，默认20"
// @Param X-Presence-Guest header string false "签名游客凭证"
// @Success 200 {object} response.Response{data=liveRes.PresenceViewers}
// @Router /v1/app/live/presence/active-viewers [get]
func (a *PresenceApi) ActiveViewers(c *gin.Context) {
	q, ok := presenceQuery(c)
	if !ok {
		return
	}
	i, ok := presenceIdentity(c)
	if !ok {
		return
	}
	v, e := presenceService.Viewers(c.Request.Context(), i, q, true)
	presenceReply(c, v, e)
}

// AdminStats
// @Tags LivePresenceAdmin
// @Summary 批量查询当前或历史场次，Redis故障返回stale及空在线字段
// @Security ApiKeyAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.PresenceAdminBatch true "roomNos和sessionNos合计最多100个"
// @Success 200 {object} response.Response{data=[]liveRes.PresenceStats}
// @Router /live/presence/stats [post]
func (a *PresenceApi) AdminStats(c *gin.Context) {
	var r req.PresenceAdminBatch
	if !presenceBind(c, &r) {
		return
	}
	v, e := presenceService.AdminStats(c.Request.Context(), r)
	presenceReply(c, v, e)
}
