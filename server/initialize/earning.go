package initialize

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"tb_live_module/global"
	model "tb_live_module/model/earning"
	gm "tb_live_module/model/gift"
	"tb_live_module/model/system"
	es "tb_live_module/service/earning"
	gs "tb_live_module/service/gift"
	ps "tb_live_module/service/payout"
	"time"
)

func earningModels() []any {
	return []any{&model.RoundingBalance{}, &model.IncomeLock{}, &model.Income{}, &model.FinanceControl{}, &model.SettlementRule{}, &model.Settlement{}, &model.SettlementItem{}, &model.Allocation{}, &model.Account{}, &model.AccountLedger{}, &model.Withdrawal{}, &model.Reconciliation{}, &model.FinanceAudit{}, &model.AdminPermission{}}
}
func MigrateEarning(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("earning requires database")
	}
	if db.Dialector.Name() != "mysql" && db.Dialector.Name() != "sqlite" {
		return fmt.Errorf("earning requires MySQL8 or development SQLite")
	}
	return withGiftMigrationLock(db, func(conn *gorm.DB) error {
		target := conn
		if conn.Dialector.Name() == "mysql" {
			if e := validateWalletMySQLRuntime(conn); e != nil {
				return e
			}
			target = conn.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
		}
		if e := prepareManualChecks(conn, true); e != nil {
			return e
		}
		if e := target.AutoMigrate(earningModels()...); e != nil {
			return e
		}
		if e := conn.Where("id=1").FirstOrCreate(&model.FinanceControl{ID: 1}).Error; e != nil {
			return e
		}
		triggers, e := earningTriggers(conn)
		if e != nil {
			return e
		}
		for _, t := range triggers {
			if conn.Dialector.Name() == "mysql" {
				var n int64
				if e := conn.Raw("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=?", t.name).Scan(&n).Error; e != nil {
					return e
				}
				if n == 0 {
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
		if e := upgradeRootReviewTriggers(conn, triggers); e != nil {
			return e
		}
		if e := upgradeManualEarningTriggers(conn); e != nil {
			return e
		}
		return ValidateEarningSchema(conn)
	})
}

// Backfill is explicit and restartable. It preserves old financial facts and only mirrors them by permanent source key.
func BackfillGiftIncome(db *gorm.DB) error {
	var after uint64
	for {
		var facts []model.Ledger
		if e := db.Where("id>?", after).Order("id ASC").Limit(100).Find(&facts).Error; e != nil {
			return e
		}
		if len(facts) == 0 {
			return nil
		}
		for _, f := range facts {
			if e := db.Transaction(func(tx *gorm.DB) error {
				var revenue gm.Revenue
				if e := tx.Where("business_no=? AND gift_order_no=?", f.BusinessNo, f.GiftOrderNo).Take(&revenue).Error; e != nil {
					return e
				}
				income, e := es.RecordGiftIncomeInTx(tx, f, revenue.GrossCoinAmount)
				if e != nil {
					return e
				}
				return gs.Default.VerifyIncomeSources(tx, []model.Income{income})
			}); e != nil {
				return e
			}
			after = f.ID
		}
	}
}
func earningTriggers(db *gorm.DB) ([]giftTrigger, error) {
	return earningTriggersVersion(db, true)
}
func earningTriggersVersion(db *gorm.DB, manual bool) ([]giftTrigger, error) {
	all := []giftTrigger{}
	for _, m := range earningModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return nil, e
		}
		table := st.Schema.Table
		if table == "anchor_income_lock" || table == "anchor_finance_control" || table == "anchor_finance_permission" {
			continue
		}
		for _, event := range []string{"DELETE", "UPDATE"} {
			allowed := map[string]bool{}
			conditions := []string{}
			if event == "UPDATE" {
				switch table {
				case "anchor_settlement_rule", "anchor_settlement":
					allowed = map[string]bool{"status": true, "approved_by": true, "approved_at": true, "review_reason": true}
					target := "approved"
					if table == "anchor_settlement_rule" {
						target = "active"
					}
					conditions = append(conditions, "OLD.status <> 'pending'", "NEW.status NOT IN ('"+target+"','rejected')", "NEW.approved_by=0", rootSelfReviewCondition("approved_by"), "NEW.approved_at<OLD.created_at", "NEW.review_reason=''")
				case "anchor_withdrawal_order":
					allowed = map[string]bool{"status": true, "reconciliation_status": true, "reconciliation_no": true, "reviewed_by": true, "reviewed_at": true, "review_reason": true}
					conditions = append(conditions, "OLD.status <> 'pending_review'", "NEW.status NOT IN ('pending_review','approved','rejected')", "(NEW.status='pending_review' AND (NEW.reviewed_by<>0 OR NEW.reviewed_at<>0 OR NEW.review_reason<>''))", "(NEW.status<>'pending_review' AND (NEW.reviewed_by=0 OR NEW.reviewed_at<OLD.created_at OR NEW.review_reason=''))", "(NEW.status='approved' AND (NEW.reconciliation_status<>'passed' OR NEW.reconciliation_no=''))")
					if manual {
						conditions = []string{"NOT ((" + strings.Join(negateConditions(conditions), " AND ") + ") OR (" + withdrawalTerminalGuard(db) + "))"}
					}
				case "anchor_settlement_rounding":
					allowed = map[string]bool{"remainder": true, "version": true, "last_settlement_no": true, "updated_at": true}
					conditions = append(conditions, "NEW.version<>OLD.version+1", "NEW.updated_at<OLD.updated_at", "NEW.last_settlement_no=OLD.last_settlement_no", "NOT EXISTS (SELECT 1 FROM anchor_settlement s WHERE s.settlement_no=NEW.last_settlement_no AND s.anchor_id=OLD.anchor_id AND s.currency=OLD.currency AND s.status='approved' AND s.approved_at=NEW.updated_at)")
				case "anchor_withdrawable_account":
					allowed = map[string]bool{"available": true, "frozen": true, "debt": true, "version": true, "updated_at": true}
					conditions = append(conditions, "NEW.version<>OLD.version+1", "NEW.updated_at<OLD.updated_at", "NOT EXISTS (SELECT 1 FROM anchor_withdrawable_ledger l WHERE l.anchor_id=OLD.anchor_id AND l.currency=OLD.currency AND l.version=NEW.version AND l.available_before=OLD.available AND l.available_after=NEW.available AND l.frozen_before=OLD.frozen AND l.frozen_after=NEW.frozen AND l.debt_before=OLD.debt AND l.debt_after=NEW.debt)")
					if manual {
						allowed["paid"] = true
						conditions[2] = strings.TrimSuffix(conditions[2], ")") + " AND l.paid_before=OLD.paid AND l.paid_after=NEW.paid)"
					}
				}
			}
			if len(allowed) > 0 {
				for _, f := range st.Schema.Fields {
					if f.DBName == "" || allowed[f.DBName] {
						continue
					}
					if db.Dialector.Name() == "mysql" {
						conditions = append(conditions, "NOT (BINARY OLD.`"+f.DBName+"` <=> BINARY NEW.`"+f.DBName+"`)")
					} else {
						conditions = append(conditions, "OLD.`"+f.DBName+"` IS NOT NEW.`"+f.DBName+"`")
					}
				}
			}
			message := "earning financial fact immutable"
			if table == "anchor_income_fact" {
				message = "gift financial fact immutable"
			}
			body := "BEGIN SELECT RAISE(ABORT,'" + message + "'); END"
			if db.Dialector.Name() == "mysql" {
				body = "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='" + message + "'"
				if len(conditions) > 0 {
					body = "BEGIN IF " + strings.Join(conditions, " OR ") + " THEN " + body + "; END IF; END"
				}
			} else if len(conditions) > 0 {
				body = "WHEN " + strings.Join(conditions, " OR ") + " " + body
			}
			name := "trg_" + table + "_" + strings.ToLower(event)
			if event == "UPDATE" && (table == "anchor_settlement_rule" || table == "anchor_settlement") {
				name += "_root_v2"
			}
			if manual && event == "UPDATE" && (table == "anchor_withdrawable_account" || table == "anchor_withdrawal_order") {
				name += "_manual_v3"
			}
			all = append(all, giftTrigger{name, table, event, body})
		}
	}
	if manual {
		c := []string{
			`NEW.entry_type='withdrawal_paid' AND NOT EXISTS (SELECT 1 FROM anchor_withdrawal_order w JOIN payout_order o ON o.business_no=w.withdrawal_no JOIN payout_status_history h ON h.payout_no=o.payout_no JOIN payout_attempt a ON a.attempt_no=h.attempt_no JOIN payout_audit_log au ON au.transition_no=h.event_no WHERE w.withdrawal_no=NEW.business_no AND w.status='approved' AND w.anchor_id=NEW.anchor_id AND w.currency=NEW.currency AND w.amount=NEW.change_amount AND o.status IN ('pending_confirmation','unknown') AND h.sequence=o.state_version+1 AND h.from_status=o.status AND h.to_status='succeeded' AND a.status='succeeded' AND a.amount=o.amount AND a.currency=o.currency AND a.reviewer_id=h.actor_id AND au.actor_id=h.actor_id)`,
			`NEW.entry_type='withdrawal_release' AND EXISTS (SELECT 1 FROM anchor_withdrawal_order w WHERE w.withdrawal_no=NEW.business_no AND w.status<>'pending_review') AND NOT EXISTS (SELECT 1 FROM anchor_withdrawal_order w JOIN payout_order o ON o.business_no=w.withdrawal_no JOIN payout_status_history h ON h.payout_no=o.payout_no JOIN payout_audit_log au ON au.transition_no=h.event_no WHERE w.withdrawal_no=NEW.business_no AND w.status='approved' AND w.anchor_id=NEW.anchor_id AND w.currency=NEW.currency AND w.amount=NEW.change_amount AND o.status IN ('ready','failed') AND h.sequence=o.state_version+1 AND h.from_status=o.status AND h.to_status='cancelled' AND au.actor_id=h.actor_id AND NOT EXISTS (SELECT 1 FROM payout_attempt a WHERE a.payout_no=o.payout_no AND a.status NOT IN ('failed','abandoned')))`,
		}
		all = append(all, giftTrigger{"trg_anchor_withdrawable_ledger_insert_manual_v3", "anchor_withdrawable_ledger", "INSERT", guardBody(db, c)})
	}
	return all, nil
}
func ValidateEarningSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("earning database unavailable")
	}
	triggers, e := earningTriggers(db)
	if e != nil {
		return e
	}
	if db.Dialector.Name() == "mysql" {
		return validateFinancialMySQLSchema(db, earningModels(), triggers)
	}
	if e := validateRootReviewUpgrade(db, triggers); e != nil {
		return e
	}
	for _, m := range earningModels() {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return e
		}
		if !db.Migrator().HasTable(m) {
			return fmt.Errorf("earning table missing: %s", st.Schema.Table)
		}
		for _, f := range st.Schema.Fields {
			if f.DBName != "" && !db.Migrator().HasColumn(m, f.DBName) {
				return fmt.Errorf("earning column missing: %s.%s", st.Schema.Table, f.DBName)
			}
		}
		indexes, e := db.Migrator().GetIndexes(m)
		if e != nil {
			return e
		}
		for _, index := range st.Schema.ParseIndexes() {
			ok := false
			for _, actual := range indexes {
				if actual.Name() != index.Name {
					continue
				}
				cols := []string{}
				for _, f := range index.Fields {
					cols = append(cols, f.Field.DBName)
				}
				unique, _ := actual.Unique()
				ok = strings.Join(cols, ",") == strings.Join(actual.Columns(), ",") && (index.Class != "UNIQUE" || unique)
			}
			if !ok {
				return fmt.Errorf("earning index incompatible: %s", index.Name)
			}
		}
		for name := range st.Schema.ParseCheckConstraints() {
			if !db.Migrator().HasConstraint(m, name) {
				return fmt.Errorf("earning check missing: %s", name)
			}
		}
	}
	for _, t := range triggers {
		var body string
		if e := db.Raw("SELECT sql FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name=?", t.name, t.table).Scan(&body).Error; e != nil {
			return e
		}
		expected := "CREATE TRIGGER " + t.name + " BEFORE " + t.event + " ON " + t.table + " " + t.body
		if strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(expected), " ") {
			return fmt.Errorf("earning trigger incompatible: %s", t.name)
		}
	}
	return nil
}

