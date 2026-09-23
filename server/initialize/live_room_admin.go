package initialize

import (
	"errors"
	"strconv"

	"tb_live_module/model/system"

	"gorm.io/gorm"
)

// syncLiveRoomAdminMetadata 为存量数据库补齐直播间、直播场次菜单和接口元数据。
// 只有新建菜单时才给超级管理员追加菜单关系，避免覆盖人工分配结果。
func syncLiveRoomAdminMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var parent system.SysBaseMenu
		if err := tx.Where("name = ?", "liveManagement").First(&parent).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			parent = system.SysBaseMenu{
				MenuLevel: 0, ParentId: 0, Path: "liveManagement", Name: "liveManagement",
				Component: "view/routerHolder.vue", Sort: 4,
				Meta: system.Meta{Title: "直播管理", Icon: "video-camera"},
			}
			if err = tx.Create(&parent).Error; err != nil {
				return err
			}
		}

		menus := []system.SysBaseMenu{
			{MenuLevel: 1, ParentId: parent.ID, Path: "room", Name: "liveRoom", Component: "view/live/room/index.vue", Sort: 3, Meta: system.Meta{Title: "直播间管理", Icon: "video-play", KeepAlive: true}},
			{MenuLevel: 1, ParentId: parent.ID, Path: "session", Name: "liveSession", Component: "view/live/session/index.vue", Sort: 4, Meta: system.Meta{Title: "直播场次", Icon: "data-analysis", KeepAlive: true}},
		}
		createdIDs := make([]uint, 0, len(menus))
		for index := range menus {
			menu := menus[index]
			var existing system.SysBaseMenu
			if err := tx.Where("name = ?", menu.Name).First(&existing).Error; err == nil {
				continue
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(&menu).Error; err != nil {
				return err
			}
			createdIDs = append(createdIDs, menu.ID)
		}

		for _, api := range liveRoomAdminAPIs() {
			if err := tx.Where("path = ? AND method = ?", api.Path, api.Method).FirstOrCreate(&api).Error; err != nil {
				return err
			}
		}
		if len(createdIDs) == 0 {
			return nil
		}
		var authority system.SysAuthority
		if err := tx.Where("authority_id = ?", 888).First(&authority).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		createdIDs = append([]uint{parent.ID}, createdIDs...)
		for _, menuID := range createdIDs {
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

func liveRoomAdminAPIs() []system.SysApi {
	return []system.SysApi{
		{ApiGroup: "直播间管理", Method: "GET", Path: "/live/room/list", Description: "分页查询直播间"},
		{ApiGroup: "直播间管理", Method: "GET", Path: "/live/room/detail", Description: "获取直播间后台详情"},
		{ApiGroup: "直播间管理", Method: "POST", Path: "/live/room/update", Description: "修改直播间资料"},
		{ApiGroup: "直播间管理", Method: "POST", Path: "/live/room/status/update", Description: "修改直播间状态"},
		{ApiGroup: "直播场次", Method: "GET", Path: "/live/session/list", Description: "分页查询直播场次"},
		{ApiGroup: "直播场次", Method: "GET", Path: "/live/session/detail", Description: "获取直播场次后台详情"},
		{ApiGroup: "直播场次", Method: "POST", Path: "/live/session/end", Description: "后台强制结束直播场次"},
	}
}
