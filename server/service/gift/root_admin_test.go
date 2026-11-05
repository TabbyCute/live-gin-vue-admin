package gift_test

import (
	"github.com/stretchr/testify/require"
	"strings"
	"tb_live_module/initialize"
	er "tb_live_module/model/earning/request"
	gm "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	sys "tb_live_module/model/system"
	svc "tb_live_module/service/gift"
	"testing"
	"time"
)

func TestRootGiftPermissionsAndSelfReview(t *testing.T) {
	testRootGiftPermissionsAndSelfReview(t, false)
}
func TestMySQLRootGiftPermissionsAndSelfReview(t *testing.T) {
	testRootGiftPermissionsAndSelfReview(t, true)
}
func testRootGiftPermissionsAndSelfReview(t *testing.T, mysql bool) {
	s := fixture(t, mysql)
	if mysql {
		// Simulate upgrading an existing production schema, including an interrupted upgrade.
		for _, table := range []string{"anchor_revenue_rule", "anchor_rule_termination"} {
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
		require.ErrorContains(t, initialize.ValidateGiftSchema(s.DB), "legacy review trigger")
		require.NoError(t, initialize.MigrateGift(s.DB))
		require.NoError(t, initialize.MigrateGift(s.DB))
	}

	root := svc.Actor{ID: 1, IP: "127.0.0.1"}
	require.NoError(t, s.DB.Where("sys_user_id=1").Delete(&sys.SysUserAuthority{}).Error)
	require.NoError(t, s.DB.Where("1=1").Delete(&gm.AdminPermission{}).Error)
	require.True(t, s.CanReadRevenue(ctx, root))
	require.False(t, s.CanReadRevenue(ctx, reviewer))
	rule, err := s.CreateRule(ctx, root, er.Rule{Name: "root分成", ScopeType: "gift", ScopeNo: "gift0001", AnchorShareBPS: "4000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
	require.NoError(t, err)
	rule, err = s.ReviewRule(ctx, root, er.Review{RuleNo: rule.RuleNo, Approve: true})
	require.NoError(t, err)
	require.Equal(t, root.ID, rule.ApprovedBy)
	_, err = s.ReviewRule(ctx, root, er.Review{RuleNo: rule.RuleNo, Approve: true})
	require.NoError(t, err)
	termination, err := s.TerminateRule(ctx, root, er.TerminateRule{RuleNo: rule.RuleNo, RequestID: "root_termination", Reason: "root自行终止"})
	require.NoError(t, err)
	termination, err = s.ReviewTermination(ctx, root, er.ReviewTermination{TerminationNo: termination.TerminationNo, Approve: true})
	require.NoError(t, err)
	require.Equal(t, root.ID, termination.ReviewedBy)
	order, err := send(s, "root_refund_order", "1")
	require.NoError(t, err)
	_, err = s.Refund(ctx, root, req.Refund{OrderNo: order.OrderNo, RequestID: "root_refund", Quantity: "1", Reason: "root退款"})
	require.NoError(t, err)
	require.NoError(t, s.DB.Model(&sys.SysUser{}).Where("id=1").Update("enable", 2).Error)
	require.False(t, s.CanReadRevenue(ctx, root))
}
