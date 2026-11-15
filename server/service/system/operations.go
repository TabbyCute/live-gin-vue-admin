package system

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"runtime"
	"tb_live_module/global"
	"tb_live_module/internal/telemetry"
	model "tb_live_module/model/system"
	res "tb_live_module/model/system/response"
	"time"
)

var ErrOperationsForbidden = errors.New("operations requires active root administrator")

// Operations is a metadata-only, root-authorized view. It returns no DB DSN,
// hostname, bank identifiers, user details, message payloads or filesystem paths.
func (s *SystemConfigService) Operations(ctx context.Context, actor uint) (res.Operations, error) {
	if !model.IsSuperAdmin(actor) || global.GVA_DB == nil {
		return res.Operations{}, ErrOperationsForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	db := global.GVA_DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).WithContext(ctx)
	var count int64
	if e := db.Model(&model.SysUser{}).Where("id=? AND enable=1", actor).Count(&count).Error; e != nil {
		return res.Operations{}, e
	}
	if count != 1 {
		return res.Operations{}, ErrOperationsForbidden
	}
	return collectOperations(ctx, db), nil
}

func collectOperations(ctx context.Context, db *gorm.DB) res.Operations {
	out := res.Operations{At: time.Now().UnixMilli(), Database: "unavailable", Redis: "disabled", Metrics: telemetry.Default.Snapshot(), Backlogs: []res.OperationBacklog{}, Alerts: []res.OperationAlert{}}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	out.Goroutines, out.HeapBytes = runtime.NumGoroutine(), mem.HeapAlloc
	if db != nil {
		if pool, e := db.DB(); e == nil {
			stats := pool.Stats()
			out.DatabasePool = res.OperationPool{MaxOpen: stats.MaxOpenConnections, Open: stats.OpenConnections, InUse: stats.InUse, Idle: stats.Idle, WaitCount: stats.WaitCount, WaitMillis: stats.WaitDuration.Milliseconds()}
			ping, stop := context.WithTimeout(ctx, 500*time.Millisecond)
			if pool.PingContext(ping) == nil {
				out.Database = "ready"
			}
			stop()
		}
	}
	if global.GVA_CONFIG.System.UseRedis {
		out.Redis = "unavailable"
		if global.GVA_REDIS != nil {
			ping, stop := context.WithTimeout(ctx, 500*time.Millisecond)
			if global.GVA_REDIS.Ping(ping).Err() == nil {
				out.Redis = "ready"
			}
			stop()
		}
	}
	if out.Database != "ready" {
		out.Alerts = append(out.Alerts, res.OperationAlert{Code: "database_unavailable", Severity: "critical", Message: "数据库探测失败"})
	}
	if out.Redis == "unavailable" {
		out.Alerts = append(out.Alerts, res.OperationAlert{Code: "redis_unavailable", Severity: "critical", Message: "Redis探测失败"})
	}
	for _, w := range out.Metrics.Workers {
		if w.Stale || w.ConsecutiveFailures >= 3 {
			out.Alerts = append(out.Alerts, res.OperationAlert{Code: w.Name + "_unhealthy", Severity: "warning", Message: "后台任务停滞或连续失败"})
		}
	}
	if out.Database == "ready" {
		// Status + created_at uses the existing payout index and LIMIT 1001;
		// large queues are explicitly lower bounds, never an unbounded COUNT.
		for _, status := range []string{"unknown", "pending_confirmation", "ready"} {
			b := res.OperationBacklog{Name: "payout_" + status, State: "unavailable"}
			query, stop := context.WithTimeout(ctx, 400*time.Millisecond)
			var dates []int64
			e := db.WithContext(query).Table("payout_order").Where("status=?", status).Order("created_at").Limit(1001).Pluck("created_at", &dates).Error
			stop()
			if e == nil {
				b.State, b.Count = "ready", len(dates)
				if len(dates) > 1000 {
					b.Count, b.Truncated = 1000, true
				}
				if len(dates) > 0 {
					b.OldestAt = dates[0]
				}
				if status == "unknown" && b.Count > 0 {
					out.Alerts = append(out.Alerts, res.OperationAlert{Code: "payout_unknown", Severity: "critical", Message: "存在结果未知的出款，须调查，禁止盲目重付"})
				}
				if status == "pending_confirmation" && b.OldestAt > 0 && out.At-b.OldestAt > int64(time.Hour/time.Millisecond) {
					out.Alerts = append(out.Alerts, res.OperationAlert{Code: "payout_old_pending_confirmation", Severity: "warning", Message: "存在创建超过1小时且当前待复核的出款"})
				}
			}
			out.Backlogs = append(out.Backlogs, b)
		}
	}
	return out
}
