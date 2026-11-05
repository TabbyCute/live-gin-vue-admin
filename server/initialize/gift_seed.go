package initialize

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	model "tb_live_module/model/gift"
	"time"
)

// SeedGiftCatalog creates editable catalog drafts only. It never changes an existing
// configuration, creates financial facts, enables benefits, or invents a revenue rule.
// Invoke explicitly with the migration CLI after checking the user-authorized target database.
func SeedGiftCatalog(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var control model.Control
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&control, 1).Error; e != nil {
			return e
		}
		now := time.Now().UnixMilli()
		created := 0
		for i, c := range []struct{ no, name string }{{"basic_affection", "基础心意"}, {"basic_support", "热场应援"}, {"basic_premium", "豪华礼遇"}} {
			row := model.Category{CategoryNo: c.no, Name: c.name, Status: "enabled", SortOrder: int32((i + 1) * 10), Version: 1, CreatedAt: now, UpdatedAt: now}
			r := tx.Where("category_no=?", c.no).FirstOrCreate(&row)
			if r.Error != nil {
				return r.Error
			}
			created += int(r.RowsAffected)
		}
		for i, g := range []struct {
			no, category, name string
			price              int64
			premium            bool
		}{
			{"basic_rose", "basic_affection", "玫瑰", 1, false},
			{"basic_heart", "basic_affection", "爱心", 10, false},
			{"basic_cheer", "basic_support", "加油棒", 30, false},
			{"basic_cake", "basic_support", "庆祝蛋糕", 100, false},
			{"basic_rocket", "basic_premium", "火箭", 1000, true},
			{"basic_crown", "basic_premium", "皇冠", 5000, true},
		} {
			row := model.Item{GiftNo: g.no, CategoryNo: g.category, Name: g.name, CoinPrice: g.price, Status: "draft", AnimationType: "none", BenefitType: "none", BenefitStatus: "disabled", IsPremium: g.premium, MaxQuantityPerOrder: 99, SortOrder: int32((i + 1) * 10), Version: 1, CreatedAt: now, UpdatedAt: now, Description: "基础草稿：配置真实HTTPS图片并核定价格后再启用"}
			r := tx.Where("gift_no=?", g.no).FirstOrCreate(&row)
			if r.Error != nil {
				return r.Error
			}
			created += int(r.RowsAffected)
		}
		if created == 0 {
			return nil
		}
		if control.Version == math.MaxInt64 {
			return fmt.Errorf("catalog version exhausted")
		}
		if e := tx.Model(&model.Control{}).Where("id=1").Update("version", gorm.Expr("version+1")).Error; e != nil {
			return e
		}
		b, _ := json.Marshal(map[string]any{"created": created, "catalog": "basic-v1", "giftsStatus": "draft"})
		return tx.Create(&model.Audit{ObjectNo: "basic-v1", Action: "catalog/seed", ActorIP: "local-migration", Snapshot: string(b), CreatedAt: now}).Error
	})
}
