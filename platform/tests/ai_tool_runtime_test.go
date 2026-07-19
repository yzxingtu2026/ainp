package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

type fakeToolProvider struct {
	source aitool.Source
	tools  []aitool.Definition
}

func (p *fakeToolProvider) Source() aitool.Source                             { return p.source }
func (p *fakeToolProvider) List(context.Context) ([]aitool.Definition, error) { return p.tools, nil }
func (p *fakeToolProvider) Get(_ context.Context, id string) (*aitool.Definition, error) {
	for _, d := range p.tools {
		if d.ID == id {
			return &d, nil
		}
	}
	return nil, context.Canceled
}

type fakeToolExecutor struct{ called bool }

func (e *fakeToolExecutor) Execute(_ context.Context, _ aitool.LoadedTool, _ json.RawMessage) (*aitool.Result, error) {
	e.called = true
	return &aitool.Result{Status: "completed"}, nil
}

func toolFixture(id, name, summary string) aitool.Definition {
	return aitool.Definition{ID: id, Source: aitool.SourceAction, Name: name, DescriptionForLLM: summary, DescriptionForHuman: summary, Capability: "demo", Tags: []string{"采购", "订单"}, InputSchema: json.RawMessage(`{"type":"object","properties":{"supplier_id":{"type":"string"}},"required":["supplier_id"]}`), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, RuntimeRef: strings.TrimPrefix(id, "action:"), SchemaVersion: 1, Status: "enabled"}
}

func TestToolRegistryRejectsDuplicateProviderAndInvalidDefinition(t *testing.T) {
	r := aitool.NewRegistry()
	p := &fakeToolProvider{source: aitool.SourceAction, tools: []aitool.Definition{toolFixture("action:purchase.order_create", "创建采购单", "创建供应商采购订单")}}
	e := &fakeToolExecutor{}
	if err := r.RegisterProvider(p, e); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterProvider(p, e); err == nil {
		t.Fatal("duplicate provider must fail")
	}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("action:purchase.order_create"); !ok {
		t.Fatal("tool missing")
	}
	before, _ := r.Get("action:purchase.order_create")
	p.tools[0].Risk = aitool.RiskHigh
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := r.Get("action:purchase.order_create")
	if before.SchemaHash == after.SchemaHash {
		t.Fatal("security metadata change must invalidate handles")
	}
	p.tools[0].ID = "not-namespaced"
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("invalid tool must fail")
	}
}

