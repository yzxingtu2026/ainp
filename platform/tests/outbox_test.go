package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

type reconnectingOutboxListener struct {
	calls   atomic.Int32
	trigger chan struct{}
	stopped chan struct{}
}

func (l *reconnectingOutboxListener) Listen(ctx context.Context, wake func()) error {
	if l.calls.Add(1) == 1 {
		return errors.New("listener disconnected")
	}
	select {
	case <-ctx.Done():
		select {
		case <-l.stopped:
		default:
			close(l.stopped)
		}
		return ctx.Err()
	case <-l.trigger:
		wake()
		<-ctx.Done()
		select {
		case <-l.stopped:
		default:
			close(l.stopped)
		}
		return ctx.Err()
	}
}

func TestOutboxStatusAndCodeSet(t *testing.T) {
	if !status.OutboxPending.Valid() || status.OutboxStatus("x").Valid() {
		t.Fatal("outbox status validity")
	}
	if !status.DeliverySucceeded.Valid() || status.DeliveryStatus("x").Valid() {
		t.Fatal("delivery status validity")
	}
	if err := i18n.LoadDir("../i18n"); err != nil {
		t.Fatal(err)
	}
	for _, set := range []codeset.CodeSet{codeset.OutboxStatus(), codeset.DeliveryStatus()} {
		for _, lang := range i18n.AvailableLangs() {
			if got := i18n.T(lang, set.NameKey); got == set.NameKey {
				t.Fatalf("missing %s in %s", set.NameKey, lang)
			}
			for _, item := range set.Items {
				if got := i18n.T(lang, item.LabelKey); got == item.LabelKey {
					t.Fatalf("missing %s in %s", item.LabelKey, lang)
				}
			}
		}
	}
}

