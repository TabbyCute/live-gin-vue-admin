package initialize

import (
	"errors"
	"strconv"

	"gorm.io/gorm"

	"tb_live_module/model/system"
)

// syncLiveCategoryAdminMetadata 为已经完成过 InitDB 的存量数据库补齐直播分类管理元数据。
// 仅在本次新建分类菜单时给超级管理员追加权限，避免后续启动覆盖人工权限调整。
func syncLiveCategoryAdminMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		parent := system.SysBaseMenu{}
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
		}

		categoryMenu := system.SysBaseMenu{}
		categoryMenuCreated := false
		if err := tx.Where("name = ?", "liveCategory").First(&categoryMenu).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			categoryMenu = system.SysBaseMenu{
				MenuLevel: 1,
				ParentId:  parent.ID,
				Path:      "category",
				Name:      "liveCategory",
				Component: "view/live/category/index.vue",
				Sort:      2,
				Meta:      system.Meta{Title: "直播分类", Icon: "collection-tag", KeepAlive: true},
			}
			if err = tx.Create(&categoryMenu).Error; err != nil {
				return err
			}
			categoryMenuCreated = true
		}

		apis := liveCategoryAdminAPIs()
		for index := range apis {
			api := apis[index]
			if err := tx.Where("path = ? AND method = ?", api.Path, api.Method).FirstOrCreate(&api).Error; err != nil {
				return err
			}
		}

		if !categoryMenuCreated {
			return nil
		}
		var authority system.SysAuthority
		if err := tx.Where("authority_id = ?", 888).First(&authority).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		for _, menuID := range []uint{parent.ID, categoryMenu.ID} {
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
		return nil
	})
}

func liveCategoryAdminAPIs() []system.SysApi {
	return []system.SysApi{
		{ApiGroup: "直播分类", Method: "GET", Path: "/live/category/list", Description: "分页查询直播分类"},
		{ApiGroup: "直播分类", Method: "GET", Path: "/live/category/tree", Description: "获取后台直播分类树"},
		{ApiGroup: "直播分类", Method: "POST", Path: "/live/category/create", Description: "创建直播分类"},
		{ApiGroup: "直播分类", Method: "POST", Path: "/live/category/update", Description: "修改直播分类资料"},
		{ApiGroup: "直播分类", Method: "POST", Path: "/live/category/status/update", Description: "启用或停用直播分类"},
		{ApiGroup: "直播分类", Method: "POST", Path: "/live/category/delete", Description: "删除直播分类"},
	}
}
