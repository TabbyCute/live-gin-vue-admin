package payout

import (
	"encoding/base64"
	"errors"
	"github.com/stretchr/testify/require"
	"strings"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	"testing"
)

func testCrypto(t *testing.T) *Crypto {
	t.Helper()
	c, e := NewCrypto("test", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32))))
	require.NoError(t, e)
	return c
}

type brokenRandom struct{}

func (brokenRandom) Read([]byte) (int, error) { return 0, errors.New("injected entropy failure") }
func TestCryptoAuthenticatedEnvelopeAndFailure(t *testing.T) {
	c := testCrypto(t)
	p := model.PayeeAccount{PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", OwnerNo: "anchor1", Method: "bank_card", Currency: "CNY", Version: 1, PayloadVersion: 1}
	v := payload{Account: "6222021234567890123", AccountName: "测试收款人", BankCode: "ICBC"}
	require.NoError(t, c.seal(&p, v))
	require.NotContains(t, p.EncryptedPayload, v.Account)
	got, e := c.open(p)
	require.NoError(t, e)
	require.Equal(t, v, got)
	for _, mutate := range []func(*model.PayeeAccount){func(p *model.PayeeAccount) { p.OwnerNo = "other" }, func(p *model.PayeeAccount) { p.PayeeAccountNo = "other" }, func(p *model.PayeeAccount) { p.Method = "alipay" }, func(p *model.PayeeAccount) { p.MaskedAccount = "****9999" }, func(p *model.PayeeAccount) { p.Version++ }, func(p *model.PayeeAccount) { p.EncryptionKeyID = "missing" }, func(p *model.PayeeAccount) { p.EncryptedPayload = "invalid" }} {
		copy := p
		mutate(&copy)
		_, e = c.open(copy)
		require.ErrorIs(t, e, ErrCrypto)
	}
	c.random = brokenRandom{}
	require.ErrorIs(t, c.seal(&p, v), ErrCrypto)
	t.Setenv("PAYOUT_ENCRYPTION_KEYS", "")
	t.Setenv("PAYOUT_ACTIVE_KEY_ID", "")
	t.Setenv("PAYOUT_FINGERPRINT_KEY", "")
	_, e = LoadCrypto()
	require.ErrorIs(t, e, ErrCrypto)
	for _, key := range []string{"", "plain-secret", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 31)))} {
		_, e = NewCrypto("test", map[string]string{"test": key}, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32))))
		require.ErrorIs(t, e, ErrCrypto)
	}
}
func TestCryptoRotationRetainsOldReads(t *testing.T) {
	c := testCrypto(t)
	p := model.PayeeAccount{PayeeAccountNo: "old", PayloadVersion: 1}
	require.NoError(t, c.seal(&p, payload{Account: "private-account"}))
	rotated, e := NewCrypto("next", map[string]string{"test": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), "next": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("n", 32)))}, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("f", 32))))
	require.NoError(t, e)
	_, e = rotated.open(p)
	require.NoError(t, e)
	require.Equal(t, c.digest("x", "value"), rotated.digest("x", "value"))
}
func FuzzPayeeValidation(f *testing.F) {
	f.Add("bank_card", "6222021234567890123", "ICBC", "", "")
	f.Add("wechat", "anything", "", "openid", "wx1234567890abcdef")
	f.Fuzz(func(t *testing.T, method, account, bank, kind, app string) {
		if len(account) > 9000 {
			t.Skip()
		}
		_, masked, e := validate(req.CreatePayee{RequestID: "request01", Method: method, Currency: "CNY", AccountName: "test", Account: account, BankCode: bank, IdentifierType: kind, AppID: app})
		if e == nil {
			require.True(t, strings.HasPrefix(masked, "****"))
			require.NotContains(t, masked, account)
			require.LessOrEqual(t, len(masked), 20)
		}
	})
}

func TestOrderEnvelopeSeparationTamperAndEntropy(t *testing.T) {
	s := &Service{Crypto: testCrypto(t)}
	r := req.CreatePayee{RequestID: "binding_request", Method: "bank_card", Currency: "CNY", Account: "6222021234567890123", AccountName: "private", BankCode: "ICBC"}
	v, masked, e := validate(r)
	require.NoError(t, e)
	p := model.PayeeAccount{OwnerType: "anchor", OwnerNo: "anchor", PayeeAccountNo: "pa_0123456789abcdef0123456789abcdef", RequestID: r.RequestID, Method: r.Method, Currency: r.Currency, Version: 1, PayloadVersion: 1, MaskedAccount: masked}
	p.AccountFingerprint = s.fingerprint(p.OwnerNo, p.Method, p.Currency, v)
	require.NoError(t, s.Crypto.seal(&p, v))
	o := model.Order{PayoutNo: "po_0123456789abcdef0123456789abcdef", RequestID: "order_request", PayeeMethod: p.Method, Currency: p.Currency, AccountMasked: masked, Amount: 1, Status: "ready"}
	require.NoError(t, s.sealOrder(&o, p))
	require.NoError(t, s.VerifyOrder(o))
	require.NotEqual(t, p.EncryptedPayload, o.PayeeSnapshotEncrypted)
	for _, mutate := range []func(*model.Order){func(o *model.Order) { o.Amount++ }, func(o *model.Order) { o.PayoutNo += "x" }, func(o *model.Order) { o.BusinessNo = "other" }, func(o *model.Order) { o.PayeeAccountNo = "other" }, func(o *model.Order) {
		o.PayeeSnapshotEncrypted = p.EncryptedPayload
		o.PayeeSnapshotHash = snapshotHash(p.EncryptedPayload)
	}, func(o *model.Order) { o.EncryptionKeyID = "missing" }, func(o *model.Order) { o.ReconciliationSnapshot = "changed" }} {
		copy := o
		mutate(&copy)
		require.ErrorIs(t, s.VerifyOrder(copy), ErrCrypto)
	}
	s.Crypto.random = brokenRandom{}
	require.ErrorIs(t, s.sealOrder(&o, p), ErrCrypto)
}
