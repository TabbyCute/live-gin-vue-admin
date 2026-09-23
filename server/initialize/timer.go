package initialize

import (
	"fmt"
	"time"

	"tb_live_module/service"
	"tb_live_module/task"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"tb_live_module/global"
)

func Timer() {
	go func() {
		var option []cron.Option
		option = append(option, cron.WithSeconds())
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

		_, err = global.GVA_Timer.AddTaskByFunc("FinalizeLiveSessions", "*/5 * * * * *", func() {
			if processErr := service.ServiceGroupApp.LiveServiceGroup.RoomService.ProcessPendingSessions(time.Now().UnixMilli()); processErr != nil {
				global.GVA_LOG.Error("process pending live sessions failed", zap.Error(processErr))
			}
		}, "处理直播断流超时与场次结算", option...)
		if err != nil {
			global.GVA_LOG.Error("add live session timer failed", zap.Error(err))
		}

		// 其他定时任务定在这里 参考上方使用方法

		//_, err := global.GVA_Timer.AddTaskByFunc("定时任务标识", "corn表达式", func() {
		//	具体执行内容...
		//  ......
		//}, option...)
		//if err != nil {
		//	fmt.Println("add timer error:", err)
		//}
	}()
}
