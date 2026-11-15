package earning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"sync"
	"sync/atomic"
	"tb_live_module/global"
	model "tb_live_module/model/earning"
	req "tb_live_module/model/earning/request"
	live "tb_live_module/model/live"
	"tb_live_module/model/system"
	"testing"
	"time"
)

type scaleFixture struct {
	s       *Service
	a       model.Account
	facts   map[string]model.Income
	seen    atomic.Int64
	queries atomic.Int64
	first   string
	rule    model.SettlementRule
}

func newScaleFixture(t *testing.T, mySQL bool) *scaleFixture {
	t.Helper()
	var db *gorm.DB
	var e error
	if mySQL {
		raw := os.Getenv("EARNING_SCALE_MYSQL_DSN")
		if raw == "" {
			t.Skip("isolated local EARNING_SCALE_MYSQL_DSN not configured")
		}
		cfg, e := driver.ParseDSN(raw)
		require.NoError(t, e)
		require.Equal(t, "127.0.0.1:13329", cfg.Addr)
		require.Equal(t, "earning_test_scale", cfg.DBName)
		cfg.ParseTime = true
		db, e = gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
		// Only this exact disposable schema is ever cleared.
		var tables []string
		require.NoError(t, db.Raw("SHOW TABLES").Scan(&tables).Error)
		for _, table := range tables {
			require.NoError(t, db.Exec("DROP TABLE `"+table+"`").Error)
		}
	} else {
		db, e = gorm.Open(sqlite.Open("file:"+t.TempDir()+"/scale.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, e)
	}
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(12)
	t.Cleanup(func() { pool.Close() })
	migrationDB := db
	if mySQL {
		// Match the production schema contract even on a fresh MySQL instance
		// whose default collation is utf8mb4_0900_ai_ci.
		migrationDB = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	require.NoError(t, migrationDB.AutoMigrate(&model.Account{}, &model.AccountLedger{}, &model.Income{}, &model.Settlement{}, &model.SettlementItem{}, &model.Allocation{}, &model.Withdrawal{}, &model.Reconciliation{}, &model.FinanceAudit{}, &model.RoundingBalance{}, &model.AuditJob{}, &model.IncomeLock{}, &system.SysUser{}, &live.LiveAnchor{}))
	require.NoError(t, db.Create(&system.SysUser{GVA_MODEL: global.GVA_MODEL{ID: 1}, Username: "root", Enable: 1}).Error)
	require.NoError(t, db.Create(&live.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: 1}, AnchorNo: "scale_anchor"}).Error)
	f := &scaleFixture{s: &Service{DB: db}, a: model.Account{AnchorID: 1, AnchorNo: "scale_anchor", Currency: "CNY"}, facts: map[string]model.Income{}, rule: model.SettlementRule{RuleNo: "scale_rule", Currency: "CNY", CoinUnits: 3, MinorUnits: 2}}
	require.NoError(t, db.Create(&f.a).Error)
	f.s.Sources = map[string]SourceVerifier{"fixture": func(tx *gorm.DB, facts []model.Income) error {
		if len(facts) > maxSettlementFacts {
			return ErrLimit
		}
		for _, fact := range facts {
			if f.facts[fact.IncomeNo] != fact {
				return ErrInvariant
			}
			f.seen.Add(1)
		}
		return nil
	}}
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("scale_query_count", func(*gorm.DB) { f.queries.Add(1) }))
	return f
}

