package httpkit

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"sync/atomic"
	"tb_live_module/internal/modulekit"
)

func Ready(state *atomic.Bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !state.Load() {
			c.Header("Retry-After", "5")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": modulekit.Unavailable.Code, "msg": modulekit.Unavailable.Message, "data": gin.H{}})
			return
		}
		c.Next()
	}
}