var earningOperations = []struct{ Method, Path string }{
	{"GET", "overview"}, {"GET", "income/list"}, {"GET", "account/list"}, {"GET", "account/ledger"}, {"GET", "account/check"}, {"GET", "exchange/list"}, {"POST", "exchange/create"}, {"POST", "exchange/review"}, {"GET", "settlement/list"}, {"GET", "settlement/detail"}, {"POST", "settlement/create"}, {"POST", "settlement/review"}, {"GET", "withdrawal/list"}, {"GET", "withdrawal/detail"}, {"POST", "withdrawal/review"}, {"GET", "reconciliation/list"}, {"POST", "reconciliation/run"},
}

func SyncEarningMetadata(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model.FinanceControl{}, 1).Error; e != nil {
			return e
		}
		var parent system.SysBaseMenu
		if e := tx.Where("name='liveManagement'").Take(&parent).Error; e != nil {
			return e
		}
		ids := []uint{parent.ID}
		for i, item := range []struct{ Name, Title string }{{"overview", "直播收益"}, {"exchange", "结算兑换规则"}, {"settlement", "收益结算"}, {"account", "主播可提现账户"}, {"withdrawal", "提现审核"}, {"reconciliation", "收益对账"}} {
			m := system.SysBaseMenu{MenuLevel: 1, ParentId: parent.ID, Path: "earning-" + item.Name, Name: "earning-" + item.Name, Component: "view/live/earning/" + item.Name + "/index.vue", Sort: 25 + i, Meta: system.Meta{Title: item.Title, Icon: "coin", KeepAlive: true}}
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
		for _, op := range earningOperations {
			a := system.SysApi{Path: "/earning/" + op.Path, Method: op.Method, ApiGroup: "主播收益", Description: op.Path}
			if e := tx.Where("path=? AND method=?", a.Path, a.Method).FirstOrCreate(&a).Error; e != nil {
				return e
			}
			p := model.AdminPermission{AuthorityID: 888, Path: a.Path, Method: a.Method}
			if e := tx.Where("authority_id=888 AND path=? AND method=?", p.Path, p.Method).FirstOrCreate(&p).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
func InitializeEarning() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := ValidateEarningSchema(global.GVA_DB.WithContext(ctx)); e != nil {
		return e
	}
	if e := ValidateIncomeCoverage(global.GVA_DB.WithContext(ctx)); e != nil {
		return e
	}
	gs.Default.EnrichEarnings = es.MarkGiftSettlementStatus
	es.Default.DB = global.GVA_DB
	es.Default.Payees = es.DatabasePayeeProvider{Accounts: ps.Default}
	es.Default.Sources = map[string]es.SourceVerifier{"gift": gs.Default.VerifyIncomeSources}
	ps.Default.FinancialGuard = es.Default.ValidatePayoutInTx
	ps.Default.FinancialLock = es.Default.LockPayoutFinanceInTx
	ps.Default.FinancialCheck = es.Default.CheckPayoutFinanceInTx
	ps.Default.FinancialFinalize = es.Default.FinalizePayoutInTx
	return nil
}

func ValidateIncomeCoverage(db *gorm.DB) error {
	var count int64
	if e := db.Table("anchor_earning_ledger e").Where("NOT EXISTS (SELECT 1 FROM anchor_income_fact i WHERE i.source_type='gift' AND i.source_entry_no=e.earning_no)").Limit(1).Select("1").Scan(&count).Error; e != nil {
		return e
	}
	if count != 0 {
		return fmt.Errorf("gift income backfill required before earning startup")
	}
	return nil
}