func (f *scaleFixture) seed(t *testing.T, batches, perBatch int, refund bool) {
	t.Helper()
	require.NoError(t, f.s.DB.Transaction(func(tx *gorm.DB) error {
		for batch := 0; batch < batches; batch++ {
			facts := make([]model.Income, perBatch)
			for i := range facts {
				no := number("in_")
				facts[i] = model.Income{IncomeNo: no, AnchorID: 1, SourceType: "fixture", SourceNo: no, SourceEntryNo: no, EntryType: "income", Unit: "COIN", Amount: 1, GrossAmount: 1, RuleNo: "gift_rule", ShareBPS: 10000, SourceHash: hash(no), CreatedAt: 1}
				if refund {
					facts[i].EntryType = "reversal"
					facts[i].OriginalIncomeNo = f.first
					facts[i].Amount = -1
					facts[i].GrossAmount = -1
				}
			}
			if e := tx.CreateInBatches(&facts, 200).Error; e != nil {
				return e
			}
			if f.first == "" {
				f.first = facts[0].IncomeNo
			}
			for _, fact := range facts {
				f.facts[fact.IncomeNo] = fact
			}
			items, amount, e := buildItems(tx, facts, f.rule)
			if e != nil {
				return e
			}
			st := model.Settlement{SettlementNo: number("st_"), RequestID: number("rq_"), AnchorID: 1, AnchorNo: f.a.AnchorNo, PeriodStart: 0, PeriodEnd: 2, Currency: "CNY", RuleNo: f.rule.RuleNo, RuleSnapshot: encode(f.rule), Amount: amount, ItemCount: int64(len(items)), ItemsHash: hash(items), Status: "approved", CreatedBy: 2, ApprovedBy: 3, CreatedAt: 1, ApprovedAt: 2}
			if e = tx.Create(&st).Error; e != nil {
				return e
			}
			allocations := make([]model.Allocation, len(items))
			for i := range items {
				items[i].SettlementNo = st.SettlementNo
				allocations[i] = model.Allocation{IncomeNo: items[i].IncomeNo, SettlementNo: st.SettlementNo}
			}
			if e = tx.CreateInBatches(items, 200).Error; e != nil {
				return e
			}
			if e = tx.CreateInBatches(allocations, 200).Error; e != nil {
				return e
			}
			kind := "settlement_credit"
			if amount < 0 {
				kind = "negative_adjustment"
			}
			if e = post(tx, &f.a, kind, st.SettlementNo, amount, 2); e != nil {
				return e
			}
			if e = applyRounding(tx, st); e != nil {
				return e
			}
			if e = audit(tx, Actor{ID: 2}, st.SettlementNo, "settlement.create", encode(st), 1); e != nil {
				return e
			}
			if e = audit(tx, Actor{ID: 3}, st.SettlementNo, "settlement.review", encode(st), 2); e != nil {
				return e
			}
		}
		return nil
	}))
}

