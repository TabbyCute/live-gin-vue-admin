package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"tb_live_module/global"
	"tb_live_module/internal/telemetry"
)

// RequestID is server-generated, never copied from an untrusted request header.
// It correlates HTTP, financial audit and recovery logs without logging secrets.
func RequestID(c *gin.Context) string {
	if id := c.GetString("requestTrace"); id != "" {
		return id
	}
	id := "rt_" + uuid.NewString()
	c.Set("requestTrace", id)
	c.Header("X-Request-ID", id)
	return id
}

func safeRequestRoute(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	return "[unmatched]"
}

func safeRequestMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

// RequestObservability logs only low-cardinality metadata. In particular, it
// never copies raw URLs, query strings, headers, bodies, or error messages.
// Install outside Recovery so panics have a trace and their final 500 is logged.
func RequestObservability() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		telemetry.Default.BeginRequest()
		defer func() {
			telemetry.Default.EndRequest(safeRequestMethod(c.Request.Method), safeRequestRoute(c), c.Writer.Status(), time.Since(start))
		}()
		id := RequestID(c)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if c.GetHeader("Authorization") != "" || c.GetHeader("X-Token") != "" || c.GetHeader("Cookie") != "" {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
		if global.GVA_LOG == nil {
			return
		}
		fields := []zap.Field{
			zap.String("traceId", id), zap.String("method", safeRequestMethod(c.Request.Method)),
			zap.String("route", safeRequestRoute(c)), zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", time.Since(start)), zap.Int("responseBytes", c.Writer.Size()),
		}
		if c.Writer.Status() >= 500 {
			global.GVA_LOG.Error("http request", fields...)
		} else {
			global.GVA_LOG.Info("http request", fields...)
		}
	}
}
