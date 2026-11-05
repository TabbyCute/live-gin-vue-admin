package earning_test

import (
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"reflect"
	"strings"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	pm "tb_live_module/model/payout"
	pr "tb_live_module/model/payout/request"
	ps "tb_live_module/service/payout"
	"testing"
)

// Build the prior phase's exact changed columns/tags using a test-only anonymous
// model. Financial data is retained in typed rows and compared after migration.
func legacyManualModel(m any) (any, []string) {
	typ := reflect.TypeOf(m).Elem()
	var fields []reflect.StructField
	var omitted []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := f.Tag.Get("gorm")
		drop := false
		switch typ.Name() {
		case "Account":
			tag = strings.ReplaceAll(tag, "ck_withdrawable_balances_v3", "ck_withdrawable_balances")
			tag = strings.ReplaceAll(tag, "paid>=0", "paid=0")
		case "AccountLedger":
			drop = f.Name == "PaidBefore" || f.Name == "PaidAfter"
			tag = strings.ReplaceAll(tag, "ck_withdrawable_entry_v3", "ck_withdrawable_entry")
			tag = strings.ReplaceAll(tag, ",'withdrawal_paid'", "")
		case "Withdrawal":
			tag = strings.ReplaceAll(tag, "ck_withdrawal_status_v3", "ck_withdrawal_status")
			tag = strings.ReplaceAll(tag, ",'paid','cancelled'", "")
		case "Order":
			drop = f.Name == "StateVersion" || f.Name == "LastTransitionNo"
			tag = strings.ReplaceAll(tag, "ck_payout_status_v3", "ck_payout_status")
			tag = strings.ReplaceAll(tag, ",'pending_confirmation'", "")
			tag = strings.ReplaceAll(tag, "ck_payout_time_v3", "ck_payout_time")
			tag = strings.ReplaceAll(tag, "updated_at>=created_at", "updated_at=created_at")
			if f.Name == "Status" {
				tag = strings.ReplaceAll(tag, "size:24", "size:16")
			}
		case "StatusHistory":
			drop = f.Name == "Sequence" || f.Name == "AttemptNo" || f.Name == "RequestID" || f.Name == "Reason"
			if f.Name == "PayoutNo" {
				tag = "size:40;not null;uniqueIndex"
			}
			if f.Name == "ToStatus" {
				tag = "size:16;not null;check:ck_payout_history,from_status='' AND to_status='ready'"
			}
			if f.Name == "FromStatus" {
				tag = "size:16;not null"
			}
		case "OrderAudit":
			drop = f.Name == "ObjectNo" || f.Name == "TransitionNo"
			if f.Name == "Action" {
				tag = "size:32;not null;index;check:ck_payout_audit,action='order.create'"
			}
		}
		if drop {
			omitted = append(omitted, f.Name)
			continue
		}
		f.Tag = reflect.StructTag(`json:"` + f.Tag.Get("json") + `" gorm:"` + tag + `"`)
		fields = append(fields, f)
	}
	return reflect.New(reflect.StructOf(fields)).Interface(), omitted
}
func TestManualPayoutLegacyMigration(t *testing.T)      { manualLegacyMigration(t, false) }
func TestMySQLManualPayoutLegacyMigration(t *testing.T) { manualLegacyMigration(t, true) }
func manualLegacyMigration(t *testing.T, real bool) {
	s, p, payee := payoutFixture(t, real)
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	store, e := ps.OpenEvidenceStore(dir)
	require.NoError(t, e)
	t.Cleanup(func() { store.Close() })
	p.EvidenceStore = store
	r := &manualRig{s: s, p: p, payee: payee}
	o := r.order(t)
	db := r.s.DB
	var original pm.Order
	require.NoError(t, db.Where("payout_no=?", o.PayoutNo).Take(&original).Error)
	before := r.balance(t)
	var triggers []string
	if real {
		require.NoError(t, db.Raw("SELECT TRIGGER_NAME FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME LIKE '%manual_v3'").Scan(&triggers).Error)
	} else {
		require.NoError(t, db.Raw("SELECT name FROM sqlite_master WHERE type='trigger' AND name LIKE '%manual_v3'").Scan(&triggers).Error)
	}
	for _, name := range triggers {
		require.NoError(t, db.Exec("DROP TRIGGER `"+name+"`").Error)
	}
	models := []any{&em.Account{}, &em.AccountLedger{}, &em.Withdrawal{}, &pm.Order{}, &pm.StatusHistory{}, &pm.OrderAudit{}}
	// Downgrade is restricted to the fixture, which rejects remote DSNs. Never a
	// production migration operation. Recreate old tables with identical facts.
	for _, m := range models {
		st := &gorm.Statement{DB: db}
		require.NoError(t, st.Parse(m))
		table := st.Schema.Table
		rows := reflect.New(reflect.SliceOf(reflect.TypeOf(m).Elem())).Interface()
		require.NoError(t, db.Find(rows).Error)
		require.NoError(t, db.Migrator().DropTable(m))
		legacy, omit := legacyManualModel(m)
		target := db.Table(table)
		if real {
			target = target.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
		}
		require.NoError(t, target.AutoMigrate(legacy))
		if reflect.ValueOf(rows).Elem().Len() > 0 {
			require.NoError(t, db.Table(table).Omit(omit...).Create(rows).Error)
		}
	}
	require.NoError(t, initialize.MigratePayout(db))
	require.NoError(t, initialize.MigrateEarning(db))
	require.NoError(t, initialize.MigratePayout(db))
	require.NoError(t, initialize.ValidatePayoutSchema(db))
	require.NoError(t, initialize.ValidateEarningSchema(db))
	require.Equal(t, before, r.balance(t))
	var migrated pm.Order
	require.NoError(t, db.Where("payout_no=?", o.PayoutNo).Take(&migrated).Error)
	require.Equal(t, original, migrated)
	require.NoError(t, r.p.VerifyOrder(migrated))
	var invalid int64
	require.NoError(t, db.Model(&em.AccountLedger{}).Where("paid_before<>0 OR paid_after<>0").Count(&invalid).Error)
	require.Zero(t, invalid)
	var histories []pm.StatusHistory
	require.NoError(t, db.Where("payout_no=?", o.PayoutNo).Find(&histories).Error)
	require.Len(t, histories, 1)
	require.EqualValues(t, 1, histories[0].Sequence)
	company, e := p.CreateChannelAccount(ctx, payoutAdmin, "trace", pr.CreateChannelAccount{RequestID: r.key(), ChannelCode: "manual_bank", AccountName: "LEGACY TEST COMPANY", InstitutionName: "ISOLATED BANK", Currency: "CNY", Account: "LEGACYBANK200002", SingleLimit: "10000", DailyLimit: "100000"})
	require.NoError(t, e)
	r.company = company.ChannelAccountNo
	at := r.start(t, o, false)
	r.submit(t, at.AttemptNo, "ISOLATED-LEGACY-READY")
	r.review(t, at.AttemptNo, "success")
	require.EqualValues(t, 5, r.balance(t).Paid)
}
