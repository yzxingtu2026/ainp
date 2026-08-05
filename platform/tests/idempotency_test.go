package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/idempotency"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

func openIdempotencyTestDB(t *testing.T) *idempotency.Repository {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_idem_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := idempotency.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	return idempotency.NewRepository(testDB)
}

func TestIdempotencyHashStability(t *testing.T) {
	input1 := json.RawMessage(`{"b":2,"a":1}`)
	input2 := json.RawMessage(`{"a":1,"b":2}`)
	h1 := idempotency.HashRequest(input1)
	h2 := idempotency.HashRequest(input2)
	if h1 != h2 {
		t.Fatalf("same JSON with different key order should produce same hash: %s != %s", h1, h2)
	}
	if idempotency.HashSummary(h1) == "" {
		t.Fatal("hash summary should not be empty")
	}
}

func TestIdempotencyHashEmptyInput(t *testing.T) {
	h1 := idempotency.HashRequest(nil)
	h2 := idempotency.HashRequest(json.RawMessage(`{}`))
	if h1 != h2 {
		t.Fatalf("nil and empty JSON should produce same hash: %s != %s", h1, h2)
	}
}

func TestIdempotencyCreateAndGet(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	ctx := idempotencyTenantContext("t1", "u1")

	rec, err := repo.Create(ctx, "t1", "u1", "demo.action", "key-1", "hash123", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != idempotency.StatusProcessing {
		t.Fatalf("expected processing, got %s", rec.Status)
	}

	found, err := repo.GetByKey(ctx, "t1", "u1", "demo.action", "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil {
		t.Fatal("expected to find record")
	}
	if found.ID != rec.ID {
		t.Fatalf("expected same record ID")
	}
}

func TestIdempotencyTenantIsolation(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	ctx := idempotencyTenantContext("t1", "u1")

	_, err := repo.Create(ctx, "t1", "u1", "demo.action", "key-1", "hash1", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Same key but different tenant should succeed
	ctx2 := idempotencyTenantContext("t2", "u1")
	_, err = repo.Create(ctx2, "t2", "u1", "demo.action", "key-1", "hash2", 1*time.Hour)
	if err != nil {
		t.Fatal("different tenant should not conflict:", err)
	}

	// Same tenant+user+action+key should fail
	_, err = repo.Create(ctx, "t1", "u1", "demo.action", "key-1", "hash1", 1*time.Hour)
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
	if _, err := repo.GetByKey(ctx, "t2", "u1", "demo.action", "key-1"); !errors.Is(err, tenant.ErrTenantConflict) {
		t.Fatalf("mismatched tenant must fail closed: %v", err)
	}
	if _, err := repo.Create(context.Background(), "t1", "u1", "demo.action", "key-2", "hash1", time.Hour); err == nil {
		t.Fatal("missing auth context must fail closed")
	}
}

func TestIdempotencyCompleteAndReplay(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	ctx := idempotencyTenantContext("t1", "u1")

	rec, err := repo.Create(ctx, "t1", "u1", "demo.action", "key-1", "hash1", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	respJSON := []byte(`{"result":"ok"}`)
	if err := repo.Complete(ctx, rec.ID, idempotency.StatusCompleted, respJSON, nil); err != nil {
		t.Fatal(err)
	}

	found, err := repo.GetByKey(ctx, "t1", "u1", "demo.action", "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if found.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed, got %s", found.Status)
	}
	var gotResponse, wantResponse any
	if json.Unmarshal(found.ResponseJSON, &gotResponse) != nil || json.Unmarshal(respJSON, &wantResponse) != nil ||
		!reflect.DeepEqual(gotResponse, wantResponse) {
		t.Fatalf("response JSON mismatch: got=%s want=%s", found.ResponseJSON, respJSON)
	}
}

func TestIdempotencyClaimProcessing(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	ctx := idempotencyTenantContext("t1", "u1")

	rec, err := repo.Create(ctx, "t1", "u1", "demo.action", "key-1", "hash1", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Not yet timed out — claim should fail
	claimed, err := repo.ClaimProcessing(ctx, rec.ID, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("should not claim a fresh processing record")
	}
}

func TestIdempotencyCleanupExpired(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	tenantCtx := idempotencyTenantContext("t1", "u1")

	// Create with very short TTL
	_, err := repo.Create(tenantCtx, "t1", "u1", "demo.action", "key-exp", "hash1", 1*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	platformCtx := auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true,
	})
	deleted, err := repo.CleanupExpired(platformCtx)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	if _, err := repo.CleanupExpired(tenantCtx); !errors.Is(err, tenant.ErrPlatformRealmRequired) {
		t.Fatalf("tenant cleanup must fail closed: %v", err)
	}
}

func idempotencyTenantContext(tenantID, userID string) context.Context {
	return auth.WithContext(context.Background(), &auth.Context{
		TenantID: tenantID, PrincipalID: userID, Realm: auth.RealmTenant, Authenticated: true,
	})
}

func TestIdempotencyExecutorReplay(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	exec := idempotency.NewExecutor(repo)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID:       "t1",
		PrincipalID:    "u1",
		Realm:          auth.RealmTenant,
		Authenticated:  true,
		IdempotencyKey: "exec-key-1",
	})

	var callCount atomic.Int64
	handler := func(_ context.Context, _ json.RawMessage) (any, error) {
		callCount.Add(1)
		return map[string]any{"msg": "hello"}, nil
	}

	input := idempotency.ExecuteInput{
		ActionID: "demo.test",
		Input:    json.RawMessage(`{"name":"test"}`),
		Meta:     &idempotency.IdempotencyConfig{Mode: "required", TTL: 1 * time.Hour, InProgressTimeout: 30 * time.Second},
		Handler:  handler,
	}

	// First call: should execute handler
	result1, err := exec.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result1.IsReplay {
		t.Fatal("first call should not be replay")
	}

	// Second call with same key and input: should replay
	result2, err := exec.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !result2.IsReplay {
		t.Fatal("second call should be replay")
	}

	if callCount.Load() != 1 {
		t.Fatalf("handler should be called exactly once, got %d", callCount.Load())
	}
}

