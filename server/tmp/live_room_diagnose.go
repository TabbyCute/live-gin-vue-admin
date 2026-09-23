package main

import (
	"fmt"
	"os"

	"tb_live_module/config"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type row struct {
	AnchorID    uint
	AnchorNo    string
	Nickname    string
	ApplyStatus uint8
	AuditAt     int64
	RoomID      uint
	RoomNo      string
}

func main() {
	v := viper.New()
	v.SetConfigFile("config.dev.yaml")
	if err := v.ReadInConfig(); err != nil {
		panic(err)
	}
	var cfg config.Server
	if err := v.Unmarshal(&cfg); err != nil {
		panic(err)
	}
	db, err := gorm.Open(mysql.Open(cfg.Mysql.Dsn()), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	var missing int64
	if err = db.Raw(`SELECT COUNT(*)
		FROM live_anchor a
		LEFT JOIN live_room r ON r.anchor_id = a.id AND r.deleted_at IS NULL
		WHERE a.deleted_at IS NULL AND a.apply_status = 2 AND r.id IS NULL`).Scan(&missing).Error; err != nil {
		panic(err)
	}
	var rows []row
	if err = db.Raw(`SELECT a.id AS anchor_id, a.anchor_no, a.nickname, a.apply_status, a.audit_at,
		COALESCE(r.id, 0) AS room_id, COALESCE(r.room_no, '') AS room_no
		FROM live_anchor a
		LEFT JOIN live_room r ON r.anchor_id = a.id AND r.deleted_at IS NULL
		WHERE a.deleted_at IS NULL AND a.apply_status = 2
		ORDER BY a.audit_at DESC, a.id DESC LIMIT 10`).Scan(&rows).Error; err != nil {
		panic(err)
	}
	fmt.Printf("approved_without_room=%d\n", missing)
	for _, item := range rows {
		fmt.Printf("anchor_id=%d anchor_no=%s nickname=%q audit_at=%d room_id=%d room_no=%s\n",
			item.AnchorID, item.AnchorNo, item.Nickname, item.AuditAt, item.RoomID, item.RoomNo)
	}
	_ = os.Stdout.Sync()
}
