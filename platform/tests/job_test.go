package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"gopkg.in/yaml.v3"
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

func TestWorkerProfileOverridesAndAgentQueue(t *testing.T) {
	base := job.WorkerProfile{Concurrency: 10, Queues: map[string]int{job.QueueCritical: 6, job.QueueDefault: 3, job.QueueFile: 2, job.QueueNotification: 2, job.QueueAI: 1}}
	queues, concurrency := "ai=2", "4"
	resolved, err := job.ResolveWorkerProfile(base, job.WorkerProfileOverrides{Queues: &queues, Concurrency: &concurrency})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Concurrency != 4 || len(resolved.Queues) != 1 || resolved.Queues[job.QueueAI] != 2 {
		t.Fatalf("unexpected worker profile: %#v", resolved)
	}
	if base.Concurrency != 10 || len(base.Queues) != 5 {
		t.Fatalf("process override mutated shared config: %#v", base)
	}
	if def := ai.AgentRunJobDefinition(); def.Queue != job.QueueAI {
		t.Fatalf("agent run queue=%q want ai", def.Queue)
	}
	for _, input := range []job.WorkerProfileOverrides{
		{Queues: stringPtr("")},
		{Queues: stringPtr("tenant-a=1")},
		{Queues: stringPtr("ai=0")},
		{Queues: stringPtr("ai=1,ai=2")},
		{Concurrency: stringPtr("0")},
		{Concurrency: stringPtr("invalid")},
	} {
		if _, err := job.ResolveWorkerProfile(base, input); err == nil {
			t.Fatalf("invalid override accepted: %#v", input)
		}
	}
	defaulted, err := job.ResolveWorkerProfile(base, job.WorkerProfileOverrides{})
	if err != nil || defaulted.Concurrency != 10 || len(defaulted.Queues) != 5 {
		t.Fatalf("default worker compatibility failed: %#v %v", defaulted, err)
	}
}

func TestDemoWorkerProfilesAreDisjoint(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	composePath := filepath.Join(filepath.Dir(current), "..", "..", "deploy", "demo", "compose.yaml")
	body, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	var compose struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err = yaml.Unmarshal(body, &compose); err != nil {
		t.Fatal(err)
	}
	worker := compose.Services["worker"].Environment
	aiWorker := compose.Services["ai-worker"].Environment
	if worker[job.WorkerQueuesEnv] != "critical=6,default=3,file=2,notification=2" || strings.Contains(worker[job.WorkerQueuesEnv], "ai") {
		t.Fatalf("general worker queues overlap ai: %#v", worker)
	}
	if aiWorker[job.WorkerQueuesEnv] != "ai=1" {
		t.Fatalf("ai worker consumes non-ai queues: %#v", aiWorker)
	}
	if worker["LOG_SERVICE"] == aiWorker["LOG_SERVICE"] || worker["LOG_FILE_PATH"] == aiWorker["LOG_FILE_PATH"] || worker["INSTANCE_ID"] == aiWorker["INSTANCE_ID"] {
		t.Fatalf("worker identities are not distinct: worker=%#v ai=%#v", worker, aiWorker)
	}
}

func stringPtr(value string) *string { return &value }

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
		TenantID: "t1", Realm: auth.RealmTenant, PrincipalID: "u1", Authenticated: true, RequestID: "r1",
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

func TestEmbeddingJobErrorClassification(t *testing.T) {
	tests := []struct {
		category  aimodelruntime.ErrorCategory
		retryable bool
		permanent bool
	}{
		{category: aimodelruntime.ErrorInvalidRequest, permanent: true},
		{category: aimodelruntime.ErrorCapabilityNotSupported, permanent: true},
		{category: aimodelruntime.ErrorAuthenticationFailed, permanent: true},
		{category: aimodelruntime.ErrorInvalidVector, permanent: true},
		{category: aimodelruntime.ErrorTimeout, retryable: true},
		{category: aimodelruntime.ErrorRateLimited, retryable: true},
		{category: aimodelruntime.ErrorProviderUnavailable, retryable: true},
	}
	for _, test := range tests {
		runtimeErr := &aimodelruntime.RuntimeError{Category: test.category, Message: "provider secret body", Retryable: test.retryable}
		classified := job.ClassifyEmbeddingError("tool_catalog.embedding", runtimeErr)
		category, stage, summary := job.ErrorFacts(classified)
		if job.IsRetryable(classified) != test.retryable || job.IsPermanent(classified) != test.permanent || category != string(test.category) || stage != "tool_catalog.embedding" || summary == "" || strings.Contains(summary, "secret") {
			t.Fatalf("unexpected classification for %s: retryable=%v permanent=%v facts=%q/%q/%q", test.category, job.IsRetryable(classified), job.IsPermanent(classified), category, stage, summary)
		}
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
