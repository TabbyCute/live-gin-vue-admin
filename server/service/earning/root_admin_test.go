package earning_test

import (
	"errors"
	"github.com/stretchr/testify/require"
	"strings"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	sys "tb_live_module/model/system"
	es "tb_live_module/service/earning"
	"testing"
	"time"
)

func TestRootFinanceSelfReviewAndAccountIntegrity(t *testing.T) {
	testRootFinanceSelfReviewAndAccountIntegrity(t, false)
}
func TestMySQLRootFinanceSelfReviewAndAccountIntegrity(t *testing.T) {
	testRootFinanceSelfReviewAndAccountIntegrity(t, true)
}
func testRootFinanceSelfReviewAndAccountIntegrity(t *testing.T, mysql bool) {
	s, g, _ := fixture(t, mysql)
	if mysql {
		// Simulate upgrading an existing production schema, including an interrupted upgrade.
		for _, table := range []string{"anchor_settlement_rule", "anchor_settlement"} {
			name := "trg_" + table + "_update_root_v2"
			var body string
			require.NoError(t, s.DB.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=?", name).Scan(&body).Error)
			require.NotEmpty(t, body)
			column, legacy := "approved_by", "NEW.approved_by = OLD.created_by"
			if table == "anchor_rule_termination" {
				column, legacy = "reviewed_by", "NEW.reviewed_by = OLD.created_by"
			}
			if strings.HasPrefix(table, "anchor_settlement") {
				legacy = "NEW.approved_by=OLD.created_by"
			}
			body = strings.ReplaceAll(body, "(NEW."+column+" = OLD.created_by AND NEW."+column+" <> 1)", legacy)
			require.NoError(t, s.DB.Exec("CREATE TRIGGER "+strings.TrimSuffix(name, "_root_v2")+" BEFORE UPDATE ON "+table+" FOR EACH ROW "+body).Error)
		}
		require.ErrorContains(t, initialize.ValidateEarningSchema(s.DB), "legacy review trigger")
		require.NoError(t, initialize.MigrateEarning(s.DB))
		require.NoError(t, initialize.MigrateEarning(s.DB))
	}

	root := es.Actor{ID: 1, IP: "127.0.0.1"}
	require.NoError(t, s.DB.Where("sys_user_id=1").Delete(&sys.SysUserAuthority{}).Error)
	require.NoError(t, s.DB.Where("1=1").Delete(&em.AdminPermission{}).Error)
	rule, err := s.CreateExchangeRule(ctx, root, er.ExchangeRule{RequestID: "root_exchange", Name: "root兑换", SourceUnit: "COIN", Currency: "CNY", CoinUnits: "1", MinorUnits: "1", RoundingMode: "floor", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
	require.NoError(t, err)
	rule, err = s.ReviewExchangeRule(ctx, root, er.FinanceReview{No: rule.RuleNo, Approve: true, Reason: "root复核"})
	require.NoError(t, err)
	require.Equal(t, root.ID, rule.ApprovedBy)
	send(t, g, "root_income", "20")
	settlement, err := s.CreateSettlement(ctx, root, er.CreateSettlement{RequestID: "root_settlement", AnchorNo: "anchor01", RuleNo: rule.RuleNo, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()})
	require.NoError(t, err)
	decision := er.FinanceReview{No: settlement.SettlementNo, Approve: true, Reason: "root结算"}
	settlement, err = s.ReviewSettlement(ctx, root, decision)
	require.NoError(t, err)
	require.Equal(t, root.ID, settlement.ApprovedBy)
	_, err = s.ReviewSettlement(ctx, root, decision)
	require.NoError(t, err)
	check, err := s.CheckAccount(ctx, root, er.FinanceQuery{AnchorNo: "anchor01", Currency: "CNY"})
	require.NoError(t, err)
	require.Equal(t, true, check["consistent"], check["issue"])
	withdrawal := apply(t, s, "root_withdrawal", "10")
	_, err = s.ReconcileWithdrawal(ctx, root, er.Reconcile{WithdrawalNo: withdrawal.WithdrawalNo})
	require.NoError(t, err)
	_, err = s.ReviewWithdrawal(ctx, root, er.FinanceReview{No: withdrawal.WithdrawalNo, Approve: true, Reason: "root审核提现"})
	require.NoError(t, err)
	_, err = s.List(ctx, 0, &financeReviewer, "withdrawal", er.FinanceQuery{})
	require.ErrorIs(t, err, es.ErrForbidden)
	require.NoError(t, s.DB.Model(&sys.SysUser{}).Where("id=1").Update("enable", 2).Error)
	_, err = s.List(ctx, 0, &root, "withdrawal", er.FinanceQuery{})
	require.ErrorIs(t, err, es.ErrForbidden)
}

func TestMySQLRootReviewOnlyMigration(t *testing.T) {
	s, _, _ := fixture(t, true)
	for _, table := range []string{"anchor_revenue_rule", "anchor_rule_termination", "anchor_settlement_rule", "anchor_settlement"} {
		name := "trg_" + table + "_update_root_v2"
		var body string
		require.NoError(t, s.DB.Raw("SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND TRIGGER_NAME=?", name).Scan(&body).Error)
		require.NotEmpty(t, body)
		column, oldCondition := "approved_by", "NEW.approved_by = OLD.created_by"
		if table == "anchor_rule_termination" {
			column, oldCondition = "reviewed_by", "NEW.reviewed_by = OLD.created_by"
		}
		if strings.HasPrefix(table, "anchor_settlement") {
			oldCondition = "NEW.approved_by=OLD.created_by"
		}
		body = strings.ReplaceAll(body, "(NEW."+column+" = OLD.created_by AND NEW."+column+" <> 1)", oldCondition)
		require.NoError(t, s.DB.Exec("CREATE TRIGGER "+strings.TrimSuffix(name, "_root_v2")+" BEFORE UPDATE ON "+table+" FOR EACH ROW "+body).Error)
		require.NoError(t, s.DB.Exec("DROP TRIGGER "+name).Error)
	}
	before, err := initialize.PlanRootReviewMigration(s.DB)
	require.NoError(t, err)
	require.Len(t, before.Previous, 4)
	require.Len(t, before.Statements, 8)
	backupFailed := errors.New("backup failed")
	require.ErrorIs(t, initialize.MigrateRootReviewTriggers(s.DB, func(initialize.RootReviewPlan) error { return backupFailed }), backupFailed)
	after, err := initialize.PlanRootReviewMigration(s.DB)
	require.NoError(t, err)
	require.Equal(t, before, after)
	var saved initialize.RootReviewPlan
	require.NoError(t, initialize.MigrateRootReviewTriggers(s.DB, func(plan initialize.RootReviewPlan) error { saved = plan; return nil }))
	require.Equal(t, before, saved)
	require.NoError(t, initialize.MigrateRootReviewTriggers(s.DB, nil))
	after, err = initialize.PlanRootReviewMigration(s.DB)
	require.NoError(t, err)
	require.Empty(t, after.Statements)
	require.NoError(t, initialize.ValidateGiftSchema(s.DB))
	require.NoError(t, initialize.ValidateEarningSchema(s.DB))
}
