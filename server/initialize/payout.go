package initialize

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"os"
	"strings"
	"tb_live_module/global"
	earning "tb_live_module/model/earning"
	model "tb_live_module/model/payout"
	svc "tb_live_module/service/payout"
	"time"
)

func payoutModels() []any {
	return []any{&model.PayeeAccount{}, &model.OwnerLock{}, &model.Audit{}, &model.Order{}, &model.OrderRequest{}, &model.StatusHistory{}, &model.OrderAudit{}, &model.Channel{}, &model.ChannelAccount{}, &model.Attempt{}, &model.AttemptEvent{}, &model.ManualRequest{}, &model.DailyUsage{}, &model.Evidence{}, &model.RevealGrant{}, &model.TerminalEvent{}, &model.OrderReconciliation{}, &model.Investigation{}}
}
func legacyPayoutTriggers(db *gorm.DB) ([]giftTrigger, error) {
	var all []giftTrigger
	for _, m := range []any{&model.PayeeAccount{}, &model.Audit{}, &model.Order{}, &model.OrderRequest{}, &model.StatusHistory{}, &model.OrderAudit{}} {
		st := &gorm.Statement{DB: db}
		if e := st.Parse(m); e != nil {
			return nil, e
		}
		for _, event := range []string{"UPDATE", "DELETE"} {
			var conditions []string
			if event == "UPDATE" && st.Schema.Table == "payout_payee_account" {
				allowed := map[string]bool{"status": true, "default_slot": true, "updated_at": true, "disabled_at": true}
				conditions = []string{"OLD.status<>'active'", "NEW.updated_at<OLD.updated_at", "NEW.status NOT IN ('active','disabled')"}
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
			body := "BEGIN SELECT RAISE(ABORT,'payout binding immutable'); END"
			if db.Dialector.Name() == "mysql" {
				body = "SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='payout binding immutable'"
				if len(conditions) > 0 {
					body = "BEGIN IF " + strings.Join(conditions, " OR ") + " THEN " + body + "; END IF; END"
				}
			} else if len(conditions) > 0 {
				body = "WHEN " + strings.Join(conditions, " OR ") + " " + body
			}
			all = append(all, giftTrigger{"trg_" + st.Schema.Table + "_" + strings.ToLower(event), st.Schema.Table, event, body})
		}
	}
	body := "WHEN NEW.status<>'ready' BEGIN SELECT RAISE(ABORT,'payout initial state must be ready'); END"
	if db.Dialector.Name() == "mysql" {
		body = "BEGIN IF NEW.status<>'ready' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='payout initial state must be ready'; END IF; END"
	}
	all = append(all, giftTrigger{"trg_payout_order_insert", "payout_order", "INSERT", body})
	return all, nil
}
func MigratePayout(db *gorm.DB) error {
	if db != nil && db.Dialector.Name() == "sqlite" {
		return db.Transaction(migratePayout)
	}
	return migratePayout(db)
}
func migratePayout(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("payout database unavailable")
	}
	if db.Dialector.Name() != "mysql" && db.Dialector.Name() != "sqlite" {
		return fmt.Errorf("payout requires MySQL8 or development SQLite")
	}
	budget := 5 * time.Minute
	if deadline, ok := db.Statement.Context.Deadline(); ok {
		budget = min(time.Until(deadline), 30*time.Minute)
	}
	return withFinancialMigrationLock(db, budget, func(conn *gorm.DB) error {
		target := conn
		if conn.Dialector.Name() == "mysql" {
			if e := validateWalletMySQLRuntime(conn); e != nil {
				return e
			}
			target = conn.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
		}
		// A verified schema needs no DDL. Avoid repeated AutoMigrate catalog
		// round trips and metadata locks on remote production databases.
		if e := ValidatePayoutSchema(conn); e == nil {
			return nil
		}
		if e := suspendSQLiteManualTriggers(conn); e != nil {
			return e
		}
		if e := prepareManualChecks(conn, false); e != nil {
			return e
		}
		if e := target.AutoMigrate(payoutModels()...); e != nil {
			return e
		}
		if conn.Migrator().HasIndex(&model.StatusHistory{}, "idx_payout_status_history_payout_no") {
			if e := conn.Migrator().DropIndex(&model.StatusHistory{}, "idx_payout_status_history_payout_no"); e != nil {
				return e
			}
		}
		triggers, e := payoutTriggers(conn)
		if e != nil {
			return e
		}
		legacy, e := legacyPayoutTriggers(conn)
		if e != nil {
			return e
		}
		if e = upgradePayoutEarningTables(target); e != nil {
			return e
		}
		if e = installManualTriggers(conn, triggers, legacy); e != nil {
			return e
		}
		if conn.Dialector.Name() == "sqlite" {
			existing, err := earningTriggers(conn)
			if err != nil {
				return err
			}
			var restore []giftTrigger
			for _, t := range existing {
				if conn.Migrator().HasTable(t.table) {
					restore = append(restore, t)
				}
			}
			if e = installManualTriggers(conn, restore, nil); e != nil {
				return e
			}
		}
		if e = seedManualChannel(conn); e != nil {
			return e
		}
		// Add only the lookup index required for destination tracing. No earning
		// rows or snapshot columns are rewritten by this migration.
		if conn.Migrator().HasTable(&earning.Withdrawal{}) && !conn.Migrator().HasIndex(&earning.Withdrawal{}, "idx_withdrawal_payee_ref") {
			if e := conn.Migrator().CreateIndex(&earning.Withdrawal{}, "idx_withdrawal_payee_ref"); e != nil {
				return e
			}
		}
		return ValidatePayoutSchema(conn)
	})
}
func InitializePayout() error {
	crypto, e := svc.LoadCrypto()
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e = ValidatePayoutSchema(global.GVA_DB.WithContext(ctx)); e != nil {
		return e
	}
	configured := &svc.Service{DB: global.GVA_DB, Crypto: crypto}
	if e = configured.ValidateKeys(global.GVA_DB.WithContext(ctx)); e != nil {
		return e
	}
	if e = configured.ValidateOrderKeys(global.GVA_DB.WithContext(ctx)); e != nil {
		return e
	}
	if path := os.Getenv("PAYOUT_EVIDENCE_DIR"); path != "" {
		store, err := svc.OpenEvidenceStore(path, global.GVA_CONFIG.Local.StorePath)
		if err != nil {
			return err
		}
		configured.EvidenceStore = store
	}
	if e = configured.ValidateManualKeys(global.GVA_DB.WithContext(ctx)); e != nil {
		_ = configured.EvidenceStore.Close()
		return e
	}
	svc.Default.DB = global.GVA_DB
	svc.Default.Crypto = crypto
	svc.Default.EvidenceStore = configured.EvidenceStore
	return nil
}
