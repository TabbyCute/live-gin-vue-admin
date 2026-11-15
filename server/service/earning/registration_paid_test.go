package earning_test

import (
	"github.com/stretchr/testify/require"
	"os"
	"tb_live_module/global"
	em "tb_live_module/model/earning"
	er "tb_live_module/model/earning/request"
	gr "tb_live_module/model/gift/request"
	lm "tb_live_module/model/live"
	lr "tb_live_module/model/live/request"
	pr "tb_live_module/model/payout/request"
	wm "tb_live_module/model/wallet"
	wr "tb_live_module/model/wallet/request"
	gs "tb_live_module/service/gift"
	ls "tb_live_module/service/live"
	ps "tb_live_module/service/payout"
	ws "tb_live_module/service/wallet"
	"testing"
	"time"
)

func TestRegistrationToManualPaid(t *testing.T)      { registrationToPaid(t, false) }
func TestMySQLRegistrationToManualPaid(t *testing.T) { registrationToPaid(t, true) }

// Exercises fresh identities through the real services in one database. SRS
// publishing and bank transfer are external boundaries: only their test facts
// are simulated here, never on a configured business database.
func registrationToPaid(t *testing.T, mysql bool) {
	s, p, _ := payoutFixture(t, mysql)
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
	recharge, e := wallet.Create(ctx, ws.Actor{ID: 3, AuthorityID: 888, IP: "127.0.0.1"}, wr.CreateAdjustment{RequestID: "e2e_recharge", ExternalOrderNo: "E2E-ISOLATED-RECEIPT", AccountNo: viewer.AccountNo, Amount: "1000", Reason: "隔离测试资金", EvidenceRef: "test:isolated"})
	require.NoError(t, e)
	_, e = wallet.Approve(ctx, ws.Actor{ID: 2, AuthorityID: 888, IP: "127.0.0.1"}, wr.ReviewAdjustment{OrderNo: recharge.OrderNo, Reason: "独立核验测试资金"})
	require.NoError(t, e)
	gifts := &gs.Service{DB: s.DB, Wallet: wallet}
	sendReq := gr.Send{RoomNo: room.RoomNo, GiftNo: "gift0001", Quantity: "10", RequestID: "e2e_send"}
	gift, e := gifts.Send(ctx, uint64(viewer.ID), "127.0.0.1", "e2e_trace", sendReq)
	require.NoError(t, e)
	replay, e := gifts.Send(ctx, uint64(viewer.ID), "127.0.0.1", "retry_trace", sendReq)
	require.NoError(t, e)
	require.Equal(t, gift.OrderNo, replay.OrderNo)
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
	require.EqualValues(t, 900, balance.AvailableBalance)
	var income em.Account
	require.NoError(t, s.DB.Where("anchor_id=?", a.ID).First(&income).Error)
	require.EqualValues(t, 35, income.Available)
	require.EqualValues(t, 5, income.Paid)
	require.Zero(t, income.Frozen)
	require.Zero(t, income.Debt)
	var paidCount int64
	require.NoError(t, s.DB.Model(&em.AccountLedger{}).Where("business_no=? AND entry_type='withdrawal_paid'", w.WithdrawalNo).Count(&paidCount).Error)
	require.EqualValues(t, 1, paidCount)
	require.NoError(t, s.CheckPayoutFinanceInTx(s.DB, w.WithdrawalNo))
	t.Log("new registration/login → anchor application/approval → recharge → gift/idempotent replay → settlement → own payee → withdrawal → independent manual confirmation → paid once; external bank calls=0")
}
