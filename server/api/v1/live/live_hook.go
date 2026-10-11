package live

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"tb_live_module/global"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	liveReq "tb_live_module/model/live/request"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type HookApi struct{}

const (
	defaultSRSHookMaxBodyBytes    int64 = 64 << 10
	hardSRSHookMaxBodyBytes       int64 = 1 << 20
	defaultSRSHookLogMaxBodyBytes int64 = 4 << 10
	hardSRSHookLogMaxBodyBytes    int64 = 64 << 10
)

// Publish
// @Tags LiveHook
// @Summary SRS发布流回调
// @Description 请求先通过启动时冻结的 SRS Hook 共享密钥认证，并由部署层 IP 白名单提供第二层保护；param 中必须包含 prepare 接口签发的 pt，服务端会解密并校验完整载荷。请求体有硬上限；开启 live.log-srs-hook-raw-body 后也只记录有界的清洗字段，param(pt)和未知字段不会落日志。
// @Accept application/json
// @Produce application/json
// @Param X-SRS-Hook-Token header string false "SRS Hook共享密钥；与hook_token二选一且禁止同时传递"
// @Param hook_token query string false "SRS原生HTTP Hook使用的共享密钥；与Header二选一且禁止同时传递"
// @Param data body liveReq.SRSHookReq true "SRS回调"
// @Success 200 {object} response.Response
// @Failure 413 {object} response.Response "请求体超过上限"
// @Router /v1/app/live/hook/publish [post]
func (a *HookApi) Publish(c *gin.Context) {
	if !middleware.EnsureSRSHookAuthenticated(c) {
		return
	}
	req, err := bindAndLogSRSHook(c, "publish")
	if err != nil {
		respondSRSHookBindError(c, err)
		return
	}
	if req.Action != "on_publish" {
		response.FailWithMessage("回调 action 必须为 on_publish", c)
		return
	}
	result, err := roomService.OnPublish(req)
	if err != nil {
		respondLiveRoomError(c, "处理发布流回调失败", err)
		return
	}
	response.OkWithDetailed(result, "回调处理成功", c)
}

// Unpublish
// @Tags LiveHook
// @Summary SRS停止发布流回调
// @Description 请求先通过启动时冻结的 SRS Hook 共享密钥认证，并由部署层 IP 白名单提供第二层保护；不直接结束场次，而是记录断流并开启可配置的重连窗口。请求体有硬上限；开启 live.log-srs-hook-raw-body 后也只记录有界的清洗字段，param(pt)和未知字段不会落日志。
// @Accept application/json
// @Produce application/json
// @Param X-SRS-Hook-Token header string false "SRS Hook共享密钥；与hook_token二选一且禁止同时传递"
// @Param hook_token query string false "SRS原生HTTP Hook使用的共享密钥；与Header二选一且禁止同时传递"
// @Param data body liveReq.SRSHookReq true "SRS回调"
// @Success 200 {object} response.Response
// @Failure 413 {object} response.Response "请求体超过上限"
// @Router /v1/app/live/hook/unpublish [post]
func (a *HookApi) Unpublish(c *gin.Context) {
	if !middleware.EnsureSRSHookAuthenticated(c) {
		return
	}
	req, err := bindAndLogSRSHook(c, "unpublish")
	if err != nil {
		respondSRSHookBindError(c, err)
		return
	}
	if req.Action != "on_unpublish" {
		response.FailWithMessage("回调 action 必须为 on_unpublish", c)
		return
	}
	if err := roomService.OnUnpublish(req); err != nil {
		respondLiveRoomError(c, "处理断流回调失败", err)
		return
	}
	response.OkWithMessage("回调处理成功", c)
}

// bindAndLogSRSHook 在JSON绑定前读取有硬上限的SRS请求体，并按独立的更小上限记录清洗日志。
// 未知扩展字段仍由SRSHookReq保留；任何配置都不能突破代码中的1MiB请求体硬上限。
func bindAndLogSRSHook(c *gin.Context, hook string) (liveReq.SRSHookReq, error) {
	var req liveReq.SRSHookReq
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, liveSRSHookMaxBodyBytes())
	rawBody, err := c.GetRawData()
	if err != nil {
		return req, fmt.Errorf("读取原始请求体失败: %w", err)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(rawBody))
	if global.GVA_CONFIG.Live.LogSRSHookRawBody && global.GVA_LOG != nil {
		sanitizedBody := sanitizeSRSHookLogBody(rawBody)
		loggedBody, truncated := truncateSRSHookLogBody(sanitizedBody, liveSRSHookLogMaxBodyBytes())
		global.GVA_LOG.Info("SRS Hook 清洗参数",
			zap.String("hook", hook),
			zap.String("path", c.Request.URL.Path),
			zap.String("clientIp", c.ClientIP()),
			zap.String("contentType", c.ContentType()),
			zap.Int("rawBodyBytes", len(rawBody)),
			zap.Bool("rawBodySanitized", true),
			zap.Bool("rawBodyTruncated", truncated),
			zap.String("rawBody", string(loggedBody)),
		)
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return req, err
	}
	return req, nil
}

