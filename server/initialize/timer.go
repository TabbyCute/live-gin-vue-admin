package initialize

import (
	"fmt"
	"sync"
	"time"

	liveService "tb_live_module/service/live"
	"tb_live_module/task"

	"github.com/robfig/cron/v3"

	"tb_live_module/global"
)

var (
	liveSessionTimerMu          sync.Mutex
	liveSessionStopTimerMu      sync.Mutex
	liveStreamReadyTimerMu      sync.Mutex
	liveStreamReconcileTimerMu  sync.Mutex
	liveSRSRecoveryListenerOnce sync.Once
)

func Timer() {
	liveSRSRecoveryListenerOnce.Do(func() {
		go func() {
			for range liveService.SRSRecoverySignals() {
				// 信号通道容量为1；恢复风暴会被合并，四类任务自身的TryLock继续防止重入。
				runLiveSessionStop()
				runLiveStreamReadiness()
				runLiveStreamReconciliation()
				runLiveSessionLifecycle()
			}
		}()
	})
	var option []cron.Option
	option = append(option, cron.WithSeconds())
	// 配置重载会再次进入Timer；先清理同名Cron，避免重复注册和重复扫描。
	global.GVA_Timer.Clear("ClearDB")
	global.GVA_Timer.Clear("LiveSessionLifecycle")
	global.GVA_Timer.Clear("LiveSessionStop")
	global.GVA_Timer.Clear("LiveStreamReadiness")
	global.GVA_Timer.Clear("LiveStreamReconciliation")
	// 清理DB定时任务
	_, err := global.GVA_Timer.AddTaskByFunc("ClearDB", "@daily", func() {
		err := task.ClearTable(global.GVA_DB) // 定时任务方法定在task文件包中
		if err != nil {
			fmt.Println("timer error:", err)
		}
	}, "定时清理数据库【日志，黑名单】内容", option...)
	if err != nil {
		fmt.Println("add timer error:", err)
	}

	liveScanInterval := global.GVA_CONFIG.Live.EndScanIntervalSeconds
	if liveScanInterval <= 0 {
		liveScanInterval = 5
	}
	_, err = global.GVA_Timer.AddTaskByFunc(
		"LiveSessionLifecycle", fmt.Sprintf("@every %ds", liveScanInterval), runLiveSessionLifecycle,
		"只处理准备开播超时和直播断线超时，不执行外部SRS请求", option...,
	)
	if err != nil {
		fmt.Println("add live session timer error:", err)
		return
	}
	_, err = global.GVA_Timer.AddTaskByFunc(
		"LiveSessionStop", fmt.Sprintf("@every %ds", liveScanInterval), runLiveSessionStop,
		"有界并发处理结束中场次的SRS断流和结算；SRS故障不会阻塞其他生命周期扫描", option...,
	)
	if err != nil {
		fmt.Println("add live session stop timer error:", err)
		return
	}
	streamReadyInterval := global.GVA_CONFIG.Live.StreamReadyScanIntervalSeconds
	if streamReadyInterval <= 0 {
		streamReadyInterval = 1
	}
	_, err = global.GVA_Timer.AddTaskByFunc(
		"LiveStreamReadiness", fmt.Sprintf("@every %ds", streamReadyInterval), runLiveStreamReadiness,
		"按退避时间采集SRS视频尺寸，确认直播真正就绪或处理等待超时", option...,
	)
	if err != nil {
		fmt.Println("add live stream readiness timer error:", err)
		return
	}
	streamReconcileInterval := global.GVA_CONFIG.Live.StreamReconcileIntervalSeconds
	if streamReconcileInterval <= 0 {
		streamReconcileInterval = 5
	}
	_, err = global.GVA_Timer.AddTaskByFunc(
		"LiveStreamReconciliation", fmt.Sprintf("@every %ds", streamReconcileInterval), runLiveStreamReconciliation,
		"批量核对直播中的场次是否仍存在于SRS，连续缺失后进入断流重连窗口", option...,
	)
	if err != nil {
		fmt.Println("add live stream reconciliation timer error:", err)
		return
	}

	// 表结构已经初始化完成；异步立即补偿一次，服务重启无需再等待首个扫描周期。
	go func() {
		// 启动时先推进纯数据库状态，再立即处理由此产生或上次遗留的结束中场次。
		runLiveSessionLifecycle()
		runLiveSessionStop()
	}()
	go runLiveStreamReadiness()
	go runLiveStreamReconciliation()

	// 其他定时任务定在这里 参考上方使用方法

	//_, err := global.GVA_Timer.AddTaskByFunc("定时任务标识", "corn表达式", func() {
	//	具体执行内容...
	//  ......
	//}, option...)
	//if err != nil {
	//	fmt.Println("add timer error:", err)
	//}
}

func runLiveStreamReadiness() {
	if global.GVA_DB == nil {
		return
	}
	if !liveStreamReadyTimerMu.TryLock() {
		return
	}
	defer liveStreamReadyTimerMu.Unlock()
	if runErr := runLiveTaskWithLeader("stream-readiness", func() error {
		return (&liveService.RoomService{}).ProcessStreamReadiness(time.Now().UnixMilli())
	}); runErr != nil {
		fmt.Println("live stream readiness timer error:", runErr)
	}
}

func runLiveSessionLifecycle() {
	if global.GVA_DB == nil {
		return
	}
	// 单进程禁止启动扫描与周期扫描重叠；多实例之间由数据库状态条件和租约竞争处理权。
	if !liveSessionTimerMu.TryLock() {
		return
	}
	defer liveSessionTimerMu.Unlock()
	if runErr := runLiveTaskWithLeader("session-lifecycle", func() error {
		return (&liveService.RoomService{}).ProcessPendingSessions(time.Now().UnixMilli())
	}); runErr != nil {
		fmt.Println("live session timer error:", runErr)
	}
}

func runLiveSessionStop() {
	if global.GVA_DB == nil {
		return
	}
	// SRS故障只占用独立的收尾任务；有界worker和数据库租约分别限制单实例与多实例并发。
	if !liveSessionStopTimerMu.TryLock() {
		return
	}
	defer liveSessionStopTimerMu.Unlock()
	if runErr := runLiveTaskWithLeader("session-stop", func() error {
		return (&liveService.RoomService{}).ProcessEndingSessions(time.Now().UnixMilli())
	}); runErr != nil {
		fmt.Println("live session stop timer error:", runErr)
	}
}

func runLiveStreamReconciliation() {
	if global.GVA_DB == nil {
		return
	}
	// 单进程不允许启动补偿扫描和周期扫描重叠；多实例由场次的最近对账时间防止重复累计缺失次数。
	if !liveStreamReconcileTimerMu.TryLock() {
		return
	}
	defer liveStreamReconcileTimerMu.Unlock()
	if runErr := runLiveTaskWithLeader("stream-reconciliation", func() error {
		return (&liveService.RoomService{}).ProcessLiveStreamReconciliation(time.Now().UnixMilli())
	}); runErr != nil {
		fmt.Println("live stream reconciliation timer error:", runErr)
	}
}
