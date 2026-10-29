package wallet

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
	res "tb_live_module/model/wallet/response"
)

func TestRechargeBusinessIdentityAcrossAdminsAndStates(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	r := grant("recharge_key_001", "100")
	r.ExternalOrderNo = "bank-20261028:0001"
	o, err := s.Create(ctx, testCreator, r)
	require.NoError(t, err)
	require.Equal(t, "BANK-20261028:0001", o.ExternalOrderNo)
	require.Equal(t, model.SourceManual, o.SourceType)
	r.SourceType, r.ExternalOrderNo = model.SourceManual, o.ExternalOrderNo
	retry, err := s.Create(ctx, testCreator, r)
	require.NoError(t, err)
	require.Equal(t, o.OrderNo, retry.OrderNo)
	_, err = s.Create(ctx, testReviewer, r) // 同key换管理员也不能重复。
	require.ErrorIs(t, err, ErrDuplicateSource)
	r.RequestID = "recharge_key_002"
	r.AccountNo, r.Amount = "10000011", "999" // 改用户/金额也不能抢占已用凭证。
	_, err = s.Create(ctx, testReviewer, r)
	require.ErrorIs(t, err, ErrDuplicateSource)
	_, err = s.Reject(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	_, err = s.Create(ctx, testReviewer, r)
	require.ErrorIs(t, err, ErrDuplicateSource) // 驳回不释放资金标识。
	r.ExternalOrderNo = "BANK-20261028:0002"
	o2, err := s.Create(ctx, testCreator, r)
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(o2.OrderNo))
	require.NoError(t, err)
	_, err = s.Create(ctx, testReviewer, r)
	require.ErrorIs(t, err, ErrDuplicateSource)
	reverse, err := s.Reverse(ctx, testCreator, req.ReverseAdjustment{RequestID: "recharge_reverse", OrderNo: o2.OrderNo, Reason: "错误充值冲正"})
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(reverse.OrderNo))
	require.NoError(t, err)
	_, err = s.Create(ctx, testReviewer, r)
	require.ErrorIs(t, err, ErrDuplicateSource) // 冲正也不释放，不允许重放原资金。
	page, err := s.Orders(ctx, testReviewer, req.OrderSearch{ExternalOrderNo: "bank-20261028:0002"})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Equal(t, o2.OrderNo, page.List.([]res.Order)[0].OrderNo)
	count(t, s.DB, &model.WalletAdjustmentOrder{}, 3)
}

