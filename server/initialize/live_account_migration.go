package initialize

import (
	"errors"
	"strings"

	"tb_live_module/model/live"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// prepareLiveAccountAutoMigration 在GORM创建account_no唯一索引前回填存量账号，
// 避免多行空值导致唯一索引创建失败。生产环境关闭AutoMigrate时应执行配套SQL迁移。
func prepareLiveAccountAutoMigration(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&live.LiveAccount{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&live.LiveAccount{}, "AccountNo") {
		if err := addNullableLiveAccountNoColumn(db); err != nil {
			return err
		}
	}
	var accounts []live.LiveAccount
	if err := db.Unscoped().Select("id", "account_no").Where("account_no IS NULL OR account_no = ''").Find(&accounts).Error; err != nil {
		return err
	}
	for i := range accounts {
		for attempt := 0; attempt < 3; attempt++ {
			accountNo := newLegacyLiveAccountNo()
			result := db.Unscoped().Model(&live.LiveAccount{}).Where("id = ?", accounts[i].ID).UpdateColumn("account_no", accountNo)
			if result.Error == nil {
				break
			}
			if attempt == 2 {
				return result.Error
			}
			var duplicate live.LiveAccount
			if err := db.Unscoped().Select("id").Where("account_no = ?", accountNo).First(&duplicate).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return result.Error
			}
		}
	}
	return nil
}

func newLegacyLiveAccountNo() string {
	raw := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return "U" + raw[:20]
}

func ensureLiveAccountNumberSequence(db *gorm.DB) error {
	if db == nil {
		return errors.New("数据库未初始化")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		sequence := live.LiveAccountNumberSequence{ID: 1, NextValue: live.LiveAccountNumberStart}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&sequence).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sequence, 1).Error; err != nil {
			return err
		}
		if sequence.NextValue < live.LiveAccountNumberStart {
			return tx.Model(&sequence).UpdateColumn("next_value", live.LiveAccountNumberStart).Error
		}
		return nil
	})
}

// 先以可空列加入，回填完成后再由AutoMigrate施加NOT NULL和唯一索引。
// 这样可避免SQLite和严格模式MySQL在存量行上直接新增NOT NULL列失败。
func addNullableLiveAccountNoColumn(db *gorm.DB) error {
	var statement string
	switch db.Dialector.Name() {
	case "mysql":
		statement = "ALTER TABLE `live_account` ADD COLUMN `account_no` varchar(32) NULL"
	case "postgres":
		statement = `ALTER TABLE "live_account" ADD COLUMN "account_no" varchar(32)`
	case "sqlite":
		statement = `ALTER TABLE "live_account" ADD COLUMN "account_no" varchar(32)`
	case "sqlserver":
		statement = "ALTER TABLE [live_account] ADD [account_no] varchar(32) NULL"
	default:
		return db.Migrator().AddColumn(&live.LiveAccount{}, "AccountNo")
	}
	return db.Exec(statement).Error
}

func ensureLiveAccountPerformanceIndexes(db *gorm.DB) error {
	const indexName = "idx_live_account_created_at"
	if db == nil || db.Migrator().HasIndex(&live.LiveAccount{}, indexName) {
		return nil
	}
	var statement string
	switch db.Dialector.Name() {
	case "mysql":
		statement = "CREATE INDEX `" + indexName + "` ON `live_account` (`created_at`)"
	case "sqlserver":
		statement = "CREATE INDEX [" + indexName + "] ON [live_account] ([created_at])"
	default:
		statement = `CREATE INDEX "` + indexName + `" ON "live_account" ("created_at")`
	}
	return db.Exec(statement).Error
}
