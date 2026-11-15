package eventdelivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"tb_live_module/internal/telemetry"
	"time"
)

const Consumer = "realtime.v1"

var (
	ErrInvalid     = errors.New("EVENT_DELIVERY_INVALID")
	ErrForbidden   = errors.New("EVENT_DELIVERY_FORBIDDEN")
	ErrConflict    = errors.New("EVENT_DELIVERY_CONFLICT")
	ErrUnavailable = errors.New("EVENT_DELIVERY_UNAVAILABLE")
	ErrLeaseLost   = errors.New("EVENT_DELIVERY_LEASE_LOST")
)

// Envelope is the entire data allowed to cross the future IM boundary. Destination
// is trusted routing metadata (room session or private recipient), never a bearer token.
type Envelope struct {
	EventID        string `json:"eventId"`
	IdempotencyKey string `json:"idempotencyKey"`
	EventType      string `json:"eventType"`
	SchemaVersion  uint8  `json:"schemaVersion"`
	Audience       string `json:"audience"`
	Destination    string `json:"destination"`
	Payload        string `json:"payload"`
	OccurredAt     int64  `json:"occurredAt"`
}

// Ack is valid only after the downstream consumer durably records the idempotency
// key and effect atomically. Accepted by an HTTP proxy is not a delivery receipt.
type Ack struct {
	EventID, IdempotencyKey string
	Durable                 bool
}

// Adapter must honor context, be concurrency safe and deduplicate IdempotencyKey
// durably across restarts. Retries and unknown outcomes use the IDENTICAL key.
// This guarantee is at-least-once transport, not exactly-once network delivery.
// Adapter also enforces audience authorization and must not broadcast private payouts.
type Adapter interface {
	Deliver(context.Context, Envelope) (Ack, error)
}
type PermanentError struct{}

func (PermanentError) Error() string { return "PERMANENT_REJECTION" }

type Options struct {
	Batch, Concurrency                      int
	MaxAttempts                             uint32
	Lease, Timeout, BaseBackoff, MaxBackoff time.Duration
}

func (o Options) normalized() Options {
	if o.Batch < 1 || o.Batch > 200 {
		o.Batch = 50
	}
	if o.Concurrency < 1 || o.Concurrency > 8 {
		o.Concurrency = 4
	}
	if o.MaxAttempts < 1 || o.MaxAttempts > 20 {
		o.MaxAttempts = 8
	}
	if o.Timeout < time.Millisecond || o.Timeout > 30*time.Second {
		o.Timeout = 5 * time.Second
	}
	if o.Lease < o.Timeout+5*time.Second || o.Lease > 5*time.Minute {
		o.Lease = o.Timeout + 10*time.Second
	}
	if o.BaseBackoff < time.Millisecond || o.BaseBackoff > time.Minute {
		o.BaseBackoff = time.Second
	}
	if o.MaxBackoff < o.BaseBackoff || o.MaxBackoff > time.Hour {
		o.MaxBackoff = 5 * time.Minute
	}
	return o
}

type Service struct {
	DB          *gorm.DB
	Adapter     Adapter // immutable after worker starts; replace via restart
	Options     Options
	Now         func() time.Time
	tickMu      sync.Mutex
	lastTick    atomic.Int64
	lastSuccess atomic.Int64
	failures    atomic.Uint64
	lastCode    atomic.Value
}

var Default = &Service{}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Default} }
func (s *Service) now() int64 {
	if s.Now != nil {
		return s.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func stableID(source, id string) string {
	h := sha256.Sum256([]byte(source + "\x00" + id))
	return "evt_" + hex.EncodeToString(h[:])
}
func idempotencyKey(id string) string { return Consumer + ":" + id }
func hashPayload(payload string) string {
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for ctx.Err() == nil {
		run, cancel := context.WithTimeout(ctx, 45*time.Second)
		finished := telemetry.Default.StartWorker("event_delivery", time.Minute)
		err := s.Tick(run)
		finished(err)
		cancel()
		if err != nil {
			s.failures.Add(1)
			s.lastCode.Store("WORKER_FAILED")
		} else {
			s.lastSuccess.Store(s.now())
			s.lastCode.Store("")
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// StartWorker joins on cancellation; the caller owns the single process lifecycle.
func (s *Service) StartWorker(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	return done
}
func (s *Service) checkDB() error {
	if s == nil || s.DB == nil {
		return ErrUnavailable
	}
	return nil
}
func safeError(err error) string {
	switch {
	case errors.Is(err, ErrForbidden):
		return ErrForbidden.Error()
	case errors.Is(err, ErrInvalid):
		return ErrInvalid.Error()
	case errors.Is(err, ErrConflict):
		return ErrConflict.Error()
	default:
		return ErrUnavailable.Error()
	}
}
func wrapSchema(name string) error { return fmt.Errorf("event delivery schema invalid: %s", name) }
