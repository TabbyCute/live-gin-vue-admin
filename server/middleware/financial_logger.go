package middleware

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"strings"
)

// Debug access logs must not echo attacker-supplied account numbers in a rejected
// query/path. Financial routes never log bodies, raw errors or headers.
func FinancialSafeLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(p gin.LogFormatterParams) string {
		path := p.Path
		if strings.Contains(p.Request.URL.Path, "/payout/") {
			path = "/payout/[redacted]"
		}
		if strings.Contains(p.Request.URL.Path, "/earning/") {
			path = "/earning/[redacted]"
		}
		return fmt.Sprintf("[HTTP] %d %s %s %s\n", p.StatusCode, p.Method, path, p.Latency)
	})
}
