package wallet

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
)

func TestApprovalRequiresOriginalCreationAudit(t *testing.T) {
	for _, damage := range []string{"missing", "changed", "duplicate"} {
		t.Run(damage, func(t *testing.T) {
			s := fixture(t) // 无触发器恢复库中的故障注入；正常MySQL还受不可变触发器保护。
			ctx := context.Background()
			o, err := s.Create(ctx, testCreator, grant("audit_before_first_approval", "10"))
			require.NoError(t, err)
			switch damage {
			case "missing":
				require.NoError(t, s.DB.Exec("DELETE FROM wallet_audit_log WHERE object_no=?", o.OrderNo).Error)
			case "changed":
				require.NoError(t, s.DB.Exec("UPDATE wallet_audit_log SET operator_id=99 WHERE object_no=?", o.OrderNo).Error)
			case "duplicate":
				var a model.WalletAuditLog
				require.NoError(t, s.DB.Where("object_no=?", o.OrderNo).Take(&a).Error)
				a.ID = 0
				require.NoError(t, s.DB.Create(&a).Error)
			}
			_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
			require.ErrorIs(t, err, ErrInvariant)
			amount(t, s, "0")
			count(t, s.DB, &model.WalletLedger{}, 0)
			count(t, s.DB, &model.WalletDailyQuota{}, 0)
		})
	}
}

func TestReviewReplayRejectsDamagedRequestAndLedger(t *testing.T) {
	for _, damage := range []string{"evidence", "hash", "reason", "balance", "original"} {
		t.Run(damage, func(t *testing.T) {
			s := fixture(t)
			ctx := context.Background()
			o, err := s.Create(ctx, testCreator, grant("replay_corruption_001", "10"))
			require.NoError(t, err)
			_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
			require.NoError(t, err)
			switch damage {
			case "evidence":
				require.NoError(t, s.DB.Exec("UPDATE wallet_adjustment_order SET evidence_ref='changed' WHERE order_no=?", o.OrderNo).Error)
			case "hash":
				require.NoError(t, s.DB.Exec("UPDATE wallet_adjustment_order SET request_hash='changed' WHERE order_no=?", o.OrderNo).Error)
			case "reason":
				require.NoError(t, s.DB.Exec("UPDATE wallet_ledger SET reason='changed' WHERE business_no=?", o.OrderNo).Error)
			case "balance":
				require.NoError(t, s.DB.Exec("UPDATE wallet_ledger SET balance_before=1,balance_after=11 WHERE business_no=?", o.OrderNo).Error)
			case "original":
				require.NoError(t, s.DB.Exec("UPDATE wallet_ledger SET original_ledger_no='wrong' WHERE business_no=?", o.OrderNo).Error)
			}
			_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
			require.ErrorIs(t, err, ErrInvariant)
			amount(t, s, "10")
		})
	}
}

func TestReverseRefusesDamagedOriginalFacts(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, testCreator, grant("reverse_damaged_base", "10"))
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	rev, err := s.Reverse(ctx, testReviewer, req.ReverseAdjustment{RequestID: "reverse_damaged_001", OrderNo: o.OrderNo, Reason: "核对后冲正测试"})
	require.NoError(t, err)
	require.NoError(t, s.DB.Exec("DELETE FROM wallet_audit_log WHERE object_no=? AND action='approve_and_credit'", o.OrderNo).Error)
	_, err = s.Approve(ctx, testCreator, review(rev.OrderNo))
	require.ErrorIs(t, err, ErrInvariant)
	amount(t, s, "10")
	count(t, s.DB, &model.WalletLedger{}, 1)
}

func TestManualTraceRetainsFactsWhenLedgerIsMissing(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, testCreator, grant("trace_broken_ledger", "10"))
	require.NoError(t, err)
	_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, err)
	require.NoError(t, s.DB.Exec("DELETE FROM wallet_ledger WHERE business_no=?", o.OrderNo).Error)
	detail, err := s.Detail(ctx, testReviewer, o.OrderNo)
	require.NoError(t, err)
	require.Equal(t, o.OrderNo, detail.Order.OrderNo)
	require.Len(t, detail.Audit, 2)
	require.Nil(t, detail.Ledger)
	require.Contains(t, detail.Issues, "order_ledger")
	trace, err := s.Trace(ctx, testReviewer, req.TraceQuery{No: o.OrderNo})
	require.NoError(t, err)
	require.Equal(t, o.OrderNo, trace.Adjustment.Order.OrderNo)
	require.Contains(t, trace.Issues, "order_ledger")
	_, err = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.ErrorIs(t, err, ErrInvariant)
	amount(t, s, "10")
}
