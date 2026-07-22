package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

type fakeToolProvider struct {
	source aitool.Source
	tools  []aitool.Definition
}

type embeddedListRequest struct {
	pagination.Request
	Keyword string `json:"keyword,omitempty"`
	Status  string `json:"status,omitempty" validate:"omitempty,oneof=enabled disabled"`
	RoleID  string `json:"role_id,omitempty"`
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

type fakeToolEmbeddingRuntime struct {
	requests []aimodelruntime.EmbeddingRequest
	err      error
}

func (f *fakeToolEmbeddingRuntime) Embed(_ context.Context, request aimodelruntime.EmbeddingRequest) (*aimodelruntime.EmbeddingResponse, error) {
	f.requests = append(f.requests, request)
	if f.err != nil {
		return nil, f.err
	}
	vectors := make([]aimodelruntime.EmbeddingVector, len(request.Inputs))
	for i, input := range request.Inputs {
		vectors[i] = aimodelruntime.EmbeddingVector{ID: input.ID, Index: i, Vector: []float32{1, 0}}
	}
	return &aimodelruntime.EmbeddingResponse{Model: aimodelruntime.ModelSummary{Alias: request.ModelAlias, DeploymentID: "embed-deployment", RemoteModelID: "embed-model"}, Vectors: vectors, Dimensions: 2, Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm, PreprocessingVersion: aimodelruntime.EmbeddingPreprocessingVersionCosine}, nil
}

type fakeToolSettingStore struct{ alias string }

func (f fakeToolSettingStore) Get(context.Context, string, string) (*setting.StoredValue, error) {
	return &setting.StoredValue{SchemaVersion: 1, Revision: 1, Values: map[string]any{aitool.SettingFieldModelAlias: f.alias}}, nil
}
func (fakeToolSettingStore) Put(context.Context, string, string, int64, int, map[string]any, string) (*setting.StoredValue, error) {
	return nil, errors.New("not implemented")
}
func (fakeToolSettingStore) Delete(context.Context, string, string, int64) error {
	return errors.New("not implemented")
}

func toolSettingProvider(t *testing.T, alias string) *setting.Provider {
	t.Helper()
	registry := setting.NewRegistry()
	if err := registry.RegisterCategory(setting.Category{Key: "ai", Title: "AI", Icon: "mdi:brain", Surface: auth.RealmTenant}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterDefinition(setting.Definition{Key: aitool.SettingKeySearch, CategoryKey: "ai", Title: "Tool search", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1, Fields: []setting.Field{{Key: aitool.SettingFieldModelAlias, Type: setting.FieldText, Label: "Embedding Model Alias", Default: ""}}}); err != nil {
		t.Fatal(err)
	}
	return setting.NewProvider(registry, fakeToolSettingStore{alias: alias})
}

func toolFixture(id, name, summary string) aitool.Definition {
	return aitool.Definition{ID: id, Source: aitool.SourceAction, Name: name, TitleForHuman: name, DescriptionForLLM: summary, DescriptionForHuman: summary, Capability: "demo", Tags: []string{"采购", "订单"}, InputSchema: json.RawMessage(`{"type":"object","properties":{"supplier_id":{"type":"string"}},"required":["supplier_id"]}`), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingForbidden, LoadingMode: aitool.LoadingProgressive, RuntimeRef: strings.TrimPrefix(id, "action:"), SchemaVersion: 1, Status: "enabled"}
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
	p.tools[0].TitleForHuman = "新展示标题"
	p.tools[0].DescriptionForHuman = "新的展示描述"
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	displayChanged, _ := r.Get("action:purchase.order_create")
	if before.SchemaHash != displayChanged.SchemaHash {
		t.Fatal("human display metadata must not invalidate handles")
	}
	p.tools[0].Risk = aitool.RiskHigh
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := r.Get("action:purchase.order_create")
	if displayChanged.SchemaHash == after.SchemaHash {
		t.Fatal("security metadata change must invalidate handles")
	}
	p.tools[0].ID = "not-namespaced"
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("invalid tool must fail")
	}
}

func TestToolDefinitionRequiresHumanDisplayContract(t *testing.T) {
	definition := toolFixture("action:demo.read", "读取资源", "读取指定资源")
	definition.TitleForHuman = ""
	if err := definition.Validate(); err == nil {
		t.Fatal("tool definition without title_for_human must fail")
	}
	definition.TitleForHuman = "读取资源"
	definition.DescriptionForHuman = ""
	if err := definition.Validate(); err == nil {
		t.Fatal("tool definition without description_for_human must fail")
	}
}

func TestToolSearchReturnsCompactAuthorizedCandidates(t *testing.T) {
	definitions := []aitool.Definition{toolFixture("action:purchase.order_create", "创建采购单", "创建供应商采购订单"), toolFixture("action:admin.secret", "内部管理", "只允许管理员")}
	definitions[0].PermissionRefs = []string{"purchase.create"}
	definitions[1].PermissionRefs = []string{"admin.secret"}
	items, err := aitool.Search(context.Background(), definitions, "创建采购单", "", aitool.SourceAction, 5, func(d aitool.Definition) bool { return d.PermissionRefs[0] == "purchase.create" })
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

func TestToolSearchNormalizesChineseIntentTerms(t *testing.T) {
	definition := toolFixture("action:system.user_list", "用户列表", "分页查询当前租户用户")
	items, err := aitool.Search(context.Background(), []aitool.Definition{definition}, "系统用户管理", "", aitool.SourceAction, 5, func(aitool.Definition) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != definition.ID {
		t.Fatalf("Chinese lexical fallback missed user action: %#v", items)
	}
	text := aitool.SearchDocumentText(definition)
	if !strings.Contains(text, "用户") || !strings.Contains(text, "户列") {
		t.Fatalf("search document is not normalized for Chinese FTS: %s", text)
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
	err := actions.Register(action.Action{ID: "demo.read", Handler: handler, Meta: action.Metadata{Capability: "demo", Access: action.AccessAuthenticated, Name: "读取资源", AI: &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", Summary: "读取指定资源", HumanDescription: "读取资源", ResultSummary: "已读取资源", InputSchema: action.SchemaFor[exposedRequest]()}}})
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

func TestSchemaForFlattensEmbeddedOptionalListFields(t *testing.T) {
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(action.SchemaFor[embeddedListRequest](), &schema); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"page", "page_size", "keyword", "status", "role_id"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("embedded list schema missing %q: %#v", field, schema.Properties)
		}
	}
	if len(schema.Required) != 0 {
		t.Fatalf("optional list fields must not be required: %#v", schema.Required)
	}
}

func TestMetaToolSearchSchemaGuidesStableFilters(t *testing.T) {
	schema := string(action.SchemaFor[aitool.MetaSearchInput]())
	for _, expected := range []string{`"enum":["meta","action","builtin","plugin","http","mcp"]`, `"description":"Optional exact capability code`, `"maximum":10`} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("meta tool search schema missing %s: %s", expected, schema)
		}
	}
	if err := (ai.ToolSearchRequest{Query: "用户列表", Source: aitool.Source("tools")}).Validate(context.Background()); err == nil {
		t.Fatal("unknown source must fail before searching")
	}
	if err := (ai.ToolSearchRequest{Query: "用户列表", Capability: "查询系统用户列表"}).Validate(context.Background()); err == nil {
		t.Fatal("natural-language capability must fail before searching")
	}
}

func TestToolRegistryBindablePolicy(t *testing.T) {
	registry := aitool.NewRegistry()
	actionTool := toolFixture("action:system.user_list", "用户列表", "查询系统用户列表")
	pluginTool := actionTool
	pluginTool.ID = "plugin:crm.customer_list"
	pluginTool.Source = aitool.SourcePlugin
	pluginTool.BindingMode = aitool.BindingOptional
	pluginTool.LoadingMode = aitool.LoadingDirect
	pluginTool.RuntimeRef = "crm.customer_list"
	if err := registry.RegisterProvider(&fakeToolProvider{source: aitool.SourceAction, tools: []aitool.Definition{actionTool}}, &fakeToolExecutor{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(&fakeToolProvider{source: aitool.SourcePlugin, tools: []aitool.Definition{pluginTool}}, &fakeToolExecutor{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if registry.IsBindable(actionTool.ID) {
		t.Fatal("action tool must not be bindable")
	}
	if !registry.IsBindable(pluginTool.ID) {
		t.Fatal("provider plugin tool should be bindable")
	}
}

func TestToolCatalogLifecycleMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0040_ai_tool_catalog_lifecycle.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{"ai_tool_catalog", "revision", "ai_tool_search_documents", "catalog_revision", "idx_ai_tool_search_status"} {
		if !strings.Contains(text, required) {
			t.Fatalf("tool catalog lifecycle migration missing %s", required)
		}
	}
	vectorMigration, err := os.ReadFile("../migrations/0041_ai_tool_vector_search.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"CREATE EXTENSION IF NOT EXISTS vector", "embedding_vector", "embedding_deployment_id", "embedding_dimensions", "embedding_preprocessing", "idx_ai_tool_search_vector_ready"} {
		if !strings.Contains(string(vectorMigration), required) {
			t.Fatalf("tool vector migration missing %s", required)
		}
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
	actionTool := toolFixture("action:purchase.order_read", "读取采购单", "查询采购订单")
	actionTool.PermissionRefs = []string{"purchase.read"}
	provider := &fakeToolProvider{source: aitool.SourceAction, tools: []aitool.Definition{actionTool}}
	if err = registry.RegisterProvider(provider, executor); err != nil {
		t.Fatal(err)
	}
	meta := []aitool.Definition{
		{ID: "meta:tool.search", Source: aitool.SourceMeta, Name: "tool.search", TitleForHuman: "工具搜索", DescriptionForLLM: "Search visible tools by intent and return compact candidates", DescriptionForHuman: "搜索当前用户可用的工具", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaSearchInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingOptional, LoadingMode: aitool.LoadingDirect, BindingGroup: "tool_discovery", RuntimeRef: "tool.search", SchemaVersion: 1, Status: "enabled"},
		{ID: "meta:tool.load", Source: aitool.SourceMeta, Name: "tool.load", TitleForHuman: "工具加载", DescriptionForLLM: "Load a small number of candidate tools into the current scope", DescriptionForHuman: "将候选工具加载到当前调用范围", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaLoadInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingOptional, LoadingMode: aitool.LoadingDirect, BindingGroup: "tool_discovery", RuntimeRef: "tool.load", SchemaVersion: 1, Status: "enabled"},
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
	embeddingRuntime := &fakeToolEmbeddingRuntime{}
	service.SetEmbeddingRuntime(embeddingRuntime)
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", UserID: "u1", SessionID: "s1", AuthVersion: 1, Realm: auth.RealmTenant, Permissions: []string{"purchase.read"}})
	guarded := tenantdata.GuardedDB(db)
	reconciled, err := service.ReconcileCatalog(ctx)
	if err != nil || reconciled.Created != 3 {
		t.Fatalf("initial catalog reconcile failed: %#v %v", reconciled, err)
	}
	indexStatus, err := service.IndexStatus(ctx)
	if err != nil || indexStatus.State != "disabled" || indexStatus.Total != 3 || indexStatus.Pending != 3 {
		t.Fatalf("unexpected disabled tool index status: %#v %v", indexStatus, err)
	}
	var actionCatalog ai.ToolCatalog
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionCatalog).Error; err != nil || actionCatalog.Revision != 1 || actionCatalog.Status != "enabled" {
		t.Fatalf("unexpected action catalog: %#v %v", actionCatalog, err)
	}
	var actionDocument ai.ToolSearchDocument
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionDocument).Error; err != nil || actionDocument.IndexStatus != "pending" || actionDocument.CatalogRevision != 1 {
		t.Fatalf("unexpected pending search document: %#v %v", actionDocument, err)
	}
	if err = service.MarkCatalogDocumentReady(ctx, ai.ToolCatalogChangedPayload{ToolID: actionTool.ID, CatalogRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionDocument).Error; err != nil || actionDocument.IndexStatus != "ready" {
		t.Fatalf("search document was not projected: %#v %v", actionDocument, err)
	}
	reconciled, err = service.ReconcileCatalog(ctx)
	if err != nil || reconciled.Unchanged != 3 || reconciled.Created != 0 || reconciled.Updated != 0 {
		t.Fatalf("idempotent reconcile changed catalog: %#v %v", reconciled, err)
	}
	provider.tools[0].DescriptionForLLM = "查询采购订单详情"
	reconciled, err = service.ReconcileCatalog(ctx)
	if err != nil || reconciled.Updated != 1 {
		t.Fatalf("catalog update was not detected: %#v %v", reconciled, err)
	}
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionCatalog).Error; err != nil || actionCatalog.Revision != 2 {
		t.Fatalf("catalog revision did not advance: %#v %v", actionCatalog, err)
	}
	if err = service.MarkCatalogDocumentReady(ctx, ai.ToolCatalogChangedPayload{ToolID: actionTool.ID, CatalogRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionDocument).Error; err != nil || actionDocument.IndexStatus != "pending" || actionDocument.CatalogRevision != 2 {
		t.Fatalf("stale event overwrote current projection: %#v %v", actionDocument, err)
	}
	if err = service.MarkCatalogDocumentReady(ctx, ai.ToolCatalogChangedPayload{ToolID: actionTool.ID, CatalogRevision: 2}); err != nil {
		t.Fatal(err)
	}
	provider.tools = nil
	reconciled, err = service.ReconcileCatalog(ctx)
	if err != nil || reconciled.Disabled != 1 {
		t.Fatalf("removed tool was not disabled: %#v %v", reconciled, err)
	}
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionCatalog).Error; err != nil || actionCatalog.Revision != 3 || actionCatalog.Status != "disabled" {
		t.Fatalf("removed catalog state is invalid: %#v %v", actionCatalog, err)
	}
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionDocument).Error; err != nil || actionDocument.IndexStatus != "stale" || actionDocument.CatalogRevision != 3 {
		t.Fatalf("removed search document is not stale: %#v %v", actionDocument, err)
	}
	provider.tools = []aitool.Definition{actionTool}
	reconciled, err = service.ReconcileCatalog(ctx)
	if err != nil || reconciled.Updated != 1 {
		t.Fatalf("restored tool was not reactivated: %#v %v", reconciled, err)
	}
	service.SetSettingProvider(toolSettingProvider(t, "tool-embed"))
	if err = service.IndexCatalogDocument(ctx, actionTool.ID, 4, "tool-embed"); err != nil {
		t.Fatal(err)
	}
	indexStatus, err = service.IndexStatus(ctx)
	if err != nil || indexStatus.State != "pending" || indexStatus.ModelAlias != "tool-embed" || indexStatus.Ready != 1 || indexStatus.Pending != 2 {
		t.Fatalf("unexpected active tool index status: %#v %v", indexStatus, err)
	}
	if err = guarded.WithContext(ctx).Where("tool_id=?", actionTool.ID).First(&actionDocument).Error; err != nil || actionDocument.IndexStatus != "ready" || actionDocument.EmbeddingAlias != "tool-embed" || actionDocument.EmbeddingDeploymentID != "embed-deployment" || actionDocument.EmbeddingDimensions != 2 || actionDocument.EmbeddingVector == nil || len(*actionDocument.EmbeddingVector) != 2 {
		t.Fatalf("tool vector projection is invalid: %#v %v", actionDocument, err)
	}
	restrictedScope := ai.ToolScope{
		ID:                      "scope-progressive",
		TenantID:                "t1",
		UserID:                  "u1",
		Realm:                   string(auth.RealmTenant),
		SessionID:               "s1",
		AuthVersion:             1,
		AllowedSourcesJSON:      json.RawMessage(`["action"]`),
		AllowedCapabilitiesJSON: json.RawMessage(`["demo"]`),
		AllowedToolsJSON:        json.RawMessage(`["meta:tool.search","meta:tool.load"]`),
		LoadedToolsJSON:         json.RawMessage(`[]`),
		ExpiresAt:               time.Now().Add(time.Hour),
	}
	tenantDB, err := tenantdata.TenantDB(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err = tenantdata.TenantExec(tenantDB, `INSERT INTO ai_tool_scopes
(tenant_id, id, user_id, realm, session_id, auth_version, allowed_sources_json, allowed_capabilities_json, allowed_tools_json, loaded_tools_json, schema_bytes, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, 0, ?)`, restrictedScope.ID, restrictedScope.UserID, restrictedScope.Realm, restrictedScope.SessionID, restrictedScope.AuthVersion, restrictedScope.AllowedSourcesJSON, restrictedScope.AllowedCapabilitiesJSON, restrictedScope.AllowedToolsJSON, restrictedScope.LoadedToolsJSON, restrictedScope.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	var catalogEventsBefore int64
	if err = db.Model(&outbox.Event{}).Where("event_type=?", "ai.tool_catalog.changed.v1").Count(&catalogEventsBefore).Error; err != nil {
		t.Fatal(err)
	}
	searched, err := service.Search(ctx, ai.ToolSearchRequest{ScopeID: restrictedScope.ID, Query: "采购订单"})
	if err != nil {
		t.Fatal(err)
	}
	scopeID := searched.(ai.ToolSearchResponse).ScopeID
	if items := searched.(ai.ToolSearchResponse).Items; len(items) != 1 || items[0].ID != actionTool.ID {
		t.Fatalf("source/capability scope did not discover action tool: %#v", items)
	}
	semantic, err := service.Search(ctx, ai.ToolSearchRequest{ScopeID: restrictedScope.ID, Query: "完全不匹配的语义查询"})
	if err != nil {
		t.Fatal(err)
	}
	semanticResponse := semantic.(ai.ToolSearchResponse)
	if semanticResponse.Degraded || len(semanticResponse.Items) != 1 || semanticResponse.Items[0].ID != actionTool.ID {
		t.Fatalf("vector-only tool search failed: %#v", semanticResponse)
	}
	for _, request := range embeddingRuntime.requests {
		if request.Purpose != aimodelruntime.EmbeddingPurposeToolIndex || request.Preprocessing != aimodelruntime.EmbeddingPreprocessingCosineNorm {
			t.Fatalf("unexpected governed embedding request: %#v", request)
		}
	}
	service.SetEmbeddingRuntime(&fakeToolEmbeddingRuntime{err: errors.New("embedding unavailable")})
	fallback, err := service.Search(ctx, ai.ToolSearchRequest{ScopeID: restrictedScope.ID, Query: "采购订单"})
	if err != nil {
		t.Fatal(err)
	}
	fallbackResponse := fallback.(ai.ToolSearchResponse)
	if !fallbackResponse.Degraded || fallbackResponse.DegradeReason != "embedding_runtime" || len(fallbackResponse.Items) != 1 || fallbackResponse.Items[0].ID != actionTool.ID {
		t.Fatalf("embedding failure did not preserve lexical fallback: %#v", fallbackResponse)
	}
	service.SetEmbeddingRuntime(embeddingRuntime)
	var catalogEventsAfter int64
	if err = db.Model(&outbox.Event{}).Where("event_type=?", "ai.tool_catalog.changed.v1").Count(&catalogEventsAfter).Error; err != nil || catalogEventsAfter != catalogEventsBefore {
		t.Fatalf("tool search mutated catalog events: before=%d after=%d err=%v", catalogEventsBefore, catalogEventsAfter, err)
	}
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
	revoked := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", UserID: "u1", SessionID: "s1", AuthVersion: 1, Realm: auth.RealmTenant})
	if _, err = service.Execute(revoked, ai.ToolExecuteRequest{Handle: handle, ToolCallID: "call-revoked", Arguments: json.RawMessage(`{"supplier_id":"s1"}`)}); err == nil {
		t.Fatal("permission revocation must invalidate an existing handle")
	}
	other := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", UserID: "u2", SessionID: "s2", AuthVersion: 1, Realm: auth.RealmTenant, Permissions: []string{"*"}})
	if _, err = service.Load(other, ai.ToolLoadRequest{ScopeID: scopeID, ToolIDs: []string{"action:purchase.order_read"}}); err == nil {
		t.Fatal("cross-user scope access must fail")
	}
	ctxT2 := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t2", UserID: "u2", SessionID: "s2", AuthVersion: 1, Realm: auth.RealmTenant, Permissions: []string{"purchase.read"}})
	if reconciled, err = service.ReconcileCatalog(ctxT2); err != nil || reconciled.Created != 3 {
		t.Fatalf("second tenant catalog reconcile failed: %#v %v", reconciled, err)
	}
	for tenantID, tenantCtx := range map[string]context.Context{"t1": ctx, "t2": ctxT2} {
		tdb, tenantErr := tenantdata.TenantDB(tenantCtx, db)
		if tenantErr != nil {
			t.Fatal(tenantErr)
		}
		rows, tenantErr := tenantdata.TenantRaw[struct{ Count int64 }](tdb, "SELECT COUNT(*) AS count FROM ai_tool_catalog WHERE tenant_id = ?")
		if tenantErr != nil || len(rows) != 1 || rows[0].Count != 3 {
			t.Fatalf("tenant %s catalog isolation failed: %#v %v", tenantID, rows, tenantErr)
		}
	}
}
