package initialize

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	earning "tb_live_module/model/earning"
	model "tb_live_module/model/gift"
	"tb_live_module/model/system"
)

func giftModels() []any {
	return []any{&earning.IncomeLock{}, &earning.Income{}, &model.AdminPermission{}, &model.Control{}, &model.Category{}, &model.Item{}, &model.Order{}, &model.Refund{}, &model.Revenue{}, &model.BenefitGrant{}, &model.BenefitCompensation{}, &model.Event{}, &model.Audit{}, &earning.RevenueRule{}, &earning.Ledger{}, &earning.RuleTermination{}}
}

// MigrateGift only touches gift and pending-earning tables, never wallet DDL/data.
func MigrateGift(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("gift requires database")
	}
	if db.Dialector.Name() != "mysql" && db.Dialector.Name() != "sqlite" {
		return fmt.Errorf("gift requires MySQL8 or development SQLite")
	}
	if db.Dialector.Name() == "mysql" {
		if e := validateWalletMySQLRuntime(db); e != nil {
			return e
		}
	}
	return withGiftMigrationLock(db, func(conn *gorm.DB) error {
		migration := conn
		if conn.Dialector.Name() == "mysql" {
			migration = conn.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
		}
		if e := migration.AutoMigrate(giftModels()...); e != nil {
			return e
		}
		if e := conn.Where("id=1").FirstOrCreate(&model.Control{ID: 1, Version: 1}).Error; e != nil {
			return e
		}

		triggers, e := giftTriggers(conn)
		if e != nil {
			return e
		}
		for _, t := range triggers {
			if conn.Dialector.Name() == "mysql" {
				var count int64
				if e := conn.Raw("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=?", t.name).Scan(&count).Error; e != nil {
					return e
				}
				if count == 0 {
					if e := conn.Exec("CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + " FOR EACH ROW " + t.body).Error; e != nil {
						return e
					}
				}
			} else {
				if e := conn.Exec("CREATE TRIGGER IF NOT EXISTS " + t.name + " BEFORE " + t.event + " ON " + t.table + " " + t.body).Error; e != nil {
					return e
				}
			}
		}

		// Install v2 before removing the known legacy blocker: never leave core facts unprotected.
		if e := upgradeCompensationTrigger(conn); e != nil {
			return e
		}
		if e := upgradeRootReviewTriggers(conn, triggers); e != nil {
			return e
		}
		return ValidateGiftSchema(conn)
	})
}
func SyncGiftMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var control model.Control
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&control, 1).Error; e != nil {
			return e
		}
		var parent system.SysBaseMenu
		if e := tx.Where("name=?", "liveManagement").Take(&parent).Error; e != nil {
			return e
		}
		ids := []uint{parent.ID}
		for i, item := range []struct{ name, title string }{{"category", "礼物分类"}, {"item", "礼物配置"}, {"order", "礼物消费记录"}, {"ranking", "礼物消费排行"}, {"rule", "主播分成规则"}} {
			m := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: "gift-" + item.name, Name: "gift-" + item.name, Component: "view/live/gift/" + item.name + "/index.vue", Sort: 20 + i, Meta: system.Meta{Title: item.title, Icon: "present", KeepAlive: true}}
			if e := tx.Where("name=?", m.Name).FirstOrCreate(&m).Error; e != nil {
				return e
			}
			ids = append(ids, m.ID)
		}
		for _, id := range ids {
			r := system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(id), 10), AuthorityId: "888"}
			if e := tx.Where("sys_base_menu_id=? AND sys_authority_authority_id=?", r.MenuId, r.AuthorityId).FirstOrCreate(&r).Error; e != nil {
				return e
			}
		}
		for _, r := range []struct{ method, path string }{{"GET", "category/list"}, {"POST", "category/save"}, {"GET", "item/list"}, {"POST", "item/save"}, {"POST", "sort"}, {"POST", "delete"}, {"GET", "order/list"}, {"GET", "order/detail"}, {"POST", "refund"}, {"GET", "ranking"}, {"GET", "rule/list"}, {"POST", "rule/create"}, {"POST", "rule/review"}, {"POST", "rule/terminate"}, {"POST", "rule/terminate/review"}} {
			row := system.SysApi{ApiGroup: "直播礼物", Method: r.method, Path: "/gift/" + r.path, Description: r.path}
			if e := tx.Where("method=? AND path=?", row.Method, row.Path).FirstOrCreate(&row).Error; e != nil {
				return e
			}
			// Exact per-operation policies; never grant any non-super-admin role implicitly.
			permission := model.AdminPermission{AuthorityID: 888, Path: row.Path, Method: row.Method}
			if e := tx.Where("authority_id=? AND path=? AND method=?", 888, row.Path, row.Method).FirstOrCreate(&permission).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
