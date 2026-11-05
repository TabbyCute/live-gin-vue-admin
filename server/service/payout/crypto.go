package payout

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"regexp"
	model "tb_live_module/model/payout"
)

// Keys are immutable startup dependencies. Old keys remain available for reading
// old envelopes; a new active key affects only new accounts, never financial facts.
type Crypto struct {
	active      string
	keys        map[string]cipher.AEAD
	fingerprint []byte
	random      io.Reader
}

var keyIDRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// LoadCrypto never accepts keys through database-backed system configuration.
func LoadCrypto() (*Crypto, error) {
	var keys map[string]string
	if json.Unmarshal([]byte(os.Getenv("PAYOUT_ENCRYPTION_KEYS")), &keys) != nil {
		return nil, ErrCrypto
	}
	return NewCrypto(os.Getenv("PAYOUT_ACTIVE_KEY_ID"), keys, os.Getenv("PAYOUT_FINGERPRINT_KEY"))
}
func NewCrypto(active string, keys map[string]string, fingerprint string) (*Crypto, error) {
	if !keyIDRE.MatchString(active) || len(keys) == 0 || len(keys) > 16 {
		return nil, ErrCrypto
	}
	fp, e := base64.StdEncoding.Strict().DecodeString(fingerprint)
	if e != nil || len(fp) != 32 {
		return nil, ErrCrypto
	}
	c := &Crypto{active: active, keys: map[string]cipher.AEAD{}, fingerprint: fp, random: rand.Reader}
	for id, encoded := range keys {
		key, e := base64.StdEncoding.Strict().DecodeString(encoded)
		if e != nil || len(key) != 32 || !keyIDRE.MatchString(id) || hmac.Equal(key, fp) {
			return nil, ErrCrypto
		}
		block, e := aes.NewCipher(key)
		if e != nil {
			return nil, ErrCrypto
		}
		aead, e := cipher.NewGCM(block)
		if e != nil {
			return nil, ErrCrypto
		}
		c.keys[id] = aead
	}
	if c.keys[active] == nil {
		return nil, ErrCrypto
	}
	return c, nil
}
func (c *Crypto) digest(domain string, value any) string {
	b, _ := json.Marshal(value)
	h := hmac.New(sha256.New, c.fingerprint)
	h.Write([]byte(domain + "\x00"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func aad(p model.PayeeAccount) []byte {
	b, _ := json.Marshal([]any{"payout.payee.v1", p.PayeeAccountNo, p.OwnerType, p.OwnerNo, p.Method, p.Currency, p.Version, p.PayloadVersion, p.EncryptionKeyID, p.AccountFingerprint, p.MaskedAccount, p.CreatedAt, p.RequestID, p.RequestHash})
	return b
}
func (c *Crypto) seal(p *model.PayeeAccount, v payload) error {
	if c == nil || c.keys[c.active] == nil {
		return ErrCrypto
	}
	p.EncryptionKeyID = c.active
	a := c.keys[c.active]
	nonce := make([]byte, a.NonceSize())
	if _, e := io.ReadFull(c.random, nonce); e != nil {
		return ErrCrypto
	}
	b, e := json.Marshal(v)
	if e != nil {
		return ErrCrypto
	}
	p.EncryptedPayload = base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, b, aad(*p)))
	return nil
}
func (c *Crypto) open(p model.PayeeAccount) (payload, error) {
	var v payload
	if c == nil || c.keys[p.EncryptionKeyID] == nil || len(p.EncryptedPayload) > 8192 {
		return v, ErrCrypto
	}
	a := c.keys[p.EncryptionKeyID]
	b, e := base64.StdEncoding.Strict().DecodeString(p.EncryptedPayload)
	if e != nil || len(b) < a.NonceSize()+a.Overhead() {
		return v, ErrCrypto
	}
	b, e = a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], aad(p))
	if e != nil {
		return v, ErrCrypto
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrCrypto
	}
	return v, nil
}
