// Package testpay is an isolated test channel, never imported by production initialization.
package testpay

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"net"
	"strings"
	"sync"
)

var ErrUnknown = errors.New("simulator: response lost, query the original order")
var ErrSignature = errors.New("simulator: invalid signature or order binding")
var ErrConflict = errors.New("simulator: conflicting request")

type Order struct {
	No        string `json:"no"`
	AccountNo string `json:"accountNo"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
}
type Event struct {
	Order   Order  `json:"order"`
	Version uint64 `json:"version"`
	Status  string `json:"status"`
}
type Envelope struct {
	Payload   []byte
	Signature string
}

// Channel is replaceable at the test harness boundary. Real integration needs
// merchant configuration, provider cryptography and a durable verified inbox.
type Channel interface {
	Submit(context.Context, Order, bool) (Envelope, error)
	Query(context.Context, string) (Envelope, error)
	Verify(Envelope) (Event, error)
}
type Simulator struct {
	mu     sync.Mutex
	key    []byte
	orders map[string]Event
}

func New(localDSN string) (*Simulator, error) {
	cfg, e := driver.ParseDSN(localDSN)
	if e != nil {
		return nil, ErrConflict
	}
	host, _, e := net.SplitHostPort(cfg.Addr)
	if e != nil || cfg.Net != "tcp" || host != "127.0.0.1" || (!strings.HasPrefix(cfg.DBName, "bank_test_") && !strings.HasPrefix(cfg.DBName, "earning_test_")) {
		return nil, errors.New("simulator requires explicit loopback disposable database DSN")
	}
	key := make([]byte, 32)
	if _, e = rand.Read(key); e != nil {
		return nil, e
	}
	return &Simulator{key: key, orders: map[string]Event{}}, nil
}
func (s *Simulator) sign(v Event) Envelope {
	b, _ := json.Marshal(v)
	m := hmac.New(sha256.New, s.key)
	m.Write(b)
	return Envelope{b, hex.EncodeToString(m.Sum(nil))}
}
func (s *Simulator) Submit(ctx context.Context, o Order, loseResponse bool) (Envelope, error) {
	if e := ctx.Err(); e != nil {
		return Envelope{}, e
	}
	if len(o.No) < 8 || len(o.No) > 64 || o.AccountNo == "" || o.Amount <= 0 || o.Currency != "CNY" {
		return Envelope{}, ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.orders[o.No]
	if ok && v.Order != o {
		return Envelope{}, ErrConflict
	}
	if !ok {
		v = Event{Order: o, Version: 1, Status: "pending"}
		s.orders[o.No] = v
	}
	if loseResponse {
		return Envelope{}, ErrUnknown
	}
	return s.sign(v), nil
}
func (s *Simulator) Complete(no string) (Envelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.orders[no]
	if !ok {
		return Envelope{}, ErrConflict
	}
	if v.Status != "succeeded" {
		v.Status = "succeeded"
		v.Version++
		s.orders[no] = v
	}
	return s.sign(v), nil
}
func (s *Simulator) Query(ctx context.Context, no string) (Envelope, error) {
	if e := ctx.Err(); e != nil {
		return Envelope{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.orders[no]
	if !ok {
		return Envelope{}, ErrConflict
	}
	return s.sign(v), nil
}
func (s *Simulator) Verify(envelope Envelope) (Event, error) {
	var v Event
	if len(envelope.Payload) > 4096 {
		return v, ErrSignature
	}
	sig, e := hex.DecodeString(envelope.Signature)
	if e != nil {
		return v, ErrSignature
	}
	m := hmac.New(sha256.New, s.key)
	m.Write(envelope.Payload)
	if !hmac.Equal(sig, m.Sum(nil)) || json.Unmarshal(envelope.Payload, &v) != nil {
		return Event{}, ErrSignature
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	known, ok := s.orders[v.Order.No]
	if !ok || known.Order != v.Order || v.Version == 0 || v.Version > known.Version || (v.Status != "pending" && v.Status != "succeeded") {
		return Event{}, ErrSignature
	}
	return v, nil
}

// Deliver verifies first. apply MUST be durable/idempotent by Order.No. The
// harness uses existing wallet adjustments and independent manual approval.
func Deliver(channel Channel, envelope Envelope, apply func(Order) error) error {
	v, e := channel.Verify(envelope)
	if e != nil {
		return e
	}
	if v.Status != "succeeded" {
		return nil
	}
	return apply(v.Order)
}
