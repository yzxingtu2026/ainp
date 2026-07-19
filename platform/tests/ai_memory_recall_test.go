package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

type recallEmbedding struct{ fail bool }

func (e recallEmbedding) Embed(_ context.Context, r aimodelruntime.EmbeddingRequest) (*aimodelruntime.EmbeddingResponse, error) {
	if e.fail {
		return nil, fmt.Errorf("embedding unavailable")
	}
	vectors := make([]aimodelruntime.EmbeddingVector, len(r.Inputs))
	for i, input := range r.Inputs {
		vectors[i] = aimodelruntime.EmbeddingVector{ID: input.ID, Index: i, Vector: []float32{1, 0}}
	}
	return &aimodelruntime.EmbeddingResponse{Model: aimodelruntime.ModelSummary{DeploymentID: "dep-memory"}, Vectors: vectors, Dimensions: 2}, nil
}

func TestMemoryHybridRecallHardFiltersAndDegrades(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_memory_recall_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../migrations/0035_memory_hybrid_recall.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(migration)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&tenant.Tenant{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range []model.User{{ID: "user-a", TenantID: "tenant-a", Username: "alice", PasswordHash: "x", Status: status.ActivationEnabled}, {ID: "user-b", TenantID: "tenant-a", Username: "bob", PasswordHash: "x", Status: status.ActivationEnabled}} {
		if err = db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	day := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	insert := func(id, user, state, content string, local *time.Time) {
		if err := db.Exec(`INSERT INTO ai_memory_items(id,tenant_id,user_id,type,status,summary,content,confidence,sensitivity,importance,revision,occurred_timezone,local_date,date_precision,observed_at,recorded_at,policy_json,created_at,updated_at) VALUES (?,?,?,'event',?,?,?,0.9,'normal',80,1,'Asia/Shanghai',?,'exact_day',?,?, '{}'::jsonb,?,?)`, id, "tenant-a", user, state, content, content, local, now, now, now, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	insert("active", "user-a", "active", "上海会议", &day)
	insert("revoked", "user-a", "revoked", "上海会议已撤销", &day)
	insert("other", "user-b", "active", "上海会议他人", &day)
	insert("future", "user-a", "active", "上海会议未来", nil)
	if err := db.Exec(`UPDATE ai_memory_items SET valid_from=? WHERE id='future'`, future).Error; err != nil {
		t.Fatal(err)
	}
	vector, _ := json.Marshal([]float32{1, 0})
	if err := db.Exec(`INSERT INTO ai_memory_projections(id,tenant_id,user_id,item_id,kind,status,item_revision,content_hash,model_alias,deployment_id,dimensions,vector_json,created_at,updated_at) VALUES ('p1','tenant-a','user-a','active','vector','ready',1,'hash','default','dep-memory',2,?,?,?)`, vector, now, now).Error; err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true})
	snap, _ := runtimecontext.NewTimeSnapshot(now, "Asia/Shanghai", "user_preference")
	ctx = runtimecontext.WithTrusted(ctx, &runtimecontext.TrustedRuntimeContext{TenantID: "tenant-a", UserID: "user-a", AgentID: "agent-a", RunID: "run-a", Time: snap})
	from := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 18, 16, 0, 0, 0, time.UTC)
	retriever := ai.NewMemoryRetrievalService(db, recallEmbedding{})
	result, err := retriever.Retrieve(ctx, ai.MemoryRecallRequest{Query: "上海会议", AgentID: "agent-a", DateFrom: &from, DateTo: &to, Limit: 10, MaxTokens: 1000, NormalOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "active" {
		t.Fatalf("hard filters leaked memories: %#v", result)
	}
	degraded, err := ai.NewMemoryRetrievalService(db, recallEmbedding{fail: true}).Retrieve(ctx, ai.MemoryRecallRequest{Query: "上海会议", AgentID: "agent-a", Limit: 10, MaxTokens: 1000, NormalOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !degraded.Degraded || degraded.DegradeReason != "embedding_provenance" || len(degraded.Items) != 1 {
		t.Fatalf("unsafe degradation: %#v", degraded)
	}
	governance := ai.NewMemoryGovernanceService(db, nil)
	management := ai.NewMemoryManagementService(db, governance, nil)
	listed, err := management.List(ctx, ai.MemoryListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	listResult := listed.(ai.MemoryListResult)
	if listResult.Total != 3 {
		t.Fatalf("self list leaked or omitted rows: %#v", listed)
	}
	if _, err = management.Get(ctx, ai.MemoryIDRequest{ID: "other"}); err == nil {
		t.Fatal("cross-user detail read was allowed")
	}
	foreignMutationCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true, IdempotencyKey: "foreign-delete"})
	if _, err = management.Decision(foreignMutationCtx, ai.MemoryIDRequest{ID: "other", Revision: 1}, "revoke"); err == nil {
		t.Fatal("cross-user mutation was allowed")
	}
	correctCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true, IdempotencyKey: "correct-1"})
	created, err := management.Correct(correctCtx, ai.MemoryCorrectRequest{ID: "active", Revision: 1, Summary: "上海会议已改期", Content: "上海会议已改期", LocalDate: "2026-07-18", DatePrecision: aimemory.DateExactDay})
	if err != nil {
		t.Fatal(err)
	}
	if created.(ai.MemoryItem).SupersedesItemID == nil {
		t.Fatal("correction did not preserve history")
	}
	clearCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true, IdempotencyKey: "clear-1"})
	if _, err = management.Clear(clearCtx, ai.MemoryClearRequest{Scope: "all", RevisionToken: listResult.RevisionToken}); err == nil {
		t.Fatal("stale clear revision token was accepted")
	}
	refreshed, err := management.List(ctx, ai.MemoryListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = management.Clear(clearCtx, ai.MemoryClearRequest{Scope: "all", RevisionToken: refreshed.(ai.MemoryListResult).RevisionToken}); err != nil {
		t.Fatal(err)
	}
	ctx = runtimecontext.WithTrusted(ctx, &runtimecontext.TrustedRuntimeContext{TenantID: "tenant-a", UserID: "user-a", AgentID: "agent-a", RunID: "run-a", Time: snap})
	afterClear, err := retriever.Retrieve(ctx, ai.MemoryRecallRequest{Query: "上海会议", AgentID: "agent-a", Limit: 10, MaxTokens: 1000, NormalOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(afterClear.Items) != 0 {
		t.Fatalf("cleared memory remained recallable: %#v", afterClear)
	}
}
