package httpkit

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"sync"
	"tb_live_module/middleware"
	"time"
)

// Fixed-size striped token buckets bound memory. Hash collisions may conservatively
// throttle two users, but never permit bypass by creating unbounded identity entries.
func UserRateLimit() gin.HandlerFunc {
	type bucket struct {
		mu     sync.Mutex
		at     time.Time
		tokens float64
	}
	buckets := make([]bucket, 4096)
	return func(c *gin.Context) {
		id, ok := middleware.GetAppAccountID(c)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		b := &buckets[uint64(id)%uint64(len(buckets))]
		now := time.Now()
		b.mu.Lock()
		if b.at.IsZero() {
			b.tokens = 20
		} else {
			b.tokens += now.Sub(b.at).Seconds() * 10
			if b.tokens > 20 {
				b.tokens = 20
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
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 49007, "msg": "请求过于频繁，请稍后用原幂等键重试", "data": gin.H{}})
			return
		}
		c.Next()
	}
}
