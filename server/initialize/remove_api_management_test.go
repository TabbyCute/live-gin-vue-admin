package initialize

import (
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tb_live_module/model/system"
)

func TestRemoveAPIManagement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:remove-api-management?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&system.SysApi{},
		&system.SysBaseMenu{},
		&system.SysBaseMenuParameter{},
		&system.SysBaseMenuBtn{},
		&system.SysAuthorityMenu{},
		&legacySysIgnoreAPI{},
		&legacyCasbinRule{},
		&legacyDataAuthority{},
		&legacyAuthorityBtn{},
	))

	menu := system.SysBaseMenu{
		Path:      "api",
		Name:      "api",
		Component: "view/superAdmin/api/api.vue",
		Meta:      system.Meta{Title: "api管理"},
	}
	require.NoError(t, db.Create(&menu).Error)
	require.NoError(t, db.Create(&system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(menu.ID), 10), AuthorityId: "888"}).Error)
	require.NoError(t, db.Create(&system.SysBaseMenuParameter{SysBaseMenuID: menu.ID, Key: "legacy"}).Error)
	require.NoError(t, db.Create(&system.SysBaseMenuBtn{SysBaseMenuID: menu.ID, Name: "legacy"}).Error)

	legacyAPIs := []system.SysApi{
		{Path: "/api/getAllApis", Method: "POST", ApiGroup: "api"},
		{Path: "/api/createApi", Method: "POST", ApiGroup: "api"},
		{Path: "/casbin/getAllApis", Method: "POST", ApiGroup: "casbin"},
		{Path: "/authority/setDataAuthority", Method: "POST", ApiGroup: "角色"},
		{Path: "/authorityBtn/setAuthorityBtn", Method: "POST", ApiGroup: "按钮权限"},
		{Path: "/user/getUserInfo", Method: "GET", ApiGroup: "系统用户"},
	}
	require.NoError(t, db.Create(&legacyAPIs).Error)
	require.NoError(t, db.Create(&legacyCasbinRule{}).Error)
	require.NoError(t, db.Create(&legacyDataAuthority{SysAuthorityAuthorityID: 888, DataAuthorityIDAuthorityID: 888}).Error)
	require.NoError(t, db.Create(&legacyAuthorityBtn{}).Error)
	require.NoError(t, db.Create(&legacySysIgnoreAPI{Path: "/swagger/*any", Method: "GET"}).Error)

	require.NoError(t, removeAPIManagement(db))
	require.NoError(t, removeAPIManagement(db), "cleanup must be idempotent")
	require.NoError(t, removeLegacyRolePermissions(db))
	require.NoError(t, removeLegacyRolePermissions(db), "role permission cleanup must be idempotent")

	var count int64
	require.NoError(t, db.Unscoped().Model(&system.SysBaseMenu{}).Where("id = ?", menu.ID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&system.SysAuthorityMenu{}).Where("sys_base_menu_id = ?", menu.ID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Unscoped().Model(&system.SysBaseMenuParameter{}).Where("sys_base_menu_id = ?", menu.ID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Unscoped().Model(&system.SysBaseMenuBtn{}).Where("sys_base_menu_id = ?", menu.ID).Count(&count).Error)
	require.Zero(t, count)

	require.NoError(t, db.Unscoped().Model(&system.SysApi{}).Where("path LIKE ?", "/api/%").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&system.SysApi{}).Where("path = ?", "/user/getUserInfo").Count(&count).Error)
	require.EqualValues(t, 1, count)
	for _, route := range removedRolePermissionRoutes {
		require.NoError(t, db.Unscoped().Model(&system.SysApi{}).
			Where("path = ? AND method = ?", route.Path, route.Method).Count(&count).Error)
		require.Zero(t, count)
	}
	require.False(t, db.Migrator().HasTable(&legacySysIgnoreAPI{}))
	require.False(t, db.Migrator().HasTable(&legacyCasbinRule{}))
	require.False(t, db.Migrator().HasTable(&legacyDataAuthority{}))
	require.False(t, db.Migrator().HasTable(&legacyAuthorityBtn{}))
}
