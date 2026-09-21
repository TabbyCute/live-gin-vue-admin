package initialize

import (
	"errors"
	"strconv"

	adapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"

	"tb_live_module/model/system"
)

// syncLiveAnchorAdminMetadata 为已经完成过 InitDB 的存量数据库补齐主播管理元数据。
// 只有本次新建菜单时才给超级管理员追加菜单和 Casbin 权限，避免后续启动覆盖人工调整。
func syncLiveAnchorAdminMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		parent := system.SysBaseMenu{}
		moduleMenuCreated := false
		if err := tx.Where("name = ?", "liveManagement").First(&parent).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			parent = system.SysBaseMenu{
				MenuLevel: 0,
				ParentId:  0,
				Path:      "liveManagement",
				Name:      "liveManagement",
				Component: "view/routerHolder.vue",
				Sort:      4,
				Meta:      system.Meta{Title: "直播管理", Icon: "video-camera"},
			}
			if err = tx.Create(&parent).Error; err != nil {
				return err
			}
			moduleMenuCreated = true
		}

		anchorMenu := system.SysBaseMenu{}
		if err := tx.Where("name = ?", "liveAnchor").First(&anchorMenu).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			anchorMenu = system.SysBaseMenu{
				MenuLevel: 1,
				ParentId:  parent.ID,
				Path:      "anchor",
				Name:      "liveAnchor",
				Component: "view/live/anchor/index.vue",
				Sort:      1,
				Meta:      system.Meta{Title: "主播管理", Icon: "user-filled", KeepAlive: true},
			}
			if err = tx.Create(&anchorMenu).Error; err != nil {
				return err
			}
			moduleMenuCreated = true
		}

		apis := []system.SysApi{
			{ApiGroup: "主播管理", Method: "GET", Path: "/live/anchor/list", Description: "分页查询主播列表"},
			{ApiGroup: "主播管理", Method: "GET", Path: "/live/anchor/detail", Description: "获取主播后台详情"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/audit", Description: "审核主播申请"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/status/update", Description: "修改主播账号状态"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/permission/update", Description: "修改主播功能权限"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/profile/update", Description: "后台修改主播资料"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/recommend/update", Description: "修改主播运营推荐属性"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/signed/update", Description: "修改主播签约状态"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/agency/update", Description: "修改主播公会归属"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/risk/update", Description: "修改主播风险等级"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/remark/update", Description: "修改主播后台备注"},
			{ApiGroup: "主播管理", Method: "POST", Path: "/live/anchor/cert/update", Description: "修改主播认证状态"},
		}
		for index := range apis {
			api := apis[index]
			if err := tx.Where("path = ? AND method = ?", api.Path, api.Method).FirstOrCreate(&api).Error; err != nil {
				return err
			}
		}

		if !moduleMenuCreated {
			return nil
		}

		var authority system.SysAuthority
		if err := tx.Where("authority_id = ?", 888).First(&authority).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		for _, menuID := range []uint{parent.ID, anchorMenu.ID} {
			relation := system.SysAuthorityMenu{
				MenuId:      strconv.FormatUint(uint64(menuID), 10),
				AuthorityId: strconv.FormatUint(uint64(authority.AuthorityId), 10),
			}
			if err := tx.Where(
				"sys_base_menu_id = ? AND sys_authority_authority_id = ?",
				relation.MenuId,
				relation.AuthorityId,
			).FirstOrCreate(&relation).Error; err != nil {
				return err
			}
		}

		for _, api := range apis {
			policy := adapter.CasbinRule{Ptype: "p", V0: "888", V1: api.Path, V2: api.Method}
			if err := tx.Where(&policy).FirstOrCreate(&policy).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
