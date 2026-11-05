package payout

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"gorm.io/gorm"
	"io"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
)

func orderAAD(o model.Order) []byte {
	// v1 sealed updated_at equalled created_at. Keep that authenticated value
	// forever while allowing the separately protected state timestamp to advance.
	b, _ := json.Marshal([]any{"payout.order.v1", o.PayoutNo, o.RequestID, o.RequestHash, o.BusinessType, o.BusinessNo, o.AnchorID, o.AnchorNo, o.ApplicantUserID, o.Amount, o.Currency, o.PayeeAccountNo, o.PayeeVersion, o.PayeeMethod, o.AccountName, o.AccountMasked, o.EncryptionKeyID, o.PayloadVersion, o.ReconciliationSnapshot, o.CreatedBy, o.CreatedAt, o.CreatedAt})
	return b
}
func snapshotHash(ciphertext string) string {
	h := sha256.Sum256([]byte(ciphertext))
	return hex.EncodeToString(h[:])
}
func (s *Service) sealOrder(o *model.Order, p model.PayeeAccount) error {
	if err := s.VerifyBinding(p); err != nil {
		return err
	}
	v, err := s.Crypto.open(p)
	if err != nil {
		return err
	}
	if s.Crypto.keys[s.Crypto.active] == nil {
		return ErrCrypto
	}
	o.EncryptionKeyID = s.Crypto.active
	o.PayloadVersion = 1
	a := s.Crypto.keys[o.EncryptionKeyID]
	nonce := make([]byte, a.NonceSize())
	if _, err = io.ReadFull(s.Crypto.random, nonce); err != nil {
		return ErrCrypto
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ErrCrypto
	}
	o.PayeeSnapshotEncrypted = base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, b, orderAAD(*o)))
	o.PayeeSnapshotHash = snapshotHash(o.PayeeSnapshotEncrypted)
	return nil
}

// VerifyOrder authenticates an order-specific envelope without exposing plaintext.
// The namespace and immutable order metadata prevent copying binding ciphertext.
func (s *Service) VerifyOrder(o model.Order) error {
	if !ValidOrderStatus(o.Status) {
		return ErrCrypto
	}
	if s.Crypto == nil || o.PayloadVersion != 1 || s.Crypto.keys[o.EncryptionKeyID] == nil || len(o.PayeeSnapshotEncrypted) > 8192 || o.PayeeSnapshotHash != snapshotHash(o.PayeeSnapshotEncrypted) {
		return ErrCrypto
	}
	a := s.Crypto.keys[o.EncryptionKeyID]
	b, e := base64.StdEncoding.Strict().DecodeString(o.PayeeSnapshotEncrypted)
	if e != nil || len(b) < a.NonceSize()+a.Overhead() {
		return ErrCrypto
	}
	b, e = a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], orderAAD(o))
	if e != nil {
		return ErrCrypto
	}
	var v payload
	if json.Unmarshal(b, &v) != nil {
		return ErrCrypto
	}
	_, masked, e := validate(req.CreatePayee{RequestID: o.RequestID, Method: o.PayeeMethod, Currency: o.Currency, AccountName: v.AccountName, Account: v.Account, BankCode: v.BankCode, AppID: v.AppID, IdentifierType: v.IdentifierType})
	if e != nil || masked != o.AccountMasked {
		return ErrCrypto
	}
	return nil
}
func (s *Service) ValidateOrderKeys(db *gorm.DB) error {
	var ids []string
	if e := db.Model(&model.Order{}).Distinct("encryption_key_id").Limit(17).Pluck("encryption_key_id", &ids).Error; e != nil {
		return e
	}
	if len(ids) > 16 {
		return ErrCrypto
	}
	for _, id := range ids {
		var o model.Order
		if e := db.Where("encryption_key_id=?", id).Order("id ASC").Take(&o).Error; e != nil {
			return e
		}
		if e := s.VerifyOrder(o); e != nil {
			return e
		}
	}
	return nil
}
