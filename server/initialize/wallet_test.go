package initialize

import (
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/model/system"
	model "tb_live_module/model/wallet"
	"testing"
)

func TestWalletMigrationAndDatabaseImmutability(t *testing.T) {
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/wallet.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, _ := db.DB()
	defer pool.Close()
	require.NoError(t, MigrateWallet(db))
	require.NoError(t, MigrateWallet(db))
	require.NoError(t, ValidateWalletSchema(db))
	w := model.WalletAccount{AccountID: 1, CurrencyCode: "COIN", Status: "normal"}
	require.NoError(t, db.Create(&w).Error)
	l := model.WalletLedger{LedgerNo: "test", WalletAccountID: w.ID, WalletVersion: 1, ChangeAmount: 1, BalanceAfter: 1, BusinessType: "ADMIN_GRANT", BusinessNo: "order", OperatorID: 1, Reason: "test"}
	require.NoError(t, db.Create(&l).Error)
	a := model.WalletAuditLog{ObjectNo: "order", Action: "create", OperatorID: 1, OperatorIP: "127.0.0.1", Reason: "test"}
	require.NoError(t, db.Create(&a).Error)
	o := model.WalletAdjustmentOrder{OrderNo: "order", SourceType: model.SourceManual, ExternalOrderNo: "BANK-1", RequestID: "request", RequestHash: "hash", Kind: "grant", AccountID: 1, AccountNo: "10000001", CurrencyCode: "COIN", Amount: 1, Reason: "test", Status: "pending", CreatedBy: 1, CreatedIP: "127.0.0.1"}
	require.NoError(t, db.Create(&o).Error)
	for _, col := range walletOrderCoreColumns {
		value := any("changed")
		switch col {
		case "id", "account_id", "amount", "created_by", "created_at":
			value = 99
		}
		require.ErrorContains(t, db.Exec(fmt.Sprintf("UPDATE wallet_adjustment_order SET %s=? WHERE id=?", col), value, o.ID).Error, "wallet record is immutable", col)
	}
	require.ErrorContains(t, db.Exec("UPDATE wallet_adjustment_order SET evidence_ref=' '").Error, "wallet record is immutable")
	require.NoError(t, db.Exec("UPDATE wallet_adjustment_order SET amount=amount, status='rejected', rejected_by=2").Error)
	for _, sql := range []string{"UPDATE wallet_ledger SET reason='modified'", "DELETE FROM wallet_ledger", "UPDATE wallet_audit_log SET reason='modified'", "DELETE FROM wallet_audit_log", "DELETE FROM wallet_account", "DELETE FROM wallet_adjustment_order"} {
		require.Error(t, db.Exec(sql).Error, sql)
	}
	require.Error(t, db.Exec("UPDATE wallet_account SET available_balance=-1").Error)
	require.Error(t, db.Exec("INSERT INTO wallet_account(account_id,currency_code,status,created_at,updated_at) VALUES (1,'COIN','normal',0,0)").Error)
	require.NoError(t, db.Exec("DROP INDEX uk_wallet_ledger_business").Error)
	require.ErrorContains(t, ValidateWalletSchema(db), "uk_wallet_ledger_business")
	require.NoError(t, MigrateWallet(db))
	require.NoError(t, db.Exec("DROP TRIGGER trg_wallet_ledger_no_update").Error)
	require.ErrorContains(t, ValidateWalletSchema(db), "trg_wallet_ledger_no_update")
}

func TestSyncWalletMetadataCreatesMenusAndIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/wallet-menu.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&system.SysApi{}, &system.SysBaseMenu{}, &system.SysAuthority{}, &system.SysAuthorityMenu{}))
	require.NoError(t, db.Create(&system.SysAuthority{AuthorityId: 888, AuthorityName: "超级管理员"}).Error)
	require.NoError(t, syncWalletMetadata(db))
	require.NoError(t, syncWalletMetadata(db))

	var parent system.SysBaseMenu
	require.NoError(t, db.Where("name = ?", "walletManagement").Take(&parent).Error)
	require.Equal(t, "view/routerHolder.vue", parent.Component)
	var children []system.SysBaseMenu
	require.NoError(t, db.Where("parent_id = ?", parent.ID).Order("sort").Find(&children).Error)
	require.Len(t, children, 4)
	require.Equal(t, []string{"walletIntroduction", "walletAccount", "walletLedger", "walletRecharge"}, []string{children[0].Name, children[1].Name, children[2].Name, children[3].Name})
	require.Equal(t, []string{"view/wallet/introduction/index.vue", "view/wallet/account/index.vue", "view/wallet/ledger/index.vue", "view/wallet/recharge/index.vue"}, []string{children[0].Component, children[1].Component, children[2].Component, children[3].Component})

	var menuCount, apiCount, relationCount int64
	require.NoError(t, db.Model(&system.SysBaseMenu{}).Where("name IN ?", []string{"walletManagement", "walletIntroduction", "walletAccount", "walletLedger", "walletRecharge"}).Count(&menuCount).Error)
	require.NoError(t, db.Model(&system.SysApi{}).Where("api_group = ?", "钱包账务").Count(&apiCount).Error)
	require.NoError(t, db.Model(&system.SysAuthorityMenu{}).Where("sys_authority_authority_id = ?", "888").Count(&relationCount).Error)
	require.EqualValues(t, 5, menuCount)
	require.EqualValues(t, 10, apiCount)
	require.EqualValues(t, 5, relationCount)

	// 菜单已经存在后不得在每次启动重新补权限，避免覆盖运营侧主动收回的访问权。
	require.NoError(t, db.Where("sys_base_menu_id = ?", children[0].ID).Delete(&system.SysAuthorityMenu{}).Error)
	require.NoError(t, syncWalletMetadata(db))
	require.NoError(t, db.Model(&system.SysAuthorityMenu{}).Where("sys_authority_authority_id = ?", "888").Count(&relationCount).Error)
	require.EqualValues(t, 4, relationCount)

	// 模拟旧版本数据库尚无“钱包介绍”：缺失菜单应在升级时创建并给默认888追加关系。
	require.NoError(t, db.Delete(&system.SysBaseMenu{}, children[0].ID).Error)
	require.NoError(t, syncWalletMetadata(db))
	var introduction system.SysBaseMenu
	require.NoError(t, db.Where("name = ?", "walletIntroduction").Take(&introduction).Error)
	require.EqualValues(t, parent.ID, introduction.ParentId)
	require.NoError(t, db.Model(&system.SysAuthorityMenu{}).Where("sys_authority_authority_id = ?", "888").Count(&relationCount).Error)
	require.EqualValues(t, 5, relationCount)
}

func TestWalletMySQLVersionAndSQLMode(t *testing.T) {
	for _, v := range []string{"8.0.16", "8.0.46", "8.4.6", "8.0.46-0ubuntu0.24.04.2"} {
		require.NoError(t, checkWalletMySQLVersion(v, "ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION"), v)
	}
	for _, v := range []string{"5.7.44", "8.0.15", "9.0.1", "10.11.2-MariaDB", "8.0.46-MariaDB", "8.0", "8.0.16garbage"} {
		require.Error(t, checkWalletMySQLVersion(v, "STRICT_TRANS_TABLES"), v)
	}
	require.Error(t, checkWalletMySQLVersion("8.0.46", "ONLY_FULL_GROUP_BY"))
	require.NoError(t, checkWalletMySQLVersion("8.4.6", "STRICT_ALL_TABLES"))
}
