package core

import (
	"context"
	"fmt"
	earningService "tb_live_module/service/earning"
	socialService "tb_live_module/service/social"
	"time"

	"tb_live_module/global"
	"tb_live_module/initialize"
	"tb_live_module/service/system"

	"go.uber.org/zap"
)

func RunServer() {
	defer func() { _ = zap.L().Sync() }()
	workerContext, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	if global.GVA_CONFIG.System.UseMongo {
		if err := initialize.Mongo.Initialization(); err != nil {
			zap.L().Error(fmt.Sprintf("%+v", err))
		}
	}

	if global.GVA_DB != nil {
		system.LoadAll()
	}
	if socialService.Default.DB != nil && socialService.Default.Redis != nil {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); socialService.Default.Run(ctx) }()
		defer func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				zap.L().Warn("social worker shutdown timed out")
			}
		}()
	}
	stopPresence := initialize.StartPresenceWorker()
	defer stopPresence()
	eventDone := initialize.StartEventDeliveryWorker(workerContext)
	mediaDone := initialize.StartAnchorHomeMediaCleanup(workerContext)
	auditDone := make(chan struct{})
	go func() { defer close(auditDone); earningService.Default.RunAuditWorker(workerContext) }()
	defer func() {
		stopWorkers()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for _, done := range []<-chan struct{}{eventDone, auditDone, mediaDone} {
			select {
			case <-done:
			case <-timer.C:
				zap.L().Warn("business workers shutdown timed out")
				return
			}
		}
	}()
	Router := initialize.Routers()
	stopEngagement := initialize.StartEngagementWorkers()
	defer stopEngagement()
	address := fmt.Sprintf(":%d", global.GVA_CONFIG.System.Addr)
	initServer(address, Router, 10*time.Minute, 10*time.Minute)
}
