package initialize

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"sync"
	"tb_live_module/global"
	model "tb_live_module/model/guard"
	"tb_live_module/service/gift"
	"tb_live_module/service/guard"
	"tb_live_module/service/ranking"
	"time"
)

func InitializeEngagement() error {
	global.GuardReady.Store(false)
	global.RankingReady.Store(false)
	gift.Default.Guard = nil
	gift.Default.GuardEnabled = false
	// Transitional release: with sales disabled an installation containing no
	// new module tables keeps ordinary gifts available until the offline migration.
	present, e := engagementPresence(global.GVA_DB)
	if e != nil {
		return e
	}
	if present == 0 {
		if global.GVA_CONFIG.Gift.GuardEnabled {
			return fmt.Errorf("guard schema missing; cannot enable sales")
		}
		return nil
	}
	if e := global.GVA_CONFIG.Ranking.Validate(); e != nil {
		return e
	}
	if e := ValidateGuardSchema(global.GVA_DB); e != nil {
		return e
	}
	if e := ValidateRankingSchema(global.GVA_DB); e != nil {
		return e
	}
	guard.Default.DB = global.GVA_DB
	ranking.Default.DB = global.GVA_DB
	ranking.Default.Timezone = global.GVA_CONFIG.Ranking.Zone()
	gift.Default.Guard = guard.Default
	// All worker types are installed together in StartEngagementWorkers before listening.
	if global.GVA_CONFIG.Gift.GuardEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		e := guard.Default.VerifyAll(ctx)
		cancel()
		if e != nil {
			return fmt.Errorf("guard readiness consistency failed: %w", e)
		}
		var count int64
		if e := global.GVA_DB.Model(&model.Plan{}).Where("status='enabled'").Count(&count).Error; e != nil {
			return e
		}
		if count == 0 {
			return fmt.Errorf("guard cannot enable without an enabled plan")
		}
		var failed int64
		if e := global.GVA_DB.Model(&model.Task{}).Where("status='failed' AND retry_count>=20").Count(&failed).Error; e != nil {
			return e
		}
		if failed > 0 {
			return fmt.Errorf("guard cannot enable with exhausted fulfillment tasks")
		}
	}
	global.GuardReady.Store(true)
	global.RankingReady.Store(true)
	gift.Default.GuardEnabled = global.GVA_CONFIG.Gift.GuardEnabled
	return SyncEngagementMetadata(global.GVA_DB)
}
func StartEngagementWorkers() func() {
	if !global.GuardReady.Load() || !global.RankingReady.Load() {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	tasks := []struct {
		name string
		run  func(context.Context) error
	}{
		{"guard_fulfillment", func(c context.Context) error { _, e := gift.Default.ProcessBenefits(c, 20); return e }},
		{"guard_expiry", func(c context.Context) error { _, e := guard.Default.ExpireBatch(c, 50); return e }},
		{"ranking_projection", func(c context.Context) error { _, e := ranking.Default.Consume(c, 20); return e }},
	}
	for _, task := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				call, stop := context.WithTimeout(ctx, 30*time.Second)
				e := task.run(call)
				stop()
				if e != nil && ctx.Err() == nil {
					global.GVA_LOG.Warn("engagement worker failed", zap.String("worker", task.name))
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	return func() {
		cancel()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			global.GVA_LOG.Warn("engagement worker shutdown timed out")
		}
	}
}
