package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"gorm.io/gorm"
)

func validAgentContract() aiagent.Contract {
	return aiagent.Contract{Key: "demo.assistant", Version: "1.0.0", Name: "Demo", BaseInstructions: "Keep tenant boundaries.", RequiredModelCapabilities: []string{"chat", "tools"}, RequiredToolSelectors: []string{"meta:tool.search"}, ConfigSchema: json.RawMessage(`{"type":"object"}`), SingletonScope: aiagent.SingletonNone, ManagedFields: []string{"source", "contract_key"}, UpgradePolicy: "managed-fields"}
}

func TestAgentActionRequestsAcceptSnakeCaseJSON(t *testing.T) {
	create := action.JSON(func(_ context.Context, request ai.AgentCreateRequest) (any, error) { return request, nil })
	input := json.RawMessage(`{"code":"test","name":"测试","description":"","source":"personal","visibility":"private","model_alias":"test","tenant_instructions":"hhh","generation_config":{},"tool_policy":{}}`)
	result, err := create(context.Background(), input)
	if err != nil {
		t.Fatalf("decode create request: %v", err)
	}
	if result.(ai.AgentCreateRequest).ModelAlias != "test" {
		t.Fatalf("model_alias was not decoded: %#v", result)
	}

	empty := action.JSON(func(_ context.Context, request ai.AgentEmptyRequest) (any, error) { return request, nil })
	if _, err := empty(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("decode reconcile request: %v", err)
	}
	status := action.JSON(func(_ context.Context, request ai.AgentStatusRequest) (any, error) { return request, nil })
	if _, err := status(context.Background(), json.RawMessage(`{"id":"agent-1","status":"enabled","revision":2}`)); err != nil {
		t.Fatalf("decode status request: %v", err)
	}
	search := action.JSON(func(_ context.Context, request ai.AgentToolSearchRequest) (any, error) { return request, nil })
	result, err = search(context.Background(), json.RawMessage(`{"query":"","limit":10}`))
	if err != nil {
		t.Fatalf("empty agent tool query must list available tools: %v", err)
	}
	if result.(ai.AgentToolSearchRequest).Limit != 10 {
		t.Fatalf("tool search limit was not decoded: %#v", result)
	}
}

func TestAgentDetailExposesReadOnlyContractOverlay(t *testing.T) {
	detail := ai.AgentDetail{
		Agent:                 ai.Agent{ID: "a1", Code: "demo", Name: "Demo", Source: "builtin", Visibility: "tenant", Status: "enabled", Revision: 1},
		BaseInstructions:      "Keep tenant boundaries.",
		ManagedFields:         []string{"code", "source"},
		RequiredToolSelectors: []string{"meta:tool.search"},
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{`"base_instructions":"Keep tenant boundaries."`, `"managed_fields":["code","source"]`, `"required_tool_selectors":["meta:tool.search"]`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}

func TestAgentContractRegistryFailClosed(t *testing.T) {
	r := aiagent.NewRegistry()
	c := validAgentContract()
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(c); err == nil {
		t.Fatal("duplicate contract must fail")
	}
	bad := c
	bad.Version = "next"
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("invalid version must fail")
	}
	bad = c
	bad.RequiredModelCapabilities = []string{"unknown"}
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("unknown capability must fail")
	}
	bad = c
	bad.RequiredToolSelectors = []string{"tool.search"}
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("non-namespaced selector must fail")
	}
	bad = c
	bad.ManagedFields = []string{"tenant_id"}
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("unsafe managed field must fail")
	}
	if got, ok := r.Get(c.Key, c.Version); !ok || got.Name != c.Name {
		t.Fatalf("registered contract missing: %#v", got)
	}
}

func openAgentManagementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "agent_management_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	db, err := database.Open(fmt.Sprintf("%s search_path=%s", dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrateAgents(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAgentEnableRequiresFirstPublishedVersion(t *testing.T) {
	db := openAgentManagementTestDB(t)
	owner := "user-a"
	row := ai.Agent{
		ID: "agent-without-release", TenantID: "tenant-a", OwnerUserID: &owner,
		Code: "draft-agent", Name: "Draft Agent", Source: "personal", Visibility: "private",
		Status: "draft", CurrentVersion: 0, Revision: 1,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	service := ai.NewAgentService(db, aiagent.NewRegistry(), nil, aitool.NewRegistry())
	ctx := auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant,
	})

	_, err := service.Status(ctx, ai.AgentStatusRequest{ID: row.ID, Status: "enabled", Revision: row.Revision})
	var appErr *errs.AppError
	if !errors.As(err, &appErr) || appErr.Code != errs.CodeConflict || appErr.Key() != "ai.agent.publish_required" {
		t.Fatalf("expected publish-required conflict, got %#v", err)
	}

	value, err := service.Diagnostics(ctx, ai.IDRequest{ID: row.ID})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := value.(ai.AgentDiagnostics)
	if diagnostics.Healthy || len(diagnostics.Issues) != 1 || diagnostics.Issues[0].Code != "version.missing" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestAgentServiceTenantRepositoryIsolation(t *testing.T) {
	db := openAgentManagementTestDB(t)
	service := ai.NewAgentService(db, aiagent.NewRegistry(), nil, aitool.NewRegistry())
	platform := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true})
	sdb, err := tenantdata.SystemDB(platform, db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := tenantdata.NewSystemRepository[ai.Agent](sdb)
	if err != nil {
		t.Fatal(err)
	}
	ownerA, ownerB := "user-a", "user-b"
	for _, row := range []ai.Agent{{ID: "agent-a", TenantID: "tenant-a", OwnerUserID: &ownerA, Code: "a", Name: "A", Source: "personal", Visibility: "private", Status: "draft", Revision: 1}, {ID: "agent-b", TenantID: "tenant-b", OwnerUserID: &ownerB, Code: "b", Name: "B", Source: "personal", Visibility: "private", Status: "draft", Revision: 1}} {
		current := row
		if err = repo.Create(&current); err != nil {
			t.Fatal(err)
		}
	}
	ctxA := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant})
	if _, err = service.Get(ctxA, ai.IDRequest{ID: "agent-b"}); err == nil {
		t.Fatal("tenant A read tenant B agent")
	}
	if got, err := service.Get(ctxA, ai.IDRequest{ID: "agent-a"}); err != nil || got.(ai.AgentDetail).ID != "agent-a" {
		t.Fatalf("same tenant get failed: %#v %v", got, err)
	}
	if err = db.First(&ai.Agent{}, "id=?", "agent-a").Error; !errors.Is(err, tenantdata.ErrTenantDBRequired) {
		t.Fatalf("naked Agent query did not fail closed: %v", err)
	}
}

func TestBuilderPreviewConversationUsesAgentTenantRepository(t *testing.T) {
	db := openAgentManagementTestDB(t)
	_ = ai.NewAgentService(db, aiagent.NewRegistry(), nil, aitool.NewRegistry())
	if err := db.AutoMigrate(&ai.Conversation{}); err != nil {
		t.Fatal(err)
	}
	platform := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true})
	sdb, err := tenantdata.SystemDB(platform, db)
	if err != nil {
		t.Fatal(err)
	}
	agents, _ := tenantdata.NewSystemRepository[ai.Agent](sdb)
	versions, _ := tenantdata.NewSystemRepository[ai.AgentVersion](sdb)
	owner := "user-a"
	agent := ai.Agent{ID: "preview-agent", TenantID: "tenant-a", OwnerUserID: &owner, Code: "preview", Name: "Preview", Source: "personal", Visibility: "private", Status: "draft", Revision: 1}
	version := ai.AgentVersion{ID: "preview-v1", TenantID: "tenant-a", AgentID: agent.ID, Version: 1, Kind: ai.AgentVersionKindPreview, ModelAlias: "mock", CreatedBy: owner}
	if err = agents.Create(&agent); err != nil {
		t.Fatal(err)
	}
	if err = versions.Create(&version); err != nil {
		t.Fatal(err)
	}
	service := ai.NewConversationService(db, nil)
	ctxA := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", UserID: owner, Realm: auth.RealmTenant})
	value, err := service.Create(ctxA, ai.ConversationCreateRequest{AgentID: agent.ID, AgentVersionID: version.ID, Purpose: ai.ConversationPurposeBuilderPreview, Visibility: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if value.(ai.Conversation).Purpose != ai.ConversationPurposeBuilderPreview {
		t.Fatalf("unexpected conversation: %#v", value)
	}
	ctxB := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-b", UserID: "user-b", Realm: auth.RealmTenant})
	if _, err = service.Create(ctxB, ai.ConversationCreateRequest{AgentID: agent.ID, AgentVersionID: version.ID, Purpose: ai.ConversationPurposeBuilderPreview}); err == nil {
		t.Fatal("tenant B created preview for tenant A agent")
	}
}
