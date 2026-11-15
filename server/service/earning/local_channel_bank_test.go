package earning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"strconv"
	"strings"
	"sync"
	"tb_live_module/global"
	"tb_live_module/initialize"
	"tb_live_module/internal/testpay"
	bm "tb_live_module/model/bankreconcile"
	br "tb_live_module/model/bankreconcile/request"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	gr "tb_live_module/model/gift/request"
	lm "tb_live_module/model/live"
	lr "tb_live_module/model/live/request"
	pr "tb_live_module/model/payout/request"
	wm "tb_live_module/model/wallet"
	wr "tb_live_module/model/wallet/request"
	bs "tb_live_module/service/bankreconcile"
	gs "tb_live_module/service/gift"
	ls "tb_live_module/service/live"
	ps "tb_live_module/service/payout"
	ws "tb_live_module/service/wallet"
	"testing"
	"time"
)

func TestMySQLLocalChannelToBankReconciliation(t *testing.T) { localChannelToBank(t) }

// Exercises fresh identities through the real services in one database. SRS
// publishing and bank transfer are external boundaries: only their test facts
// are simulated here, never on a configured business database.
func localChannelToBank(t *testing.T) {
	s, p, _ := payoutFixture(t, true)
	require.NoError(t, initialize.MigrateBankReconcile(s.DB))
	require.NoError(t, initialize.MigrateBankReconcile(s.DB))
	for _, op := range []struct{ path, method string }{{"import", "POST"}, {"run", "POST"}, {"review", "POST"}, {"list", "GET"}, {"detail", "GET"}} {
		require.NoError(t, s.DB.Create(&em.AdminPermission{AuthorityID: 888, Path: "/payout/bank-reconciliation/" + op.path, Method: op.method}).Error)
	}
	bank := &bs.Service{DB: s.DB, Payout: p}

	oldDB, oldCode := global.GVA_DB, global.LiveAccountSuperCode()
	global.GVA_DB = s.DB
	global.SetLiveAccountSuperCode("654321")
	t.Cleanup(func() { global.GVA_DB = oldDB; global.SetLiveAccountSuperCode(oldCode) })
	require.NoError(t, s.DB.AutoMigrate(&lm.LiveAccountNumberSequence{}))
	accounts := &ls.AccountService{}
	register := func(phone string) lm.LiveAccount {
		r := lr.Register{Phone: phone, SmsCode: "654321", Password: "test-password-123", Nickname: "隔离验收"}
		a, e := accounts.RegisterWithContext(ctx, r)
		require.NoError(t, e)
		_, e = accounts.RegisterWithContext(ctx, r)
		require.ErrorIs(t, e, ls.ErrPhoneExists)
		logged, e := accounts.LoginWithContext(ctx, lr.Login{Phone: phone, Password: r.Password})
		require.NoError(t, e)
		require.Equal(t, a.ID, logged.ID)
		_, e = accounts.LoginWithContext(ctx, lr.Login{Phone: phone, Password: "wrong-password"})
		require.ErrorIs(t, e, ls.ErrInvalidCredentials)
		return a
	}
	viewer := register("+60198765001")
	broadcaster := register("+60198765002")
	anchors := &ls.AnchorService{}
	a, e := anchors.ApplyAnchor(uint64(broadcaster.ID), lr.AnchorApplyReq{ChannelId: 1, Nickname: "新注册主播"})
	require.NoError(t, e)
	require.NoError(t, anchors.AuditAnchor(3, lr.AnchorAuditReq{AnchorId: a.ID, ApplyStatus: 2}))
	on, off := uint8(1), uint8(0)
	require.NoError(t, anchors.UpdateAnchorPermission(lr.AnchorPermissionUpdateReq{AnchorId: a.ID, LivePermission: &on, PkPermission: &off, RecommendPermission: &on, WithdrawPermission: &on}))
	var room lm.LiveRoom
	require.NoError(t, s.DB.Where("anchor_id=?", a.ID).First(&room).Error)
	// A valid published SRS session is provided by the isolated fixture.
	session := lm.LiveSession{SessionNo: "e2e_session", RoomId: room.ID, AnchorId: a.ID, Status: 1}
	require.NoError(t, s.DB.Create(&session).Error)
	require.NoError(t, s.DB.Model(&room).Updates(map[string]any{"status": 1, "visibility": 1, "live_status": 2, "current_session_id": session.ID}).Error)
	wallet := &ws.Service{DB: s.DB}
	simulator, e := testpay.New(os.Getenv("EARNING_TEST_MYSQL_DSN"))
	require.NoError(t, e)
	external := testpay.Order{No: "SIM-LOCAL-RECHARGE-001", AccountNo: viewer.AccountNo, Amount: 1000, Currency: "CNY"}
	_, e = simulator.Submit(ctx, external, true)
	require.ErrorIs(t, e, testpay.ErrUnknown)
	pending, e := simulator.Query(ctx, external.No)
	require.NoError(t, e)
	apply := func(o testpay.Order) error {
		recharge, e := wallet.Create(ctx, ws.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}, wr.CreateAdjustment{RequestID: "local_channel_recharge", ExternalOrderNo: o.No, AccountNo: o.AccountNo, Amount: strconv.FormatInt(o.Amount, 10), Reason: "隔离测试资金", EvidenceRef: "test:verified-simulator"})
		if e != nil {
			return e
		}
		_, e = wallet.Approve(ctx, ws.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}, wr.ReviewAdjustment{OrderNo: recharge.OrderNo, Reason: "独立核验测试资金"})
		return e
	}
	require.NoError(t, testpay.Deliver(simulator, pending, apply))
	notification, e := simulator.Complete(external.No)
	require.NoError(t, e)
	bad := notification
	bad.Signature = "wrong"
	require.ErrorIs(t, testpay.Deliver(simulator, bad, apply), testpay.ErrSignature)
	require.NoError(t, testpay.Deliver(simulator, notification, apply))
	// Lost acknowledgement and repeated notification must not create second credit.
	require.NoError(t, testpay.Deliver(simulator, notification, apply))
	compensated, e := simulator.Query(ctx, external.No)
	require.NoError(t, e)
	require.NoError(t, testpay.Deliver(simulator, compensated, apply))
	require.NoError(t, testpay.Deliver(simulator, pending, apply)) // out of order
	var rechargeCount int64
	require.NoError(t, s.DB.Model(&wm.WalletAdjustmentOrder{}).Where("external_order_no=?", external.No).Count(&rechargeCount).Error)
	require.EqualValues(t, 1, rechargeCount)
	gifts := &gs.Service{DB: s.DB, Wallet: wallet}
	sendReq := gr.Send{RoomNo: room.RoomNo, GiftNo: "gift0001", Quantity: "10", RequestID: "e2e_send"}
	gift, e := gifts.Send(ctx, uint64(viewer.ID), "127.0.0.1", "e2e_trace", sendReq)
	require.NoError(t, e)
	replay, e := gifts.Send(ctx, uint64(viewer.ID), "127.0.0.1", "retry_trace", sendReq)
	require.NoError(t, e)
	require.Equal(t, gift.OrderNo, replay.OrderNo)
	refundReq := gr.Refund{OrderNo: gift.OrderNo, Quantity: "1", RequestID: "local_gift_refund", Reason: "隔离退款验收"}
	refund, e := gifts.Refund(ctx, gs.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}, refundReq)
	require.NoError(t, e)
	refundAgain, e := gifts.Refund(ctx, gs.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}, refundReq)
	require.NoError(t, e)
	require.Equal(t, refund.RefundNo, refundAgain.RefundNo)
	var rule em.SettlementRule
	require.NoError(t, s.DB.First(&rule).Error)
	settlement, e := s.CreateSettlement(ctx, financeAdmin, er.CreateSettlement{RequestID: "e2e_settle", AnchorNo: a.AnchorNo, RuleNo: rule.RuleNo, PeriodStart: 1, PeriodEnd: time.Now().UnixMilli()})
	require.NoError(t, e)
	_, e = s.ReviewSettlement(ctx, financeReviewer, er.FinanceReview{No: settlement.SettlementNo, Approve: true, Reason: "独立核验测试收益"})
	require.NoError(t, e)
	payee, e := p.Create(ctx, uint64(broadcaster.ID), "e2e_trace", pr.CreatePayee{RequestID: "e2e_payee", Method: "bank_card", Currency: "CNY", Account: "6222029876543210001", AccountName: "隔离主播", BankCode: "ICBC"})
	require.NoError(t, e)
	withdrawReq := er.ApplyWithdrawal{RequestID: "e2e_withdraw", Amount: "5", Currency: "CNY", PayeeAccountNo: payee.PayeeAccountNo}
	_, e = s.ApplyWithdrawal(ctx, uint64(viewer.ID), withdrawReq)
	require.Error(t, e)
	w, e := s.ApplyWithdrawal(ctx, uint64(broadcaster.ID), withdrawReq)
	require.NoError(t, e)
	_, e = s.ReviewWithdrawal(ctx, financeReviewer, er.FinanceReview{No: w.WithdrawalNo, Approve: true, Reason: "核验测试提现"})
	require.NoError(t, e)
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	store, e := ps.OpenEvidenceStore(dir)
	require.NoError(t, e)
	t.Cleanup(func() { store.Close() })
	p.EvidenceStore = store
	company, e := p.CreateChannelAccount(ctx, payoutAdmin, "e2e_trace", pr.CreateChannelAccount{RequestID: "e2e_company", ChannelCode: "manual_bank", AccountName: "TEST COMPANY", InstitutionName: "TEST BANK", Currency: "CNY", Account: "TESTBANK200001", SingleLimit: "100000", DailyLimit: "1000000"})
	require.NoError(t, e)
	order, e := p.CreateOrder(ctx, payoutAdmin, "e2e_trace", pr.CreateOrder{RequestID: "e2e_order", WithdrawalNo: w.WithdrawalNo})
	require.NoError(t, e)
	at, e := p.StartOrder(ctx, payoutAdmin, "e2e_trace", pr.StartOrder{RequestID: "e2e_start", PayoutNo: order.PayoutNo, ChannelAccountNo: company.ChannelAccountNo, Reason: "领取隔离测试"}, false)
	require.NoError(t, e)
	ev, e := p.UploadEvidence(ctx, payoutAdmin, "e2e_trace", pr.UploadEvidence{RequestID: "e2e_evidence", AttemptNo: at.AttemptNo, MIME: "image/png", Content: testReceipt(t)})
	require.NoError(t, e)
	_, e = p.SubmitAttempt(ctx, payoutAdmin, "e2e_trace", pr.SubmitAttempt{RequestID: "e2e_submit", AttemptNo: at.AttemptNo, PaymentReference: "E2E-SIMULATED-NO-BANK-TRANSFER", PaymentTime: time.Now().UnixMilli(), EvidenceNo: ev.EvidenceNo, OperatorNote: "隔离测试模拟回单", ConfirmRealTransfer: true})
	require.NoError(t, e)
	// Imported statements must not finalize pending payouts or manufacture evidence.
	stamp := time.Now().UTC().Truncate(time.Second)
	csvHeader := "bank_reference,company_account,recipient_account,currency,amount_minor,status,booked_at,direction\n"
	row := "E2E-SIMULATED-NO-BANK-TRANSFER,TESTBANK200001,6222029876543210001,CNY,5,settled," + stamp.Format(time.RFC3339) + ",debit\n"
	bankImport := br.Import{RequestID: "local_bank_import", ChannelAccountNo: company.ChannelAccountNo, PeriodFrom: stamp.Add(-time.Hour).UnixMilli(), PeriodTo: stamp.Add(time.Hour).UnixMilli(), DeclaredComplete: true, CSV: csvHeader + row}
	var beforeBank em.Account
	require.NoError(t, s.DB.Where("anchor_id=?", a.ID).First(&beforeBank).Error)
	imported, e := bank.Import(ctx, payoutAdmin, "local_bank", bankImport)
	require.NoError(t, e)
	_, e = bank.Run(ctx, payoutAdmin, "local_bank", br.Run{RequestID: "bank_run_before_paid", BatchNo: imported.No})
	require.NoError(t, e)
	detail, e := bank.Detail(ctx, payoutAdmin, imported.No)
	require.NoError(t, e)
	require.Equal(t, "platform_not_succeeded", detail.Checks[0].Result)
	staleCheckNo := detail.Checks[0].CheckNo
	var afterBank em.Account
	require.NoError(t, s.DB.Where("anchor_id=?", a.ID).First(&afterBank).Error)
	require.Equal(t, beforeBank, afterBank)
	_, e = bank.Review(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "local_bank", br.Review{RequestID: "reject_premature_ack", CheckNo: detail.Checks[0].CheckNo, Decision: "acknowledge", Reason: "尚未确认内部出款"})
	require.ErrorIs(t, e, bs.ErrState)
	confirmation := pr.ReviewAttempt{RequestID: "e2e_review", AttemptNo: at.AttemptNo, Reason: "独立复核模拟回单", ConfirmExternalResult: true}
	_, e = p.ReviewAttempt(ctx, payoutAdmin, "e2e_trace", confirmation, "success")
	require.ErrorIs(t, e, ps.ErrSelfReview)
	for i := 0; i < 2; i++ {
		paid, e := p.ReviewAttempt(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "e2e_trace", confirmation, "success")
		require.NoError(t, e)
		require.Equal(t, "succeeded", paid.Status)
	}
	var balance wm.WalletAccount
	require.NoError(t, s.DB.Where("account_id=?", viewer.ID).First(&balance).Error)
	require.EqualValues(t, 910, balance.AvailableBalance)
	var income em.Account
	require.NoError(t, s.DB.Where("anchor_id=?", a.ID).First(&income).Error)
	require.EqualValues(t, 31, income.Available)
	require.EqualValues(t, 5, income.Paid)
	require.Zero(t, income.Frozen)
	require.Zero(t, income.Debt)
	var paidCount int64
	require.NoError(t, s.DB.Model(&em.AccountLedger{}).Where("business_no=? AND entry_type='withdrawal_paid'", w.WithdrawalNo).Count(&paidCount).Error)
	require.EqualValues(t, 1, paidCount)
	require.NoError(t, s.CheckPayoutFinanceInTx(s.DB, w.WithdrawalNo))
	_, e = bank.Run(ctx, payoutAdmin, "local_bank", br.Run{RequestID: "bank_run_after_paid", BatchNo: imported.No})
	require.NoError(t, e)
	detail, e = bank.Detail(ctx, payoutAdmin, imported.No)
	require.NoError(t, e)
	require.Len(t, detail.Checks, 1)
	require.Equal(t, "matched", detail.Checks[0].Result)
	review := br.Review{RequestID: "bank_review_matched", CheckNo: detail.Checks[0].CheckNo, Decision: "acknowledge", Reason: "独立核对银行声明"}
	_, e = bank.Review(ctx, payoutAdmin, "local_bank", review)
	require.ErrorIs(t, e, bs.ErrSelfReview)
	_, e = bank.Review(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "local_bank", review)
	require.NoError(t, e)
	_, e = bank.Review(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "local_bank", review)
	require.NoError(t, e)
	// Run and review contenders must resolve the same committed permanent key.
	parallelRun := br.Run{RequestID: "bank_parallel_run", BatchNo: imported.No}
	runErrs := make(chan error, 8)
	var runWG sync.WaitGroup
	for i := 0; i < 8; i++ {
		runWG.Add(1)
		go func() {
			defer runWG.Done()
			_, err := bank.Run(ctx, payoutAdmin, "parallel_run", parallelRun)
			runErrs <- err
		}()
	}
	runWG.Wait()
	close(runErrs)
	for err := range runErrs {
		require.NoError(t, err)
	}
	parallelDetail, e := bank.Detail(ctx, payoutAdmin, imported.No)
	require.NoError(t, e)
	parallelReview := br.Review{RequestID: "bank_parallel_review", CheckNo: parallelDetail.Checks[0].CheckNo, Decision: "acknowledge", Reason: "并发相同请求固定复核"}
	reviewErrs := make(chan error, 8)
	var reviewWG sync.WaitGroup
	for i := 0; i < 8; i++ {
		reviewWG.Add(1)
		go func() {
			defer reviewWG.Done()
			_, err := bank.Review(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "parallel_review", parallelReview)
			reviewErrs <- err
		}()
	}
	reviewWG.Wait()
	close(reviewErrs)
	for err := range reviewErrs {
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := bank.Import(ctx, payoutAdmin, "concurrent_bank", bankImport)
			if e == nil && v.No != imported.No {
				e = bs.ErrConflict
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	alias := bankImport
	alias.RequestID = "bank_second_request"
	reimported, e := bank.Import(ctx, payoutAdmin, "local_bank", alias)
	require.NoError(t, e)
	require.Equal(t, imported.No, reimported.No)
	changed := bankImport
	changed.CSV = strings.Replace(row, "CNY,5,", "CNY,6,", 1)
	changed.CSV = csvHeader + changed.CSV
	_, e = bank.Import(ctx, payoutAdmin, "local_bank", changed)
	require.ErrorIs(t, e, bs.ErrConflict)
	changed.RequestID = "different_content_request"
	_, e = bank.Import(ctx, payoutAdmin, "local_bank", changed)
	require.ErrorIs(t, e, bs.ErrConflict)
	denied := bankImport
	denied.RequestID = "revoked_actor_import"
	_, e = bank.Import(ctx, ps.Actor{ID: 3, AuthorityID: 999}, "local_bank", denied)
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	wrong := bankImport
	wrong.RequestID = "wrong_company_import"
	wrong.CSV = strings.Replace(wrong.CSV, "TESTBANK200001", "TESTBANK200002", 1)
	_, e = bank.Import(ctx, payoutAdmin, "local_bank", wrong)
	require.ErrorIs(t, e, bs.ErrInvalid)
	huge := bankImport
	huge.RequestID = "too_large_import"
	huge.CSV = strings.Repeat("x", bs.MaxCSVBytes+1)
	_, e = bank.Import(ctx, payoutAdmin, "local_bank", huge)
	require.ErrorIs(t, e, bs.ErrLimit)
	for _, complete := range []bool{true, false} {
		missing := bankImport
		missing.RequestID = "bank_missing_" + strconv.FormatBool(complete)
		missing.DeclaredComplete = complete
		missing.CSV = strings.Replace(missing.CSV, "E2E-SIMULATED-NO-BANK-TRANSFER", "OTHER-SAME-AMOUNT-REFERENCE", 1)
		imp, e := bank.Import(ctx, payoutAdmin, "local_bank", missing)
		require.NoError(t, e)
		_, e = bank.Run(ctx, payoutAdmin, "local_bank", br.Run{RequestID: "run_missing_" + strconv.FormatBool(complete), BatchNo: imp.No})
		require.NoError(t, e)
		d, e := bank.Detail(ctx, payoutAdmin, imp.No)
		require.NoError(t, e)
		results := []string{}
		for _, c := range d.Checks {
			results = append(results, c.Result)
		}
		require.Contains(t, results, "unmatched_bank")
		if complete {
			require.Contains(t, results, "missing_in_statement")
		} else {
			require.Contains(t, results, "not_seen_partial")
		}
	}
	bytes, e := json.Marshal(detail)
	require.NoError(t, e)
	for _, secret := range []string{"TESTBANK200001", "6222029876543210001", "E2E-SIMULATED-NO-BANK-TRANSFER", "referenceFingerprint", "recipientFingerprint"} {
		require.NotContains(t, string(bytes), secret)
	}
	require.Error(t, s.DB.Model(&bm.Line{}).Where("line_no=?", detail.Lines[0].LineNo).Update("amount", 7).Error)
	require.Error(t, s.DB.Where("batch_no=?", imported.No).Delete(&bm.Batch{}).Error)
	require.NoError(t, initialize.ValidateBankReconcileSchema(s.DB))
	// A report observed before the manual terminal transition cannot be reviewed as current.
	_, e = bank.Review(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "local_bank", br.Review{RequestID: "bank_stale_review", CheckNo: staleCheckNo, Decision: "investigate", Reason: "陈旧报告要求重新核对"})
	require.ErrorIs(t, e, bs.ErrState)
	// Authorization is checked again even for a successful permanent request replay.
	require.NoError(t, s.DB.Where("authority_id=888 AND path='/payout/bank-reconciliation/import' AND method='POST'").Delete(&em.AdminPermission{}).Error)
	_, e = bank.Import(ctx, payoutAdmin, "local_bank", bankImport)
	require.ErrorIs(t, e, ps.ErrOrderForbidden)
	require.NoError(t, s.DB.Create(&em.AdminPermission{AuthorityID: 888, Path: "/payout/bank-reconciliation/import", Method: "POST"}).Error)
	_, e = bank.Import(ctx, ps.Actor{ID: 2, AuthorityID: 888}, "local_bank", bankImport)
	require.ErrorIs(t, e, bs.ErrConflict)
	for i, badCSV := range []string{
		strings.Replace(csvHeader+row, "CNY,5,", "CNY,05,", 1), strings.Replace(csvHeader+row, "CNY,5,", "CNY,5.0,", 1),
		strings.Replace(csvHeader+row, "CNY,5,", "CNY,9223372036854775808,", 1), strings.Replace(csvHeader+row, "debit", "credit", 1),
		strings.Replace(csvHeader+row, "E2E-SIMULATED-NO-BANK-TRANSFER", "=HYPERLINK", 1), csvHeader + row + row,
		strings.Replace(csvHeader+row, stamp.Format(time.RFC3339), time.UnixMilli(bankImport.PeriodTo).UTC().Format(time.RFC3339), 1),
		strings.Replace(csvHeader+row, "E2E-SIMULATED-NO-BANK-TRANSFER", "\"BANK\nMULTILINE\"", 1),
	} {
		badReq := bankImport
		badReq.RequestID = fmt.Sprintf("bank_invalid_%d", i)
		badReq.CSV = badCSV
		_, e = bank.Import(ctx, payoutAdmin, "local_bank", badReq)
		require.Error(t, e)
	}
	zone := bankImport
	zone.RequestID = "bank_equivalent_timezone"
	zone.CSV = strings.Replace(zone.CSV, stamp.Format(time.RFC3339), stamp.In(time.FixedZone("local", 8*3600)).Format(time.RFC3339), 1)
	zoneResult, e := bank.Import(ctx, payoutAdmin, "local_bank", zone)
	require.NoError(t, e)
	require.Equal(t, imported.No, zoneResult.No)
	// Bounded real 1000-row batch; all reads/writes use batches, not one query per row.
	var csvRows strings.Builder
	csvRows.WriteString(csvHeader)
	for i := 0; i < bs.MaxRows; i++ {
		fmt.Fprintf(&csvRows, "LOAD-BANK-%06d,TESTBANK200001,6222029876543210001,CNY,5,settled,%s,debit\n", i, stamp.Format(time.RFC3339))
	}
	loadReq := bankImport
	loadReq.RequestID = "bank_load_1000_rows"
	loadReq.CSV = csvRows.String()
	budget, cancel := context.WithTimeout(ctx, 15*time.Second)
	started := time.Now()
	loadResult, e := bank.Import(budget, payoutAdmin, "load_bank", loadReq)
	cancel()
	require.NoError(t, e)
	importElapsed := time.Since(started)
	budget, cancel = context.WithTimeout(ctx, 15*time.Second)
	started = time.Now()
	_, e = bank.Run(budget, payoutAdmin, "load_bank", br.Run{RequestID: "run_load_1000_rows", BatchNo: loadResult.No})
	cancel()
	require.NoError(t, e)
	t.Logf("1000 rows: import=%s, reconcile=%s", importElapsed, time.Since(started))
	over := loadReq
	over.RequestID = "bank_load_1001_rows"
	over.CSV += row
	_, e = bank.Import(ctx, payoutAdmin, "load_bank", over)
	require.ErrorIs(t, e, bs.ErrLimit)
	var explain []struct {
		Key          string
		PossibleKeys string
		Rows         int64
		Extra        string
	}
	require.NoError(t, s.DB.Raw("EXPLAIN SELECT attempt_no FROM payout_attempt WHERE channel_account_no=? AND status='succeeded' AND payment_time>=? AND payment_time<? LIMIT 1001", company.ChannelAccountNo, bankImport.PeriodFrom, bankImport.PeriodTo).Scan(&explain).Error)
	require.NotEmpty(t, explain)
	require.Contains(t, explain[0].PossibleKeys, "idx_bank_attempt_account_status_time")
	t.Logf("EXPLAIN tiny fixture optimizer index=%s rows=%d; range index available=%s", explain[0].Key, explain[0].Rows, explain[0].PossibleKeys)
	// Same-named weakened guards/constraints must fail validation. Restore exact schema afterwards.
	require.NoError(t, s.DB.Exec("ALTER TABLE payout_bank_line DROP CHECK ck_bank_amount, ADD CONSTRAINT ck_bank_amount CHECK(amount>=0)").Error)
	require.ErrorContains(t, initialize.ValidateBankReconcileSchema(s.DB), "check definition")
	require.NoError(t, s.DB.Exec("ALTER TABLE payout_bank_line DROP CHECK ck_bank_amount, ADD CONSTRAINT ck_bank_amount CHECK(amount>0)").Error)
	require.NoError(t, s.DB.Exec("DROP TRIGGER trg_payout_bank_line_update").Error)
	require.Error(t, initialize.ValidateBankReconcileSchema(s.DB))
	require.NoError(t, initialize.MigrateBankReconcile(s.DB))
	require.NoError(t, initialize.MigrateBankReconcile(s.DB))
	var finalIncome em.Account
	require.NoError(t, s.DB.Where("anchor_id=?", a.ID).First(&finalIncome).Error)
	require.Equal(t, income, finalIncome)
	t.Log("LOCAL ONLY: signed simulated recharge → duplicate/lost response/query compensation/out-of-order → one wallet credit → gift/refund → settlement → manual payout independent proof/review → immutable two-way bank reconciliation; external bank calls=0")
}

func TestMySQLBankDiscrepancyMatrix(t *testing.T) {
	s, p, payee := payoutFixture(t, true)
	require.NoError(t, initialize.MigrateBankReconcile(s.DB))
	bank := &bs.Service{DB: s.DB, Payout: p}
	root := ps.Actor{ID: 1}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	store, e := ps.OpenEvidenceStore(dir)
	require.NoError(t, e)
	t.Cleanup(func() { store.Close() })
	p.EvidenceStore = store
	company, e := p.CreateChannelAccount(ctx, payoutAdmin, "matrix", pr.CreateChannelAccount{RequestID: "matrix_company", ChannelCode: "manual_bank", AccountName: "LOCAL COMPANY", InstitutionName: "LOCAL BANK", Currency: "CNY", Account: "TESTBANK300001", SingleLimit: "100000", DailyLimit: "1000000"})
	require.NoError(t, e)
	cases := []struct{ amount, currency, recipient, status, result string }{{"6", "CNY", "6222021234567890123", "settled", "amount_mismatch"}, {"5", "USD", "6222021234567890123", "settled", "currency_mismatch"}, {"5", "CNY", "6222021234567890124", "settled", "recipient_mismatch"}, {"5", "CNY", "6222021234567890123", "pending", "bank_pending"}, {"5", "CNY", "6222021234567890123", "failed", "bank_failed"}, {"5", "CNY", "6222021234567890123", "reversed", "bank_reversed"}}
	for i, tc := range cases {
		t.Run(tc.result, func(t *testing.T) {
			key := fmt.Sprintf("matrix_%d", i)
			w := payoutWithdrawal(t, s, payee, key+"_withdraw", true)
			order, e := p.CreateOrder(ctx, payoutAdmin, "matrix", pr.CreateOrder{RequestID: key + "_order", WithdrawalNo: w.WithdrawalNo})
			require.NoError(t, e)
			at, e := p.StartOrder(ctx, payoutAdmin, "matrix", pr.StartOrder{RequestID: key + "_start", PayoutNo: order.PayoutNo, ChannelAccountNo: company.ChannelAccountNo, Reason: "本地矩阵领取"}, false)
			require.NoError(t, e)
			evidence, e := p.UploadEvidence(ctx, payoutAdmin, "matrix", pr.UploadEvidence{RequestID: key + "_evidence", AttemptNo: at.AttemptNo, MIME: "image/png", Content: testReceipt(t)})
			require.NoError(t, e)
			reference := fmt.Sprintf("LOCAL-MATRIX-BANK-%d", i)
			_, e = p.SubmitAttempt(ctx, payoutAdmin, "matrix", pr.SubmitAttempt{RequestID: key + "_submit", AttemptNo: at.AttemptNo, PaymentReference: reference, PaymentTime: time.Now().UnixMilli(), EvidenceNo: evidence.EvidenceNo, OperatorNote: "隔离银行声明模拟", ConfirmRealTransfer: true})
			require.NoError(t, e)
			stamp := time.Now().UTC().Truncate(time.Second)
			csv := fmt.Sprintf("bank_reference,company_account,recipient_account,currency,amount_minor,status,booked_at,direction\n%s,TESTBANK300001,%s,%s,%s,%s,%s,debit\n", reference, tc.recipient, tc.currency, tc.amount, tc.status, stamp.Format(time.RFC3339))
			var before em.Account
			require.NoError(t, s.DB.First(&before).Error)
			result, e := bank.Import(ctx, root, "matrix", br.Import{RequestID: key + "_import", ChannelAccountNo: company.ChannelAccountNo, PeriodFrom: stamp.Add(-time.Hour).UnixMilli(), PeriodTo: stamp.Add(time.Hour).UnixMilli(), CSV: csv})
			require.NoError(t, e)
			_, e = bank.Run(ctx, root, "matrix", br.Run{RequestID: key + "_run", BatchNo: result.No})
			require.NoError(t, e)
			detail, e := bank.Detail(ctx, root, result.No)
			require.NoError(t, e)
			require.Len(t, detail.Checks, 1)
			require.Equal(t, tc.result, detail.Checks[0].Result)
			_, e = bank.Review(ctx, root, "matrix", br.Review{RequestID: key + "_review", CheckNo: detail.Checks[0].CheckNo, Decision: "investigate", Reason: "root例外明确记录"})
			require.NoError(t, e)
			detail, e = bank.Detail(ctx, root, result.No)
			require.NoError(t, e)
			require.True(t, detail.Reviews[0].RootException)
			var after em.Account
			require.NoError(t, s.DB.First(&after).Error)
			require.Equal(t, before, after)
		})
	}
}
