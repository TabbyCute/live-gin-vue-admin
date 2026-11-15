package initialize

import (
	"context"
	"gorm.io/gorm"
	"strconv"
	"tb_live_module/internal/eventcapture"
	model "tb_live_module/model/eventdelivery"
	"tb_live_module/model/system"
	svc "tb_live_module/service/eventdelivery"
)

func MigrateEventDelivery(db *gorm.DB) error        { return svc.Migrate(db) }
func ValidateEventDeliverySchema(db *gorm.DB) error { return svc.ValidateSchema(db) }

// InitializeEventDelivery does no DDL. Call only when enabled, after database setup,
// before accepting requests. No real adapter is installed by default.
func InitializeEventDelivery(db *gorm.DB, adapter svc.Adapter) error {
	if err := ValidateEventDeliverySchema(db); err != nil {
		return err
	}
	svc.Default.DB = db
	svc.Default.Adapter = adapter
	eventcapture.Install(svc.Default)
	return nil
}
func StartEventDeliveryWorker(ctx context.Context) <-chan struct{} {
	return svc.Default.StartWorker(ctx)
}
func SyncEventDeliveryMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		menu := system.SysBaseMenu{Path: "event-delivery", Name: "event-delivery", Component: "view/operations/event-delivery/index.vue", Sort: 95, Meta: system.Meta{Title: "事件投递", Icon: "bell", KeepAlive: true}}
		if err := tx.Where("name=?", menu.Name).FirstOrCreate(&menu).Error; err != nil {
			return err
		}
		// Menu visibility is a convenience; all operations independently require enabled root ID 1.
		grant := system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(menu.ID), 10), AuthorityId: "888"}
		if err := tx.Where("sys_base_menu_id=? AND sys_authority_authority_id=?", grant.MenuId, grant.AuthorityId).FirstOrCreate(&grant).Error; err != nil {
			return err
		}
		for _, r := range []struct{ method, path string }{{"GET", "list"}, {"GET", "detail"}, {"POST", "retry"}} {
			api := system.SysApi{ApiGroup: "事件投递", Method: r.method, Path: "/event-delivery/" + r.path, Description: "root事件投递"}
			if err := tx.Where("method=? AND path=?", api.Method, api.Path).FirstOrCreate(&api).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// EventDeliveryModels is provided to migration tooling, not broad startup AutoMigrate.
func EventDeliveryModels() []any { return model.Models() }
