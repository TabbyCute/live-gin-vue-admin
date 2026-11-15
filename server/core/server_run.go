package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"tb_live_module/internal/telemetry"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type server interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

// initServer 启动服务并实现优雅关闭
func initServer(address string, router *gin.Engine, readTimeout, writeTimeout time.Duration) {
	// 创建服务
	srv := &http.Server{
		Addr:              address,
		Handler:           router,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		WriteTimeout:      writeTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	// 在goroutine中启动服务
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("listen: %s\n", err)
			zap.L().Error("server启动失败", zap.Error(err))
			os.Exit(1)
		}
	}()

	// 等待中断信号以优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	// kill (无参数) 默认发送 syscall.SIGTERM
	// kill -2 发送 syscall.SIGINT
	// kill -9 发送 syscall.SIGKILL，但是无法被捕获，所以不需要添加
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	<-quit
	telemetry.Draining.Store(true)
	zap.L().Info("关闭WEB服务...")

	// Finish bounded financial requests before canceling background workers.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Warn("WEB服务优雅关闭超时，关闭剩余连接")
		_ = srv.Close()
	}

	zap.L().Info("WEB服务已关闭")
}
