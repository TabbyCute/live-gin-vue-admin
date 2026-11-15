package initialize

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"tb_live_module/global"
	"tb_live_module/internal/telemetry"
	"time"
)

// readiness checks required dependencies only. It neither runs migrations nor
// reads financial/user data, and is separate from the process liveness probe.
func readiness(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if telemetry.Draining.Load() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ready": false})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	ready := false
	if global.GVA_DB != nil {
		pool, err := global.GVA_DB.DB()
		ready = err == nil && pool.PingContext(ctx) == nil
	}
	if ready && global.GVA_CONFIG.System.UseRedis {
		ready = global.GVA_REDIS != nil && global.GVA_REDIS.Ping(ctx).Err() == nil
	}
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"ready": ready})
}
