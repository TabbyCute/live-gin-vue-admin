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

var liveSessionTimerMu sync.Mutex

func Timer() {
	var option []cron.Option
	option = append(option, cron.WithSeconds())
	// 配置重载会再次进入Timer；先清理同名Cron，避免重复注册和重复扫描。
	global.GVA_Timer.Clear("ClearDB")
	global.GVA_Timer.Clear("LiveSessionLifecycle")
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
		"处理准备开播超时、直播断线超时和结束中场次的SRS断流重试", option...,
	)
	if err != nil {
		fmt.Println("add live session timer error:", err)
		return
	}

	// 表结构已经初始化完成；异步立即补偿一次，服务重启无需再等待首个扫描周期。
	go runLiveSessionLifecycle()

	// 其他定时任务定在这里 参考上方使用方法

	//_, err := global.GVA_Timer.AddTaskByFunc("定时任务标识", "corn表达式", func() {
	//	具体执行内容...
	//  ......
	//}, option...)
	//if err != nil {
	//	fmt.Println("add timer error:", err)
	//}
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
	if runErr := (&liveService.RoomService{}).ProcessPendingSessions(time.Now().UnixMilli()); runErr != nil {
		fmt.Println("live session timer error:", runErr)
	}
}
