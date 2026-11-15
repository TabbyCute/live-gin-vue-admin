package initialize

import (
	"fmt"
	"strconv"

	"gorm.io/gorm"
	"tb_live_module/model/system"
)

const AnchorHomeMenuName = "anchor-home-album"

// SyncAnchorHomeMetadata adds only this module's menu, default 888 menu binding,
// and API catalog entries. API authorization independently checks live DB roles.
// Existing rows are never repaired, updated, deleted, or granted broader access.
func SyncAnchorHomeMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var parent system.SysBaseMenu
		if err := tx.Where("name=?", "liveManagement").Take(&parent).Error; err != nil {
			return err
		}
		var matches []system.SysBaseMenu
		if err := tx.Unscoped().Where("name=? OR path=? OR component=?", AnchorHomeMenuName, AnchorHomeMenuName, "view/live/anchor-home/index.vue").Find(&matches).Error; err != nil {
			return err
		}
		menu := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: AnchorHomeMenuName, Name: AnchorHomeMenuName, Component: "view/live/anchor-home/index.vue", Sort: 32, Meta: system.Meta{Title: "主播主页相册", Icon: "picture", KeepAlive: true}}
		if len(matches) > 1 {
			return fmt.Errorf("anchor_home_metadata_conflict")
		}
		if len(matches) == 1 {
			menu = matches[0]
			if menu.DeletedAt.Valid || menu.Name != AnchorHomeMenuName || menu.Path != AnchorHomeMenuName || menu.Component != "view/live/anchor-home/index.vue" || menu.ParentId != parent.ID {
				return fmt.Errorf("anchor_home_metadata_conflict")
			}
		} else if err := tx.Omit("SysAuthoritys", "Parameters", "MenuBtn").Create(&menu).Error; err != nil {
			return err
		}
		grant := system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(menu.ID), 10), AuthorityId: "888"}
		if err := tx.Where("sys_base_menu_id=? AND sys_authority_authority_id=?", grant.MenuId, grant.AuthorityId).FirstOrCreate(&grant).Error; err != nil {
			return err
		}
		for _, op := range anchorHomeAPIs() {
			var entries []system.SysApi
			if err := tx.Unscoped().Where("path=? AND method=?", op.Path, op.Method).Find(&entries).Error; err != nil {
				return err
			}
			if len(entries) > 1 || len(entries) == 1 && entries[0].DeletedAt.Valid {
				return fmt.Errorf("anchor_home_metadata_conflict")
			}
			if len(entries) == 0 {
				if err := tx.Create(&op).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func anchorHomeAPIs() []system.SysApi {
	return []system.SysApi{
		{Path: "/v1/admin/anchor-home/album/list", Method: "POST", ApiGroup: "主播主页相册", Description: "分页查询主页照片"},
		{Path: "/v1/admin/anchor-home/album/preview", Method: "GET", ApiGroup: "主播主页相册", Description: "鉴权预览主页照片"},
		{Path: "/v1/admin/anchor-home/album/review", Method: "POST", ApiGroup: "主播主页相册", Description: "审核或下架主页照片"},
	}
}
