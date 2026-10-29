package initialize

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"tb_live_module/global"
	"tb_live_module/model/system"
	model "tb_live_module/model/wallet"
	svc "tb_live_module/service/wallet"
)

type walletTrigger struct{ name, table, event string }

func walletTriggers() []walletTrigger {
	return []walletTrigger{
		{"trg_wallet_ledger_no_update", "wallet_ledger", "UPDATE"}, {"trg_wallet_ledger_no_delete", "wallet_ledger", "DELETE"},
		{"trg_wallet_audit_no_update", "wallet_audit_log", "UPDATE"}, {"trg_wallet_audit_no_delete", "wallet_audit_log", "DELETE"},
		{"trg_wallet_order_no_delete", "wallet_adjustment_order", "DELETE"}, {"trg_wallet_account_no_delete", "wallet_account", "DELETE"},
		{"trg_wallet_order_core_immutable", "wallet_adjustment_order", "UPDATE"},
	}
}

const walletTriggerBody = "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'wallet record is immutable'"

// 创建事实全部冻结；审核状态、审核人、审核时间及关联流水由工作流更新。
var walletOrderCoreColumns = []string{"id", "order_no", "account_id", "account_no", "currency_code", "amount", "kind", "created_by", "request_id", "request_hash", "original_order_no", "source_type", "external_order_no", "reason", "evidence_ref", "created_ip", "created_at"}

func (t walletTrigger) body(dialect string) string {
	if t.name != "trg_wallet_order_core_immutable" {
		if dialect == "mysql" {
			return walletTriggerBody
		}
		return "BEGIN SELECT RAISE(ABORT, 'wallet record is immutable'); END"
	}
	var changed []string
	for _, col := range walletOrderCoreColumns {
		if dialect == "mysql" {
			// BINARY避免VARCHAR的PAD SPACE排序规则把尾部空格变化视为相等。
			changed = append(changed, fmt.Sprintf("NOT (BINARY OLD.`%s` <=> BINARY NEW.`%s`)", col, col))
		} else {
			changed = append(changed, fmt.Sprintf("OLD.`%s` IS NOT NEW.`%s`", col, col))
		}
	}
	if dialect == "mysql" {
		return "BEGIN IF " + strings.Join(changed, " OR ") + " THEN " + walletTriggerBody + "; END IF; END"
	}
	return "WHEN " + strings.Join(changed, " OR ") + " BEGIN SELECT RAISE(ABORT, 'wallet record is immutable'); END"
}

// MigrateWallet 可供离线迁移执行；必须使用DDL账号，运行账号无需DDL权限。
// SQLite仅用于本地开发和单元测试；生产基线为MySQL8/InnoDB。
func MigrateWallet(db *gorm.DB) error {
	return withWalletMigrationLock(db, migrateWallet)
}

func migrateWallet(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("wallet requires database")
	}
	dialect := db.Dialector.Name()
	if dialect != "mysql" && dialect != "sqlite" {
		return fmt.Errorf("wallet supports MySQL 8 and development SQLite, got %s", dialect)
	}
	migration := db
	if dialect == "mysql" {
		if err := validateWalletMySQLRuntime(db); err != nil {
			return err
		}
		migration = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	// 老版本没有真实凭证号：保留事实但标记legacy，不得冒充已核验充值。
	// 必须在创建唯一索引/检查约束之前补迁；重复执行只处理尚未迁移的行。
	if db.Migrator().HasTable(&model.WalletAdjustmentOrder{}) {
		for _, col := range []string{"SourceType", "ExternalOrderNo"} {
			if !db.Migrator().HasColumn(&model.WalletAdjustmentOrder{}, col) {
				if err := db.Migrator().AddColumn(&model.WalletAdjustmentOrder{}, col); err != nil {
					return err
				}
			}
		}
		if err := db.Exec("UPDATE wallet_adjustment_order SET external_order_no = order_no, source_type = CASE WHEN kind = 'reverse' THEN 'reversal' ELSE 'legacy' END WHERE source_type = 'legacy' AND external_order_no = ''").Error; err != nil {
			return err
		}
	}
	if err := migration.AutoMigrate(model.Models()...); err != nil {
		return fmt.Errorf("migrate wallet: %w", err)
	}
	for _, t := range walletTriggers() {
		if dialect == "sqlite" {
			sql := fmt.Sprintf("CREATE TRIGGER IF NOT EXISTS %s BEFORE %s ON %s %s", t.name, t.event, t.table, t.body(dialect))
			if err := db.Exec(sql).Error; err != nil {
				return err
			}
		} else {
			var count int64
			if err := db.Raw("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = DATABASE() AND TRIGGER_NAME = ?", t.name).Scan(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := db.Exec(fmt.Sprintf("CREATE TRIGGER %s BEFORE %s ON %s FOR EACH ROW %s", t.name, t.event, t.table, t.body(dialect))).Error; err != nil {
					return err
				}
			}
		}
	}
	return ValidateWalletSchema(db)
}

