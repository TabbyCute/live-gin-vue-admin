package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"strconv"
	model "tb_live_module/model/earning"
	"tb_live_module/model/system"
)

// MigrateEarningAudit is additive: no original financial row or financial
// trigger is changed. Old binaries safely ignore this operational table.
func MigrateEarningAudit(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("earning audit database unavailable")
	}
	if db.Dialector.Name() == "mysql" {
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	return db.AutoMigrate(&model.AuditJob{})
}

// SyncEarningAuditMetadata only adds this release's audit UI/API metadata. It
// never repairs or broadens unrelated historical earning permissions.
func SyncEarningAuditMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var parent system.SysBaseMenu
		if e := tx.Where("name='liveManagement'").Take(&parent).Error; e != nil {
			return e
		}
		menu := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: "earning-audit", Name: "earning-audit", Component: "view/live/earning/audit/index.vue", Sort: 31, Meta: system.Meta{Title: "收益深度核验", Icon: "coin", KeepAlive: true}}
		if e := tx.Where("name=?", menu.Name).FirstOrCreate(&menu).Error; e != nil {
			return e
		}
		grant := system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(menu.ID), 10), AuthorityId: "888"}
		if e := tx.Where("sys_base_menu_id=? AND sys_authority_authority_id=?", grant.MenuId, grant.AuthorityId).FirstOrCreate(&grant).Error; e != nil {
			return e
		}
		for _, op := range []struct{ path, method string }{{"/earning/audit/start", "POST"}, {"/earning/audit/cancel", "POST"}, {"/earning/audit/list", "GET"}} {
			api := system.SysApi{Path: op.path, Method: op.method, ApiGroup: "主播收益", Description: "收益深度核验"}
			if e := tx.Where("path=? AND method=?", op.path, op.method).FirstOrCreate(&api).Error; e != nil {
				return e
			}
			permission := model.AdminPermission{AuthorityID: 888, Path: op.path, Method: op.method}
			if e := tx.Where("authority_id=888 AND path=? AND method=?", op.path, op.method).FirstOrCreate(&permission).Error; e != nil {
				return e
			}
		}
		return nil
	})
}

func ValidateEarningAuditSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("earning audit database unavailable")
	}
	if db.Dialector.Name() == "mysql" {
		return validateFinancialMySQLSchema(db, []any{&model.AuditJob{}}, nil)
	}
	st := &gorm.Statement{DB: db}
	if e := st.Parse(&model.AuditJob{}); e != nil {
		return e
	}
	for _, f := range st.Schema.Fields {
		if f.DBName != "" && !db.Migrator().HasColumn(&model.AuditJob{}, f.DBName) {
			return fmt.Errorf("earning audit column missing: %s", f.DBName)
		}
	}
	return nil
}
