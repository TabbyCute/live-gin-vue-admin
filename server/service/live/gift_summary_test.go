package live

import (
	"encoding/json"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sync/atomic"
	"tb_live_module/global"
	model "tb_live_module/model/gift"
	res "tb_live_module/model/live/response"
	"testing"
)

func TestGiftSummaryAuthorizationAndBatch(t *testing.T) {
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/summary.db"), &gorm.Config{})
	require.NoError(t, e)
	pool, _ := db.DB()
	defer pool.Close()
	require.NoError(t, db.AutoMigrate(&model.Order{}))
	old := global.GiftFactsReady.Swap(true)
	defer global.GiftFactsReady.Store(old)
	for i := uint64(1); i <= 2; i++ {
		o := model.Order{OrderNo: string(rune('a' + i)), RequestID: string(rune('a' + i)), SenderUserID: i, SessionID: 1, UnitPrice: 3, Quantity: 2, GrossCoinAmount: 6, RefundedCoinAmount: 3, AnchorShareBPS: 4000, WalletTransactionNo: string(rune('a' + i)), WalletLedgerNo: string(rune('a' + i)), Status: "partial_refund"}
		require.NoError(t, db.Create(&o).Error)
	}
	hidden, e := authorizedGiftSummary(db, "session_id", 1, nil)
	require.NoError(t, e)
	require.Nil(t, hidden)
	var queries atomic.Int64
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("count_gift_aggregate", func(*gorm.DB) { queries.Add(1) }))
	rows, e := authorizedGiftSummaries(db, "session_id", []uint{1, 2}, []bool{true})
	require.NoError(t, e)
	require.Equal(t, int64(1), queries.Load(), "one aggregate for the entire page")
	require.Equal(t, "12", rows[1].GrossCoin)
	require.Equal(t, "6", rows[1].RefundCoin)
	require.Equal(t, "6", rows[1].NetCoin)
	require.Equal(t, "2", rows[1].PendingEarningCoin)
	require.Equal(t, "2", rows[1].Users)
	require.Equal(t, "4", rows[1].GrossQuantity)
	require.Equal(t, "2", rows[1].RefundQuantity)
	require.Equal(t, "2", rows[1].Quantity)
	require.Equal(t, "0", rows[2].NetCoin)
	require.Equal(t, "0", rows[2].GrossQuantity)
	queries.Store(0)
	hidden, e = authorizedGiftSummary(db, "session_id", 1, []bool{false})
	require.NoError(t, e)
	require.Nil(t, hidden)
	require.Zero(t, queries.Load(), "unauthorized reads do not execute financial aggregation")
	_, e = giftSummaries(db, "session_id; DROP TABLE gift_order", []uint{1})
	require.Error(t, e)
	// Full refunds remain in gross statistics but no longer count as effective gifts/users.
	require.NoError(t, db.Model(&model.Order{}).Where("session_id=1").Update("refunded_coin_amount", 6).Error)
	full, e := giftSummary(db, "session_id", 1)
	require.NoError(t, e)
	require.Equal(t, "4", full.GrossQuantity)
	require.Equal(t, "4", full.RefundQuantity)
	require.Equal(t, "0", full.Quantity)
	require.Equal(t, "0", full.Users)
	encoded, e := json.Marshal(res.LiveSessionAdminItem{})
	require.NoError(t, e)
	require.NotContains(t, string(encoded), "giftSummary", "missing permission is not reported as a zero balance")
	public, e := json.Marshal(res.LiveRoomPublicItemV2{LatestSession: res.LiveSessionPublicSummary{SessionNo: "session-public"}})
	require.NoError(t, e)
	for _, field := range []string{"giftSummary", "grossCoin", "grossQuantity", "refundQuantity", "pendingEarningCoin"} {
		require.NotContains(t, string(public), field, "public room/session DTO must not expose admin financial statistics")
	}
}
