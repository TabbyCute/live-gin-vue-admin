package initialize

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	em "tb_live_module/model/earning"
	"tb_live_module/model/system"
	ps "tb_live_module/service/payout"
)

// Preserve the prior phase's three role-888 permissions. New manual payment
// capabilities are registered, never implicitly granted to any role. ID 1
// retains its existing bypass; every other operator needs explicit grants.
func SyncPayoutMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&em.FinanceControl{}, 1).Error; e != nil {
			return e
		}
		parent := system.SysBaseMenu{ParentId: 0, Path: "payoutCenter", Name: "payoutCenter", Component: "view/routerHolder.vue", Sort: 30, Meta: system.Meta{Title: "打款中心", Icon: "money"}}
		if e := tx.Where("name=?", parent.Name).FirstOrCreate(&parent).Error; e != nil {
			return e
		}
		if e := tx.Model(&parent).Updates(map[string]any{"parent_id": 0, "path": "payoutCenter", "component": "view/routerHolder.vue", "title": "打款中心"}).Error; e != nil {
			return e
		}
		child := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: "payoutOrders", Name: "payoutOrders", Component: "view/payout/order/index.vue", Sort: 1, Meta: system.Meta{Title: "出款订单", Icon: "tickets", KeepAlive: true}}
		if e := tx.Where("name=?", child.Name).FirstOrCreate(&child).Error; e != nil {
			return e
		}
		if e := tx.Model(&child).Updates(map[string]any{"parent_id": parent.ID, "path": "payoutOrders", "component": "view/payout/order/index.vue", "title": "出款订单"}).Error; e != nil {
			return e
		}
		company := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: "payoutCompanyAccounts", Name: "payoutCompanyAccounts", Component: "view/payout/company/index.vue", Sort: 2, Meta: system.Meta{Title: "公司出款账户", Icon: "credit-card"}}
		if e := tx.Where("name=?", company.Name).FirstOrCreate(&company).Error; e != nil {
			return e
		}
		for _, id := range []uint{parent.ID, child.ID, company.ID} {
			m := system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(id), 10), AuthorityId: "888"}
			if e := tx.Where("sys_base_menu_id=? AND sys_authority_authority_id=?", m.MenuId, m.AuthorityId).FirstOrCreate(&m).Error; e != nil {
				return e
			}
		}
		for _, op := range ps.ManualOperations {
			api := system.SysApi{Path: "/payout/" + op.Path, Method: op.Method, ApiGroup: "打款中心", Description: op.Description}
			if e := tx.Where("path=? AND method=?", api.Path, api.Method).FirstOrCreate(&api).Error; e != nil {
				return e
			}
		}
		for _, op := range []struct{ path, method, description string }{{"create", "POST", "创建待出款订单"}, {"list", "GET", "查询出款订单"}, {"detail", "GET", "查看出款订单详情"}} {
			api := system.SysApi{Path: "/payout/order/" + op.path, Method: op.method, ApiGroup: "打款中心", Description: op.description}
			if e := tx.Where("path=? AND method=?", api.Path, api.Method).FirstOrCreate(&api).Error; e != nil {
				return e
			}
			p := em.AdminPermission{AuthorityID: 888, Path: api.Path, Method: api.Method}
			if e := tx.Where("authority_id=? AND path=? AND method=?", p.AuthorityID, p.Path, p.Method).FirstOrCreate(&p).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
