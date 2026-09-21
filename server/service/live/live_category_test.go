package live

import (
	"testing"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCategoryTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveCategory{}, &liveModel.LiveAnchor{}))
	previousDB := global.GVA_DB
	global.GVA_DB = db
	t.Cleanup(func() { global.GVA_DB = previousDB })
}

func categoryStatusPtr(value uint8) *uint8 { return &value }

func createTestCategory(t *testing.T, service *CategoryService, parentID uint, code, name string, status uint8, sort int32) liveModel.LiveCategory {
	t.Helper()
	item, err := service.CreateCategory(liveReq.LiveCategoryCreateReq{
		ParentId: parentID, Code: code, Name: name, Sort: sort, Status: categoryStatusPtr(status),
	})
	require.NoError(t, err)
	var category liveModel.LiveCategory
	require.NoError(t, global.GVA_DB.First(&category, item.ID).Error)
	return category
}

func TestLiveCategoryCRUDTreeAndList(t *testing.T) {
	setupCategoryTestDB(t)
	service := CategoryService{}
	root := createTestCategory(t, &service, 0, "music", "音乐", liveModel.LiveCategoryStatusEnabled, 100)
	child := createTestCategory(t, &service, root.ID, "singing", "唱歌", liveModel.LiveCategoryStatusEnabled, 80)
	grandchild := createTestCategory(t, &service, child.ID, "folk", "民谣", liveModel.LiveCategoryStatusEnabled, 60)

	publicTree, err := service.GetPublicCategoryTree()
	require.NoError(t, err)
	require.Len(t, publicTree, 1)
	require.Equal(t, root.ID, publicTree[0].ID)
	require.Len(t, publicTree[0].Children, 1)
	require.Equal(t, child.ID, publicTree[0].Children[0].ID)
	require.Len(t, publicTree[0].Children[0].Children, 1, "分类树必须保留三层及更深层级")
	require.Equal(t, grandchild.ID, publicTree[0].Children[0].Children[0].ID)

	listReq := liveReq.LiveCategoryAdminListReq{}
	listReq.Page = 1
	listReq.PageSize = 20
	listReq.ParentId = &root.ID
	list, total, err := service.GetAdminCategoryList(&listReq)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, root.Name, list[0].ParentName)

	_, err = service.CreateCategory(liveReq.LiveCategoryCreateReq{
		Code: "MUSIC", Name: "其他音乐", Status: categoryStatusPtr(liveModel.LiveCategoryStatusEnabled),
	})
	require.ErrorIs(t, err, ErrLiveCategoryCodeExists)
	_, err = service.CreateCategory(liveReq.LiveCategoryCreateReq{
		Code: "music-2", Name: "音乐", Status: categoryStatusPtr(liveModel.LiveCategoryStatusEnabled),
	})
	require.ErrorIs(t, err, ErrLiveCategoryNameExists)

	_, err = service.UpdateCategory(liveReq.LiveCategoryUpdateReq{
		ID: root.ID, ParentId: grandchild.ID, Name: root.Name, Sort: root.Sort,
	})
	require.ErrorIs(t, err, ErrLiveCategoryCycle)
}

func TestLiveCategoryStatusCascadeAndDeleteGuards(t *testing.T) {
	setupCategoryTestDB(t)
	service := CategoryService{}
	root := createTestCategory(t, &service, 0, "game", "游戏", liveModel.LiveCategoryStatusEnabled, 100)
	child := createTestCategory(t, &service, root.ID, "mobile-game", "手游", liveModel.LiveCategoryStatusEnabled, 80)
	leaf := createTestCategory(t, &service, child.ID, "moba", "MOBA", liveModel.LiveCategoryStatusEnabled, 60)

	require.NoError(t, service.UpdateCategoryStatus(liveReq.LiveCategoryStatusUpdateReq{
		ID: root.ID, Status: categoryStatusPtr(liveModel.LiveCategoryStatusDisabled),
	}))
	var disabledCount int64
	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveCategory{}).
		Where("id IN ? AND status = ?", []uint{root.ID, child.ID, leaf.ID}, liveModel.LiveCategoryStatusDisabled).
		Count(&disabledCount).Error)
	require.Equal(t, int64(3), disabledCount)

	err := service.UpdateCategoryStatus(liveReq.LiveCategoryStatusUpdateReq{
		ID: leaf.ID, Status: categoryStatusPtr(liveModel.LiveCategoryStatusEnabled),
	})
	require.ErrorIs(t, err, ErrLiveCategoryParentDisabled)
	require.ErrorIs(t, service.DeleteCategory(root.ID), ErrLiveCategoryHasChildren)

	anchor := liveModel.LiveAnchor{UserId: 99, AnchorNo: "1100099", Nickname: "引用分类的主播", CategoryId: uint64(leaf.ID)}
	require.NoError(t, global.GVA_DB.Create(&anchor).Error)
	require.ErrorIs(t, service.DeleteCategory(leaf.ID), ErrLiveCategoryInUse)

	require.NoError(t, global.GVA_DB.Model(&anchor).Update("category_id", 0).Error)
	require.NoError(t, service.DeleteCategory(leaf.ID))
	require.NoError(t, service.DeleteCategory(child.ID))
	require.NoError(t, service.DeleteCategory(root.ID))
}

func TestLiveCategoryValidationForAnchorCategoryID(t *testing.T) {
	setupCategoryTestDB(t)
	service := CategoryService{}
	require.NoError(t, service.EnsureEnabledCategory(0))
	require.ErrorIs(t, service.EnsureEnabledCategory(999), ErrLiveCategoryNotFound)

	enabled := createTestCategory(t, &service, 0, "talk", "聊天", liveModel.LiveCategoryStatusEnabled, 10)
	disabled := createTestCategory(t, &service, 0, "hidden", "隐藏", liveModel.LiveCategoryStatusDisabled, 0)
	require.NoError(t, service.EnsureEnabledCategory(uint64(enabled.ID)))
	require.ErrorIs(t, service.EnsureEnabledCategory(uint64(disabled.ID)), ErrLiveCategoryNotFound)

	anchorService := AnchorService{}
	_, err := anchorService.ApplyAnchor(101, liveReq.AnchorApplyReq{
		Nickname: "Invalid Category", ChannelId: 1, CategoryId: uint64(disabled.ID),
	})
	require.ErrorIs(t, err, ErrLiveCategoryNotFound)
	anchor, err := anchorService.ApplyAnchor(101, liveReq.AnchorApplyReq{
		Nickname: "Valid Category", ChannelId: 1, CategoryId: uint64(enabled.ID),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(enabled.ID), anchor.CategoryId)
}
