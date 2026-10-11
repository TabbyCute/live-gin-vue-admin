package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"

	"tb_live_module/model/common/response"

	"github.com/gin-gonic/gin"
)

const (
	SRSHookTokenEnv        = "SRS_HOOK_TOKEN"
	SRSHookTokenHeader     = "X-SRS-Hook-Token"
	SRSHookTokenQuery      = "hook_token"
	minSRSHookTokenBytes   = 32
	maxSRSHookTokenBytes   = 256
	srsHookAuthFailedError = "SRS Hook认证失败"
	srsHookAuthContextKey  = "srs-hook-authenticated"
)

var ErrSRSHookTokenInvalid = errors.New("SRS Hook共享密钥配置无效")

// SRSHookAuthenticator 保存启动时生成的不可变密钥摘要。请求热路径只做有界长度检查、
// SHA-256 和固定长度常量时间比较，不读取环境变量或动态配置，也不访问 Redis/数据库。
type SRSHookAuthenticator struct {
	tokenDigest [sha256.Size]byte
}

// NewSRSHookAuthenticator 校验共享密钥并构造不可变认证器。
// 密钥限制为 URL-safe 字符，兼容 SRS 原生 http_hooks 的查询参数传递方式。
func NewSRSHookAuthenticator(token string) (*SRSHookAuthenticator, error) {
	if len(token) < minSRSHookTokenBytes {
		return nil, fmt.Errorf("%w: 长度不能少于%d字节", ErrSRSHookTokenInvalid, minSRSHookTokenBytes)
	}
	if len(token) > maxSRSHookTokenBytes {
		return nil, fmt.Errorf("%w: 长度不能超过%d字节", ErrSRSHookTokenInvalid, maxSRSHookTokenBytes)
	}
	for i := 0; i < len(token); i++ {
		ch := token[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return nil, fmt.Errorf("%w: 只能使用Base64URL安全字符", ErrSRSHookTokenInvalid)
	}
	return &SRSHookAuthenticator{tokenDigest: sha256.Sum256([]byte(token))}, nil
}

// Handler 强制每个 SRS Hook 请求携带且只携带一种共享密钥。
// Header 适合可信反向代理注入；SRS 直接回调时使用 hook_token 查询参数。
func (a *SRSHookAuthenticator) Handler() gin.HandlerFunc {
	expected := a.tokenDigest
	return func(c *gin.Context) {
		provided, ok := singleSRSHookToken(c)
		redactSRSHookCredentialFromRequest(c)
		if !ok || len(provided) < minSRSHookTokenBytes || len(provided) > maxSRSHookTokenBytes {
			abortSRSHookUnauthorized(c)
			return
		}
		actual := sha256.Sum256([]byte(provided))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			abortSRSHookUnauthorized(c)
			return
		}
		c.Set(srsHookAuthContextKey, true)
		c.Next()
	}
}

// redactSRSHookCredentialFromRequest 防止后续Handler和“请求完成后记录”的日志中间件再次看到密钥。
// 入口代理在请求到达应用前已记录的查询串仍需由部署层单独关闭或脱敏。
func redactSRSHookCredentialFromRequest(c *gin.Context) {
	c.Request.Header.Del(SRSHookTokenHeader)
	query := c.Request.URL.Query()
	if _, exists := query[SRSHookTokenQuery]; !exists {
		return
	}
	query.Set(SRSHookTokenQuery, "[REDACTED]")
	c.Request.URL.RawQuery = query.Encode()
	c.Request.RequestURI = c.Request.URL.RequestURI()
}

// EnsureSRSHookAuthenticated 让Hook Handler在被误注册到无认证路由时仍保持失败关闭。
// 正常路径只读取一次Gin上下文，不重复计算摘要。
func EnsureSRSHookAuthenticated(c *gin.Context) bool {
	value, exists := c.Get(srsHookAuthContextKey)
	authenticated, ok := value.(bool)
	if !exists || !ok || !authenticated {
		abortSRSHookUnauthorized(c)
		return false
	}
	return true
}

func singleSRSHookToken(c *gin.Context) (string, bool) {
	headers := c.Request.Header.Values(SRSHookTokenHeader)
	queries, hasQuery := c.Request.URL.Query()[SRSHookTokenQuery]

	// 两种传输方式同时存在或同一字段重复，都按歧义请求拒绝，避免代理和应用取值规则不同。
	if len(headers) > 1 || len(queries) > 1 || (len(headers) == 1 && hasQuery) {
		return "", false
	}
	if len(headers) == 1 {
		return headers[0], headers[0] != ""
	}
	if len(queries) == 1 {
		return queries[0], queries[0] != ""
	}
	return "", false
}

func abortSRSHookUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, response.Response{
		Code: response.ERROR,
		Data: map[string]interface{}{},
		Msg:  srsHookAuthFailedError,
	})
}
