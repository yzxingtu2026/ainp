package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

type fakeJobTransport struct {
	mu        sync.Mutex
	enqueued  []job.Envelope
	failTimes int
	calls     int
}

func (t *fakeJobTransport) Enqueue(_ context.Context, env job.Envelope, _ string, _ job.EnqueueTransportOpts) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	if t.failTimes > 0 {
		t.failTimes--
		return errors.New("transport down")
	}
	t.enqueued = append(t.enqueued, env)
	return nil
}
func (t *fakeJobTransport) Ping(context.Context) error { return nil }
func (t *fakeJobTransport) Close() error               { return nil }

func TestJobRunStatusCodeSet(t *testing.T) {
	if !status.JobQueued.Valid() || status.JobRunStatus("x").Valid() {
		t.Fatal("job status validity")
	}
	if err := i18n.LoadDir("../i18n"); err != nil {
		t.Fatal(err)
	}
	set := codeset.JobRunStatus()
	for _, lang := range i18n.AvailableLangs() {
		if got := i18n.T(lang, set.NameKey); got == set.NameKey {
			t.Fatalf("missing %s", set.NameKey)
		}
		for _, item := range set.Items {
			if got := i18n.T(lang, item.LabelKey); got == item.LabelKey {
				t.Fatalf("missing %s", item.LabelKey)
			}
		}
	}
}

func TestJobRegistryAndDisabledEnqueue(t *testing.T) {
	db := openJobTestDB(t)
	reg := job.NewRegistry()
	_ = reg.Register(job.Definition{
		Type: "demo.ping", SchemaVersion: 1, Queue: job.QueueDefault,
		MaxAttempts: 2, Timeout: time.Second,
	}, job.HandlerFunc{JobType: "demo.ping", Fn: func(context.Context, job.Envelope, json.RawMessage, job.Reporter) error { return nil }})
	if err := reg.Register(job.Definition{
		Type: "demo.ping", SchemaVersion: 1, Queue: job.QueueDefault,
		MaxAttempts: 2, Timeout: time.Second,
	}, job.HandlerFunc{JobType: "demo.ping", Fn: func(context.Context, job.Envelope, json.RawMessage, job.Reporter) error { return nil }}); err == nil {
		t.Fatal("duplicate type must fail")
	}
	eq := job.NewEnqueuer(db, reg, job.NewRunStore(db), nil, false, 1024)
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "t1", Realm: auth.RealmTenant, Authenticated: true})
	if _, err := eq.Enqueue(ctx, job.EnqueueInput{Type: "demo.ping", Payload: map[string]string{"m": "x"}}); !errors.Is(err, job.ErrUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
}

