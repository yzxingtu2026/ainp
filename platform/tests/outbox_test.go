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
		TenantID: "t1", Realm: auth.RealmTenant, UserID: "u1", Authenticated: true, RequestID: "r1",
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
		TenantID: "t1", Realm: auth.RealmTenant, UserID: "u1", Authenticated: true, RequestID: "r1",
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
		TenantID: "t1", Realm: auth.RealmTenant, UserID: "u1", Authenticated: true,
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
		TenantID: "t1", Realm: auth.RealmTenant, UserID: "u1", Authenticated: true,
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
		TenantID: "t1", Realm: auth.RealmTenant, UserID: "u1", Authenticated: true,
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
