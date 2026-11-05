package initialize

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	gm "tb_live_module/model/guard"
	rm "tb_live_module/model/ranking"
	"tb_live_module/model/system"
)

func SyncEngagementMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var control gm.Task
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_no='guard-admin-control'").Take(&control).Error; e != nil {
			return e
		}
		var parent system.SysBaseMenu
		if e := tx.Where("name='liveManagement'").Take(&parent).Error; e != nil {
			return e
		}
		for i, item := range []struct{ name, title, component string }{{"guard-plan", "守护方案", "guard/plan"}, {"guard-member", "守护会员", "guard/member"}, {"guard-ledger", "守护权益流水", "guard/ledger"}, {"guard-task", "守护异常任务", "guard/task"}, {"live-ranking", "直播榜单", "ranking"}} {
			m := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: item.name, Name: item.name, Component: "view/live/" + item.component + "/index.vue", Sort: 40 + i, Meta: system.Meta{Title: item.title, Icon: "medal", KeepAlive: true}}
			if e := tx.Where("name=?", m.Name).FirstOrCreate(&m).Error; e != nil {
				return e
			}
			r := system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(m.ID), 10), AuthorityId: "888"}
			if e := tx.Where("sys_base_menu_id=? AND sys_authority_authority_id=?", r.MenuId, r.AuthorityId).FirstOrCreate(&r).Error; e != nil {
				return e
			}
		}
		for _, op := range []struct{ path, method string }{{"/guard/plan/list", "GET"}, {"/guard/plan/save", "POST"}, {"/guard/member/list", "GET"}, {"/guard/member/detail", "GET"}, {"/guard/grant/list", "GET"}, {"/guard/compensation/list", "GET"}, {"/guard/ledger/list", "GET"}, {"/guard/task/list", "GET"}, {"/guard/task/retry", "POST"}, {"/guard/rebuild", "POST"}, {"/guard/reconcile", "POST"}} {
			api := system.SysApi{ApiGroup: "主播守护", Path: op.path, Method: op.method, Description: op.path}
			if e := tx.Where("path=? AND method=?", op.path, op.method).FirstOrCreate(&api).Error; e != nil {
				return e
			}
			p := gm.AdminPermission{AuthorityID: 888, Path: op.path, Method: op.method}
			if e := tx.Where("authority_id=? AND path=? AND method=?", 888, op.path, op.method).FirstOrCreate(&p).Error; e != nil {
				return e
			}
		}
		for _, op := range []struct{ path, method string }{{"/ranking/admin/board", "GET"}, {"/ranking/admin/status", "GET"}, {"/ranking/admin/job/list", "GET"}, {"/ranking/admin/rebuild", "POST"}, {"/ranking/admin/reconcile", "POST"}} {
			api := system.SysApi{ApiGroup: "直播榜单", Path: op.path, Method: op.method, Description: op.path}
			if e := tx.Where("path=? AND method=?", op.path, op.method).FirstOrCreate(&api).Error; e != nil {
				return e
			}
			p := rm.AdminPermission{AuthorityID: 888, Path: op.path, Method: op.method}
			if e := tx.Where("authority_id=? AND path=? AND method=?", 888, op.path, op.method).FirstOrCreate(&p).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
