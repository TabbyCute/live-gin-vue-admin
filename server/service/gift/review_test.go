package gift_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"tb_live_module/config"
	"tb_live_module/global"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	gm "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	res "tb_live_module/model/gift/response"
	lm "tb_live_module/model/live"
	wm "tb_live_module/model/wallet"
	router "tb_live_module/router/gift"
	es "tb_live_module/service/earning"
	svc "tb_live_module/service/gift"
	"tb_live_module/utils"
	"testing"
	"time"
)

func reviewFixes(t *testing.T, mysql bool) {
	t.Run("self_gift_no_financial_facts", func(t *testing.T) {
		s := fixture(t, mysql)
		for _, e := range parallel(24, func(i int) error {
			_, e := s.Send(ctx, 11, "127.0.0.1", "self-test", req.Send{RoomNo: "room0001", GiftNo: "gift0001", Quantity: "1", RequestID: fmt.Sprintf("self_%08d", i)})
			return e
		}) {
			require.ErrorIs(t, e, svc.ErrSelfGift)
		}
		for _, m := range []any{&gm.Order{}, &gm.Revenue{}, &em.Ledger{}, &gm.Event{}, &wm.WalletTransaction{}} {
			count(t, s, m, 0)
		}
		balance(t, s, "1000")
	})
	t.Run("scope_create_review_and_delete", func(t *testing.T) {
		s := fixture(t, mysql)
		for _, kind := range []string{"category", "gift", "anchor"} {
			_, e := s.CreateRule(ctx, admin, er.Rule{Name: "错误范围", ScopeType: kind, ScopeNo: "missing_object", AnchorShareBPS: "1000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
			require.ErrorIs(t, e, svc.ErrNotFound)
		}
		r, e := s.CreateRule(ctx, admin, er.Rule{Name: "主播范围", ScopeType: "anchor", ScopeNo: "anchor01", AnchorShareBPS: "1000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
		require.NoError(t, e)
		require.NoError(t, s.DB.Delete(&lm.LiveAnchor{}, 1).Error)
		_, e = s.ReviewRule(ctx, reviewer, er.Review{RuleNo: r.RuleNo, Approve: true})
		require.ErrorIs(t, e, svc.ErrNotFound)
		_, e = s.CreateRule(ctx, admin, er.Rule{Name: "礼物范围", ScopeType: "gift", ScopeNo: "gift0001", AnchorShareBPS: "1000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
		require.NoError(t, e)
		require.ErrorIs(t, s.Delete(ctx, admin, req.Delete{Kind: "item", No: "gift0001", Version: "1"}), svc.ErrConflict)
		_, e = s.SaveCategory(ctx, admin, req.Category{CategoryNo: "reserved", Name: "保留分类", Status: "enabled"})
		require.NoError(t, e)
		_, e = s.CreateRule(ctx, admin, er.Rule{Name: "分类范围", ScopeType: "category", ScopeNo: "reserved", AnchorShareBPS: "1000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
		require.NoError(t, e)
		require.ErrorIs(t, s.Delete(ctx, admin, req.Delete{Kind: "category", No: "reserved", Version: "1"}), svc.ErrConflict)
	})
	t.Run("dual_review_termination", func(t *testing.T) {
		s := fixture(t, mysql)
		o, e := send(s, "before_termination", "2")
		require.NoError(t, e)
		r := er.TerminateRule{RuleNo: o.RevenueRuleNo, RequestID: "terminate_request", Reason: "纠正错误规则"}
		_, e = s.TerminateRule(ctx, svc.Actor{ID: 10, AuthorityID: 888}, r)
		require.ErrorIs(t, e, svc.ErrForbidden)
		decision, e := s.TerminateRule(ctx, admin, r)
		require.NoError(t, e)
		for _, e := range parallel(12, func(int) error {
			same, e := s.TerminateRule(ctx, admin, r)
			if e == nil && same.TerminationNo != decision.TerminationNo {
				return errors.New("duplicate termination")
			}
			return e
		}) {
			require.NoError(t, e)
		}
		_, e = s.ReviewTermination(ctx, admin, er.ReviewTermination{TerminationNo: decision.TerminationNo, Approve: true})
		require.ErrorIs(t, e, svc.ErrConflict)
		require.Error(t, s.DB.Model(&em.RuleTermination{}).Where("id=?", decision.ID).Updates(map[string]any{"status": "approved", "reviewed_by": admin.ID, "reviewed_at": time.Now().UnixMilli()}).Error)
		_, e = send(s, "pending_termination", "1")
		require.NoError(t, e)
		decision, e = s.ReviewTermination(ctx, reviewer, er.ReviewTermination{TerminationNo: decision.TerminationNo, Approve: true})
		require.NoError(t, e)
		_, e = s.ReviewTermination(ctx, reviewer, er.ReviewTermination{TerminationNo: decision.TerminationNo, Approve: true})
		require.NoError(t, e)
		_, e = send(s, "after_termination", "1")
		require.ErrorIs(t, e, es.ErrNoRule)
		_, e = s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, RequestID: "termination_refund", Quantity: "1", Reason: "历史快照退款"})
		require.NoError(t, e)
		d, e := s.Detail(ctx, admin, req.Query{No: o.OrderNo})
		require.NoError(t, e)
		require.True(t, d.Consistent, d.Issues)
		list, e := s.Rules(ctx, admin, req.Query{})
		require.NoError(t, e)
		require.Equal(t, "approved", list.List.([]res.Rule)[0].Termination.Status)
		newRule, e := s.CreateRule(ctx, admin, er.Rule{Name: "更正规则", ScopeType: "global", AnchorShareBPS: "3000", EffectiveFrom: decision.ReviewedAt, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
		require.NoError(t, e)
		_, e = s.ReviewRule(ctx, reviewer, er.Review{RuleNo: newRule.RuleNo, Approve: true})
		require.NoError(t, e)
		next, e := send(s, "replacement_rule", "1")
		require.NoError(t, e)
		require.Equal(t, int64(3000), next.AnchorShareBPS)
		old, e := s.Result(ctx, 10, req.Query{No: o.OrderNo})
		require.NoError(t, e)
		require.Equal(t, int64(4000), old.AnchorShareBPS)
		require.Error(t, s.DB.Model(&em.RuleTermination{}).Where("id=?", decision.ID).Update("reason", "rewrite").Error)
	})
	t.Run("compensation_retry_and_upgrade", func(t *testing.T) {
		s := fixture(t, mysql)
		// Reconstruct the previous table definition, with a persisted pending row.
		require.NoError(t, s.DB.Migrator().DropTable(&gm.BenefitCompensation{}))
		legacyDB := s.DB
		if mysql {
			legacyDB = s.DB.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
		}
		require.NoError(t, legacyDB.AutoMigrate(&legacyCompensation{}))
		legacy := legacyCompensation{CompensationNo: "legacy_comp", RefundNo: "legacy_refund", GiftOrderNo: "legacy_order", ReceiverUserID: 10, AnchorID: 1, DurationDays: 2, Status: "pending", CreatedAt: time.Now().UnixMilli()}
		require.NoError(t, s.DB.Create(&legacy).Error)
		body := "BEGIN SELECT RAISE(ABORT,'gift financial fact immutable'); END"
		if mysql {
			body = "FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='gift financial fact immutable'"
		}
		require.NoError(t, s.DB.Exec("CREATE TRIGGER trg_gift_benefit_compensation_update BEFORE UPDATE ON gift_benefit_compensation "+body).Error)
		require.Error(t, initialize.ValidateGiftSchema(s.DB))
		require.NoError(t, initialize.MigrateGift(s.DB))
		require.NoError(t, initialize.MigrateGift(s.DB))
		var upgraded gm.BenefitCompensation
		require.NoError(t, s.DB.Where("compensation_no=?", legacy.CompensationNo).Take(&upgraded).Error)
		require.Equal(t, "pending", upgraded.Status)
		require.Equal(t, int64(2), upgraded.DurationDays)
		require.Zero(t, upgraded.RetryCount)
		c := gm.BenefitCompensation{CompensationNo: "comp_test", RefundNo: "refund_test", GiftOrderNo: "order_test", ReceiverUserID: 10, AnchorID: 1, DurationDays: 3, Status: "pending", CreatedAt: time.Now().UnixMilli()}
		require.NoError(t, s.DB.Create(&c).Error)
		require.ErrorIs(t, s.ProcessBenefitCompensation(ctx, c.CompensationNo, nil), svc.ErrGuard)
		require.Error(t, s.DB.Model(&c).Update("status", "succeeded").Error)
		var calls atomic.Int32
		provider := &guardFixture{compensate: func(context.Context, string, string, int64) error {
			if calls.Add(1) == 1 {
				return errors.New("remote unavailable sensitive-data")
			}
			return nil
		}}
		require.Error(t, s.ProcessBenefitCompensation(ctx, c.CompensationNo, provider))
		require.NoError(t, s.DB.First(&c, c.ID).Error)
		require.Equal(t, "failed", c.Status)
		require.Equal(t, int64(1), c.RetryCount)
		require.NotContains(t, c.LastError, "sensitive-data")
		require.NoError(t, s.ProcessBenefitCompensation(ctx, c.CompensationNo, provider))
		require.NoError(t, s.ProcessBenefitCompensation(ctx, c.CompensationNo, provider))
		require.Equal(t, int32(2), calls.Load())
		require.NoError(t, s.DB.First(&c, c.ID).Error)
		require.Equal(t, "succeeded", c.Status)
		require.Positive(t, c.CompletedAt)
		require.Error(t, s.DB.Model(&c).Update("duration_days", 999).Error)
		require.Error(t, s.DB.Model(&c).Update("status", "pending").Error)
		require.Error(t, s.DB.Delete(&c).Error)
		// A crashed worker's expired lease can be reclaimed; its later callback is fenced out.
		started, release := make(chan struct{}), make(chan struct{})
		result := make(chan error, 1)
		blocking := &guardFixture{compensate: func(context.Context, string, string, int64) error { close(started); <-release; return nil }}
		go func() { result <- s.ProcessBenefitCompensation(ctx, legacy.CompensationNo, blocking) }()
		<-started
		require.ErrorIs(t, s.ProcessBenefitCompensation(ctx, legacy.CompensationNo, provider), svc.ErrConflict)
		require.NoError(t, s.DB.Where("compensation_no=?", legacy.CompensationNo).Take(&upgraded).Error)
		reclaimed := *s
		future := time.UnixMilli(upgraded.LeaseUntil + 1)
		reclaimed.Now = func() time.Time { return future }
		require.NoError(t, reclaimed.ProcessBenefitCompensation(ctx, legacy.CompensationNo, provider))
		close(release)
		require.ErrorIs(t, <-result, svc.ErrConflict)
		require.NoError(t, s.DB.Where("compensation_no=?", legacy.CompensationNo).Take(&upgraded).Error)
		require.Equal(t, "succeeded", upgraded.Status)
		require.Equal(t, int64(2), upgraded.RetryCount)
	})
	t.Run("same_totals_wrong_refund_links", func(t *testing.T) {
		s := fixture(t, mysql)
		o, e := send(s, "linked_order", "3")
		require.NoError(t, e)
		first, e := s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: "linked_refund_1", Reason: "逐笔核验"})
		require.NoError(t, e)
		second, e := s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: "linked_refund_2", Reason: "逐笔核验"})
		require.NoError(t, e)
		// Only this isolated fixture removes the gift trigger to simulate historical corruption.
		require.NoError(t, s.DB.Exec("DROP TRIGGER trg_gift_refund_order_update").Error)
		for _, field := range []string{"wallet_refund_transaction_no", "wallet_refund_ledger_no", "earning_reversal_no"} {
			var values []string
			require.NoError(t, s.DB.Model(&gm.Refund{}).Order("id ASC").Pluck(field, &values).Error)
			// Three updates also work if the references have unique constraints.
			require.NoError(t, s.DB.Model(&gm.Refund{}).Where("id=?", first.ID).Update(field, "temporary_reference").Error)
			require.NoError(t, s.DB.Model(&gm.Refund{}).Where("id=?", second.ID).Update(field, values[0]).Error)
			require.NoError(t, s.DB.Model(&gm.Refund{}).Where("id=?", first.ID).Update(field, values[1]).Error)
			d, e := s.Detail(ctx, admin, req.Query{No: o.OrderNo})
			require.NoError(t, e)
			require.False(t, d.Consistent, field)
			require.NotEmpty(t, d.Issues)
			_, e = send(s, "linked_order", "3")
			require.ErrorIs(t, e, svc.ErrInvariant)
			_, e = s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: "blocked_refund", Reason: "不得在异常上继续记账"})
			require.ErrorIs(t, e, svc.ErrInvariant)
			require.NoError(t, s.DB.Model(&gm.Refund{}).Where("id=?", first.ID).Update(field, "temporary_reference").Error)
			require.NoError(t, s.DB.Model(&gm.Refund{}).Where("id=?", second.ID).Update(field, values[1]).Error)
			require.NoError(t, s.DB.Model(&gm.Refund{}).Where("id=?", first.ID).Update(field, values[0]).Error)
		}
		balance(t, s, "990")
	})
	t.Run("catalog_seed_idempotent", func(t *testing.T) {
		s := fixture(t, mysql)
		require.NoError(t, initialize.SeedGiftCatalog(s.DB))
		require.NoError(t, initialize.SeedGiftCatalog(s.DB))
		count(t, s, &gm.Category{}, 4)
		count(t, s, &gm.Item{}, 7)
		var seeded gm.Item
		require.NoError(t, s.DB.Where("gift_no='basic_rose'").Take(&seeded).Error)
		require.Equal(t, "draft", seeded.Status)
		list, e := s.Catalog(ctx, 10, nil, "item", req.Query{})
		require.NoError(t, e)
		require.Equal(t, int64(1), list.Total)
		require.NoError(t, s.DB.Model(&seeded).Update("coin_price", 17).Error)
		require.NoError(t, initialize.SeedGiftCatalog(s.DB))
		require.NoError(t, s.DB.First(&seeded, seeded.ID).Error)
		require.Equal(t, int64(17), seeded.CoinPrice)
	})
}

type guardFixture struct {
	compensate func(context.Context, string, string, int64) error
}

type legacyCompensation struct {
	ID             uint64 `gorm:"primaryKey"`
	CompensationNo string `gorm:"size:40;not null;uniqueIndex"`
	RefundNo       string `gorm:"size:40;not null;uniqueIndex"`
	GiftOrderNo    string `gorm:"size:40;not null;index"`
	ReceiverUserID uint64 `gorm:"not null"`
	AnchorID       uint64 `gorm:"not null"`
	DurationDays   int64  `gorm:"not null"`
	Status         string `gorm:"size:20;not null"`
	CreatedAt      int64  `gorm:"not null"`
}

func (legacyCompensation) TableName() string { return "gift_benefit_compensation" }

func (*guardFixture) Grant(context.Context, string, uint64, uint64, string, int64) error {
	return errors.New("unused test grant")
}
func (g *guardFixture) Compensate(c context.Context, n, o string, d int64) error {
	return g.compensate(c, n, o, d)
}

func TestGiftReviewFixes(t *testing.T)      { reviewFixes(t, false) }
func TestMySQLGiftReviewFixes(t *testing.T) { reviewFixes(t, true) }

func TestAppOrderHTTPAllowlist(t *testing.T) {
	s := fixture(t, false)
	gin.SetMode(gin.TestMode)
	oldDB, oldConfig, oldService, oldLog := global.GVA_DB, global.GVA_CONFIG, *svc.Default, global.GVA_LOG
	global.GVA_DB = s.DB
	*svc.Default = *s
	global.GVA_LOG = zap.NewNop()
	global.GVA_CONFIG.JWTApp = config.JWTApp{SigningKey: "gift-review-app-key-only", Issuer: "gift-test", ExpiresTime: "1h"}
	t.Cleanup(func() {
		global.GVA_DB = oldDB
		global.GVA_CONFIG = oldConfig
		*svc.Default = oldService
		global.GVA_LOG = oldLog
	})
	token, _, e := utils.NewAppJWT().CreateToken(10, "user10", 1)
	require.NoError(t, e)
	other, _, e := utils.NewAppJWT().CreateToken(11, "user11", 1)
	require.NoError(t, e)
	r := gin.New()
	router.RouterGroup{}.InitGiftRouter(r.Group("/api/v1/app"))
	call := func(method, path, body, token string) map[string]any {
		request := httptest.NewRequest(method, "/api/v1/app/gift/"+path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		require.Equal(t, 200, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		var out map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out
	}
	check := func(out map[string]any) string {
		require.Equal(t, float64(0), out["code"])
		data := out["data"].(map[string]any)
		if list, ok := data["list"]; ok {
			data = list.([]any)[0].(map[string]any)
		}
		keys := []string{"orderNo", "requestId", "anchorNo", "roomNo", "sessionNo", "categoryNo", "categoryName", "giftNo", "giftName", "iconUrl", "animationUrl", "animationType", "unitPrice", "quantity", "grossCoinAmount", "refundedCoinAmount", "status", "createdAt"}
		require.Len(t, data, len(keys))
		for _, key := range keys {
			require.Contains(t, data, key)
		}
		require.IsType(t, "", data["grossCoinAmount"])
		require.NotEmpty(t, data["orderNo"])
		return data["orderNo"].(string)
	}
	body := `{"roomNo":"room0001","giftNo":"gift0001","quantity":"1","requestId":"http_public_order"}`
	no := check(call("POST", "send", body, token))
	check(call("POST", "send", body, token))
	check(call("GET", "result?no="+no, "", token))
	check(call("GET", "result?requestId=http_public_order", "", token))
	check(call("GET", "order/list", "", token))
	require.Equal(t, float64(48006), call("GET", "result?no="+no, "", other)["code"])
	require.Equal(t, float64(48012), call("POST", "send", body, other)["code"])
	adminList, e := s.Orders(ctx, 0, &admin, req.Query{})
	require.NoError(t, e)
	require.NotEmpty(t, adminList.List.([]gm.Order)[0].WalletTransactionNo)
}
