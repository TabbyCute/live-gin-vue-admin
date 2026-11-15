// Package telemetry records bounded process-local metadata. It never accepts
// customer identifiers, request bodies, error messages or remote metric labels.
package telemetry

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const maxRoutes = 512

var Bounds = [...]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

type HTTP struct {
	Route          string `json:"route"`
	Method         string `json:"method"`
	Calls          uint64 `json:"calls,string"`
	Errors         uint64 `json:"errors,string"`
	Rejected       uint64 `json:"rejected,string"`
	DurationMicros uint64 `json:"durationMicros,string"`
	// Buckets are disjoint; the last bucket contains durations over 10 seconds.
	Buckets [12]uint64 `json:"buckets"`
}

type Worker struct {
	Name                string `json:"name"`
	Runs                uint64 `json:"runs,string"`
	Failures            uint64 `json:"failures,string"`
	ConsecutiveFailures uint64 `json:"consecutiveFailures,string"`
	LastStartedAt       int64  `json:"lastStartedAt"`
	LastFinishedAt      int64  `json:"lastFinishedAt"`
	LastSuccessAt       int64  `json:"lastSuccessAt"`
	LastDurationMs      int64  `json:"lastDurationMs"`
	ErrorKind           string `json:"errorKind"`
	Running             int    `json:"running"`
	Stale               bool   `json:"stale"`
	staleAfter          time.Duration
}

type Snapshot struct {
	StartedAt int64     `json:"startedAt"`
	InFlight  int64     `json:"inFlight"`
	HTTP      []HTTP    `json:"http"`
	Workers   []Worker  `json:"workers"`
	BoundsMs  [11]int64 `json:"boundsMs"`
}

type Registry struct {
	mu       sync.Mutex
	started  time.Time
	inFlight atomic.Int64
	http     map[string]*HTTP
	workers  map[string]*Worker
}

func New() *Registry {
	return &Registry{started: time.Now(), http: make(map[string]*HTTP), workers: make(map[string]*Worker)}
}

var Default = New()

// Draining is set before HTTP shutdown so dependency probes stop admitting work.
var Draining atomic.Bool

func (r *Registry) BeginRequest() { r.inFlight.Add(1) }

// EndRequest requires a registered route template, never URL.Path. Call once
// for each BeginRequest, including when the handler panics.
func (r *Registry) EndRequest(method, route string, status int, elapsed time.Duration) {
	r.inFlight.Add(-1)
	if elapsed < 0 {
		elapsed = 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := method + " " + route
	entry := r.http[key]
	if entry == nil {
		if len(r.http) >= maxRoutes || len(route) > 256 || len(method) > 16 {
			key, method, route = "OTHER [overflow]", "OTHER", "[overflow]"
			entry = r.http[key]
		}
		if entry == nil {
			entry = &HTTP{Method: method, Route: route}
			r.http[key] = entry
		}
	}
	entry.Calls++
	if status >= 500 {
		entry.Errors++
	}
	if status == 429 || status == 503 {
		entry.Rejected++
	}
	entry.DurationMicros += uint64(elapsed.Microseconds())
	bucket := len(Bounds)
	for i, b := range Bounds {
		if elapsed <= time.Duration(b)*time.Millisecond {
			bucket = i
			break
		}
	}
	entry.Buckets[bucket]++
}

// Only fixed worker names can allocate state, preventing metric cardinality
// from growing with room IDs, users, tasks, error strings or adapters.
func workerAllowed(name string) bool {
	switch name {
	case "anchor_home_media_cleanup", "guard_fulfillment", "guard_expiry", "ranking_projection", "presence_checkpoint", "presence_projection",
		"social_audit", "social_history", "social_prune", "social_outbox", "event_delivery", "event_capture", "earning_audit", "live_readiness", "live_lifecycle", "live_stop", "live_reconciliation":
		return true
	}
	return false
}

func (r *Registry) StartWorker(name string, staleAfter time.Duration) func(error) {
	if !workerAllowed(name) {
		return func(error) {}
	}
	if staleAfter < time.Second {
		staleAfter = time.Second
	}
	start := time.Now()
	r.mu.Lock()
	w := r.workers[name]
	if w == nil {
		w = &Worker{Name: name}
		r.workers[name] = w
	}
	w.LastStartedAt, w.staleAfter = start.UnixMilli(), staleAfter
	w.Running++
	r.mu.Unlock()
	var once sync.Once
	return func(err error) {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			w.Running--
			w.Runs++
			w.LastFinishedAt, w.LastDurationMs = time.Now().UnixMilli(), time.Since(start).Milliseconds()
			w.ErrorKind = ErrorKind(err)
			if err == nil {
				w.LastSuccessAt = w.LastFinishedAt
				w.ConsecutiveFailures = 0
			} else {
				w.Failures++
				w.ConsecutiveFailures++
			}
		})
	}
}

func ErrorKind(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	default:
		return "failed"
	}
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := Snapshot{StartedAt: r.started.UnixMilli(), InFlight: r.inFlight.Load(), BoundsMs: Bounds, HTTP: []HTTP{}, Workers: []Worker{}}
	for _, v := range r.http {
		out.HTTP = append(out.HTTP, *v)
	}
	for _, v := range r.workers {
		copy := *v
		last := copy.LastFinishedAt
		if last < copy.LastStartedAt {
			last = copy.LastStartedAt
		}
		copy.Stale = time.Since(time.UnixMilli(last)) > copy.staleAfter
		out.Workers = append(out.Workers, copy)
	}
	sort.Slice(out.HTTP, func(i, j int) bool {
		if out.HTTP[i].Route == out.HTTP[j].Route {
			return out.HTTP[i].Method < out.HTTP[j].Method
		}
		return out.HTTP[i].Route < out.HTTP[j].Route
	})
	sort.Slice(out.Workers, func(i, j int) bool { return out.Workers[i].Name < out.Workers[j].Name })
	return out
}
