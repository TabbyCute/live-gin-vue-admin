package payout

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"sync"
	"tb_live_module/middleware"
	systemReq "tb_live_module/model/system/request"
	"time"
)

// Fixed-size striped token buckets bound memory. Hash collisions may conservatively
// throttle two users, but never permit bypass by creating unbounded identity entries.
func userRateLimit() gin.HandlerFunc {
	return identityRateLimit(func(c *gin.Context) (uint64, bool) { id, ok := middleware.GetAppAccountID(c); return uint64(id), ok })
}
func adminRateLimit() gin.HandlerFunc {
	return identityRateLimit(adminIdentity)
}
func sensitiveRateLimit() gin.HandlerFunc { return identityRateLimitConfig(adminIdentity, 2, 0.1) }
func adminIdentity(c *gin.Context) (uint64, bool) {
	v, _ := c.Get("claims")
	a, ok := v.(*systemReq.CustomClaims)
	if !ok || a == nil {
		return 0, false
	}
	return uint64(a.BaseClaims.ID), a.BaseClaims.ID != 0
}
func identityRateLimit(identity func(*gin.Context) (uint64, bool)) gin.HandlerFunc {
	return identityRateLimitConfig(identity, 10, 1)
}
func identityRateLimitConfig(identity func(*gin.Context) (uint64, bool), burst, rate float64) gin.HandlerFunc {
	type bucket struct {
		mu     sync.Mutex
		at     time.Time
		tokens float64
	}
	buckets := make([]bucket, 4096)
	return func(c *gin.Context) {
		id, ok := identity(c)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		b := &buckets[uint64(id)%uint64(len(buckets))]
		now := time.Now()
		b.mu.Lock()
		if b.at.IsZero() {
			b.tokens = burst
		} else {
			b.tokens += now.Sub(b.at).Seconds() * rate
			if b.tokens > burst {
				b.tokens = burst
			}
		}
		b.at = now
		allowed := b.tokens >= 1
		if allowed {
			b.tokens--
		}
		b.mu.Unlock()
		if !allowed {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 49107, "msg": "请求过于频繁，请稍后用原幂等键重试", "data": gin.H{}})
			return
		}
		c.Next()
	}
}
