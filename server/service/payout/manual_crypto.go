package payout

import (
	"encoding/base64"
	"encoding/json"
	"io"
	model "tb_live_module/model/payout"
)

func (c *Crypto) sealBytes(key string, aad, plain []byte) ([]byte, error) {
	if c == nil || c.keys[key] == nil {
		return nil, ErrCrypto
	}
	a := c.keys[key]
	nonce := make([]byte, a.NonceSize())
	if _, e := io.ReadFull(c.random, nonce); e != nil {
		return nil, ErrCrypto
	}
	return a.Seal(nonce, nonce, plain, aad), nil
}
func (c *Crypto) openBytes(key string, aad, sealed []byte) ([]byte, error) {
	if c == nil || c.keys[key] == nil {
		return nil, ErrCrypto
	}
	a := c.keys[key]
	if len(sealed) < a.NonceSize()+a.Overhead() {
		return nil, ErrCrypto
	}
	b, e := a.Open(nil, sealed[:a.NonceSize()], sealed[a.NonceSize():], aad)
	if e != nil {
		return nil, ErrCrypto
	}
	return b, nil
}
func companyAAD(a model.ChannelAccount) []byte {
	b, _ := json.Marshal([]any{"payout.company.v1", a.ChannelAccountNo, a.ChannelCode, a.AccountName, a.InstitutionName, a.Currency, a.MaskedAccount, a.AccountFingerprint, a.SingleLimit, a.DailyLimit, a.CreatedBy, a.CreatedAt, a.EncryptionKeyID})
	return b
}
func (s *Service) sealCompany(a *model.ChannelAccount, account string) error {
	if s.Crypto == nil {
		return ErrCrypto
	}
	a.EncryptionKeyID = s.Crypto.active
	b, e := s.Crypto.sealBytes(a.EncryptionKeyID, companyAAD(*a), []byte(account))
	if e != nil {
		return e
	}
	a.EncryptedAccount = base64.StdEncoding.EncodeToString(b)
	return nil
}
func (s *Service) openCompany(a model.ChannelAccount) (string, error) {
	if len(a.EncryptedAccount) > 8192 {
		return "", ErrCrypto
	}
	b, e := base64.StdEncoding.Strict().DecodeString(a.EncryptedAccount)
	if e != nil {
		return "", ErrCrypto
	}
	b, e = s.Crypto.openBytes(a.EncryptionKeyID, companyAAD(a), b)
	if e != nil {
		return "", e
	}
	v := string(b)
	if !companyAccountRE.MatchString(v) || a.MaskedAccount != maskCompany(v) || s.Crypto.digest("payout.company.fingerprint.v1", []string{a.ChannelCode, a.Currency, v}) != a.AccountFingerprint {
		return "", ErrCrypto
	}
	return v, nil
}
func (s *Service) openOrder(o model.Order) (payload, error) {
	var v payload
	if e := s.VerifyOrder(o); e != nil {
		return v, e
	}
	b, e := base64.StdEncoding.Strict().DecodeString(o.PayeeSnapshotEncrypted)
	if e != nil {
		return v, ErrCrypto
	}
	b, e = s.Crypto.openBytes(o.EncryptionKeyID, orderAAD(o), b)
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrCrypto
	}
	return v, nil
}
func evidenceAAD(e model.Evidence) []byte {
	b, _ := json.Marshal([]any{"payout.evidence.v1", e.EvidenceNo, e.PayoutNo, e.AttemptNo, e.ObjectReference, e.SHA256, e.SourceSHA256, e.MIME, e.Size, e.CreatedBy, e.CreatedAt, e.EncryptionKeyID})
	return b
}