func (f *scaleFixture) check(ctx context.Context) error {
	return f.s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return f.s.verifyAccount(tx, f.a, true) }, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func checkScaleHistory(t *testing.T, f *scaleFixture) {
	f.seed(t, 130, 200, false) // 26000 source facts and two ledger pages.
	f.seed(t, 1, 1, true)      // Original is before the page boundary and uses its old rule.
	f.seen.Store(0)
	f.queries.Store(0)
	start := time.Now()
	require.NoError(t, f.check(context.Background()))
	elapsed := time.Since(start)
	require.Equal(t, int64(26001), f.seen.Load())
	// Query cost tracks bounded settlement batches, never one query per income.
	require.Less(t, f.queries.Load(), int64(650))
	t.Logf("full-history: incomeFacts=26001 ledgerVersions=%d queries=%d duration=%s", f.a.Version, f.queries.Load(), elapsed)
	require.Less(t, elapsed, 25*time.Second)
	before := f.a
	require.NoError(t, f.s.DB.Model(&model.Income{}).Where("income_no=?", f.first).Update("source_hash", hash("tampered")).Error)
	require.ErrorIs(t, f.check(context.Background()), ErrInvariant)
	var saved model.Account
	require.NoError(t, f.s.DB.First(&saved, f.a.ID).Error)
	require.Equal(t, before, saved)
	require.NoError(t, f.s.DB.Model(&model.Income{}).Where("income_no=?", f.first).Update("source_hash", f.facts[f.first].SourceHash).Error)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, f.check(ctx), context.Canceled)
	// A late failure rolls back an otherwise valid monetary append.
	sentinel := errors.New("rollback after proof")
	err := f.s.DB.Transaction(func(tx *gorm.DB) error {
		if e := f.s.verifyAccount(tx, f.a, true); e != nil {
			return e
		}
		a := f.a
		if e := post(tx, &a, "withdrawal_freeze", "rollback_probe", 1, 3); e != nil {
			return e
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	require.NoError(t, f.s.DB.First(&saved, f.a.ID).Error)
	require.Equal(t, before, saved)
}

func TestAuditScaleHistory(t *testing.T) { checkScaleHistory(t, newScaleFixture(t, false)) }

func TestAuditScaleMySQL(t *testing.T) {
	f := newScaleFixture(t, true)
	checkScaleHistory(t, f)
	root := Actor{ID: 1}
	job, e := f.s.StartAudit(context.Background(), root, req.StartAudit{RequestID: "mysql_scale_audit", AnchorNo: f.a.AnchorNo, Currency: "CNY"})
	require.NoError(t, e)
	var wg sync.WaitGroup
	claimed := atomic.Int64{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ran, e := f.s.RunNextAudit(context.Background())
			if e != nil {
				t.Error(e)
			}
			if ran {
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), claimed.Load())
	require.NoError(t, f.s.DB.Where("job_no=?", job.JobNo).Take(&job).Error)
	require.Equal(t, "passed", job.Status)
}

func TestAuditJobCancelRetryAndStaleReport(t *testing.T) {
	f := newScaleFixture(t, false)
	f.seed(t, 2, 3, false)
	ctx := context.Background()
	root := Actor{ID: 1}
	r := req.StartAudit{RequestID: "audit_job_start", AnchorNo: f.a.AnchorNo, Currency: "CNY"}
	_, e := f.s.StartAudit(ctx, Actor{}, r)
	require.ErrorIs(t, e, ErrForbidden)
	job, e := f.s.StartAudit(ctx, root, r)
	require.NoError(t, e)
	again, e := f.s.StartAudit(ctx, root, r)
	require.NoError(t, e)
	require.Equal(t, job.JobNo, again.JobNo)
	_, e = f.s.CancelAudit(ctx, root, req.CancelAudit{JobNo: job.JobNo})
	require.NoError(t, e)
	ran, e := f.s.RunNextAudit(ctx)
	require.NoError(t, e)
	require.False(t, ran)
	r.RequestID = "audit_job_retry"
	r.RetryOf = job.JobNo
	job, e = f.s.StartAudit(ctx, root, r)
	require.NoError(t, e)
	ran, e = f.s.RunNextAudit(ctx)
	require.NoError(t, e)
	require.True(t, ran)
	require.NoError(t, f.s.DB.Where("job_no=?", job.JobNo).Take(&job).Error)
	require.Equal(t, "passed", job.Status)
	require.Equal(t, hash(f.a), job.SnapshotHash)
	// A historical passed report is never read by the monetary verifier.
	require.NoError(t, f.s.DB.Model(&model.Income{}).Where("income_no=?", f.first).Update("source_hash", hash("later_corruption")).Error)
	require.ErrorIs(t, f.check(ctx), ErrInvariant)
	r.RequestID = "audit_job_failed"
	r.RetryOf = ""
	bad, e := f.s.StartAudit(ctx, root, r)
	require.NoError(t, e)
	_, e = f.s.RunNextAudit(ctx)
	require.ErrorIs(t, e, ErrInvariant)
	require.NoError(t, f.s.DB.Where("job_no=?", bad.JobNo).Take(&bad).Error)
	require.Equal(t, "failed", bad.Status)
	require.Equal(t, "invariant_violation", bad.IssueCode)
	_, e = f.s.CancelAudit(ctx, root, req.CancelAudit{JobNo: job.JobNo})
	require.ErrorIs(t, e, ErrConflict)
	t.Log(fmt.Sprintf("cancelled=%s passed=%s failure=%s", r.RetryOf, job.JobNo, bad.IssueCode))
}

func TestAuditRunningCancellationCannotPublishPassed(t *testing.T) {
	f := newScaleFixture(t, false)
	f.seed(t, 1, 1, false)
	root, ctx := Actor{ID: 1}, context.Background()
	job, e := f.s.StartAudit(ctx, root, req.StartAudit{RequestID: "running_cancel", AnchorNo: f.a.AnchorNo, Currency: "CNY"})
	require.NoError(t, e)
	entered, release := make(chan struct{}), make(chan struct{})
	f.s.Sources["fixture"] = func(tx *gorm.DB, facts []model.Income) error { close(entered); <-release; return nil }
	done := make(chan error, 1)
	go func() { _, e := f.s.RunNextAudit(ctx); done <- e }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not enter proof")
	}
	_, e = f.s.CancelAudit(ctx, root, req.CancelAudit{JobNo: job.JobNo})
	require.NoError(t, e)
	close(release)
	require.ErrorIs(t, <-done, errAuditCancelled)
	require.NoError(t, f.s.DB.Where("job_no=?", job.JobNo).Take(&job).Error)
	require.Equal(t, "cancelled", job.Status)
	require.Equal(t, uint64(1), job.CancelledBy)
}

func TestAuditRejectsNonpositiveLedgerVersion(t *testing.T) {
	f := newScaleFixture(t, false)
	require.NoError(t, f.s.DB.Create(&model.AccountLedger{LedgerNo: "invalid_zero_version", AnchorID: 1, Currency: "CNY", EntryType: "settlement_credit", BusinessNo: "orphan", Version: 0}).Error)
	require.ErrorIs(t, f.check(context.Background()), ErrInvariant)
}
