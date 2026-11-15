package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"tb_live_module/global"
	"tb_live_module/model/system"
	"tb_live_module/service"
)

// GinRecovery recover掉项目可能出现的panic，并使用zap记录相关日志
func GinRecovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				if strings.Contains(c.Request.URL.Path, "/payout/") || strings.Contains(c.Request.URL.Path, "/earning/") {
					if global.GVA_LOG != nil {
						global.GVA_LOG.Error("financial request panic", zap.String("path", safeRequestRoute(c)), zap.String("traceId", RequestID(c)))
					}
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 49103, "data": gin.H{}, "msg": "服务暂不可用，请查询原请求结果"})
					return
				}
				// Check for a broken connection, as it is not really a
				// condition that warrants a panic stack trace.
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				httpRequest := fmt.Sprintf("%s %s traceId=%s", safeRequestMethod(c.Request.Method), safeRequestRoute(c), RequestID(c))
				// Panic values can contain credentials, SQL arguments or entire bodies.
				panicType := fmt.Sprintf("%T", err)
				if brokenPipe {
					recoveryLogger().Error("request connection closed",
						zap.String("panicType", panicType),
						zap.String("request", string(httpRequest)),
					)
					// If the connection is dead, we can't write a status to it.
					// Do not put the original panic in gin.Errors: other loggers may serialize it.
					c.Abort()
					return
				}

				if stack {
					form := "后端"
					info := fmt.Sprintf("Panic type: %s\nRequest: %s\nStack: %s", panicType, httpRequest, string(debug.Stack()))
					level := "error"
					persistRecoveryError(&system.SysError{
						Form:  &form,
						Info:  &info,
						Level: level,
					})
					recoveryLogger().Error("[Recovery from panic]",
						zap.String("panicType", panicType),
						zap.String("request", string(httpRequest)),
					)
				} else {
					form := "后端"
					info := fmt.Sprintf("Panic type: %s\nRequest: %s", panicType, httpRequest)
					level := "error"
					persistRecoveryError(&system.SysError{
						Form:  &form,
						Info:  &info,
						Level: level,
					})
					recoveryLogger().Error("[Recovery from panic]",
						zap.String("panicType", panicType),
						zap.String("request", string(httpRequest)),
					)
				}
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

// Recovery must still work when the database is unavailable or its error writer
// itself panics. Persist only sanitized data, within a bounded shutdown budget.
func persistRecoveryError(record *system.SysError) {
	defer func() { _ = recover() }()
	if global.GVA_DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = service.ServiceGroupApp.SystemServiceGroup.SysErrorService.CreateSysError(ctx, record)
}

func recoveryLogger() *zap.Logger {
	if global.GVA_LOG != nil {
		return global.GVA_LOG
	}
	return zap.NewNop()
}
