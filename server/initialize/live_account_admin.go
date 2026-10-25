package initialize

import (
	"errors"
	"strconv"

	"tb_live_module/model/system"

	"gorm.io/gorm"
)

// syncLiveAccountAdminMetadata 为存量数据库补齐客户端用户管理菜单和接口元数据。
func syncLiveAccountAdminMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		created := false
		var parent system.SysBaseMenu
		if err := tx.Where("name = ?", "appUserManagement").First(&parent).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			parent = system.SysBaseMenu{
				MenuLevel: 0, ParentId: 0, Path: "appUserManagement", Name: "appUserManagement",
				Component: "view/routerHolder.vue", Sort: 4,
				Meta: system.Meta{Title: "用户管理", Icon: "user-filled"},
			}
			if err = tx.Create(&parent).Error; err != nil {
				return err
			}
			created = true
		}

		var child system.SysBaseMenu
		if err := tx.Where("name = ?", "liveAccount").First(&child).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			child = system.SysBaseMenu{
				MenuLevel: 1, ParentId: parent.ID, Path: "account", Name: "liveAccount",
				Component: "view/user/account/index.vue", Sort: 1,
				Meta: system.Meta{Title: "用户列表", Icon: "user", KeepAlive: true},
			}
			if err = tx.Create(&child).Error; err != nil {
				return err
			}
			created = true
		}

		for _, api := range liveAccountAdminAPIs() {
			if err := tx.Where("path = ? AND method = ?", api.Path, api.Method).FirstOrCreate(&api).Error; err != nil {
				return err
			}
		}
		if !created {
			return nil
		}
		var authority system.SysAuthority
		if err := tx.Where("authority_id = ?", 888).First(&authority).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		for _, menuID := range []uint{parent.ID, child.ID} {
			relation := system.SysAuthorityMenu{
				MenuId: strconv.FormatUint(uint64(menuID), 10), AuthorityId: strconv.FormatUint(uint64(authority.AuthorityId), 10),
			}
			if err := tx.Where("sys_base_menu_id = ? AND sys_authority_authority_id = ?", relation.MenuId, relation.AuthorityId).
				FirstOrCreate(&relation).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func liveAccountAdminAPIs() []system.SysApi {
	return []system.SysApi{
		{ApiGroup: "客户端用户", Method: "GET", Path: "/live/account/list", Description: "分页查询客户端用户"},
		{ApiGroup: "客户端用户", Method: "GET", Path: "/live/account/detail", Description: "获取客户端用户详情"},
		{ApiGroup: "客户端用户", Method: "POST", Path: "/live/account/profile/update", Description: "后台修改客户端用户资料"},
		{ApiGroup: "客户端用户", Method: "POST", Path: "/live/account/status/update", Description: "修改客户端用户状态"},
		{ApiGroup: "客户端用户", Method: "POST", Path: "/live/account/phone/unbind", Description: "解绑客户端用户手机号"},
		{ApiGroup: "客户端用户", Method: "POST", Path: "/live/account/logout-all", Description: "强制客户端用户全部登录失效"},
	}
}
