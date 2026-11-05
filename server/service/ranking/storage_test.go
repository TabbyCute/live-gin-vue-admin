package ranking_test

import (
	"context"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/global"
	"tb_live_module/initialize"
	gift "tb_live_module/model/gift"
	live "tb_live_module/model/live"
	model "tb_live_module/model/ranking"
	req "tb_live_module/model/ranking/request"
	system "tb_live_module/model/system"
	svc "tb_live_module/service/ranking"
	"testing"
	"time"
)

func TestProjectionLateCommitLargeTotalsRebuildAndTies(t *testing.T) {
	ctx := context.Background()
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/rank.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	// Synthetic immutable source fixtures test projection math, not wallet correctness.
	require.NoError(t, db.AutoMigrate(&gift.Order{}, &gift.Revenue{}, &live.LiveAccount{}, &live.LiveAnchor{}, &system.SysUser{}))
	require.NoError(t, initialize.MigrateRanking(db, "Asia/Kuala_Lumpur"))
	require.NoError(t, db.Create(&system.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 1}, Enable: 1}).Error)
	for i := uint(1); i <= 2; i++ {
		require.NoError(t, db.Create(&live.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: i}, AccountNo: fmt.Sprintf("u%d", i), Username: fmt.Sprintf("u%d", i), Status: 1}).Error)
	}
	at := time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC).UnixMilli()
	s := &svc.Service{DB: db, Timezone: "Asia/Kuala_Lumpur", Now: func() time.Time { return time.UnixMilli(at + 86400000) }}
	insert := func(id uint64, user string, amount int64) {
		order := gift.Order{OrderNo: fmt.Sprintf("o%d", id), RequestID: fmt.Sprintf("request%d", id), SenderUserID: 1, SenderAccountNo: user, AnchorNo: "anchor", RoomNo: "room", SessionNo: "session", CategoryNo: "category", GiftNo: "gift", UnitPrice: amount, Quantity: 1, GrossCoinAmount: amount, Status: "succeeded", WalletTransactionNo: fmt.Sprintf("wt%d", id), WalletLedgerNo: fmt.Sprintf("wl%d", id), CreatedAt: at}
		require.NoError(t, db.Create(&order).Error)
		require.NoError(t, db.Create(&gift.Revenue{ID: id, RevenueNo: fmt.Sprintf("r%d", id), GiftOrderNo: order.OrderNo, BusinessNo: order.OrderNo, EntryType: "gift", GrossCoinAmount: amount, Quantity: 1, CreatedAt: at}).Error)
	}
	amount := int64(9223372036854775807)
	insert(100, "u1", amount)
	n, e := s.Consume(ctx, 20)
	require.NoError(t, e)
	require.Equal(t, 1, n)
	insert(10, "u1", amount)
	insert(101, "u2", amount)
	insert(102, "u2", amount)
	n, e = s.Consume(ctx, 20)
	require.NoError(t, e)
	require.Equal(t, 3, n)
	_, e = s.Consume(ctx, 20)
	require.NoError(t, e)
	board, e := s.Board(ctx, 2, "wealth", req.Query{Period: "all", PageSize: 1})
	require.NoError(t, e)
	require.Len(t, board.List, 1)
	require.Equal(t, "u1", board.List[0].SubjectNo)
	require.Equal(t, "18446744073709551614", board.List[0].Score)
	require.Equal(t, "2", board.MyRank.Rank)
	// Refund is attributed to the original date, not its own next-month timestamp.
	require.NoError(t, db.Create(&gift.Revenue{ID: 103, RevenueNo: "refund103", GiftOrderNo: "o10", BusinessNo: "refund03", EntryType: "refund", GrossCoinAmount: -amount, Quantity: -1, CreatedAt: at + 3*86400000}).Error)
	_, e = s.Consume(ctx, 20)
	require.NoError(t, e)
	day, e := s.Board(ctx, 1, "wealth", req.Query{Period: "day", PeriodKey: "2026-10-31"})
	require.NoError(t, e)
	require.Equal(t, "u2", day.List[0].SubjectNo)
	require.Equal(t, "9223372036854775807", day.MyRank.Score)
	next, e := s.Board(ctx, 1, "wealth", req.Query{Period: "month", PeriodKey: "2026-11"})
	require.NoError(t, e)
	require.Empty(t, next.List)
	out, e := s.AdminAction(ctx, svc.Actor{ID: 1}, "rebuild", req.Action{RequestID: "rebuild_rank_test", Scope: "all"})
	require.NoError(t, e)
	require.Contains(t, out, "running")
	for i := 0; i < 10; i++ {
		n, e = s.Consume(ctx, 2)
		require.NoError(t, e)
		if n == 0 {
			break
		}
	}
	proof, e := s.ReconcileSnapshot(ctx)
	require.NoError(t, e)
	require.Equal(t, true, proof.(map[string]any)["consistent"])
	// No event is lost or duplicated when processing the same source again.
	var count int64
	require.NoError(t, db.Model(&model.AppliedEvent{}).Count(&count).Error)
	require.Equal(t, int64(5), count)
	require.Error(t, db.Model(&model.AppliedEvent{}).Where("1=1").Update("delta_coin", 1).Error)
}
