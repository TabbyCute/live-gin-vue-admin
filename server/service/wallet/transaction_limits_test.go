package wallet

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	model "tb_live_module/model/wallet"
	req "tb_live_module/model/wallet/request"
)

func TestMovementRefundPartLimitAndReplay(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	seedMovement(t, s, "1001")
	d, e := s.Debit(ctx, debitInput("many-parts", "1001"))
	require.NoError(t, e)
	// 不创建1000次事务，单事务调用真实内核，仍逐次验证额度/流水/审计。
	tx := s.DB.Begin()
	require.NoError(t, tx.Error)
	for i := 0; i < maxRefundParts; i++ {
		_, e = s.RefundInTx(tx, refundInput(fmt.Sprintf("part-%d", i), "1", d.TransactionNo))
		if e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
	}
	require.NoError(t, tx.Commit().Error)
	_, e = s.Refund(ctx, refundInput("part-overflow", "1", d.TransactionNo))
	require.ErrorIs(t, e, ErrLimit)
	_, e = s.Refund(ctx, refundInput("part-0", "1", d.TransactionNo))
	require.NoError(t, e)
	c, e := s.Check(ctx, testReviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}, Limit: 1000})
	require.NoError(t, e)
	require.True(t, c.Consistent, c.Issues)
	require.True(t, c.HasMore)
}
func TestMovementRefundBalanceOverflowAndVersionLimit(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	s.Config.MaxGrantAmount = math.MaxInt64
	s.Config.AccountDailyLimit = math.MaxInt64
	s.Config.OperatorDailyLimit = math.MaxInt64
	seedMovement(t, s, strconv.FormatInt(math.MaxInt64, 10))
	d, e := s.Debit(ctx, debitInput("overflow-refund", "10"))
	require.NoError(t, e)
	// 次日正常充值把余额补到MaxInt64；原扣款、尾链和每日额度均合法。
	next := s.now().Add(24 * time.Hour)
	s.Now = func() time.Time { return next }
	o, e := s.Create(ctx, testCreator, grant("overflow_topup", "10"))
	require.NoError(t, e)
	_, e = s.Approve(ctx, testReviewer, review(o.OrderNo))
	require.NoError(t, e)
	_, e = s.Refund(ctx, refundInput("overflow", "10", d.TransactionNo))
	require.ErrorIs(t, e, ErrLimit)
	amount(t, s, strconv.FormatInt(math.MaxInt64, 10))
	count(t, s.DB, &model.WalletTransaction{}, 1)
	c, e := s.Check(ctx, testReviewer, req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: "10000010"}})
	require.NoError(t, e)
	require.True(t, c.Consistent, c.Issues)
	// 版本耗尽也拒绝，不允许环绕成负数。
	require.NoError(t, s.DB.Model(&model.WalletAccount{}).Where("account_id=10").Update("version", int64(math.MaxInt64)).Error)
	_, e = s.Debit(ctx, debitInput("max-version", "1"))
	require.ErrorIs(t, e, ErrInvariant)
}
