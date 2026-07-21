package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

type fixedPageResolver struct{ snapshot pagecontext.Snapshot }

func (r fixedPageResolver) Resolve(context.Context, pagecontext.Envelope) (pagecontext.Snapshot, error) {
	return r.snapshot, nil
}

func TestPageContextEnvelopeBudgetsAndResolverOwnership(t *testing.T) {
	valid := pagecontext.Envelope{SchemaVersion: pagecontext.SchemaVersion, PageType: "demo.orders", RouteName: "demo_orders", Mode: "list", UIState: map[string]any{"keyword": "pending"}, ToolHints: pagecontext.ToolHints{Capabilities: []string{"forged"}}}
	registry := pagecontext.NewRegistry()
	if err := registry.Register("demo.orders", fixedPageResolver{snapshot: pagecontext.Snapshot{SafeSummary: "server summary", ToolHints: pagecontext.ToolHints{Capabilities: []string{"demo"}}}}); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(context.Background(), valid)
	if err != nil || resolved.SafeSummary != "server summary" || len(resolved.ToolHints.Capabilities) != 1 || resolved.ToolHints.Capabilities[0] != "demo" {
		t.Fatalf("resolver did not replace untrusted projection: %#v %v", resolved, err)
	}
	for name, envelope := range map[string]pagecontext.Envelope{
		"schema":  {SchemaVersion: "2.0", PageType: "demo.orders", RouteName: "demo_orders", Mode: "list"},
		"unknown": {SchemaVersion: pagecontext.SchemaVersion, PageType: "demo.unknown", RouteName: "demo_orders", Mode: "list"},
		"nested":  {SchemaVersion: pagecontext.SchemaVersion, PageType: "demo.orders", RouteName: "demo_orders", Mode: "list", UIState: map[string]any{"state": []any{[]any{"secret"}}}},
		"bytes":   {SchemaVersion: pagecontext.SchemaVersion, PageType: "demo.orders", RouteName: "demo_orders", Mode: "list", Summary: strings.Repeat("x", pagecontext.MaxBytes)},
	} {
		if _, err := registry.Resolve(context.Background(), envelope); err == nil {
			t.Fatalf("%s envelope must fail", name)
		}
	}
	oversizedRegistry := pagecontext.NewRegistry()
	if err := oversizedRegistry.Register("demo.orders", fixedPageResolver{snapshot: pagecontext.Snapshot{SafeSummary: strings.Repeat("x", 1025)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := oversizedRegistry.Resolve(context.Background(), valid); err == nil {
		t.Fatal("oversized resolver projection must fail")
	}
}

func TestSystemUserPageContextRevalidatesPermissionTenantAndRevision(t *testing.T) {
	db := openOutboxTestDB(t)
	if err := system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	for _, row := range []tenant.Tenant{{ID: "t1", Name: "Tenant 1", Status: status.ActivationEnabled}, {ID: "t2", Name: "Tenant 2", Status: status.ActivationEnabled}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	application := app.New(&config.Config{}, db, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, user := range []model.User{
		{ID: "u1", TenantID: "t1", Username: "alpha", DisplayName: "Alpha", PasswordHash: "x", Status: "enabled", UpdatedAt: now},
		{ID: "u2", TenantID: "t2", Username: "beta", DisplayName: "Beta", PasswordHash: "x", Status: "enabled", UpdatedAt: now},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	envelope := pagecontext.Envelope{SchemaVersion: pagecontext.SchemaVersion, PageType: system.PageTypeSystemUsers, RouteName: "system_users", Mode: "edit", ResourceRefs: []pagecontext.ResourceRef{{ResourceType: "system.user", ResourceID: "u1", Revision: now.Format(time.RFC3339Nano)}}, Summary: "forged admin"}
	allowed := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant, UserID: "viewer", Permissions: []string{system.ActionUserGet}})
	resolved, err := application.PageContexts.Resolve(allowed, envelope)
	if err != nil || len(resolved.TrustedRefs) != 1 || strings.Contains(resolved.SafeSummary, "forged") {
		t.Fatalf("trusted user projection failed: %#v %v", resolved, err)
	}
	denied := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant, UserID: "viewer"})
	if _, err := application.PageContexts.Resolve(denied, envelope); err == nil {
		t.Fatal("page context without permission must fail")
	}
	envelope.ResourceRefs[0].ResourceID = "u2"
	if _, err := application.PageContexts.Resolve(allowed, envelope); err == nil {
		t.Fatal("cross-tenant resource reference must fail opaquely")
	}
	envelope.ResourceRefs[0].ResourceID = "u1"
	envelope.ResourceRefs[0].Revision = now.Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := application.PageContexts.Resolve(allowed, envelope); err == nil {
		t.Fatal("stale resource revision must fail")
	}
}

func TestPageContextBoostsOnlyVisibleToolCandidates(t *testing.T) {
	db := openOutboxTestDB(t)
	if err := system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&tenant.Tenant{ID: "t1", Name: "Tenant 1", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	registry := aitool.NewRegistry()
	operationsTool := toolFixture("action:operations.audit_list", "审计", "查询记录")
	operationsTool.Capability = "operations"
	systemTool := toolFixture("action:system.user_list", "用户", "查询记录")
	systemTool.Capability = "system"
	if err := registry.RegisterProvider(&fakeToolProvider{source: aitool.SourceAction, tools: []aitool.Definition{operationsTool, systemTool}}, &fakeToolExecutor{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	signer, err := aitool.NewHandleSigner(strings.Repeat("p", 32))
	if err != nil {
		t.Fatal(err)
	}
	service := ai.NewToolService(db, registry, signer, nil)
	base := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant, UserID: "u1", SessionID: "s1", AuthVersion: 1})
	ctx := pagecontext.WithSnapshot(base, &pagecontext.Snapshot{SchemaVersion: pagecontext.SchemaVersion, PageType: "system.users", ToolHints: pagecontext.ToolHints{Capabilities: []string{"system"}}})
	value, err := service.Search(ctx, ai.ToolSearchRequest{Query: "查询记录", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	items := value.(ai.ToolSearchResponse).Items
	if len(items) != 2 || items[0].ID != systemTool.ID {
		t.Fatalf("page hint did not prioritize the visible candidate: %#v", items)
	}
	restricted := ai.ToolScope{ID: "scope-operations", TenantID: "t1", UserID: "u1", Realm: string(auth.RealmTenant), SessionID: "s1", AuthVersion: 1, AllowedSourcesJSON: json.RawMessage(`["action"]`), AllowedCapabilitiesJSON: json.RawMessage(`["operations"]`), AllowedToolsJSON: json.RawMessage(`[]`), LoadedToolsJSON: json.RawMessage(`[]`), ExpiresAt: time.Now().Add(time.Minute)}
	tdb, err := tenant.TenantDB(base, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := tenant.TenantExec(tdb, `INSERT INTO ai_tool_scopes
(tenant_id, id, user_id, realm, session_id, auth_version, allowed_sources_json, allowed_capabilities_json, allowed_tools_json, loaded_tools_json, schema_bytes, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, 0, ?)`, restricted.ID, restricted.UserID, restricted.Realm, restricted.SessionID, restricted.AuthVersion, restricted.AllowedSourcesJSON, restricted.AllowedCapabilitiesJSON, restricted.AllowedToolsJSON, restricted.LoadedToolsJSON, restricted.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	value, err = service.Search(ctx, ai.ToolSearchRequest{ScopeID: restricted.ID, Query: "查询记录", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	items = value.(ai.ToolSearchResponse).Items
	if len(items) != 1 || items[0].ID != operationsTool.ID {
		t.Fatalf("page hint expanded Agent ToolScope: %#v", items)
	}
}

func TestPageContextMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_page_context_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE ai_agent_runs (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE ai_context_snapshots (id varchar(64) PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../migrations/0043_page_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND ((table_name='ai_agent_runs' AND column_name IN ('page_context_json','page_context_type','page_context_status','page_context_bytes')) OR (table_name='ai_context_snapshots' AND column_name='page_context_refs'))`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("page context columns=%d, want 5", count)
	}
}

func TestPageContextFrontendUsesRegistryWithoutDOMOrPromptAssembly(t *testing.T) {
	registry, err := os.ReadFile("../../frontend/src/composables/page-context.ts")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.ReadFile("../../frontend/src/views/ai/workspace/use-workspace.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"document.querySelector", "document.body", "innerHTML", "system_prompt", "systemPrompt"} {
		if strings.Contains(string(registry), forbidden) || strings.Contains(string(workspace), forbidden) {
			t.Fatalf("frontend page context contains forbidden coupling %q", forbidden)
		}
	}
	if !strings.Contains(string(workspace), "capturePageContext()") {
		t.Fatal("Agent Run start does not capture the registered page context")
	}
	if strings.Contains(string(workspace), "page_type: 'system.users'") {
		t.Fatal("global AI consumer contains a business page branch")
	}
}

func TestPageContextSnapshotDoesNotPersistCanonicalMessageOrSensitivePayloads(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0043_page_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(migration))
	for _, forbidden := range []string{"prompt", "dom", "form_draft", "credential", "secret", "ai_messages"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("page context migration persists forbidden field %q", forbidden)
		}
	}
}

func TestAgentRunPageContextUsesCanonicalIdempotentSnapshot(t *testing.T) {
	source, err := os.ReadFile("../modules/ai/agent_runtime_service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	idempotentLookup := strings.Index(text, `Where("user_id=? AND idempotency_key=?"`)
	resolve := strings.Index(text, "s.pageContexts.Resolve(ctx")
	if idempotentLookup < 0 || resolve < 0 || idempotentLookup > resolve {
		t.Fatal("existing Run must be returned before Page Context is resolved again")
	}
	projectionStart := strings.Index(text, "func safePageContextProjection")
	if projectionStart < 0 {
		t.Fatal("safe page context projection is missing")
	}
	projectionEnd := strings.Index(text[projectionStart:], "\n}")
	if projectionEnd < 0 {
		t.Fatal("safe page context projection is incomplete")
	}
	projection := text[projectionStart : projectionStart+projectionEnd]
	for _, forbidden := range []string{"SafeSummary", "ToolHints", "safe_summary", "tool_hints"} {
		if strings.Contains(projection, forbidden) {
			t.Fatalf("ordinary Run projection exposes internal page context field %q", forbidden)
		}
	}
}
