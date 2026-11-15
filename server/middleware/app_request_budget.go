package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// AppRequestBudget is the outer bound for client JSON and SRS hook routes.
// Financial/auth routes retain their stricter limits. Admin evidence uploads
// are registered separately and do not pass through this JSON-only boundary.
// Handlers run synchronously: no goroutine may write to a recycled gin.Context.
func AppRequestBudget() gin.HandlerFunc {
	return func(c *gin.Context) {
		const maxBody = 64 << 10
		// Keep the deadline through net/http's unread-body cleanup as well.
		controller := http.NewResponseController(c.Writer)
		_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
		if c.Request.ContentLength > maxBody {
			abortAccountAuth(c, http.StatusRequestEntityTooLarge, "请求体过大", 0)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		if ctx.Err() != nil {
			c.AbortWithStatus(http.StatusRequestTimeout)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
		c.Next()
	}
}
