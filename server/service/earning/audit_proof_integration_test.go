package earning_test

import (
	"database/sql"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"sync/atomic"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	gm "tb_live_module/model/gift"
	gr "tb_live_module/model/gift/request"
	wr "tb_live_module/model/wallet/request"
	es "tb_live_module/service/earning"
	gs "tb_live_module/service/gift"
	wallet "tb_live_module/service/wallet"
	"testing"
	"time"
)

func TestGiftAuditMemoizationIsSnapshotScoped(t *testing.T) {
	s, g, _ := fixture(t, false)
	rule := exchange(t, s, "3", "2")
	order := send(t, g, "memo_income_order", "3")
	settle(t, s, rule.RuleNo, "memo_settle_initial")
	_, e := g.Refund(ctx, admin, gr.Refund{OrderNo: order.OrderNo, RequestID: "memo_refund_one", Quantity: "1", Reason: "第一次退款"})
	require.NoError(t, e)
	settle(t, s, rule.RuleNo, "memo_settle_refund_one")
	_, e = g.Refund(ctx, admin, gr.Refund{OrderNo: order.OrderNo, RequestID: "memo_refund_two", Quantity: "1", Reason: "第二次退款"})
	require.NoError(t, e)
	settle(t, s, rule.RuleNo, "memo_settle_refund_two")
	var facts []em.Income
	require.NoError(t, s.DB.Order("id").Find(&facts).Error)
	require.Len(t, facts, 3)
	var reads atomic.Int64
	require.NoError(t, s.DB.Callback().Query().After("gorm:query").Register("memo_count", func(*gorm.DB) { reads.Add(1) }))
	for round := 0; round < 2; round++ {
		require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error {
			tx = gs.WithIncomeProofCache(tx)
			reads.Store(0)
			if e := g.VerifyIncomeSources(tx, facts[:1]); e != nil {
				return e
			}
			first := reads.Load()
			require.Greater(t, first, int64(0))
			if e := g.VerifyIncomeSources(tx, facts[1:]); e != nil {
				return e
			}
			require.Equal(t, first, reads.Load(), "same gift's refund proof must reuse this snapshot only")
			bad := facts[1]
			bad.Amount--
			require.ErrorIs(t, g.VerifyIncomeSources(tx, []em.Income{bad}), es.ErrInvariant)
			t.Logf("snapshot %d source queries=%d; cached refund proof queries=0", round+1, first)
			return nil
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}))
	}
}

func TestMySQLEarningAuditAdditiveMigration(t *testing.T) {
	raw := os.Getenv("EARNING_SCALE_MYSQL_DSN")
	if raw == "" {
		t.Skip("isolated local MySQL not configured")
	}
	cfg, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	require.Equal(t, "127.0.0.1:13329", cfg.Addr)
	require.Equal(t, "earning_test_scale", cfg.DBName)
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	defer pool.Close()
	var before int64
	require.NoError(t, db.Model(&em.AccountLedger{}).Count(&before).Error)
	require.NoError(t, initialize.MigrateEarningAudit(db))
	require.NoError(t, initialize.MigrateEarningAudit(db))
	require.NoError(t, initialize.ValidateEarningAuditSchema(db))
	var after int64
	require.NoError(t, db.Model(&em.AccountLedger{}).Count(&after).Error)
	require.Equal(t, before, after)
}

func TestRealGiftAuditSecondPageTampering(t *testing.T) {
	s, g, _ := fixture(t, false)
	s.SourceAuditScope = gs.WithIncomeProofCache
	w := &wallet.Service{DB: s.DB}
	top, e := w.Create(ctx, wallet.Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}, wr.CreateAdjustment{RequestID: "scale_gift_wallet", ExternalOrderNo: "SCALE-GIFT-WALLET", AccountNo: "10000010", Amount: "2000", Reason: "真实礼物分页测试", EvidenceRef: "test:gift-scale"})
	require.NoError(t, e)
	_, e = w.Approve(ctx, wallet.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}, wr.ReviewAdjustment{OrderNo: top.OrderNo, Reason: "受控资金确认"})
	require.NoError(t, e)
	rule := exchange(t, s, "3", "2")
	var first, last gm.Order
	for i := 0; i < 130; i++ {
		order := send(t, g, fmt.Sprintf("real_scale_gift_%03d", i), "1")
		if i == 0 {
			first = order
		}
		last = order
		settle(t, s, rule.RuleNo, fmt.Sprintf("real_scale_settle_%03d", i))
	}
	_, e = g.Refund(ctx, admin, gr.Refund{OrderNo: first.OrderNo, RequestID: "real_scale_refund", Quantity: "1", Reason: "跨页退款"})
	require.NoError(t, e)
	settle(t, s, rule.RuleNo, "real_scale_refund_settle")
	var proofs atomic.Int64
	original := s.Sources["gift"]
	s.Sources["gift"] = func(tx *gorm.DB, facts []em.Income) error { proofs.Add(1); return original(tx, facts) }
	started := time.Now()
	result, e := s.CheckAccount(ctx, financeAdmin, er.FinanceQuery{AnchorNo: "anchor01", Currency: "CNY"})
	require.NoError(t, e)
	require.Equal(t, true, result["consistent"])
	t.Logf("real gift full proof: gifts=130 refund=1 accountVersions=131 sourceBatches=%d duration=%s", proofs.Load(), time.Since(started))
	require.Equal(t, int64(2), proofs.Load())
	require.NoError(t, s.DB.Exec("DROP TRIGGER trg_gift_revenue_ledger_update").Error)
	require.NoError(t, s.DB.Model(&gm.Revenue{}).Where("gift_order_no=?", last.OrderNo).Update("gross_coin_amount", 11).Error)
	proofs.Store(0)
	result, e = s.CheckAccount(ctx, financeAdmin, er.FinanceQuery{AnchorNo: "anchor01", Currency: "CNY"})
	require.NoError(t, e)
	require.Equal(t, false, result["consistent"])
	require.Equal(t, int64(2), proofs.Load(), "tampering must be rejected in the second ledger page")
}