// ValidateWalletSchema 对关闭自动迁移的部署同样失败关闭，不能带着缺失唯一约束运行。
func ValidateWalletSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("wallet requires database")
	}
	// 生产库可能跨网络部署；校验包含列、索引、约束和触发器的完整读取，15秒会在
	// 正常慢链路上误超时。保留总上限防止启动无限挂起，同时让底层错误原样返回。
	ctx, cancel := context.WithTimeout(db.Statement.Context, 60*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	if d := db.Dialector.Name(); d != "mysql" && d != "sqlite" {
		return fmt.Errorf("unsupported wallet database %s", d)
	}
	if db.Dialector.Name() == "mysql" {
		return validateWalletMySQLSchema(db)
	}
	for _, m := range model.Models() {
		st := &gorm.Statement{DB: db}
		if err := st.Parse(m); err != nil {
			return err
		}
		if err := db.Model(m).Select(st.Schema.DBNames).Limit(0).Find(m).Error; err != nil {
			return fmt.Errorf("wallet schema %s: %w", st.Schema.Table, err)
		}
		indexes, err := db.Migrator().GetIndexes(m)
		if err != nil {
			return err
		}
		for _, idx := range st.Schema.ParseIndexes() {
			found := false
			for _, actual := range indexes {
				if actual.Name() == idx.Name {
					unique, known := actual.Unique()
					if idx.Class == "UNIQUE" && (!known || !unique) {
						return fmt.Errorf("wallet index must be unique: %s", idx.Name)
					}
					expected := []string{}
					for _, field := range idx.Fields {
						expected = append(expected, field.Field.DBName)
					}
					if strings.Join(actual.Columns(), ",") != strings.Join(expected, ",") {
						return fmt.Errorf("wallet index columns incompatible: %s", idx.Name)
					}
					found = true
				}
			}
			if !found {
				return fmt.Errorf("wallet index missing: %s", idx.Name)
			}
		}
		if db.Dialector.Name() == "sqlite" {
			checks := st.Schema.ParseCheckConstraints()
			checkNames := make([]string, 0, len(checks))
			for name := range checks {
				checkNames = append(checkNames, name)
			}
			sort.Strings(checkNames)
			for _, name := range checkNames {
				exists, checkErr := walletSQLiteCheckConstraintExists(db, st.Schema.Table, name)
				if checkErr != nil {
					return fmt.Errorf("query wallet check %s.%s: %w", st.Schema.Table, name, checkErr)
				}
				if !exists {
					return fmt.Errorf("wallet check missing: %s", name)
				}
			}
		}
	}
	for _, t := range walletTriggers() {
		var statement string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?", t.name).Scan(&statement).Error; err != nil {
			return err
		}
		expected := fmt.Sprintf("CREATE TRIGGER %s BEFORE %s ON %s %s", t.name, t.event, t.table, t.body("sqlite"))
		if statement != expected {
			return fmt.Errorf("wallet immutable trigger missing: %s", t.name)
		}
	}
	return nil
}

// walletSQLiteCheckConstraintExists 不使用 Migrator.HasConstraint，因为它会吞掉底层
// 查询错误并把超时等故障误报为约束缺失。
func walletSQLiteCheckConstraintExists(db *gorm.DB, table, name string) (bool, error) {
	var count int64
	err := db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND tbl_name=? AND (sql LIKE ? OR sql LIKE ? OR sql LIKE ? OR sql LIKE ? OR sql LIKE ?)",
		table,
		`%CONSTRAINT "`+name+`" %`,
		`%CONSTRAINT `+name+` %`,
		"%CONSTRAINT `"+name+"`%",
		"%CONSTRAINT ["+name+"]%",
		"%CONSTRAINT \t"+name+"\t%",
	).Scan(&count).Error
	return count == 1, err
}

