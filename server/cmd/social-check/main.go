// social-check 为离线维护入口；不加载线上配置、不打印DSN、不执行DDL。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	req "tb_live_module/model/social/request"
	live "tb_live_module/service/live"
	social "tb_live_module/service/social"
	"time"
)

func main() {
	account := flag.String("account-no", "", "待核对的客户端公开账号")
	repair := flag.Bool("repair", false, "持锁修复计数")
	confirm := flag.String("confirm", "", "修复必须填REPAIR")
	operator := flag.String("operator", "", "修复操作人（写入维护审计输出）")
	reason := flag.String("reason", "", "修复原因")
	flag.Parse()
	if *account == "" || (*repair && (*confirm != "REPAIR" || *operator == "" || len(*reason) < 4)) {
		fmt.Fprintln(os.Stderr, "必须指定 --account-no；修复另需 --confirm REPAIR --operator --reason")
		os.Exit(2)
	}
	dsn := os.Getenv("SOCIAL_MYSQL_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "缺少SOCIAL_MYSQL_DSN，需parseTime=true&loc=UTC及连接超时")
		os.Exit(2)
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "数据库连接失败")
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		os.Exit(1)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(2)
	service := &social.Service{DB: db, Directory: live.SocialDirectory{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := service.AuditCounters(ctx, req.Target{AccountNo: *account}, *repair)
	output := map[string]interface{}{"at": time.Now().UTC().Format(time.RFC3339Nano), "operator": *operator, "reason": *reason, "repair": *repair, "result": result, "success": err == nil}
	_ = json.NewEncoder(os.Stdout).Encode(output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "核对/修复失败，未完成事务已回滚；检查数据库及权限")
		os.Exit(1)
	}
}
