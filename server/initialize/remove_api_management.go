package initialize

import (
	"gorm.io/gorm"

	"tb_live_module/model/system"
)

var removedAPIManagementRoutes = []struct {
	Path   string
	Method string
}{
	{Path: "/api/createApi", Method: "POST"},
	{Path: "/api/deleteApi", Method: "POST"},
	{Path: "/api/updateApi", Method: "POST"},
	{Path: "/api/getApiList", Method: "POST"},
	{Path: "/api/getAllApis", Method: "POST"},
	{Path: "/api/getApiById", Method: "POST"},
	{Path: "/api/deleteApisByIds", Method: "DELETE"},
	{Path: "/api/syncApi", Method: "GET"},
	{Path: "/api/getApiGroups", Method: "GET"},
	{Path: "/api/enterSyncApi", Method: "POST"},
	{Path: "/api/ignoreApi", Method: "POST"},
	{Path: "/api/getApiRoles", Method: "GET"},
	{Path: "/api/setApiRoles", Method: "POST"},
	{Path: "/api/freshCasbin", Method: "GET"},
}

type legacySysIgnoreAPI struct {
	ID     uint   `gorm:"primaryKey"`
	Path   string `gorm:"column:path"`
	Method string `gorm:"column:method"`
}

func (legacySysIgnoreAPI) TableName() string {
	return "sys_ignore_apis"
}

// removeAPIManagement 清理已经移除的 API 管理页面、接口和初始化数据。
// sys_apis 仍供代码生成器维护接口元数据，因此这里只删除已下线功能自身的记录。
func removeAPIManagement(db *gorm.DB) error {
	if err := db.Transaction(func(tx *gorm.DB) error {
		var menus []system.SysBaseMenu
		if err := tx.Unscoped().
			Where("(name = ? AND path = ?) OR component = ?", "api", "api", "view/superAdmin/api/api.vue").
			Find(&menus).Error; err != nil {
			return err
		}

		if len(menus) > 0 {
			menuIDs := make([]uint, 0, len(menus))
			for _, menu := range menus {
				menuIDs = append(menuIDs, menu.ID)
			}
			if err := tx.Where("sys_base_menu_id IN ?", menuIDs).Delete(&system.SysAuthorityMenu{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("sys_base_menu_id IN ?", menuIDs).Delete(&system.SysBaseMenuParameter{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("sys_base_menu_id IN ?", menuIDs).Delete(&system.SysBaseMenuBtn{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("id IN ?", menuIDs).Delete(&system.SysBaseMenu{}).Error; err != nil {
				return err
			}
		}

		for _, route := range removedAPIManagementRoutes {
			if err := tx.Unscoped().Where("path = ? AND method = ?", route.Path, route.Method).Delete(&system.SysApi{}).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	if db.Migrator().HasTable(&legacySysIgnoreAPI{}) {
		return db.Migrator().DropTable(&legacySysIgnoreAPI{})
	}
	return nil
}

type legacyCasbinRule struct {
	ID uint `gorm:"primaryKey"`
}

func (legacyCasbinRule) TableName() string { return "casbin_rule" }

type legacyDataAuthority struct {
	SysAuthorityAuthorityID    uint `gorm:"column:sys_authority_authority_id"`
	DataAuthorityIDAuthorityID uint `gorm:"column:data_authority_id_authority_id"`
}

func (legacyDataAuthority) TableName() string { return "sys_data_authority_id" }

type legacyAuthorityBtn struct {
	ID uint `gorm:"primaryKey"`
}

func (legacyAuthorityBtn) TableName() string { return "sys_authority_btns" }

var removedRolePermissionRoutes = []struct {
	Path   string
	Method string
}{
	{Path: "/authority/setDataAuthority", Method: "POST"},
	{Path: "/casbin/updateCasbin", Method: "POST"},
	{Path: "/casbin/getPolicyPathByAuthorityId", Method: "POST"},
	{Path: "/casbin/getAllApis", Method: "POST"},
	{Path: "/authorityBtn/setAuthorityBtn", Method: "POST"},
	{Path: "/authorityBtn/getAuthorityBtn", Method: "POST"},
	{Path: "/authorityBtn/canRemoveAuthorityBtn", Method: "POST"},
}

// removeLegacyRolePermissions 清理旧版 API、数据范围、按钮权限及 Casbin 策略表。
// 角色和菜单关系保留在 sys_authority_menus 中。
func removeLegacyRolePermissions(db *gorm.DB) error {
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, route := range removedRolePermissionRoutes {
			if err := tx.Unscoped().Where("path = ? AND method = ?", route.Path, route.Method).
				Delete(&system.SysApi{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, table := range []any{&legacyCasbinRule{}, &legacyDataAuthority{}, &legacyAuthorityBtn{}} {
		if db.Migrator().HasTable(table) {
			if err := db.Migrator().DropTable(table); err != nil {
				return err
			}
		}
	}
	return nil
}