func TestOutboxRegistryRejectsDuplicates(t *testing.T) {
	defs := outbox.NewDefinitionRegistry()
	if err := defs.Register(outbox.Definition{EventType: "demo.x.v1", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err := defs.Register(outbox.Definition{EventType: "demo.x.v1", SchemaVersion: 2}); err == nil {
		t.Fatal("duplicate event type must fail")
	}
	handlers := outbox.NewHandlerRegistry()
	h := outbox.HandlerFunc{HandlerName: "h1", Types: []string{"demo.x.v1"}, Fn: func(context.Context, outbox.Envelope) error { return nil }}
	if err := handlers.Register(h); err != nil {
		t.Fatal(err)
	}
	if err := handlers.Register(h); err == nil {
		t.Fatal("duplicate handler name must fail")
	}
}

func TestOutboxWriterFailClosedWithoutTenant(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	_ = defs.Register(outbox.Definition{EventType: "demo.x.v1", SchemaVersion: 1})
	w := outbox.NewWriter(defs)
	ctx := context.Background()
	err := outbox.WithinTx(ctx, db, w, func(tx *gorm.DB, writer outbox.Writer) error {
		_, err := writer.Append(ctx, tx, outbox.AppendInput{
			EventType: "demo.x.v1", AggregateType: "demo", AggregateID: "1", Payload: map[string]string{"a": "b"},
		})
		return err
	})
	if err == nil {
		t.Fatal("expected fail-closed without tenant")
	}
}

func TestOutboxSameTransactionRollback(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	_ = defs.Register(outbox.Definition{EventType: "demo.x.v1", SchemaVersion: 1})
	w := outbox.NewWriter(defs)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true, RequestID: "r1",
	})
	type row struct {
		ID string `gorm:"primaryKey"`
	}
	_ = db.AutoMigrate(&row{})
	err := outbox.WithinTx(ctx, db, w, func(tx *gorm.DB, writer outbox.Writer) error {
		if err := tx.Create(&row{ID: "biz-1"}).Error; err != nil {
			return err
		}
		if _, err := writer.Append(ctx, tx, outbox.AppendInput{
			EventType: "demo.x.v1", AggregateType: "demo", AggregateID: "biz-1", Payload: map[string]any{"ok": true},
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var biz int64
	_ = db.Model(&row{}).Count(&biz)
	var events int64
	_ = db.Model(&outbox.Event{}).Count(&events)
	if biz != 0 || events != 0 {
		t.Fatalf("rollback incomplete: biz=%d events=%d", biz, events)
	}
}

func TestOutboxDispatchIdempotentHandlersAndFakeTransport(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	_ = defs.Register(outbox.Definition{EventType: "demo.x.v1", SchemaVersion: 1})
	handlers := outbox.NewHandlerRegistry()
	var aCalls, bCalls atomic.Int64
	_ = handlers.Register(outbox.HandlerFunc{
		HandlerName: "handler-a", Types: []string{"demo.x.v1"},
		Fn: func(context.Context, outbox.Envelope) error { aCalls.Add(1); return nil },
	})
	_ = handlers.Register(outbox.HandlerFunc{
		HandlerName: "handler-b", Types: []string{"demo.x.v1"},
		Fn: func(context.Context, outbox.Envelope) error {
			if bCalls.Add(1) == 1 {
				return errors.New("transient")
			}
			return nil
		},
	})
	deliveries := outbox.NewDeliveryStore(db)
	transport := outbox.NewInProcessTransport(handlers, deliveries)
	w := outbox.NewWriter(defs)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true, RequestID: "r1",
	})
	var env outbox.Envelope
	if err := outbox.WithinTx(ctx, db, w, func(tx *gorm.DB, writer outbox.Writer) error {
		var err error
		env, err = writer.Append(ctx, tx, outbox.AppendInput{
			EventType: "demo.x.v1", AggregateType: "demo", AggregateID: "x1", Payload: map[string]any{"n": 1},
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	disp := outbox.NewDispatcher(db, transport, outbox.DispatcherConfig{
		BatchSize: 10, MaxAttempts: 5, PollInterval: time.Hour, LockTimeout: time.Minute,
		Backoff: outbox.BackoffConfig{Base: time.Millisecond, Max: time.Millisecond},
	})
	if err := disp.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var ev outbox.Event
	if err := db.First(&ev, "id = ?", env.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ev.Status != status.OutboxFailed {
		t.Fatalf("status=%s want failed", ev.Status)
	}
	if aCalls.Load() != 1 || bCalls.Load() != 1 {
		t.Fatalf("calls a=%d b=%d", aCalls.Load(), bCalls.Load())
	}

	// Retry: A already succeeded — must not re-run; B retries and succeeds.
	if err := outbox.RetryEvent(ctx, db, env.ID); err != nil {
		t.Fatal(err)
	}
	if err := disp.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&ev, "id = ?", env.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ev.Status != status.OutboxPublished {
		t.Fatalf("status=%s want published", ev.Status)
	}
	if aCalls.Load() != 1 {
		t.Fatalf("handler-a re-executed: %d", aCalls.Load())
	}
	if bCalls.Load() != 2 {
		t.Fatalf("handler-b calls=%d", bCalls.Load())
	}

	// Fake transport swap: Writer/handlers unchanged.
	fake := &outbox.RecordingTransport{}
	disp2 := outbox.NewDispatcher(db, fake, outbox.DispatcherConfig{BatchSize: 10, MaxAttempts: 3})
	ctx2 := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true,
	})
	var env2 outbox.Envelope
	_ = outbox.WithinTx(ctx2, db, w, func(tx *gorm.DB, writer outbox.Writer) error {
		var err error
		env2, err = writer.Append(ctx2, tx, outbox.AppendInput{
			EventType: "demo.x.v1", AggregateType: "demo", AggregateID: "x2", Payload: map[string]any{"n": 2},
		})
		return err
	})
	if err := disp2.Tick(ctx2); err != nil {
		t.Fatal(err)
	}
	if len(fake.Published) != 1 || fake.Published[0].ID != env2.ID {
		t.Fatalf("fake transport = %#v", fake.Published)
	}
}

func TestOutboxConcurrentClaim(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	_ = defs.Register(outbox.Definition{EventType: "demo.x.v1", SchemaVersion: 1})
	w := outbox.NewWriter(defs)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true,
	})
	const n = 20
	for i := 0; i < n; i++ {
		_ = outbox.WithinTx(ctx, db, w, func(tx *gorm.DB, writer outbox.Writer) error {
			_, err := writer.Append(ctx, tx, outbox.AppendInput{
				EventType: "demo.x.v1", AggregateType: "demo", AggregateID: fmt.Sprintf("c%d", i),
				Payload: map[string]int{"i": i},
			})
			return err
		})
	}
	fakeA := &outbox.RecordingTransport{}
	fakeB := &outbox.RecordingTransport{}
	dispA := outbox.NewDispatcher(db, fakeA, outbox.DispatcherConfig{BatchSize: 5, WorkerID: "A", MaxAttempts: 3})
	dispB := outbox.NewDispatcher(db, fakeB, outbox.DispatcherConfig{BatchSize: 5, WorkerID: "B", MaxAttempts: 3})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = dispA.Tick(ctx) }()
	go func() { defer wg.Done(); _ = dispB.Tick(ctx) }()
	wg.Wait()
	for i := 0; i < 10; i++ {
		_ = dispA.Tick(ctx)
		_ = dispB.Tick(ctx)
	}
	var published int64
	_ = db.Model(&outbox.Event{}).Where("status = ?", status.OutboxPublished).Count(&published)
	if published != n {
		t.Fatalf("published=%d want %d (a=%d b=%d)", published, n, len(fakeA.Published), len(fakeB.Published))
	}
	seen := map[string]struct{}{}
	for _, env := range append(append([]outbox.Envelope{}, fakeA.Published...), fakeB.Published...) {
		if _, ok := seen[env.ID]; ok {
			t.Fatalf("event %s published twice", env.ID)
		}
		seen[env.ID] = struct{}{}
	}
}

func TestOutboxLockRecoveryAndDead(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	_ = defs.Register(outbox.Definition{EventType: "demo.x.v1", SchemaVersion: 1})
	w := outbox.NewWriter(defs)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true,
	})
	var env outbox.Envelope
	_ = outbox.WithinTx(ctx, db, w, func(tx *gorm.DB, writer outbox.Writer) error {
		var err error
		env, err = writer.Append(ctx, tx, outbox.AppendInput{
			EventType: "demo.x.v1", AggregateType: "demo", AggregateID: "dead-1", Payload: map[string]any{},
		})
		return err
	})
	stale := time.Now().UTC().Add(-2 * time.Hour)
	_ = db.Model(&outbox.Event{}).Where("id = ?", env.ID).Updates(map[string]any{
		"status": status.OutboxProcessing, "locked_by": "dead-worker", "locked_at": stale,
	})
	failing := &outbox.RecordingTransport{Err: errors.New("down")}
	disp := outbox.NewDispatcher(db, failing, outbox.DispatcherConfig{
		BatchSize: 5, MaxAttempts: 2, LockTimeout: time.Minute,
		Backoff: outbox.BackoffConfig{Base: time.Millisecond, Max: time.Millisecond},
	})
	_ = disp.Tick(ctx) // recover + fail → failed
	_ = outbox.RetryEvent(ctx, db, env.ID)
	_ = disp.Tick(ctx) // fail again → dead (attempts >= 2)
	var ev outbox.Event
	_ = db.First(&ev, "id = ?", env.ID)
	if ev.Status != status.OutboxDead {
		t.Fatalf("status=%s attempts=%d", ev.Status, ev.Attempts)
	}
}

