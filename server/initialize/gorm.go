package initialize

import (
	"os"

	"tb_live_module/global"
	"tb_live_module/model/example"
	"tb_live_module/model/live"
	"tb_live_module/model/system"
	liveService "tb_live_module/service/live"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func Gorm() *gorm.DB {
	switch global.GVA_CONFIG.System.DbType {
	case "mysql":
		global.GVA_ACTIVE_DBNAME = &global.GVA_CONFIG.Mysql.Dbname
		return GormMysql()
	case "pgsql":
		global.GVA_ACTIVE_DBNAME = &global.GVA_CONFIG.Pgsql.Dbname
		return GormPgSql()
	case "oracle":
		global.GVA_ACTIVE_DBNAME = &global.GVA_CONFIG.Oracle.Dbname
		return GormOracle()
	case "mssql":
		global.GVA_ACTIVE_DBNAME = &global.GVA_CONFIG.Mssql.Dbname
		return GormMssql()
	case "sqlite":
		global.GVA_ACTIVE_DBNAME = &global.GVA_CONFIG.Sqlite.Dbname
		return GormSqlite()
	default:
		global.GVA_ACTIVE_DBNAME = &global.GVA_CONFIG.Mysql.Dbname
		return GormMysql()
	}
}

func RegisterTables() {
	if global.GVA_CONFIG.System.DisableAutoMigrate {
		global.GVA_LOG.Info("auto-migrate is disabled, skipping table registration")
		return
	}

	db := global.GVA_DB
	err := db.AutoMigrate(

		system.SysApi{},
		system.SysUser{},
		system.SysBaseMenu{},
		system.JwtBlacklist{},
		system.SysAuthority{},
		system.SysDictionary{},
		system.SysOperationRecord{},
		system.SysAutoCodeHistory{},
		system.SysDictionaryDetail{},
		system.SysBaseMenuParameter{},
		system.SysBaseMenuBtn{},
		system.SysAutoCodePackage{},
		system.SysExportTemplate{},
		system.Condition{},
		system.JoinTemplate{},
		system.SysParams{},
		system.SysVersion{},
		system.SysError{},
		system.SysApiToken{},
		system.SysLoginLog{},

		example.ExaFile{},
		example.ExaCustomer{},
		example.ExaFileChunk{},
		example.ExaFileUploadAndDownload{},
		example.ExaAttachmentCategory{},

		live.LiveAccount{},
		live.LiveAnchor{},
		live.LiveCategory{},
		live.LiveRoom{},
		live.LiveSession{},
	)
	if err != nil {
		global.GVA_LOG.Error("register table failed", zap.Error(err))
		os.Exit(1)
	}
	if err = migrateLiveDurationColumn(db); err != nil {
		global.GVA_LOG.Error("migrate live duration column failed", zap.Error(err))
		os.Exit(1)
	}
	if err = (&liveService.RoomService{}).BackfillApprovedAnchorRooms(db); err != nil {
		global.GVA_LOG.Error("backfill approved anchor rooms failed", zap.Error(err))
		os.Exit(1)
	}

	if err = removeAPIManagement(db); err != nil {
		global.GVA_LOG.Error("remove api management failed", zap.Error(err))
		os.Exit(1)
	}
	if err = removeLegacyRolePermissions(db); err != nil {
		global.GVA_LOG.Error("remove legacy role permissions failed", zap.Error(err))
		os.Exit(1)
	}

	if err = syncLiveAnchorAdminMetadata(db); err != nil {
		global.GVA_LOG.Error("sync live anchor admin metadata failed", zap.Error(err))
		os.Exit(1)
	}
	if err = syncLiveCategoryAdminMetadata(db); err != nil {
		global.GVA_LOG.Error("sync live category admin metadata failed", zap.Error(err))
		os.Exit(1)
	}
	if err = syncLiveRoomAdminMetadata(db); err != nil {
		global.GVA_LOG.Error("sync live room admin metadata failed", zap.Error(err))
		os.Exit(1)
	}

	err = bizModel()

	if err != nil {
		global.GVA_LOG.Error("register biz_table failed", zap.Error(err))
		os.Exit(1)
	}
	global.GVA_LOG.Info("register table success")
}
