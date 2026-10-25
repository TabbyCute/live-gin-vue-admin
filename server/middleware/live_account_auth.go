package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"tb_live_module/global"
	"tb_live_module/model/common/response"
	"tb_live_module/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const accountAuthMaxBody = 4096
const accountAuthMaxLimitEntries = 10000

// INCR和首次设置TTL必须原子执行，否则并发请求可以穿透计数检查。
var accountAuthLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
if count > tonumber(ARGV[2]) then
  return math.max(1, redis.call('PTTL', KEYS[1]))
end
return 0
`)

type accountAuthLimitEntry struct {
	count   int
	expires time.Time
}

// AccountAuthGuard 对公开认证接口统一限流、限制请求大小和正在执行的昂贵操作。
type AccountAuthGuard struct {
	key            []byte
	mu             sync.Mutex
	entries        map[string]accountAuthLimitEntry
	nextCleanup    time.Time
	authSlots      chan struct{}
	smsSlots       chan struct{}
	trustedProxies []netip.Prefix
	requireRedis   bool
}

func NewAccountAuthGuard() *AccountAuthGuard {
	key := []byte(global.GVA_CONFIG.JWTApp.SigningKey)
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("生成客户端认证限流密钥失败")
		}
	}
	guard := &AccountAuthGuard{
		key: key, entries: make(map[string]accountAuthLimitEntry),
		authSlots: make(chan struct{}, 16), smsSlots: make(chan struct{}, 128),
		requireRedis: global.GVA_CONFIG.System.UseRedis,
	}
	for _, raw := range global.GVA_CONFIG.LiveAccountAuth.TrustedProxies {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			address, addrErr := netip.ParseAddr(raw)
			if addrErr != nil {
				panic("live-account-auth.trusted-proxies必须为IP或CIDR")
			}
			address = address.Unmap()
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		if prefix.Bits() == 0 {
			panic("客户端认证禁止信任所有来源代理")
		}
		guard.trustedProxies = append(guard.trustedProxies, prefix)
	}
	return guard
}

func (g *AccountAuthGuard) Handler(action string, ipLimit, phoneLimit int, phoneWindow time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		if c.Request.Context().Err() != nil {
			c.Abort()
			return
		}
		if !g.allow(c, action+":ip", g.clientIP(c), ipLimit, time.Minute) {
			return
		}
		slots := g.authSlots
		if action == "sms" {
			slots = g.smsSlots
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			abortAccountAuth(c, http.StatusTooManyRequests, "服务繁忙，请稍后再试", 1)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, accountAuthMaxBody)
		// 认证正文很小，不应继承上传接口的10分钟读取超时并长期占住并发槽。
		controller := http.NewResponseController(c.Writer)
		_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			var timedOut net.Error
			if errors.As(err, &tooLarge) {
				abortAccountAuth(c, http.StatusRequestEntityTooLarge, "请求体过大", 0)
			} else if errors.As(err, &timedOut) && timedOut.Timeout() {
				abortAccountAuth(c, http.StatusRequestTimeout, "请求读取超时", 0)
			} else {
				abortAccountAuth(c, http.StatusBadRequest, "请求参数错误", 0)
			}
			return
		}
		_ = controller.SetReadDeadline(time.Time{})
		var input struct {
			Phone string `json:"phone"`
		}
		if json.Unmarshal(body, &input) != nil {
			abortAccountAuth(c, http.StatusBadRequest, "请求参数错误", 0)
			return
		}
		phone, err := utils.NormalizeAccountPhone(input.Phone)
		if err != nil {
			abortAccountAuth(c, http.StatusBadRequest, err.Error(), 0)
			return
		}
		if !g.allow(c, action+":phone", phone, phoneLimit, phoneWindow) {
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Next()
	}
}

func (g *AccountAuthGuard) allow(c *gin.Context, scope, value string, limit int, window time.Duration) bool {
	mac := hmac.New(sha256.New, g.key)
	_, _ = mac.Write([]byte(value))
	key := "live:account:auth:" + scope + ":" + hex.EncodeToString(mac.Sum(nil))
	var retry time.Duration
	if client := global.GVA_REDIS; client != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		milliseconds, err := accountAuthLimitScript.Run(ctx, client, []string{key}, window.Milliseconds(), limit).Int64()
		if err != nil {
			// Redis故障时拒绝认证请求，不能退回进程内计数而绕过分布式限流。
			abortAccountAuth(c, http.StatusServiceUnavailable, "认证服务暂不可用", 1)
			return false
		}
		retry = time.Duration(milliseconds) * time.Millisecond
	} else if g.requireRedis {
		abortAccountAuth(c, http.StatusServiceUnavailable, "认证服务暂不可用", 1)
		return false
	} else {
		// 未接Redis的单实例开发环境依然有有界、并发安全的限流。
		retry = g.allowLocal(key, limit, window, time.Now())
	}
	if retry > 0 {
		abortAccountAuth(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试", int((retry+time.Second-1)/time.Second))
		return false
	}
	return true
}

// 只解析明确配置的可信代理链，从右向左取第一个非可信地址。
func (g *AccountAuthGuard) clientIP(c *gin.Context) string {
	remote := c.RemoteIP()
	trusted := func(value string) bool {
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return false
		}
		for _, prefix := range g.trustedProxies {
			if prefix.Contains(addr.Unmap()) {
				return true
			}
		}
		return false
	}
	if !trusted(remote) {
		return remote
	}
	forwarded := c.GetHeader("X-Forwarded-For")
	if len(forwarded) > 2048 {
		return remote
	}
	addresses := strings.Split(forwarded, ",")
	for i := len(addresses) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(addresses[i]))
		if err != nil {
			return remote
		}
		value := addr.Unmap().String()
		if i == 0 || !trusted(value) {
			return value
		}
	}
	return remote
}

func (g *AccountAuthGuard) allowLocal(key string, limit int, window time.Duration, now time.Time) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !now.Before(g.nextCleanup) {
		for k, entry := range g.entries {
			if !now.Before(entry.expires) {
				delete(g.entries, k)
			}
		}
		g.nextCleanup = now.Add(time.Minute)
	}
	entry, exists := g.entries[key]
	if !exists || !now.Before(entry.expires) {
		if !exists && len(g.entries) >= accountAuthMaxLimitEntries {
			return time.Minute
		}
		g.entries[key] = accountAuthLimitEntry{count: 1, expires: now.Add(window)}
		return 0
	}
	if entry.count >= limit {
		return entry.expires.Sub(now)
	}
	entry.count++
	g.entries[key] = entry
	return 0
}

func abortAccountAuth(c *gin.Context, status int, message string, retrySeconds int) {
	if retrySeconds > 0 {
		c.Header("Retry-After", strconv.Itoa(retrySeconds))
	}
	c.AbortWithStatusJSON(status, response.Response{Code: response.ERROR, Msg: message})
}
