package gift_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"sort"
	"strings"
	"sync"
	"tb_live_module/global"
	"tb_live_module/initialize"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	gm "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	res "tb_live_module/model/gift/response"
	lm "tb_live_module/model/live"
	sm "tb_live_module/model/social"
	sys "tb_live_module/model/system"
	wm "tb_live_module/model/wallet"
	wr "tb_live_module/model/wallet/request"
	es "tb_live_module/service/earning"
	svc "tb_live_module/service/gift"
	wallet "tb_live_module/service/wallet"
	"testing"
	"time"
)

var ctx = context.Background()
var admin = svc.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}
var reviewer = svc.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}

func fixture(t *testing.T, isMySQL bool) *svc.Service {
	t.Helper()
	var db *gorm.DB
	var e error
	if isMySQL {
		raw := os.Getenv("GIFT_TEST_MYSQL_DSN")
		if raw == "" {
			t.Skip("GIFT_TEST_MYSQL_DSN not configured")
		}
		cfg, e := driver.ParseDSN(raw)
		require.NoError(t, e)
		require.Equal(t, "127.0.0.1:13327", cfg.Addr)
		require.True(t, strings.HasPrefix(cfg.DBName, "gift_test_"))
		cfg.ParseTime = true
		db, e = gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
		// The fixture refuses every address except the dedicated local disposable container.
		var tables []string
		require.NoError(t, db.Raw("SHOW TABLES").Scan(&tables).Error)
		for _, table := range tables {
			require.NoError(t, db.Exec("DROP TABLE `"+table+"`").Error)
		}
	} else {
		db, e = gorm.Open(sqlite.Open("file:"+t.TempDir()+"/gift.db"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
	}
	pool, e := db.DB()
	require.NoError(t, e)
	if !isMySQL {
		pool.SetMaxOpenConns(1)
	} else {
		pool.SetMaxOpenConns(32)
	}
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&lm.LiveAccount{}, &lm.LiveAnchor{}, &lm.LiveRoom{}, &lm.LiveSession{}, &sm.SocialFollow{}, &sm.SocialBlock{}, &sys.SysUser{}, &sys.SysBaseMenu{}, &sys.SysAuthorityMenu{}, &sys.SysApi{}))
	require.NoError(t, initialize.MigrateWallet(db))
	require.NoError(t, initialize.MigrateGift(db))
	require.NoError(t, initialize.MigrateGift(db))
	require.NoError(t, db.Create(&sys.SysBaseMenu{Name: "liveManagement", Path: "liveManagement"}).Error)
	require.NoError(t, initialize.SyncGiftMetadata(db))
	require.NoError(t, initialize.SyncGiftMetadata(db))
	require.NoError(t, db.Create(&sys.SysAuthority{AuthorityId: 888, AuthorityName: "测试超级管理员"}).Error)
	for _, id := range []uint{1, 2, 3} {
		require.NoError(t, db.Create(&sys.SysUser{GVA_MODEL: global.GVA_MODEL{ID: id}, Username: fmt.Sprintf("admin%d", id), Enable: 1, AuthorityId: 888}).Error)
		require.NoError(t, db.Exec("INSERT INTO sys_user_authority(sys_user_id,sys_authority_authority_id) VALUES (?,888)", id).Error)
	}
	for _, id := range []uint{10, 11} {
		require.NoError(t, db.Create(&lm.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: fmt.Sprintf("100000%d", id), Phone: ptr(fmt.Sprintf("+601000000%d", id)), Username: fmt.Sprintf("user%d", id), Nickname: "Sender", Status: 1}).Error)
	}
	anchor := lm.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: 1}, UserId: 11, AnchorNo: "anchor01", Status: 1, ApplyStatus: 2, LivePermission: 1}
	require.NoError(t, db.Create(&anchor).Error)
	room := lm.LiveRoom{GVA_MODEL: global.GVA_MODEL{ID: 1}, RoomNo: "room0001", AnchorId: 1, Status: 1, LiveStatus: 2, CurrentSessionId: 1, Visibility: 1, StreamName: "teststream"}
	require.NoError(t, db.Create(&room).Error)
	session := lm.LiveSession{GVA_MODEL: global.GVA_MODEL{ID: 1}, SessionNo: "session01", RoomId: 1, AnchorId: 1, Status: 1}
	require.NoError(t, db.Create(&session).Error)
	w := &wallet.Service{DB: db}
	s := &svc.Service{DB: db, Wallet: w}
	o, e := w.Create(ctx, wallet.Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}, wr.CreateAdjustment{RequestID: "seed_wallet", ExternalOrderNo: "GIFT-SEED", AccountNo: "10000010", Amount: "1000", Reason: "测试钱包初始资金", EvidenceRef: "test:controlled-fixture"})
	require.NoError(t, e)
	_, e = w.Approve(ctx, wallet.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}, wr.ReviewAdjustment{OrderNo: o.OrderNo, Reason: "核验测试资金"})
	require.NoError(t, e)
	_, e = s.SaveCategory(ctx, admin, req.Category{CategoryNo: "category1", Name: "鲜花", Status: "enabled"})
	require.NoError(t, e)
	_, e = s.SaveItem(ctx, admin, item())
	require.NoError(t, e)
	r, e := s.CreateRule(ctx, admin, er.Rule{Name: "测试分成", ScopeType: "global", AnchorShareBPS: "4000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(24 * time.Hour).UnixMilli()})
	require.NoError(t, e)
	_, e = s.ReviewRule(ctx, reviewer, er.Review{RuleNo: r.RuleNo, Approve: true})
	require.NoError(t, e)
	return s
}
func item() req.Item {
	return req.Item{GiftNo: "gift0001", CategoryNo: "category1", Name: "玫瑰", IconURL: "https://cdn.example.com/rose.png", AnimationType: "none", CoinPrice: "10", Status: "enabled", MaxQuantityPerOrder: "100", BenefitType: "none", BenefitStatus: "disabled"}
}
func send(s *svc.Service, key, quantity string) (gm.Order, error) {
	return s.Send(ctx, 10, "127.0.0.1", "trace_fixture", req.Send{RoomNo: "room0001", GiftNo: "gift0001", Quantity: quantity, RequestID: key})
}
func count(t *testing.T, s *svc.Service, m any, want int64) {
	var n int64
	require.NoError(t, s.DB.Model(m).Count(&n).Error)
	require.Equal(t, want, n)
}
func balance(t *testing.T, s *svc.Service, want string) {
	b, e := s.Wallet.(*wallet.Service).Balance(ctx, 10, "")
	require.NoError(t, e)
	require.Equal(t, want, b.AvailableBalance)
}
func lifecycle(t *testing.T, s *svc.Service) {
	o, e := send(s, "order0001", "3")
	require.NoError(t, e)
	balance(t, s, "970")
	same, e := send(s, "order0001", "3")
	require.NoError(t, e)
	require.Equal(t, o.OrderNo, same.OrderNo)
	_, e = send(s, "order0001", "2")
	require.ErrorIs(t, e, svc.ErrConflict)
	_, e = s.Result(ctx, 11, req.Query{No: o.OrderNo})
	require.ErrorIs(t, e, svc.ErrNotFound)
	// A current price change cannot alter an earlier result or the refund unit price.
	r := item()
	r.Version = "1"
	r.CoinPrice = "999"
	updated, updateErr := s.SaveItem(ctx, admin, r)
	require.NoError(t, updateErr)
	require.Greater(t, updated.UpdatedAt, int64(1700000000000))
	e = updateErr
	require.NoError(t, e)
	same, e = send(s, "order0001", "3")
	require.NoError(t, e)
	require.Equal(t, int64(10), same.UnitPrice)
	refund := req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: "refund01", Reason: "测试部分退款"}
	f, e := s.Refund(ctx, admin, refund)
	require.NoError(t, e)
	require.Equal(t, int64(10), f.RefundCoinAmount)
	again, e := s.Refund(ctx, admin, refund)
	require.NoError(t, e)
	require.Equal(t, f.RefundNo, again.RefundNo)
	d, e := s.Detail(ctx, admin, req.Query{No: o.OrderNo})
	require.NoError(t, e)
	require.True(t, d.Consistent, d.Issues)
	require.Len(t, d.Events, 1)
	require.Equal(t, "pending", d.Events[0].DeliveryStatus)
	ranking, e := s.Ranking(ctx, admin, req.Query{Period: "day", Dimension: "sender"})
	require.NoError(t, e)
	rows := ranking.List.([]res.Ranking)
	require.Len(t, rows, 1)
	require.Equal(t, "20", rows[0].Net)
	require.Equal(t, "8", rows[0].Earning)
	refund.Quantity = "3"
	refund.RequestID = "refund02"
	_, e = s.Refund(ctx, admin, refund)
	require.ErrorIs(t, e, svc.ErrRefund)
	refund.Quantity = "2"
	_, e = s.Refund(ctx, admin, refund)
	require.NoError(t, e)
	balance(t, s, "1000")
	d, e = s.Detail(ctx, admin, req.Query{No: o.OrderNo})
	require.NoError(t, e)
	require.True(t, d.Consistent, d.Issues)
	require.Error(t, s.DB.Model(&gm.Order{}).Where("order_no=?", o.OrderNo).Update("gross_coin_amount", 1).Error)
	require.Error(t, s.DB.Where("order_no=?", o.OrderNo).Delete(&gm.Order{}).Error)
	require.Error(t, s.DB.Model(&em.Ledger{}).Where("gift_order_no=?", o.OrderNo).Update("earning_coin_amount", 5).Error)
}
func TestGiftLifecycle(t *testing.T)      { lifecycle(t, fixture(t, false)) }
func TestMySQLGiftLifecycle(t *testing.T) { lifecycle(t, fixture(t, true)) }
func parallel(n int, fn func(int) error) []error {
	out := make([]error, n)
	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func(i int) { defer wg.Done(); out[i] = fn(i) }(i)
	}
	wg.Wait()
	return out
}
func concurrency(t *testing.T, s *svc.Service) {
	for _, e := range parallel(48, func(int) error { _, e := send(s, "same_key", "20"); return e }) {
		require.NoError(t, e)
	}
	count(t, s, &gm.Order{}, 1)
	count(t, s, &gm.Event{}, 1)
	balance(t, s, "800")
	success := 0
	for _, e := range parallel(40, func(i int) error { _, e := send(s, fmt.Sprintf("compete%04d", i), "10"); return e }) {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, wallet.ErrInsufficient)
		}
	}
	require.Equal(t, 8, success)
	balance(t, s, "0")
	o, e := send(s, "same_key", "20")
	require.NoError(t, e)
	success = 0
	for _, e := range parallel(40, func(i int) error {
		_, e := s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: fmt.Sprintf("refund%04d", i), Reason: "并发退款测试"})
		return e
	}) {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, svc.ErrRefund)
		}
	}
	require.Equal(t, 20, success)
	balance(t, s, "200")
	d, e := s.Detail(ctx, admin, req.Query{No: o.OrderNo})
	require.NoError(t, e)
	require.True(t, d.Consistent, d.Issues)
	check, e := s.Wallet.(*wallet.Service).Check(ctx, wallet.Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}, wr.CheckSearch{AccountQuery: wr.AccountQuery{AccountNo: "10000010"}, Limit: 1000})
	require.NoError(t, e)
	require.True(t, check.Consistent, check.Issues)
}
func TestGiftConcurrentSQLite(t *testing.T) { concurrency(t, fixture(t, false)) }
func TestMySQLGiftConcurrency(t *testing.T) { concurrency(t, fixture(t, true)) }
func rollback(t *testing.T, s *svc.Service) {
	for _, table := range []string{"gift_order", "gift_revenue_ledger", "anchor_earning_ledger", "gift_event_outbox"} {
		sql := "CREATE TRIGGER injected_failure BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'failure'); END"
		if s.DB.Dialector.Name() == "mysql" {
			sql = "CREATE TRIGGER injected_failure BEFORE INSERT ON " + table + " FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='failure'"
		}
		require.NoError(t, s.DB.Exec(sql).Error)
		_, e := send(s, "rollback1", "2")
		require.Error(t, e)
		balance(t, s, "1000")
		count(t, s, &gm.Order{}, 0)
		count(t, s, &wm.WalletTransaction{}, 0)
		count(t, s, &gm.Event{}, 0)
		require.NoError(t, s.DB.Exec("DROP TRIGGER injected_failure").Error)
	}
	o, e := send(s, "rollback1", "2")
	require.NoError(t, e)
	for _, table := range []string{"gift_refund_order", "gift_audit_log", "anchor_earning_ledger", "gift_revenue_ledger"} {
		sql := "CREATE TRIGGER injected_failure BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'failure'); END"
		if s.DB.Dialector.Name() == "mysql" {
			sql = "CREATE TRIGGER injected_failure BEFORE INSERT ON " + table + " FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='failure'"
		}
		require.NoError(t, s.DB.Exec(sql).Error)
		_, e = s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: "rollback_refund", Reason: "故障退款测试"})
		require.Error(t, e)
		balance(t, s, "980")
		count(t, s, &gm.Refund{}, 0)
		require.NoError(t, s.DB.Exec("DROP TRIGGER injected_failure").Error)
	}
}
func TestGiftRollback(t *testing.T)      { rollback(t, fixture(t, false)) }
func TestMySQLGiftRollback(t *testing.T) { rollback(t, fixture(t, true)) }
func TestGiftValidationAndPermissions(t *testing.T) {
	s := fixture(t, false)
	for _, v := range []string{"0", "-1", "01", "1.1", "1e2", "10001", "9223372036854775808"} {
		_, e := send(s, "invalid1", v)
		require.ErrorIs(t, e, svc.ErrInvalid)
	}
	for _, test := range []struct {
		table, column string
		bad, good     any
	}{{"gift_item", "status", "disabled", "enabled"}, {"gift_category", "status", "disabled", "enabled"}, {"live_anchor", "status", 0, 1}, {"live_room", "status", 2, 1}, {"live_session", "status", 3, 1}, {"live_account", "status", 2, 1}} {
		require.NoError(t, s.DB.Table(test.table).Where("1=1").Update(test.column, test.bad).Error)
		_, e := send(s, "state0001", "1")
		require.Error(t, e)
		require.NoError(t, s.DB.Table(test.table).Where("1=1").Update(test.column, test.good).Error)
		balance(t, s, "1000")
	}
	r := item()
	r.Version = "1"
	r.CoinPrice = "9223372036854775807"
	_, e := s.SaveItem(ctx, admin, r)
	require.NoError(t, e)
	_, e = send(s, "overflow1", "2")
	require.ErrorIs(t, e, svc.ErrInvalid)
	r.Version = "2"
	r.BenefitType = "anchor_guard"
	r.BenefitStatus = "enabled"
	_, e = s.SaveItem(ctx, admin, r)
	require.ErrorIs(t, e, svc.ErrGuard)
	_, e = s.SaveCategory(ctx, svc.Actor{ID: 3, AuthorityID: 999}, req.Category{CategoryNo: "catother", Name: "越权", Status: "enabled"})
	require.ErrorIs(t, e, svc.ErrForbidden)
	require.NoError(t, s.DB.Where("authority_id=? AND path=?", 888, "/gift/refund").Delete(&gm.AdminPermission{}).Error)
	_, e = s.Refund(ctx, admin, req.Refund{OrderNo: "go_nonexistent", Quantity: "1", RequestID: "forbid01", Reason: "越权测试"})
	require.ErrorIs(t, e, svc.ErrForbidden)
	newRule, e := s.CreateRule(ctx, admin, er.Rule{Name: "重叠规则", ScopeType: "global", AnchorShareBPS: "1000", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
	require.NoError(t, e)
	_, e = s.ReviewRule(ctx, admin, er.Review{RuleNo: newRule.RuleNo, Approve: true})
	require.ErrorIs(t, e, es.ErrRuleConflict)
	_, e = s.ReviewRule(ctx, reviewer, er.Review{RuleNo: newRule.RuleNo, Approve: true})
	require.ErrorIs(t, e, es.ErrRuleConflict)
	s.Now = func() time.Time { return time.Now().AddDate(1, 0, 0) }
	r = item()
	r.Version = "2"
	_, e = s.SaveItem(ctx, admin, r)
	require.NoError(t, e)
	_, e = send(s, "norule01", "1")
	require.True(t, errors.Is(e, es.ErrNoRule), e)
}

func ptr(s string) *string { return &s }

func TestGiftCatalogRulesVisibility(t *testing.T) {
	s := fixture(t, false)
	_, e := s.SaveCategory(ctx, admin, req.Category{CategoryNo: "category2", Name: "豪华", Status: "disabled"})
	require.NoError(t, e)
	p, e := s.Catalog(ctx, 10, nil, "category", req.Query{})
	require.NoError(t, e)
	require.Equal(t, int64(1), p.Total)
	p, e = s.Catalog(ctx, 0, &admin, "category", req.Query{})
	require.NoError(t, e)
	require.ErrorIs(t, s.Sort(ctx, admin, req.Sort{Kind: "category", Version: p.Version, Numbers: []string{"category1"}}), svc.ErrConflict)
	require.ErrorIs(t, s.Sort(ctx, admin, req.Sort{Kind: "category", Version: p.Version, Numbers: []string{"category1", "category1"}}), svc.ErrInvalid)
	require.NoError(t, s.Sort(ctx, admin, req.Sort{Kind: "category", Version: p.Version, Numbers: []string{"category2", "category1"}}))
	require.ErrorIs(t, s.Sort(ctx, admin, req.Sort{Kind: "category", Version: p.Version, Numbers: []string{"category1", "category2"}}), svc.ErrConflict)
	p, e = s.Catalog(ctx, 0, &admin, "category", req.Query{})
	require.NoError(t, e)
	require.Equal(t, "category2", p.List.([]gm.Category)[0].CategoryNo)
	require.Error(t, s.Delete(ctx, admin, req.Delete{Kind: "category", No: "category1", Version: "2"}))
	require.NoError(t, s.DB.Model(&lm.LiveRoom{}).Where("id=1").Update("visibility", 0).Error)
	_, e = send(s, "private1", "1")
	require.ErrorIs(t, e, svc.ErrState)
	require.NoError(t, s.DB.Model(&lm.LiveRoom{}).Where("id=1").Update("visibility", 2).Error)
	_, e = send(s, "follower1", "1")
	require.ErrorIs(t, e, svc.ErrState)
	require.NoError(t, s.DB.Create(&sm.SocialFollow{UserID: 10, TargetUserID: 11}).Error)
	_, e = send(s, "follower1", "1")
	require.NoError(t, e)
	require.NoError(t, s.DB.Create(&sm.SocialBlock{UserID: 11, TargetUserID: 10}).Error)
	_, e = send(s, "blocked1", "1")
	require.ErrorIs(t, e, svc.ErrState)
	require.NoError(t, s.DB.Where("1=1").Delete(&sm.SocialBlock{}).Error)
	for _, v := range []struct{ scope, no, bps string }{{"category", "category1", "2000"}, {"gift", "gift0001", "3000"}, {"anchor", "anchor01", "5000"}} {
		r, e := s.CreateRule(ctx, admin, er.Rule{Name: "范围规则", ScopeType: v.scope, ScopeNo: v.no, AnchorShareBPS: v.bps, EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
		require.NoError(t, e)
		_, e = s.ReviewRule(ctx, reviewer, er.Review{RuleNo: r.RuleNo, Approve: true})
		require.NoError(t, e)
		o, e := send(s, "scope_"+v.scope, "1")
		require.NoError(t, e)
		require.Equal(t, v.bps, fmt.Sprint(o.AnchorShareBPS))
	}
	original, e := send(s, "follower1", "1")
	require.NoError(t, e)
	require.Equal(t, int64(4000), original.AnchorShareBPS)
	_, e = s.Orders(ctx, 10, nil, req.Query{Sort: "id;DROP TABLE gift_order"})
	require.ErrorIs(t, e, svc.ErrInvalid)
}
func TestGiftBenefitCompensationFutureFact(t *testing.T) {
	s := fixture(t, false)
	o, e := send(s, "benefit01", "2")
	require.NoError(t, e)
	// Controlled historical fact fixture only; production cannot enable an unfulfillable rule.
	grant := gm.BenefitGrant{GrantNo: "grant_fixture", GiftOrderNo: o.OrderNo, BenefitType: "anchor_guard", BenefitSnapshot: `{"guardDurationDays":"3"}`, ReceiverUserID: 10, AnchorID: 1, Status: "pending", CreatedAt: time.Now().UnixMilli()}
	require.NoError(t, s.DB.Create(&grant).Error)
	require.Error(t, s.DB.Create(&grant).Error)
	r := req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: "benefit_refund", Reason: "权益补偿事实测试"}
	f, e := s.Refund(ctx, admin, r)
	require.NoError(t, e)
	require.Equal(t, "pending", f.BenefitCompensationStatus)
	_, e = s.Refund(ctx, admin, r)
	require.NoError(t, e)
	count(t, s, &gm.BenefitCompensation{}, 1)
	var c gm.BenefitCompensation
	require.NoError(t, s.DB.First(&c).Error)
	require.Equal(t, uint64(10), c.ReceiverUserID)
	require.Equal(t, int64(3), c.DurationDays)
}

func TestGiftSchemaTampering(t *testing.T) {
	s := fixture(t, false)
	require.NoError(t, s.DB.Exec("DROP TRIGGER trg_gift_order_update").Error)
	require.Error(t, initialize.ValidateGiftSchema(s.DB))
	require.NoError(t, s.DB.Exec("CREATE TRIGGER trg_gift_order_update BEFORE UPDATE ON gift_order BEGIN SELECT 'gift financial fact immutable'; END").Error)
	require.Error(t, initialize.ValidateGiftSchema(s.DB))
}
func TestMySQLGiftLoad(t *testing.T) {
	if os.Getenv("GIFT_RUN_LOAD") != "1" {
		t.Skip("set GIFT_RUN_LOAD=1 for load exercise")
	}
	s := fixture(t, true)
	// 100 competing financial transactions on one hot wallet, then idempotent replay.
	start := time.Now()
	durations := make([]time.Duration, 100)
	errs := parallel(100, func(i int) error {
		begin := time.Now()
		_, e := send(s, fmt.Sprintf("load_%04d", i), "1")
		durations[i] = time.Since(begin)
		return e
	})
	elapsed := time.Since(start)
	for _, e := range errs {
		require.NoError(t, e)
	}
	balance(t, s, "0")
	count(t, s, &gm.Order{}, 100)
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	t.Logf("hot-wallet writes=100 elapsed=%s throughput=%.2f/s p50=%s p95=%s p99=%s", elapsed, 100/elapsed.Seconds(), durations[49], durations[94], durations[98])
	start = time.Now()
	for _, e := range parallel(100, func(i int) error { _, e := send(s, fmt.Sprintf("load_%04d", i), "1"); return e }) {
		require.NoError(t, e)
	}
	t.Logf("idempotent replay=100 elapsed=%s", time.Since(start))
	balance(t, s, "0")
	count(t, s, &gm.Event{}, 100)
	var plan []struct {
		Key  *string
		Rows int64
	}
	require.NoError(t, s.DB.Raw("EXPLAIN SELECT * FROM gift_order WHERE sender_user_id=10 AND request_id='load_0001'").Scan(&plan).Error)
	require.NotEmpty(t, plan)
	require.NotNil(t, plan[0].Key)
	require.Equal(t, "uk_gift_request", *plan[0].Key)
}

func TestMySQLGiftIndependentWallets(t *testing.T) {
	s := fixture(t, true)
	for id := uint(12); id < 20; id++ {
		no := fmt.Sprintf("100000%d", id)
		require.NoError(t, s.DB.Create(&lm.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, Username: fmt.Sprintf("user%d", id), AccountNo: no, Phone: ptr(fmt.Sprintf("+601000000%d", id)), Status: 1}).Error)
		w := s.Wallet.(*wallet.Service)
		o, e := w.Create(ctx, wallet.Actor{ID: 1, AuthorityID: 888, IP: "127.0.0.1"}, wr.CreateAdjustment{RequestID: fmt.Sprintf("credit_%d", id), ExternalOrderNo: fmt.Sprintf("GIFT-CREDIT-%d", id), AccountNo: no, Amount: "100", Reason: "多账户并发测试", EvidenceRef: "test:controlled-fixture"})
		require.NoError(t, e)
		_, e = w.Approve(ctx, wallet.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}, wr.ReviewAdjustment{OrderNo: o.OrderNo, Reason: "核验多账户测试资金"})
		require.NoError(t, e)
	}
	for _, e := range parallel(80, func(i int) error {
		_, e := s.Send(ctx, uint64(12+i%8), "127.0.0.1", "trace_independent", req.Send{RoomNo: "room0001", GiftNo: "gift0001", Quantity: "1", RequestID: fmt.Sprintf("account_%03d", i)})
		return e
	}) {
		require.NoError(t, e)
	}
	count(t, s, &gm.Order{}, 80)
	for id := uint64(12); id < 20; id++ {
		b, e := s.Wallet.(*wallet.Service).Balance(ctx, id, "")
		require.NoError(t, e)
		require.Equal(t, "0", b.AvailableBalance)
	}
}
func TestGiftFractionalRefundRounding(t *testing.T) {
	s := fixture(t, false)
	r := item()
	r.Version = "1"
	r.CoinPrice = "1"
	_, e := s.SaveItem(ctx, admin, r)
	require.NoError(t, e)
	rule, e := s.CreateRule(ctx, admin, er.Rule{Name: "三分之一", ScopeType: "gift", ScopeNo: "gift0001", AnchorShareBPS: "3333", EffectiveFrom: 1, EffectiveTo: time.Now().Add(time.Hour).UnixMilli()})
	require.NoError(t, e)
	_, e = s.ReviewRule(ctx, reviewer, er.Review{RuleNo: rule.RuleNo, Approve: true})
	require.NoError(t, e)
	o, e := send(s, "rounding1", "10")
	require.NoError(t, e)
	require.Equal(t, int64(3), o.EstimatedAnchorEarningCoin)
	for i := 0; i < 10; i++ {
		_, e = s.Refund(ctx, admin, req.Refund{OrderNo: o.OrderNo, Quantity: "1", RequestID: fmt.Sprintf("fraction%02d", i), Reason: "分次退款舍入测试"})
		require.NoError(t, e)
		d, e := s.Detail(ctx, admin, req.Query{No: o.OrderNo})
		require.NoError(t, e)
		require.True(t, d.Consistent, d.Issues)
	}
	balance(t, s, "1000")
}