func TestOutboxRuntimeConfig(t *testing.T) {
	db := openOutboxTestDB(t)
	enabled := false
	rt, err := outbox.NewRuntime(db, config.OutboxConfig{Enabled: &enabled, Transport: "postgres"})
	if err != nil || rt.Enabled {
		t.Fatalf("disabled runtime: %#v %v", rt, err)
	}
	rt2, err := outbox.NewRuntime(db, config.OutboxConfig{Transport: "postgres"})
	if err != nil || !rt2.Enabled {
		t.Fatalf("default enabled: %#v %v", rt2, err)
	}
	if _, err := outbox.NewRuntime(db, config.OutboxConfig{Transport: "kafka"}); err == nil {
		t.Fatal("unsupported transport must fail")
	}
}

func TestOutboxCommitNotificationWakesDispatcher(t *testing.T) {
	db := openOutboxTestDB(t)
	rt, err := outbox.NewRuntime(db, config.OutboxConfig{PollInterval: "1h"})
	if err != nil {
		t.Fatal(err)
	}
	if err = rt.RegisterDefinition(outbox.Definition{EventType: "demo.wakeup.v1", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	published := make(chan string, 2)
	if err = rt.RegisterHandler(outbox.HandlerFunc{HandlerName: "wakeup-handler", Types: []string{"demo.wakeup.v1"}, Fn: func(_ context.Context, env outbox.Envelope) error {
		published <- env.ID
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt.Start(ctx)
	t.Cleanup(func() { cancel(); rt.Stop() })
	eventCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true})
	var committed outbox.Envelope
	if err = outbox.WithinTx(eventCtx, db, rt.Writer, func(tx *gorm.DB, writer outbox.Writer) error {
		committed, err = writer.Append(eventCtx, tx, outbox.AppendInput{EventType: "demo.wakeup.v1", AggregateType: "demo", AggregateID: "committed", Payload: map[string]any{"ok": true}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-published:
		if id != committed.ID {
			t.Fatalf("published id=%s want %s", id, committed.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("committed event waited for periodic poll instead of notification")
	}
	rollbackErr := outbox.WithinTx(eventCtx, db, rt.Writer, func(tx *gorm.DB, writer outbox.Writer) error {
		if _, appendErr := writer.Append(eventCtx, tx, outbox.AppendInput{EventType: "demo.wakeup.v1", AggregateType: "demo", AggregateID: "rolled-back", Payload: map[string]any{}}); appendErr != nil {
			return appendErr
		}
		return errors.New("rollback")
	})
	if rollbackErr == nil {
		t.Fatal("expected rollback")
	}
	select {
	case id := <-published:
		t.Fatalf("rolled-back event was published: %s", id)
	case <-time.After(150 * time.Millisecond):
	}
	var count int64
	if err = db.Model(&outbox.Event{}).Where("aggregate_id=?", "rolled-back").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rolled-back event persisted: count=%d err=%v", count, err)
	}
}

func TestOutboxListenerReconnectsAndStopsGracefully(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	if err := defs.Register(outbox.Definition{EventType: "demo.reconnect.v1", SchemaVersion: 1}); err != nil {
		t.Fatal(err)
	}
	writer := outbox.NewWriter(defs)
	listener := &reconnectingOutboxListener{trigger: make(chan struct{}, 1), stopped: make(chan struct{})}
	transport := &outbox.RecordingTransport{}
	dispatcher := outbox.NewDispatcher(db, transport, outbox.DispatcherConfig{PollInterval: time.Hour, BatchSize: 2})
	dispatcher.SetNotificationListener(listener)
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher.Start(ctx)
	eventCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true})
	var env outbox.Envelope
	if err := outbox.WithinTx(eventCtx, db, writer, func(tx *gorm.DB, w outbox.Writer) error {
		var appendErr error
		env, appendErr = w.Append(eventCtx, tx, outbox.AppendInput{EventType: "demo.reconnect.v1", AggregateType: "demo", AggregateID: "reconnect", Payload: map[string]any{}})
		return appendErr
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for listener.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if listener.calls.Load() < 2 {
		t.Fatal("listener did not reconnect")
	}
	listener.trigger <- struct{}{}
	if !waitOutboxStatus(db, env.ID, status.OutboxPublished, 2*time.Second) {
		t.Fatal("reconnected listener did not wake dispatcher")
	}
	cancel()
	dispatcher.Stop()
	select {
	case <-listener.stopped:
	case <-time.After(time.Second):
		t.Fatal("listener did not stop with dispatcher")
	}
}

func TestOutboxPollingRemainsReliableFallback(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	_ = defs.Register(outbox.Definition{EventType: "demo.poll.v1", SchemaVersion: 1})
	writer := outbox.NewWriter(defs)
	transport := &outbox.RecordingTransport{}
	dispatcher := outbox.NewDispatcher(db, transport, outbox.DispatcherConfig{PollInterval: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher.Start(ctx)
	t.Cleanup(func() { cancel(); dispatcher.Stop() })
	eventCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true})
	var env outbox.Envelope
	if err := outbox.WithinTx(eventCtx, db, writer, func(tx *gorm.DB, w outbox.Writer) error {
		var appendErr error
		env, appendErr = w.Append(eventCtx, tx, outbox.AppendInput{EventType: "demo.poll.v1", AggregateType: "demo", AggregateID: "poll", Payload: map[string]any{}})
		return appendErr
	}); err != nil {
		t.Fatal(err)
	}
	if !waitOutboxStatus(db, env.ID, status.OutboxPublished, time.Second) {
		t.Fatal("poll fallback did not publish event")
	}
}

func TestOutboxCoalescedWakeDrainsMultipleBatches(t *testing.T) {
	db := openOutboxTestDB(t)
	defs := outbox.NewDefinitionRegistry()
	_ = defs.Register(outbox.Definition{EventType: "demo.coalesced.v1", SchemaVersion: 1})
	writer := outbox.NewWriter(defs)
	transport := &outbox.RecordingTransport{}
	dispatcher := outbox.NewDispatcher(db, transport, outbox.DispatcherConfig{PollInterval: time.Hour, BatchSize: 2})
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher.Start(ctx)
	t.Cleanup(func() { cancel(); dispatcher.Stop() })
	// Let the initial startup scan finish before inserting without notifications.
	time.Sleep(50 * time.Millisecond)
	eventCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true})
	for i := 0; i < 5; i++ {
		if err := outbox.WithinTx(eventCtx, db, writer, func(tx *gorm.DB, w outbox.Writer) error {
			_, appendErr := w.Append(eventCtx, tx, outbox.AppendInput{EventType: "demo.coalesced.v1", AggregateType: "demo", AggregateID: fmt.Sprintf("coalesced-%d", i), Payload: map[string]any{}})
			return appendErr
		}); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher.Wake()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var published int64
		if err := db.Model(&outbox.Event{}).Where("status=?", status.OutboxPublished).Count(&published).Error; err == nil && published == 5 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("one coalesced wake did not drain all batches")
}

func waitOutboxStatus(db *gorm.DB, eventID string, expected status.OutboxStatus, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var event outbox.Event
		if err := db.First(&event, "id=?", eventID).Error; err == nil && event.Status == expected {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func openOutboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_outbox_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	return testDB
}