func InitializeWallet() error {
	cfg, err := global.GVA_CONFIG.Wallet.Normalize()
	if err != nil {
		return err
	}
	if err = ValidateWalletSchema(global.GVA_DB); err != nil {
		return err
	}
	svc.Default.DB = global.GVA_DB
	svc.Default.Config = cfg
	svc.Default.Log = global.GVA_LOG
	return nil
}
func syncWalletMetadata(db *gorm.DB) error {
	return withWalletMigrationLock(db, syncWalletMetadataUnlocked)
}

func syncWalletMetadataUnlocked(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var parent system.SysBaseMenu
		parentCreated := false
		if err := tx.Where("name = ?", "walletManagement").First(&parent).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			parent = system.SysBaseMenu{MenuLevel: 0, ParentId: 0, Path: "walletManagement", Name: "walletManagement", Component: "view/routerHolder.vue", Sort: 5, Meta: system.Meta{Title: "钱包管理", Icon: "wallet"}}
			if err = tx.Create(&parent).Error; err != nil {
				return err
			}
			parentCreated = true
		}
		menus := []system.SysBaseMenu{
			{MenuLevel: 1, ParentId: parent.ID, Path: "introduction", Name: "walletIntroduction", Component: "view/wallet/introduction/index.vue", Sort: 0, Meta: system.Meta{Title: "钱包介绍", Icon: "reading", KeepAlive: true}},
			{MenuLevel: 1, ParentId: parent.ID, Path: "account", Name: "walletAccount", Component: "view/wallet/account/index.vue", Sort: 2, Meta: system.Meta{Title: "钱包账户", Icon: "credit-card", KeepAlive: true}},
			{MenuLevel: 1, ParentId: parent.ID, Path: "ledger", Name: "walletLedger", Component: "view/wallet/ledger/index.vue", Sort: 3, Meta: system.Meta{Title: "余额流水", Icon: "tickets", KeepAlive: true}},
			{MenuLevel: 1, ParentId: parent.ID, Path: "recharge", Name: "walletRecharge", Component: "view/wallet/recharge/index.vue", Sort: 4, Meta: system.Meta{Title: "人工充值单", Icon: "coin", KeepAlive: true}},
		}
		createdIDs := make([]uint, 0, len(menus))
		for i := range menus {
			menu := menus[i]
			var existing system.SysBaseMenu
			if err := tx.Where("name = ?", menu.Name).First(&existing).Error; err == nil {
				continue
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(&menu).Error; err != nil {
				return err
			}
			createdIDs = append(createdIDs, menu.ID)
		}
		for _, r := range []struct{ method, path, description string }{
			{"GET", "account/detail", "查询钱包余额"}, {"GET", "account/check", "分批只读账务对账"}, {"POST", "account/status", "停用或启用钱包"},
			{"GET", "ledger/list", "查询余额流水"}, {"POST", "adjustment/create", "创建人工入账单"}, {"POST", "adjustment/approve", "审核并入账"},
			{"POST", "adjustment/reject", "驳回人工入账单"}, {"POST", "adjustment/reverse", "创建冲正申请"}, {"GET", "adjustment/list", "查询人工入账单"}, {"GET", "adjustment/detail", "查看人工入账详情及审计"},
		} {
			row := system.SysApi{ApiGroup: "钱包账务", Method: r.method, Path: "/wallet/" + r.path, Description: r.description}
			if err := tx.Where("method = ? AND path = ?", row.Method, row.Path).FirstOrCreate(&row).Error; err != nil {
				return err
			}
		}
		if !parentCreated && len(createdIDs) == 0 {
			return nil
		}
		var authority system.SysAuthority
		if err := tx.Where("authority_id = ?", 888).First(&authority).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		for _, menuID := range append([]uint{parent.ID}, createdIDs...) {
			relation := system.SysAuthorityMenu{MenuId: strconv.FormatUint(uint64(menuID), 10), AuthorityId: strconv.FormatUint(uint64(authority.AuthorityId), 10)}
			if err := tx.Where("sys_base_menu_id = ? AND sys_authority_authority_id = ?", relation.MenuId, relation.AuthorityId).FirstOrCreate(&relation).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
