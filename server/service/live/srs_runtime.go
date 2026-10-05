package live

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"tb_live_module/global"
	srsclient "tb_live_module/internal/srs"
	liveModel "tb_live_module/model/live"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const liveSRSRuntimeSingletonID uint = 1

var liveSRSRecoverySignal = make(chan struct{}, 1)

var errLiveSRSStaleSnapshot = errors.New("SRS发布流快照早于当前运行时代际")

type liveSRSObservation struct {
	Runtime   liveModel.LiveSRSRuntime
	Restarted bool
	Recovered bool
}

// SRSRecoverySignals 用容量为1的边沿信号合并恢复风暴。定时器收到信号后立即处理积压；
// 未消费期间的重复恢复/重启事件不会创建更多goroutine或无界队列。
func SRSRecoverySignals() <-chan struct{} { return liveSRSRecoverySignal }

func signalLiveSRSRecovery() {
	select {
	case liveSRSRecoverySignal <- struct{}{}:
	default:
	}
}

func loadLiveSRSRuntime(db *gorm.DB) (liveModel.LiveSRSRuntime, error) {
	var runtime liveModel.LiveSRSRuntime
	err := db.First(&runtime, liveSRSRuntimeSingletonID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return liveModel.LiveSRSRuntime{ID: liveSRSRuntimeSingletonID}, nil
	}
	return runtime, err
}

func observeLiveSRSSuccess(identity srsclient.RuntimeIdentity, now int64) (liveSRSObservation, error) {
	var observation liveSRSObservation
	err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var runtime liveModel.LiveSRSRuntime
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&runtime, liveSRSRuntimeSingletonID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			seed := liveModel.LiveSRSRuntime{ID: liveSRSRuntimeSingletonID}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
				return err
			}
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&runtime, liveSRSRuntimeSingletonID).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		serverID := strings.TrimSpace(identity.ServerID)
		serviceID := strings.TrimSpace(identity.ServiceID)
		observation.Recovered = runtime.HealthState == liveModel.LiveSRSHealthDegraded
		if serverID != "" && runtime.ServerID != serverID {
			observation.Restarted = runtime.ServerID != ""
			if runtime.Generation == ^uint64(0) {
				return errors.New("SRS generation已达到上限")
			}
			runtime.Generation++
			runtime.ServerID = serverID
		}
		if serviceID != "" {
			runtime.ServiceID = serviceID
		}
		runtime.HealthState = liveModel.LiveSRSHealthHealthy
		runtime.LastSuccessAt = now
		runtime.LastError = ""
		if observation.Recovered || observation.Restarted {
			runtime.RecoveredAt = now
			runtime.DegradedAt = 0
		}
		if runtime.Version == ^uint64(0) {
			return errors.New("SRS runtime version已达到上限")
		}
		runtime.Version++
		if err = tx.Save(&runtime).Error; err != nil {
			return err
		}
		observation.Runtime = runtime
		return nil
	})
	if err != nil {
		return liveSRSObservation{}, err
	}
	if observation.Recovered || observation.Restarted {
		// 恢复事件绕过原来的慢速退避，最老的Ending会在下一次立即任务中优先处理。
		if err = global.GVA_DB.Model(&liveModel.LiveSession{}).
			Where("status = ? AND stop_next_retry_at > ?", liveModel.LiveSessionEnding, now).
			Update("stop_next_retry_at", now).Error; err != nil {
			return observation, err
		}
		signalLiveSRSRecovery()
	}
	return observation, nil
}

func observeLiveSRSFailure(now int64, cause error) error {
	if cause == nil || global.GVA_DB == nil {
		return nil
	}
	message := truncateRunes(cause.Error(), 500)
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		var runtime liveModel.LiveSRSRuntime
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&runtime, liveSRSRuntimeSingletonID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			seed := liveModel.LiveSRSRuntime{ID: liveSRSRuntimeSingletonID}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
				return err
			}
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&runtime, liveSRSRuntimeSingletonID).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		// 故障期间最多每30秒刷新一次相同错误，避免多个周期任务持续写同一行。
		if runtime.HealthState == liveModel.LiveSRSHealthDegraded &&
			runtime.LastError == message && now-runtime.LastFailureAt < 30_000 {
			return nil
		}
		if runtime.HealthState != liveModel.LiveSRSHealthDegraded {
			runtime.DegradedAt = now
		}
		runtime.HealthState = liveModel.LiveSRSHealthDegraded
		runtime.LastFailureAt = now
		runtime.LastError = message
		if runtime.Version == ^uint64(0) {
			return errors.New("SRS runtime version已达到上限")
		}
		runtime.Version++
		return tx.Save(&runtime).Error
	})
}