func TestJobEnqueueIdempotentAndPendingRecovery(t *testing.T) {
	db := openJobTestDB(t)
	reg := job.NewRegistry()
	_ = reg.Register(job.Definition{
		Type: "demo.ping", SchemaVersion: 1, Queue: job.QueueDefault,
		MaxAttempts: 3, Timeout: time.Second,
	}, job.HandlerFunc{JobType: "demo.ping", Fn: func(context.Context, job.Envelope, json.RawMessage, job.Reporter) error { return nil }})
	store := job.NewRunStore(db)
	tr := &fakeJobTransport{failTimes: 1}
	rt, err := job.NewRuntime(db, job.RuntimeOptions{Enabled: true, MaxPayload: 1024, Transport: tr})
	if err != nil {
		t.Fatal(err)
	}
	_ = rt.Register(job.Definition{
		Type: "demo.ping", SchemaVersion: 1, Queue: job.QueueDefault,
		MaxAttempts: 3, Timeout: time.Second,
	}, job.HandlerFunc{JobType: "demo.ping", Fn: func(context.Context, job.Envelope, json.RawMessage, job.Reporter) error { return nil }})
	// rebuild enqueuer with same transport after register — use NewEnqueuer directly
	eq := job.NewEnqueuer(db, rt.Registry, store, tr, true, 1024)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "t1", Realm: auth.RealmTenant, UserID: "u1", Authenticated: true, RequestID: "r1",
	})
	run1, err := eq.Enqueue(ctx, job.EnqueueInput{
		Type: "demo.ping", IdempotencyKey: "evt-1:demo.ping", Payload: map[string]any{"message": "hi", "steps": 1},
	})
	if err == nil {
		t.Fatal("expected transport failure")
	}
	if run1 == nil || run1.Status != status.JobPendingEnqueue {
		t.Fatalf("run=%#v", run1)
	}
	run2, err := eq.Enqueue(ctx, job.EnqueueInput{
		Type: "demo.ping", IdempotencyKey: "evt-1:demo.ping", Payload: map[string]any{"message": "hi", "steps": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run2.ID != run1.ID {
		t.Fatalf("idempotency broken: %s vs %s", run1.ID, run2.ID)
	}
	if run2.Status != status.JobQueued {
		t.Fatalf("status=%s", run2.Status)
	}
	tr.mu.Lock()
	n := len(tr.enqueued)
	tr.mu.Unlock()
	if n != 1 {
		t.Fatalf("enqueued=%d", n)
	}
}

func TestJobErrorClassification(t *testing.T) {
	err := job.Retryable(errors.New("tmp"))
	if !job.IsRetryable(err) || job.IsPermanent(err) {
		t.Fatal("retryable")
	}
	err = job.Permanent(errors.New("bad"))
	if !job.IsPermanent(err) {
		t.Fatal("permanent")
	}
	err = job.Cancelled(nil)
	if !job.IsCancelled(err) {
		t.Fatal("cancelled")
	}
}

func TestJobFailClosedWithoutTenant(t *testing.T) {
	db := openJobTestDB(t)
	reg := job.NewRegistry()
	_ = reg.Register(job.Definition{
		Type: "demo.ping", SchemaVersion: 1, Queue: job.QueueDefault,
		MaxAttempts: 1, Timeout: time.Second,
	}, job.HandlerFunc{JobType: "demo.ping", Fn: func(context.Context, job.Envelope, json.RawMessage, job.Reporter) error { return nil }})
	tr := &fakeJobTransport{}
	eq := job.NewEnqueuer(db, reg, job.NewRunStore(db), tr, true, 1024)
	_, err := eq.Enqueue(context.Background(), job.EnqueueInput{Type: "demo.ping", Payload: map[string]string{}})
	if err == nil {
		t.Fatal("expected fail-closed")
	}
}

func TestJobRunTenantOperations(t *testing.T) {
	db := openJobTestDB(t)
	now := time.Now().UTC()
	for _, run := range []job.Run{
		{ID: "t1-run", TenantID: "t1", Realm: string(auth.RealmTenant), JobType: "demo.ping", SchemaVersion: 1, Queue: job.QueueDefault, PayloadJSON: []byte("{}"), Status: status.JobQueued, MaxAttempts: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "t2-run", TenantID: "t2", Realm: string(auth.RealmTenant), JobType: "demo.ping", SchemaVersion: 1, Queue: job.QueueDefault, PayloadJSON: []byte("{}"), Status: status.JobQueued, MaxAttempts: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := job.NewRunStore(db)
	ctxA := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "t1"})
	if _, err := store.Get(ctxA, "t2-run"); !errors.Is(err, job.ErrNotFound) {
		t.Fatalf("cross-tenant get must be opaque: %v", err)
	}
	rows, total, err := store.List(ctxA, job.ListQuery{Page: 1, PageSize: 20})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != "t1-run" {
		t.Fatalf("tenant list rows=%#v total=%d err=%v", rows, total, err)
	}
	if err := store.MarkCancelRequested(ctxA, "t2-run"); !errors.Is(err, job.ErrConflict) {
		t.Fatalf("cross-tenant cancel must fail closed: %v", err)
	}
	if err := store.MarkCancelRequested(ctxA, "t1-run"); err != nil {
		t.Fatal(err)
	}
	run, err := store.Get(ctxA, "t1-run")
	if err != nil || run.CancelRequestedAt == nil {
		t.Fatalf("cancel request was not persisted: run=%#v err=%v", run, err)
	}
}

func openJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_job_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	return testDB
}