func TestRechargeRequiresCanonicalIdentityAndEvidence(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	for _, bad := range []string{"", " A", "A ", "A\n", "ＡBC", "K", "ſ", "凭证123", strings.Repeat("A", 129)} {
		r := grant("identity_bad", "1")
		r.ExternalOrderNo = bad
		_, err := s.Create(ctx, testCreator, r)
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
	for _, source := range []string{"alipay", "wechat", "legacy", "reversal", "MANUAL"} {
		r := grant("source_bad", "1")
		r.SourceType = source
		_, err := s.Create(ctx, testCreator, r)
		require.ErrorIs(t, err, ErrInvalid, source)
	}
	r := grant("evidence_small", "1")
	r.EvidenceRef = ""
	_, err := s.Create(ctx, testCreator, r)
	require.ErrorIs(t, err, ErrEvidence)
	r = grant("identity_max", "1")
	r.ExternalOrderNo = strings.Repeat("A", 128)
	_, err = s.Create(ctx, testCreator, r)
	require.NoError(t, err)
}

func TestSetStatusNeverOpensWalletAndLifecycle(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	require.NoError(t, s.DB.Exec("UPDATE live_account SET deleted_at=CURRENT_TIMESTAMP WHERE id=11").Error)
	for _, no := range []string{"10000010", "10000011", "10000012"} {
		for _, status := range []string{model.AccountNormal, model.AccountDisabled} {
			_, err := s.SetStatus(ctx, testReviewer, req.AccountStatus{AccountNo: no, Status: status, Reason: "状态操作测试"})
			require.Error(t, err)
		}
	}
	count(t, s.DB, &model.WalletAccount{}, 0)
	count(t, s.DB, &model.WalletAuditLog{}, 0)
	o, err := s.Create(ctx, testCreator, grant("status_open", "1"))
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	r := req.AccountStatus{AccountNo: "10000010", Status: model.AccountNormal, Reason: "不变状态操作"}
	_, err = s.SetStatus(ctx, testReviewer, r)
	require.NoError(t, err)
	count(t, s.DB, &model.WalletAuditLog{}, 3) // 创建申请、开户、审核；无变化不产生伪审计。
	r.Status = model.AccountDisabled
	_, err = s.SetStatus(ctx, testReviewer, r)
	require.NoError(t, err)
	_, err = s.SetStatus(ctx, testReviewer, r)
	require.NoError(t, err)
	count(t, s.DB, &model.WalletAuditLog{}, 4)
	require.NoError(t, s.DB.Exec("UPDATE live_account SET deleted_at=CURRENT_TIMESTAMP WHERE id=10").Error)
	r.Status = model.AccountNormal
	_, err = s.SetStatus(ctx, testReviewer, r)
	require.ErrorIs(t, err, ErrAccount)
	r.Status = model.AccountDisabled
	_, err = s.SetStatus(ctx, testReviewer, r)
	require.NoError(t, err)
	count(t, s.DB, &model.WalletAuditLog{}, 4)
}

func TestApprovalDetectsRequestCorruptionAndLegacy(t *testing.T) {
	for _, col := range []string{"amount", "account_id", "reason", "evidence_ref", "external_order_no"} {
		t.Run(col, func(t *testing.T) {
			s := fixture(t) // 此fixture故意没有触发器，验证第二层摘要检查。
			o, err := s.Create(context.Background(), testCreator, grant("hash_test_001", "1"))
			require.NoError(t, err)
			v := any("CHANGED")
			if col == "amount" {
				v = 2
			}
			if col == "account_id" {
				v = 11
			}
			require.NoError(t, s.DB.Model(&model.WalletAdjustmentOrder{}).Where("order_no=?", o.OrderNo).Update(col, v).Error)
			_, err = s.Approve(context.Background(), testReviewer, review(o.OrderNo))
			require.ErrorIs(t, err, ErrInvariant)
			count(t, s.DB, &model.WalletAccount{}, 0)
			count(t, s.DB, &model.WalletLedger{}, 0)
		})
	}
	s := fixture(t)
	o, err := s.Create(context.Background(), testCreator, grant("legacy_blocked", "1"))
	require.NoError(t, err)
	require.NoError(t, s.DB.Model(&model.WalletAdjustmentOrder{}).Where("order_no=?", o.OrderNo).Update("source_type", model.SourceLegacy).Error)
	_, err = s.Approve(context.Background(), testReviewer, review(o.OrderNo))
	require.ErrorIs(t, err, ErrLegacyOrder)
	_, err = s.Reject(context.Background(), testReviewer, review(o.OrderNo))
	require.NoError(t, err)
}

func TestInternalPostingDuplicateRollsBackBalance(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, testCreator, grant("posting_key", "10"))
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	var current model.WalletAccount
	require.NoError(t, s.DB.Take(&current).Error)
	_, err = postLedger(s.DB, current, ledgerPosting{Delta: 1, BusinessType: model.BusinessGrant, BusinessNo: "unguarded", OperatorID: 2, Reason: "非事务调用测试", Now: s.now().UnixMilli()})
	require.ErrorIs(t, err, ErrInvariant)
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		w, e := lockWallet(tx, 10, "COIN")
		if e != nil {
			return e
		}
		_, e = postLedger(tx, w, ledgerPosting{Delta: 10, BusinessType: model.BusinessGrant, BusinessNo: o.OrderNo, OperatorID: 2, Reason: "内部重放测试", Now: s.now().UnixMilli()})
		return e
	})
	require.Error(t, err)
	amount(t, s, "10")
	count(t, s.DB, &model.WalletLedger{}, 1)
}