func TestIdempotencyExecutorConflict(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	exec := idempotency.NewExecutor(repo)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID:       "t1",
		PrincipalID:    "u1",
		Realm:          auth.RealmTenant,
		Authenticated:  true,
		IdempotencyKey: "exec-key-2",
	})

	handler := func(_ context.Context, _ json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	}

	// First call
	input1 := idempotency.ExecuteInput{
		ActionID: "demo.test",
		Input:    json.RawMessage(`{"v":1}`),
		Meta:     &idempotency.IdempotencyConfig{Mode: "required", TTL: 1 * time.Hour, InProgressTimeout: 30 * time.Second},
		Handler:  handler,
	}
	if _, err := exec.Execute(ctx, input1); err != nil {
		t.Fatal(err)
	}

	// Second call with same key but different input: should conflict
	input2 := idempotency.ExecuteInput{
		ActionID: "demo.test",
		Input:    json.RawMessage(`{"v":2}`),
		Meta:     &idempotency.IdempotencyConfig{Mode: "required", TTL: 1 * time.Hour, InProgressTimeout: 30 * time.Second},
		Handler:  handler,
	}
	_, err := exec.Execute(ctx, input2)
	if err == nil {
		t.Fatal("expected conflict error for same key with different input")
	}
}

func TestIdempotencyExecutorConcurrentProcessing(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	exec := idempotency.NewExecutor(repo)

	handler := func(_ context.Context, _ json.RawMessage) (any, error) {
		time.Sleep(200 * time.Millisecond)
		return map[string]any{"done": true}, nil
	}

	input := idempotency.ExecuteInput{
		ActionID: "demo.slow",
		Input:    json.RawMessage(`{"x":1}`),
		Meta:     &idempotency.IdempotencyConfig{Mode: "required", TTL: 1 * time.Hour, InProgressTimeout: 30 * time.Second},
		Handler:  handler,
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var inProgressCount atomic.Int64

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := auth.WithContext(context.Background(), &auth.Context{
				TenantID:       "t1",
				PrincipalID:    "u1",
				Realm:          auth.RealmTenant,
				Authenticated:  true,
				IdempotencyKey: "concurrent-key",
			})
			result, err := exec.Execute(ctx, input)
			if err != nil {
				// Check if it's an in-progress error
				if containsStr(err.Error(), "in progress") {
					inProgressCount.Add(1)
				}
				return
			}
			if result != nil && !result.IsReplay {
				successCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// At least one should succeed, others may get in-progress
	if successCount.Load() < 1 {
		t.Fatalf("at least one concurrent call should succeed, got %d successes", successCount.Load())
	}
}

func TestIdempotencyExecutorRequiredNoKey(t *testing.T) {
	repo := openIdempotencyTestDB(t)
	exec := idempotency.NewExecutor(repo)
	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID:      "t1",
		PrincipalID:   "u1",
		Realm:         auth.RealmTenant,
		Authenticated: true,
		// No IdempotencyKey set
	})

	handler := func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, nil
	}

	input := idempotency.ExecuteInput{
		ActionID: "demo.test",
		Input:    json.RawMessage(`{}`),
		Meta:     &idempotency.IdempotencyConfig{Mode: "required", TTL: 1 * time.Hour, InProgressTimeout: 30 * time.Second},
		Handler:  handler,
	}

	_, err := exec.Execute(ctx, input)
	if err == nil {
		t.Fatal("expected error when mode=required and no key provided")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}