func (s *RoomService) verifyCurrentSRSRuntime(expectedServerID string, now int64) (liveSRSObservation, error) {
	controller, err := s.liveSRSController()
	if err != nil {
		_ = observeLiveSRSFailure(now, err)
		return liveSRSObservation{}, err
	}
	inspector, ok := controller.(srsclient.RuntimeController)
	if !ok {
		return liveSRSObservation{}, errors.New("当前SRS控制器不支持实例身份校验")
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveSRSRequestTimeout())
	defer cancel()
	identity, err := inspector.CurrentRuntime(ctx)
	if err != nil {
		_ = observeLiveSRSFailure(observedUnixMilli(now), err)
		return liveSRSObservation{}, err
	}
	if expected := strings.TrimSpace(expectedServerID); expected == "" || identity.ServerID != expected {
		return liveSRSObservation{}, fmt.Errorf(
			"SRS Hook实例与当前HTTP API实例不一致: hook=%q api=%q", expected, identity.ServerID,
		)
	}
	return observeLiveSRSSuccess(identity, observedUnixMilli(now))
}

func observePublisherBatch(batch srsclient.PublisherBatch, now int64, err error) (liveSRSObservation, error) {
	if err != nil {
		return liveSRSObservation{}, errors.Join(err, observeLiveSRSFailure(now, err))
	}
	// 短时共享缓存可能跨越SRS重启。新实例已经由Hook/API确认后，旧server_id缓存
	// 只能丢弃，不能把全局server_id回滚、虚增generation或累计错误的流缺失。
	if batch.CapturedAt > 0 && strings.TrimSpace(batch.Runtime.ServerID) != "" {
		runtime, loadErr := loadLiveSRSRuntime(global.GVA_DB)
		if loadErr != nil {
			return liveSRSObservation{}, loadErr
		}
		if runtime.LastSuccessAt > 0 && batch.CapturedAt <= runtime.LastSuccessAt &&
			strings.TrimSpace(runtime.ServerID) != "" &&
			strings.TrimSpace(runtime.ServerID) != strings.TrimSpace(batch.Runtime.ServerID) {
			return liveSRSObservation{}, fmt.Errorf(
				"%w: cached=%s current=%s capturedAt=%d currentAt=%d",
				errLiveSRSStaleSnapshot, batch.Runtime.ServerID, runtime.ServerID,
				batch.CapturedAt, runtime.LastSuccessAt,
			)
		}
		if batch.FromCache && strings.TrimSpace(runtime.ServerID) == strings.TrimSpace(batch.Runtime.ServerID) {
			// 缓存是某个过去时刻的可靠事实，但不是本次SRS健康探测。尤其当它早于
			// 最近一次失败时，不能用缓存把degraded误恢复为healthy，也不必重复写全局行。
			if runtime.HealthState != liveModel.LiveSRSHealthDegraded ||
				batch.CapturedAt <= runtime.LastFailureAt {
				return liveSRSObservation{Runtime: runtime}, nil
			}
		}
	}
	observation, observeErr := observeLiveSRSSuccess(batch.Runtime, now)
	return observation, observeErr
}

func effectiveLiveMediaState(session liveModel.LiveSession, runtime liveModel.LiveSRSRuntime) uint8 {
	if runtime.HealthState == liveModel.LiveSRSHealthDegraded &&
		(session.Status == liveModel.LiveSessionPreparing || session.Status == liveModel.LiveSessionLiving || session.Status == liveModel.LiveSessionEnding) {
		return liveModel.LiveMediaDegraded
	}
	return session.MediaState
}

func liveSRSRuntimeStaleForManualConfirmation(runtime liveModel.LiveSRSRuntime, now int64) bool {
	if runtime.HealthState != liveModel.LiveSRSHealthDegraded {
		return false
	}
	critical := int64(liveEndCriticalSeconds()) * int64(time.Second/time.Millisecond)
	return runtime.DegradedAt > 0 && now-runtime.DegradedAt >= critical
}
