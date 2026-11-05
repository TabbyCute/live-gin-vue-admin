package system

import (
	"context"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tb_live_module/global"
	model "tb_live_module/model/system"
	req "tb_live_module/model/system/request"
	"testing"
)

func TestRootMenusAndStrictRoleScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/root.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	previousDB, previousStrict := global.GVA_DB, global.GVA_CONFIG.System.UseStrictAuth
	global.GVA_DB, global.GVA_CONFIG.System.UseStrictAuth = db, true
	t.Cleanup(func() { global.GVA_DB, global.GVA_CONFIG.System.UseStrictAuth = previousDB, previousStrict })
	require.NoError(t, db.AutoMigrate(&model.SysAuthority{}, &model.SysBaseMenu{}, &model.SysBaseMenuBtn{}, &model.SysBaseMenuParameter{}, &model.SysAuthorityMenu{}, &model.SysAIWorkflowSession{}))
	parent := uint(0)
	role := model.SysAuthority{AuthorityId: 999, ParentId: &parent}
	require.NoError(t, db.Create(&role).Error)
	dashboard := model.SysBaseMenu{Name: "dashboard", MenuBtn: []model.SysBaseMenuBtn{{Name: "create"}}}
	require.NoError(t, db.Create(&dashboard).Error)
	menus, err := MenuServiceApp.GetMenuTree(0, 1)
	require.NoError(t, err)
	require.Len(t, menus, 1)
	require.NotZero(t, menus[0].Btns["create"])
	// New menus appear without assigning them to root's role.
	require.NoError(t, db.Create(&model.SysBaseMenu{Name: "new-feature"}).Error)
	menus, err = MenuServiceApp.GetMenuTree(999, 1)
	require.NoError(t, err)
	require.Len(t, menus, 2)
	menus, err = MenuServiceApp.GetMenuTree(999, 2)
	require.NoError(t, err)
	require.Empty(t, menus)
	base, err := MenuServiceApp.GetBaseMenuTree(0, 1)
	require.NoError(t, err)
	require.Len(t, base, 2)
	roles, err := AuthorityServiceApp.GetAuthorityInfoList(0, 1)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	require.NoError(t, AuthorityServiceApp.CheckAuthorityIDAuth(0, 999, 1))
	require.Error(t, AuthorityServiceApp.CheckAuthorityIDAuth(999, 888, 2))
	require.NoError(t, MenuServiceApp.AddMenuAuthority([]model.SysBaseMenu{dashboard}, 0, 999, 1))
	root := model.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 1}}
	MenuServiceApp.UserAuthorityDefaultRouter(&root)
	require.Equal(t, "dashboard", root.Authority.DefaultRouter)
}

func TestRootWorkflowAccessPreservesOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/workflow.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	oldDB := global.GVA_DB
	global.GVA_DB = db
	t.Cleanup(func() { global.GVA_DB = oldDB })
	require.NoError(t, db.AutoMigrate(&model.SysAIWorkflowSession{}))
	ctx := context.Background()
	row, err := AIWorkflowSessionServiceApp.Save(ctx, 2, req.SysAIWorkflowSessionUpsert{Tab: "analysis", Title: "owner"})
	require.NoError(t, err)
	_, err = AIWorkflowSessionServiceApp.GetDetail(ctx, 3, row.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	row, err = AIWorkflowSessionServiceApp.Save(ctx, 1, req.SysAIWorkflowSessionUpsert{ID: row.ID, Tab: "analysis", Title: "root update"})
	require.NoError(t, err)
	require.EqualValues(t, 2, row.UserID)
	_, err = AIWorkflowSessionServiceApp.GetDetail(ctx, 2, row.ID)
	require.NoError(t, err)
	require.NoError(t, AIWorkflowSessionServiceApp.Delete(ctx, 1, row.ID))
}