func TestToolSearchReturnsCompactAuthorizedCandidates(t *testing.T) {
	definitions := []aitool.Definition{toolFixture("action:purchase.order_create", "创建采购单", "创建供应商采购订单"), toolFixture("action:admin.secret", "内部管理", "只允许管理员")}
	definitions[0].PermissionRefs = []string{"purchase.create"}
	definitions[1].PermissionRefs = []string{"admin.secret"}
	items, err := aitool.Search(context.Background(), definitions, "创建采购单", "", aitool.SourceAction, 5, func(d aitool.Definition) bool { return d.PermissionRefs[0] == "purchase.create" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "action:purchase.order_create" {
		t.Fatalf("unexpected candidates: %#v", items)
	}
	encoded, _ := json.Marshal(items)
	if strings.Contains(string(encoded), "input_schema") || strings.Contains(string(encoded), "runtime_ref") {
		t.Fatalf("search leaked full definition: %s", encoded)
	}
}

func TestToolHandleIsActorScopeAndSchemaBound(t *testing.T) {
	s, err := aitool.NewHandleSigner(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	claims := aitool.HandleClaims{TenantID: "t1", UserID: "u1", Realm: "tenant", SessionID: "s1", AuthVersion: 2, ScopeID: "scope1", ToolID: "action:purchase.order_create", RuntimeRef: "purchase.order_create", SchemaHash: "hash1", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	token, err := s.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Verify(token, time.Now())
	if err != nil || got.SchemaHash != "hash1" {
		t.Fatalf("verify failed: %#v %v", got, err)
	}
	if _, err := s.Verify(token+"x", time.Now()); err == nil {
		t.Fatal("tampered handle must fail")
	}
	claims.ExpiresAt = time.Now().Add(-time.Second).Unix()
	expired, _ := s.Sign(claims)
	if _, err := s.Verify(expired, time.Now()); err == nil {
		t.Fatal("expired handle must fail")
	}
}

type exposedRequest struct {
	ResourceID string `json:"resource_id"`
	Count      int    `json:"count,omitempty"`
}

func TestActionToolProviderUsesTypedSchemaAndRegistryInvoke(t *testing.T) {
	actions := action.NewRegistry(permission.NewChecker(), nil)
	handler := action.JSON(func(_ context.Context, r exposedRequest) (any, error) {
		return map[string]any{"resource_id": r.ResourceID}, nil
	})
	err := actions.Register(action.Action{ID: "demo.read", Handler: handler, Meta: action.Metadata{Capability: "demo", Access: action.AccessAuthenticated, Name: "读取资源", AI: &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", Summary: "读取指定资源", HumanDescription: "读取资源", InputSchema: action.SchemaFor[exposedRequest]()}}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &aitool.ActionProvider{Actions: actions}
	items, err := provider.List(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("projection failed: %#v %v", items, err)
	}
	if !strings.Contains(string(items[0].InputSchema), `"resource_id"`) {
		t.Fatalf("typed schema missing: %s", items[0].InputSchema)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", UserID: "u1", SessionID: "s1", Realm: auth.RealmTenant})
	result, err := (&aitool.ActionExecutor{Actions: actions}).Execute(ctx, aitool.LoadedTool{Definition: items[0]}, json.RawMessage(`{"resource_id":"r1"}`))
	if err != nil || result.Status != "completed" {
		t.Fatalf("invoke failed: %#v %v", result, err)
	}
}

func TestExposedActionFailsClosedWithoutRiskAndSchema(t *testing.T) {
	actions := action.NewRegistry(permission.NewChecker(), nil)
	err := actions.Register(action.Action{ID: "demo.unsafe", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: action.Metadata{Capability: "demo", AI: &action.AIMeta{ExposeTool: true}}})
	if err == nil {
		t.Fatal("unsafe AI action registration must fail")
	}
}

func TestSchemaForRejectsAdditionalProperties(t *testing.T) {
	schema := action.SchemaFor[exposedRequest]()
	if !strings.Contains(string(schema), `"additionalProperties":false`) {
		t.Fatalf("schema must reject unknown fields: %s", schema)
	}
}

func TestToolRuntimePostgresLifecycleAndIsolation(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN not set")
	}
	schema := "ainp_tool_" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE tenants(id varchar(64) PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO tenants(id) VALUES ('t1'),('t2')`).Error; err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = outbox.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	rt, err := outbox.NewRuntime(db, config.OutboxConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"ai.tool_catalog.changed.v1", "ai.tool_invocation.finished.v1"} {
		if err = rt.RegisterDefinition(outbox.Definition{EventType: event, SchemaVersion: 1}); err != nil {
			t.Fatal(err)
		}
	}
	registry := aitool.NewRegistry()
	executor := &fakeToolExecutor{}
	provider := &fakeToolProvider{source: aitool.SourceAction, tools: []aitool.Definition{toolFixture("action:purchase.order_read", "读取采购单", "查询采购订单")}}
	if err = registry.RegisterProvider(provider, executor); err != nil {
		t.Fatal(err)
	}
	meta := []aitool.Definition{
		{ID: "meta:tool.search", Source: aitool.SourceMeta, Name: "tool.search", DescriptionForLLM: "Search visible tools by intent and return compact candidates", DescriptionForHuman: "搜索当前用户可用的工具", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaSearchInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, RuntimeRef: "tool.search", SchemaVersion: 1, Status: "enabled"},
		{ID: "meta:tool.load", Source: aitool.SourceMeta, Name: "tool.load", DescriptionForLLM: "Load a small number of candidate tools into the current scope", DescriptionForHuman: "将候选工具加载到当前调用范围", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaLoadInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, RuntimeRef: "tool.load", SchemaVersion: 1, Status: "enabled"},
	}
	if err = registry.RegisterProvider(&aitool.StaticProvider{ProviderSource: aitool.SourceMeta, Definitions: meta}, aitool.ExecuteFunc(func(context.Context, aitool.LoadedTool, json.RawMessage) (*aitool.Result, error) {
		return &aitool.Result{Status: "completed"}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err = registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	signer, _ := aitool.NewHandleSigner(strings.Repeat("s", 32))
	service := ai.NewToolService(db, registry, signer, rt.Writer)
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", UserID: "u1", SessionID: "s1", AuthVersion: 1, Realm: auth.RealmTenant, Permissions: []string{"*"}})
	searched, err := service.Search(ctx, ai.ToolSearchRequest{Query: "采购订单"})
	if err != nil {
		t.Fatal(err)
	}
	scopeID := searched.(ai.ToolSearchResponse).ScopeID
	_, projected, err := service.ProjectModelTools(ctx, scopeID)
	if err != nil || len(projected) != 2 || projected[0].Name != "meta__tool_search" || projected[1].Name != "meta__tool_load" {
		t.Fatalf("initial projection must contain only search/load: %#v %v", projected, err)
	}
	loaded, err := service.Load(ctx, ai.ToolLoadRequest{ScopeID: scopeID, ToolIDs: []string{"action:purchase.order_read"}})
	if err != nil {
		t.Fatal(err)
	}
	handle := loaded.(ai.ToolLoadResponse).Tools[0].Handle
	_, projected, err = service.ProjectModelTools(ctx, scopeID)
	if err != nil || len(projected) != 3 {
		t.Fatalf("loaded tool was not projected: %#v %v", projected, err)
	}
	if _, err = service.Execute(ctx, ai.ToolExecuteRequest{Handle: handle, ToolCallID: "call-1", Arguments: json.RawMessage(`{"supplier_id":"s1"}`)}); err != nil {
		t.Fatal(err)
	}
	if !executor.called {
		t.Fatal("executor was not called")
	}
	var inv ai.ToolInvocation
	tdb, scopeErr := tenantdata.TenantDB(ctx, db)
	if scopeErr != nil {
		t.Fatal(scopeErr)
	}
	invocations, queryErr := tenantdata.TenantRaw[ai.ToolInvocation](tdb, "SELECT * FROM ai_tool_invocations WHERE tenant_id=? AND tool_call_id=? LIMIT 1", "call-1")
	if queryErr != nil || len(invocations) != 1 {
		t.Fatalf("unexpected invocation query: %#v %v", invocations, queryErr)
	}
	inv = invocations[0]
	if inv.Status != "completed" {
		t.Fatalf("unexpected invocation: %#v %v", inv, err)
	}
	var events []outbox.Event
	if err = db.Where("event_type='ai.tool_invocation.finished.v1'").Find(&events).Error; err != nil || len(events) != 1 {
		t.Fatalf("missing outbox event: %v %d", err, len(events))
	}
	if strings.Contains(string(events[0].PayloadJSON), "supplier_id") || strings.Contains(string(events[0].PayloadJSON), "s1") {
		t.Fatalf("arguments leaked to outbox: %s", events[0].PayloadJSON)
	}
	other := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", UserID: "u2", SessionID: "s2", AuthVersion: 1, Realm: auth.RealmTenant, Permissions: []string{"*"}})
	if _, err = service.Load(other, ai.ToolLoadRequest{ScopeID: scopeID, ToolIDs: []string{"action:purchase.order_read"}}); err == nil {
		t.Fatal("cross-user scope access must fail")
	}
}