// sanitizeSRSHookLogBody 只保留排障所需的SRS标准字段，并永久移除param和未知扩展字段。
// param 包含pt，未知字段也可能由上游携带秘密；即使本地开启原始Hook日志也不得落盘。
func sanitizeSRSHookLogBody(rawBody []byte) []byte {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &source); err != nil {
		return []byte(`{"omitted":"malformed JSON"}`)
	}
	sanitized := make(map[string]json.RawMessage, 12)
	for _, key := range []string{
		"server_id", "service_id", "action", "client_id", "ip", "vhost", "app",
		"tcUrl", "stream", "stream_url", "stream_id",
	} {
		if value, ok := source[key]; ok {
			sanitized[key] = value
		}
	}
	if _, exists := source["param"]; exists {
		sanitized["param"] = json.RawMessage(`"[REDACTED]"`)
	}
	result, err := json.Marshal(sanitized)
	if err != nil {
		return []byte(`{"omitted":"unable to sanitize"}`)
	}
	return result
}

func respondSRSHookBindError(c *gin.Context, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, response.Response{
			Code: response.ERROR,
			Data: map[string]interface{}{},
			Msg:  "回调请求体过大",
		})
		return
	}
	response.FailWithMessage("回调参数错误: "+err.Error(), c)
}

func liveSRSHookMaxBodyBytes() int64 {
	limit := global.GVA_CONFIG.Live.SRSHookMaxBodyBytes
	if limit <= 0 {
		limit = defaultSRSHookMaxBodyBytes
	}
	if limit > hardSRSHookMaxBodyBytes {
		limit = hardSRSHookMaxBodyBytes
	}
	return limit
}

func liveSRSHookLogMaxBodyBytes() int64 {
	limit := global.GVA_CONFIG.Live.SRSHookLogMaxBodyBytes
	if limit <= 0 {
		limit = defaultSRSHookLogMaxBodyBytes
	}
	if limit > hardSRSHookLogMaxBodyBytes {
		limit = hardSRSHookLogMaxBodyBytes
	}
	if bodyLimit := liveSRSHookMaxBodyBytes(); limit > bodyLimit {
		limit = bodyLimit
	}
	return limit
}

func truncateSRSHookLogBody(rawBody []byte, limit int64) ([]byte, bool) {
	if limit < 0 {
		limit = 0
	}
	if int64(len(rawBody)) <= limit {
		return rawBody, false
	}
	const suffix = "...[truncated]"
	result := make([]byte, 0, int(limit)+len(suffix))
	result = append(result, rawBody[:limit]...)
	result = append(result, suffix...)
	return result, true
}

// UpdateStats
// @Tags LiveHook
// @Summary 写入直播场次实时统计快照
// @Description 请求先通过启动时冻结的 SRS Hook 共享密钥认证，并由部署层 IP 白名单提供第二层保护，供可信统计服务写入；礼物流水仍应由独立账本保存，本接口只更新场次汇总。
// @Accept application/json
// @Produce application/json
// @Param X-SRS-Hook-Token header string false "SRS Hook共享密钥；与hook_token二选一且禁止同时传递"
// @Param hook_token query string false "SRS原生HTTP Hook使用的共享密钥；与Header二选一且禁止同时传递"
// @Param data body liveReq.LiveSessionStatsReq true "场次统计"
// @Success 200 {object} response.Response
// @Failure 413 {object} response.Response "请求体超过上限"
// @Router /v1/app/live/hook/session/stats [post]
func (a *HookApi) UpdateStats(c *gin.Context) {
	if !middleware.EnsureSRSHookAuthenticated(c) {
		return
	}
	var req liveReq.LiveSessionStatsReq
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, liveSRSHookMaxBodyBytes())
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			respondSRSHookBindError(c, err)
			return
		}
		response.FailWithMessage("统计参数错误: "+err.Error(), c)
		return
	}
	if err := roomService.UpdateSessionStats(req); err != nil {
		respondLiveRoomError(c, "更新场次统计失败", err)
		return
	}
	response.OkWithMessage("更新成功", c)
}
