package eventdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"tb_live_module/internal/testlocal"
	model "tb_live_module/model/eventdelivery"
	gift "tb_live_module/model/gift"
	live "tb_live_module/model/live"
	payout "tb_live_module/model/payout"
	"testing"
	"time"
)

// Test-only durable consumer. There is no production mock HTTP endpoint.
type durableFake struct {
	mu           sync.Mutex
	effects      map[string]bool
	calls        []Envelope
	unknownFirst bool
	badAck       bool
	permanent    bool
	active       atomic.Int32
	peak         atomic.Int32
}

func (f *durableFake) Deliver(ctx context.Context, e Envelope) (Ack, error) {
	active := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		p := f.peak.Load()
		if active <= p || f.peak.CompareAndSwap(p, active) {
			break
		}
	}
	select {
	case <-ctx.Done():
		return Ack{}, ctx.Err()
	case <-time.After(2 * time.Millisecond):
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.effects == nil {
		f.effects = map[string]bool{}
	}
	f.calls = append(f.calls, e)
	if f.permanent {
		return Ack{}, PermanentError{}
	}
	first := !f.effects[e.IdempotencyKey]
	f.effects[e.IdempotencyKey] = true
	if f.unknownFirst && first {
		return Ack{}, errors.New("upstream secret URL token=never-persist")
	}
	ack := Ack{EventID: e.EventID, IdempotencyKey: e.IdempotencyKey, Durable: true}
	if f.badAck {
		ack.IdempotencyKey = "wrong"
	}
	return ack, nil
}
func TestSQLiteDelivery(t *testing.T) { runSuite(t, "") }
func TestMySQLDelivery(t *testing.T) {
	dsn := os.Getenv("EVENT_DELIVERY_MYSQL_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL DSN not set")
	}
	runSuite(t, dsn)
}
func setup(t *testing.T, dsn string) (*Service, *atomic.Int64) {
	t.Helper()
	var db *gorm.DB
	var err error
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	if dsn == "" {
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "delivery.db")), cfg)
	} else {
		validated, validationErr := testlocal.EventDeliveryMySQL(dsn)
		require.NoError(t, validationErr, "refusing non-local/destructive fixture target before connection")
		db, err = gorm.Open(mysql.Open(validated), cfg)
	}
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	if dsn == "" {
		pool.SetMaxOpenConns(1)
	} else {
		pool.SetMaxOpenConns(12)
	}
	t.Cleanup(func() { pool.Close() })
	for _, table := range []string{"event_delivery_event", "event_delivery", "event_delivery_attempt", "event_delivery_retry_audit", "event_delivery_cursor", "test_consumer_receipt", "gift_event_outbox", "payout_terminal_event", "payout_order", "live_room_event", "sys_users"} {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+table).Error)
	}
	require.NoError(t, Migrate(db))
	require.NoError(t, ValidateSchema(db))
	require.NoError(t, Migrate(db))
	require.NoError(t, db.AutoMigrate(&gift.Event{}, &payout.TerminalEvent{}, &live.LiveRoomEvent{}))
	require.NoError(t, db.Exec("CREATE TABLE payout_order (payout_no VARCHAR(40) PRIMARY KEY,applicant_user_id BIGINT NOT NULL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE sys_users (id BIGINT PRIMARY KEY,enable INTEGER NOT NULL,deleted_at DATETIME NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO sys_users (id,enable) VALUES (1,1),(2,1)").Error)
	clock := &atomic.Int64{}
	clock.Store(time.Now().UnixMilli())
	return &Service{DB: db, Now: func() time.Time { return time.UnixMilli(clock.Load()) }, Options: Options{Concurrency: 1, BaseBackoff: time.Millisecond, MaxBackoff: time.Second}}, clock
}
func giftEvent(id string, at int64) gift.Event {
	return gift.Event{EventID: id, GiftOrderNo: id, EventType: "gift.sent", RoomNo: "room_1", SessionNo: "session_1", Payload: `{"token":"secret","walletBalance":999}`, DeliveryStatus: "pending", CreatedAt: at}
}
func enqueue(t *testing.T, s *Service, id string) model.Delivery {
	t.Helper()
	require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error { return s.Gift(tx, giftEvent(id, s.now())) }))
	var d model.Delivery
	require.NoError(t, s.DB.Where("event_id=?", stableID("gift", id)).Take(&d).Error)
	return d
}
func fetch(t *testing.T, s *Service, id uint64) model.Delivery {
	t.Helper()
	var d model.Delivery
	require.NoError(t, s.DB.Where("id=?", id).Take(&d).Error)
	return d
}

