package middleware

import (
	"github.com/gin-gonic/gin"
	"time"
)

// AnchorHomeReadLimit runs before optional JWT database work. It reuses the
// authentication guard's trusted-proxy parsing, HMAC IP keys, atomic Redis TTL
// and bounded 10k-entry fallback. A client cannot mint limiter keys via XFF.
// A required Redis outage fails closed; single-node deployments without Redis
// use the same bounded process-local counters.
func AnchorHomeReadLimit(scope string, perMinute int) gin.HandlerFunc {
	if scope != "json" && scope != "media" {
		panic("invalid anchor home rate scope")
	}
	if perMinute == 0 {
		if scope == "media" {
			perMinute = 600
		} else {
			perMinute = 180
		}
	}
	if perMinute < 10 || perMinute > 6000 {
		panic("invalid anchor home rate limit")
	}
	guard := NewAccountAuthGuard()
	return func(c *gin.Context) {
		if !guard.allow(c, "anchor-home:"+scope, guard.clientIP(c), perMinute, time.Minute) {
			return
		}
		c.Next()
	}
}