func TestLegacyCreditedRechargeRemainsReversible(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, testCreator, grant("legacy_credited", "10"))
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	// 无触发器fixture模拟升级后的已入账历史数据；正式库不允许这样的UPDATE。
	require.NoError(t, s.DB.Model(&model.WalletAdjustmentOrder{}).Where("order_no=?", o.OrderNo).Updates(map[string]any{"source_type": model.SourceLegacy, "external_order_no": o.OrderNo}).Error)
	reverse, err := s.Reverse(ctx, testCreator, req.ReverseAdjustment{RequestID: "legacy_reverse", OrderNo: o.OrderNo, Reason: "历史错误入账冲正"})
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(reverse.OrderNo))
	require.NoError(t, err)
	amount(t, s, "0")
	check, err := s.Check(ctx, testReviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, err)
	require.Equal(t, []string{"legacy_source_unverified:" + o.OrderNo}, check.Issues)
}

func TestReviewReplayAndReconciliationRequireCommittedFacts(t *testing.T) {
	for _, scenario := range []string{"missing_audit", "wrong_audit_operator", "duplicate_audit", "missing_ledger", "wrong_ledger", "rejected_audit", "status_only"} {
		t.Run(scenario, func(t *testing.T) {
			s := fixture(t) // 故意无数据库触发器，模拟损坏的恢复库/旁路写入。
			ctx := context.Background()
			o, err := s.Create(ctx, testCreator, grant("replay_integrity", "10"))
			require.NoError(t, err)
			if scenario == "rejected_audit" {
				_, err = s.Reject(ctx, testReviewer, review(o.OrderNo))
			} else if scenario != "status_only" {
				_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
			}
			require.NoError(t, err)
			switch scenario {
			case "missing_audit", "rejected_audit":
				require.NoError(t, s.DB.Exec("DELETE FROM wallet_audit_log WHERE object_no=? AND action IN ('approve_and_credit','reject')", o.OrderNo).Error)
			case "wrong_audit_operator":
				require.NoError(t, s.DB.Exec("UPDATE wallet_audit_log SET operator_id=999 WHERE object_no=? AND action='approve_and_credit'", o.OrderNo).Error)
			case "duplicate_audit":
				var log model.WalletAuditLog
				require.NoError(t, s.DB.Where("object_no=? AND action='create'", o.OrderNo).Take(&log).Error)
				log.ID = 0
				require.NoError(t, s.DB.Create(&log).Error)
			case "missing_ledger":
				require.NoError(t, s.DB.Exec("DELETE FROM wallet_ledger WHERE business_no=?", o.OrderNo).Error)
			case "wrong_ledger":
				require.NoError(t, s.DB.Exec("UPDATE wallet_ledger SET operator_id=999 WHERE business_no=?", o.OrderNo).Error)
			case "status_only":
				require.NoError(t, s.DB.Exec("UPDATE wallet_adjustment_order SET status='credited', approved_by=2, review_reason=? WHERE order_no=?", review(o.OrderNo).Reason, o.OrderNo).Error)
			}
			if scenario == "rejected_audit" {
				_, err = s.Reject(ctx, testReviewer, review(o.OrderNo))
			} else {
				_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
			}
			require.ErrorIs(t, err, ErrInvariant)
			check, err := s.Check(ctx, testReviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
			require.NoError(t, err)
			require.False(t, check.Consistent, scenario)
			if scenario != "missing_ledger" && scenario != "wrong_ledger" {
				require.Contains(t, check.Issues, "order_audit:"+o.OrderNo)
			}
			if scenario == "rejected_audit" || scenario == "status_only" {
				amount(t, s, "0")
			} else {
				amount(t, s, "10")
			}
		})
	}
}