type testConsumerReceipt struct {
	Key    string `gorm:"size:128;primaryKey"`
	Effect string `gorm:"size:72;not null"`
}

func (testConsumerReceipt) TableName() string { return "test_consumer_receipt" }

type databaseConsumer struct {
	db        *gorm.DB
	loseReply bool
}

func (c *databaseConsumer) Deliver(ctx context.Context, e Envelope) (Ack, error) {
	row := testConsumerReceipt{Key: e.IdempotencyKey, Effect: e.EventID}
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error }); err != nil {
		return Ack{}, err
	}
	if c.loseReply {
		return Ack{}, errors.New("ack lost after consumer commit")
	}
	return Ack{EventID: e.EventID, IdempotencyKey: e.IdempotencyKey, Durable: true}, nil
}
func runSuite(t *testing.T, dsn string) {
	ctx := context.Background()

	t.Run("durable_consumer_and_worker_restart", func(t *testing.T) {
		s, clock := setup(t, dsn)
		require.NoError(t, s.DB.AutoMigrate(&testConsumerReceipt{}))
		s.Adapter = &databaseConsumer{db: s.DB, loseReply: true}
		d := enqueue(t, s, "gift_consumer_restart")
		require.NoError(t, s.Dispatch(ctx))
		d = fetch(t, s, d.ID)
		require.Equal(t, "pending", d.Status)
		clock.Store(d.NextAttemptAt)
		restarted := &Service{DB: s.DB, Adapter: &databaseConsumer{db: s.DB}, Now: s.Now, Options: s.Options}
		require.NoError(t, restarted.Dispatch(ctx))
		require.Equal(t, "delivered", fetch(t, s, d.ID).Status)
		var n int64
		require.NoError(t, s.DB.Model(&testConsumerReceipt{}).Count(&n).Error)
		require.EqualValues(t, 1, n)
	})

	t.Run("schema_rejects_weakened_check", func(t *testing.T) {
		s, _ := setup(t, dsn)
		if dsn != "" {
			require.NoError(t, s.DB.Exec("ALTER TABLE event_delivery DROP CHECK ck_delivery_status, ADD CONSTRAINT ck_delivery_status CHECK (1=1)").Error)
		} else {
			var ddl string
			require.NoError(t, s.DB.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='event_delivery'").Scan(&ddl).Error)
			changed := strings.Replace(ddl, "status IN ('pending','leased','blocked','delivered','dead')", "1=1", 1)
			require.NotEqual(t, ddl, changed)
			require.NoError(t, s.DB.Exec("DROP TABLE event_delivery").Error)
			require.NoError(t, s.DB.Exec(changed).Error)
		}
		require.Error(t, validateDefinitions(s.DB))
	})
	t.Run("schema_rejects_nonunique_dedup_index", func(t *testing.T) {
		s, _ := setup(t, dsn)
		require.NoError(t, s.DB.Migrator().DropIndex(&model.Delivery{}, "uk_delivery_consumer"))
		require.NoError(t, s.DB.Exec("CREATE INDEX uk_delivery_consumer ON event_delivery (event_id,consumer)").Error)
		require.Error(t, ValidateSchema(s.DB))
	})
	t.Run("blocked_and_root_authorization", func(t *testing.T) {
		s, _ := setup(t, dsn)
		d := enqueue(t, s, "gift_blocked")
		require.Equal(t, "blocked", d.Status)
		require.NoError(t, s.Dispatch(ctx))
		require.Zero(t, fetch(t, s, d.ID).Attempts)
		r := RetryRequest{RequestID: "retry_request_1", DeliveryID: d.ID, Reason: "channel setup reviewed"}
		_, err := s.Retry(ctx, 2, r)
		require.ErrorIs(t, err, ErrForbidden)
		_, err = s.Retry(ctx, 1, r)
		require.ErrorIs(t, err, ErrUnavailable)
		_, err = s.List(ctx, 2, Search{})
		require.ErrorIs(t, err, ErrForbidden)
		_, err = s.Detail(ctx, 2, d.ID)
		require.ErrorIs(t, err, ErrForbidden)
		snap, err := s.Snapshot(ctx)
		require.NoError(t, err)
		require.False(t, snap.AdapterConfigured)
		require.EqualValues(t, 1, snap.Counts["blocked"])
		require.NoError(t, s.DB.Exec("UPDATE sys_users SET enable=0 WHERE id=1").Error)
		_, err = s.List(ctx, 1, Search{})
		require.ErrorIs(t, err, ErrForbidden)
	})
	t.Run("unknown_retries_same_key_one_effect", func(t *testing.T) {
		s, clock := setup(t, dsn)
		fake := &durableFake{unknownFirst: true}
		s.Adapter = fake
		d := enqueue(t, s, "gift_unknown")
		require.NoError(t, s.Dispatch(ctx))
		d = fetch(t, s, d.ID)
		require.Equal(t, "pending", d.Status)
		require.Equal(t, "DELIVERY_UNKNOWN", d.LastCode)
		require.Greater(t, d.NextAttemptAt, clock.Load())
		require.NoError(t, s.Dispatch(ctx))
		require.Len(t, fake.calls, 1)
		clock.Store(d.NextAttemptAt)
		restarted := &Service{DB: s.DB, Adapter: fake, Now: s.Now, Options: s.Options}
		require.NoError(t, restarted.Dispatch(ctx))
		d = fetch(t, s, d.ID)
		require.Equal(t, "delivered", d.Status)
		require.EqualValues(t, 2, d.Attempts)
		require.Len(t, fake.effects, 1)
		require.Equal(t, fake.calls[0].IdempotencyKey, fake.calls[1].IdempotencyKey)
		detail, err := s.Detail(ctx, 1, d.ID)
		require.NoError(t, err)
		raw, err := json.Marshal(detail)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "secret")
		require.NotContains(t, string(raw), "token")
		require.NotContains(t, string(raw), "walletBalance")
		_, err = s.Retry(ctx, 1, RetryRequest{RequestID: "retry_delivered", DeliveryID: d.ID, Reason: "must not resend success"})
		require.ErrorIs(t, err, ErrConflict)
	})
	t.Run("restart_fences_old_lease", func(t *testing.T) {
		s, clock := setup(t, dsn)
		s.Adapter = &durableFake{}
		d := enqueue(t, s, "gift_lease")
		old, ok, err := s.claim(ctx, s.Options.normalized())
		require.NoError(t, err)
		require.True(t, ok)
		clock.Store(old.LeaseUntil + 1)
		fresh, ok, err := s.claim(ctx, s.Options.normalized())
		require.NoError(t, err)
		require.True(t, ok)
		require.NotEqual(t, old.LeaseToken, fresh.LeaseToken)
		require.ErrorIs(t, s.finish(ctx, old, s.Options.normalized(), "ACKNOWLEDGED", true, false), ErrLeaseLost)
		require.NoError(t, s.deliver(ctx, fresh, s.Options.normalized()))
		require.Equal(t, "delivered", fetch(t, s, d.ID).Status)
		var first model.Attempt
		require.NoError(t, s.DB.Where("delivery_id=? AND number=1", d.ID).Take(&first).Error)
		require.Equal(t, "LEASE_EXPIRED_UNKNOWN", first.Outcome)
	})
	t.Run("dead_letter_ack_and_controlled_retry", func(t *testing.T) {
		s, clock := setup(t, dsn)
		fake := &durableFake{badAck: true}
		s.Adapter = fake
		s.Options.MaxAttempts = 2
		d := enqueue(t, s, "gift_dead")
		require.NoError(t, s.Dispatch(ctx))
		clock.Store(fetch(t, s, d.ID).NextAttemptAt)
		require.NoError(t, s.Dispatch(ctx))
		d = fetch(t, s, d.ID)
		require.Equal(t, "dead", d.Status)
		require.Zero(t, d.DeliveredAt)
		fake.badAck = false
		r := RetryRequest{RequestID: "retry_dead_once", DeliveryID: d.ID, Generation: 0, Reason: "downstream receipt verified"}
		audit, err := s.Retry(ctx, 1, r)
		require.NoError(t, err)
		again, err := s.Retry(ctx, 1, r)
		require.NoError(t, err)
		require.Equal(t, audit, again)
		conflict := r
		conflict.Reason = "different operator rationale"
		_, err = s.Retry(ctx, 1, conflict)
		require.ErrorIs(t, err, ErrConflict)
		require.NoError(t, s.Dispatch(ctx))
		d = fetch(t, s, d.ID)
		require.Equal(t, "delivered", d.Status)
		require.EqualValues(t, 3, d.Attempts)
		require.EqualValues(t, 1, d.RetryGeneration)
		require.Len(t, fake.effects, 1)
		require.Error(t, s.DB.Model(&model.RetryAudit{}).Where("request_id=?", r.RequestID).Update("reason", "changed").Error)
	})
	t.Run("bounded_concurrent_claims", func(t *testing.T) {
		s, _ := setup(t, dsn)
		fake := &durableFake{}
		s.Adapter = fake
		s.Options.Batch = 5
		s.Options.Concurrency = 2
		for i := 0; i < 20; i++ {
			enqueue(t, s, fmt.Sprintf("gift_batch_%02d", i))
		}
		second := &Service{DB: s.DB, Adapter: fake, Now: s.Now, Options: s.Options}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, worker := range []*Service{s, second} {
			wg.Add(1)
			go func(s *Service) { defer wg.Done(); errs <- s.Dispatch(ctx) }(worker)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		// SKIP LOCKED is allowed to return early while peers hold qualifying rows;
		// the contract is an upper bound, followed by eventual drain on later ticks.
		require.Greater(t, len(fake.calls), 0)
		require.LessOrEqual(t, len(fake.calls), 10)
		require.Len(t, fake.effects, len(fake.calls))
		require.LessOrEqual(t, fake.peak.Load(), int32(4))
		var count int64
		require.NoError(t, s.DB.Model(&model.Delivery{}).Where("status='pending'").Count(&count).Error)
		require.EqualValues(t, 20-len(fake.calls), count)
		for i := 0; i < 20 && len(fake.calls) < 20; i++ {
			require.NoError(t, s.Dispatch(ctx))
		}
		require.Len(t, fake.calls, 20)
		require.Len(t, fake.effects, 20)
		require.NoError(t, s.DB.Model(&model.Delivery{}).Where("status<>'delivered'").Count(&count).Error)
		require.Zero(t, count)
	})
	t.Run("rollback_dedup_immutable_and_payload_allowlist", func(t *testing.T) {
		s, _ := setup(t, dsn)
		s.Adapter = &durableFake{}
		event := giftEvent("gift_atomic", s.now())
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			if e := tx.Create(&event).Error; e != nil {
				return e
			}
			if e := s.Gift(tx, event); e != nil {
				return e
			}
			return errors.New("rollback")
		})
		require.Error(t, err)
		var count int64
		require.NoError(t, s.DB.Model(&model.Event{}).Count(&count).Error)
		require.Zero(t, count)
		require.NoError(t, s.DB.Model(&gift.Event{}).Count(&count).Error)
		require.Zero(t, count)
		require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error { return s.Gift(tx, event) }))
		require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error { return s.Gift(tx, event) }))
		require.NoError(t, s.DB.Model(&model.Event{}).Count(&count).Error)
		require.EqualValues(t, 1, count)
		var e model.Event
		require.NoError(t, s.DB.First(&e).Error)
		require.True(t, validEnvelope(e))
		require.NotContains(t, e.Payload, "secret")
		require.Error(t, s.DB.Model(&e).Update("payload", `{"token":"secret"}`).Error)
		require.Error(t, s.DB.Delete(&e).Error)
		e.Payload = `{"orderNo":"gift_atomic","roomNo":"room_1","sessionNo":"session_1","token":"secret"}`
		e.PayloadHash = hashPayload(e.Payload)
		require.False(t, validEnvelope(e))
		private := payout.TerminalEvent{EventNo: "payout_event", PayoutNo: "payout_1", Kind: "payout.succeeded", Amount: 98765, CreatedAt: s.now()}
		require.NoError(t, s.DB.Transaction(func(tx *gorm.DB) error { return s.Payout(tx, private, 42) }))
		e = model.Event{}
		require.NoError(t, s.DB.Where("source='payout'").Take(&e).Error)
		require.Equal(t, "user", e.Audience)
		require.Equal(t, "42", e.Destination)
		require.NotContains(t, e.Payload, "98765")
		require.True(t, validEnvelope(e))
		e.Audience = "room"
		require.False(t, validEnvelope(e))
	})

	t.Run("bad_source_does_not_starve_other_sources", func(t *testing.T) {
		s, _ := setup(t, dsn)
		bad := giftEvent("bad!gift", s.now())
		require.NoError(t, s.DB.Create(&bad).Error)
		private := payout.TerminalEvent{EventNo: "payout_isolated", PayoutNo: "payout_isolated", Kind: "payout.succeeded", CreatedAt: s.now()}
		require.NoError(t, s.DB.Create(&private).Error)
		require.NoError(t, s.DB.Exec("INSERT INTO payout_order VALUES ('payout_isolated',42)").Error)
		room := live.LiveRoomEvent{EventID: "presence_isolated", EventType: "room.viewer.entered", RoomNo: "room_1", SessionNo: "session_1", OccurredAt: s.now(), Payload: "{token:secret}"}
		require.NoError(t, s.DB.Create(&room).Error)
		require.Error(t, s.CaptureSources(ctx))
		var n int64
		require.NoError(t, s.DB.Model(&model.Event{}).Where("source IN ?", []string{"payout", "presence"}).Count(&n).Error)
		require.EqualValues(t, 2, n)
		require.NoError(t, s.DB.Model(&model.Cursor{}).Where("source='gift'").Count(&n).Error)
		require.Zero(t, n)
	})
	t.Run("cursor_wrap_repairs_late_commit_keeps_source", func(t *testing.T) {
		s, _ := setup(t, dsn)
		event := giftEvent("gift_20", s.now())
		event.ID = 20
		require.NoError(t, s.DB.Create(&event).Error)
		require.NoError(t, s.CaptureSources(ctx))
		late := giftEvent("gift_10", s.now())
		late.ID = 10
		require.NoError(t, s.DB.Create(&late).Error)
		require.NoError(t, s.CaptureSources(ctx))
		require.NoError(t, s.CaptureSources(ctx))
		var count int64
		require.NoError(t, s.DB.Model(&model.Event{}).Count(&count).Error)
		require.EqualValues(t, 2, count)
		var sources []gift.Event
		require.NoError(t, s.DB.Find(&sources).Error)
		for _, e := range sources {
			require.Equal(t, "pending", e.DeliveryStatus)
			require.Zero(t, e.DeliveredAt)
			require.Contains(t, e.Payload, "secret")
		}
	})
}
