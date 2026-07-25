package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/response"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

type dynamicReferenceRequest struct {
	Values []string `json:"values" validate:"required,dive,required"`
}

func dynamicReferenceMeta(sideEffect string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, ToolTitle: "测试动态引用", Risk: map[bool]string{true: "high", false: "low"}[sideEffect == "write"], SideEffect: sideEffect, RequiresConfirmation: sideEffect == "write", Summary: "测试动态引用", HumanDescription: "测试动态引用", ResultSummary: "已完成", InputSchema: action.InputSchemaFor[dynamicReferenceRequest](), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}
}

func TestDynamicReferenceRegistrationAndSafeValidation(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	readMeta := dynamicReferenceMeta("read")
	if err := r.Register(action.Action{ID: "demo.catalog", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: action.Metadata{Capability: "demo", Access: action.AccessAuthenticated, AI: readMeta}}); err != nil {
		t.Fatal(err)
	}
	writeMeta := dynamicReferenceMeta("write")
	writeMeta.DynamicReferences = []action.DynamicReference{{Field: "values[]", ResolverID: "demo.values", DiscoveryAction: "demo.catalog", RequireEvidence: true}}
	called := false
	if err := r.Register(action.Action{ID: "demo.write", Handler: func(context.Context, json.RawMessage) (any, error) { called = true; return nil, nil }, Meta: action.Metadata{Capability: "demo", Access: action.AccessAuthenticated, AI: writeMeta}}); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDynamicReferences(); err == nil {
		t.Fatal("unknown resolver must fail closed")
	}
	if err := r.RegisterReferenceResolver("demo.values", action.ReferenceResolverFunc(func(context.Context) (action.ReferenceSnapshot, error) {
		return action.ReferenceSnapshot{Values: []string{"b", "a", "a"}}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDynamicReferences(); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "t1", UserID: "u1"})
	_, err := r.Invoke(ctx, "demo.write", json.RawMessage(`{"values":["secret-z","secret-z","a"]}`))
	appErr, ok := errs.AsAppError(err)
	if !ok || appErr.Details == nil || appErr.Details.Reason != action.InvalidReference || appErr.Details.InvalidCount != 1 || len(appErr.Details.InvalidValues) != 1 || called {
		t.Fatalf("unexpected validation result: %#v called=%v", err, called)
	}
}

func TestDynamicReferenceProjectionDoesNotEmbedCandidatesInSchema(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	readMeta := dynamicReferenceMeta("read")
	_ = r.Register(action.Action{ID: "demo.catalog", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: action.Metadata{Capability: "demo", Access: action.AccessAuthenticated, AI: readMeta}})
	writeMeta := dynamicReferenceMeta("write")
	writeMeta.DynamicReferences = []action.DynamicReference{{Field: "values[]", ResolverID: "demo.values", DiscoveryAction: "demo.catalog", RequireEvidence: true}}
	_ = r.Register(action.Action{ID: "demo.write", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: action.Metadata{Capability: "demo", Access: action.AccessAuthenticated, AI: writeMeta}})
	_ = r.RegisterReferenceResolver("demo.values", action.ReferenceResolverFunc(func(context.Context) (action.ReferenceSnapshot, error) {
		return action.ReferenceSnapshot{Values: []string{"candidate-one"}}, nil
	}))
	provider := &aitool.ActionProvider{Actions: r}
	items, err := provider.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == "action:demo.write" {
			if len(item.DynamicReferences) != 1 || item.DynamicReferences[0].DiscoveryToolID != "action:demo.catalog" {
				t.Fatalf("missing prerequisite: %#v", item)
			}
			if string(item.InputSchema) == "" || containsJSONText(item.InputSchema, "candidate-one") {
				t.Fatal("runtime candidate leaked into stable schema")
			}
			return
		}
	}
	t.Fatal("projected write tool missing")
}

func TestDynamicReferenceEvidenceMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0051_tool_dynamic_reference_evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{"ai_tool_scopes", "reference_evidence_json", "jsonb", "NOT NULL"} {
		if !strings.Contains(text, required) {
			t.Fatalf("dynamic reference migration missing %s", required)
		}
	}
}

func TestSafeDetailsHTTPEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/", nil)
	response.Fail(ctx, errs.WithSafeDetails(errs.ValidationKey("reference.invalid", "invalid reference"), errs.SafeDetails{Field: "values[]", Reason: action.InvalidReference, InvalidValues: []string{"bad"}, InvalidCount: 1, RefreshAction: "demo.catalog", RefreshRecommended: true}))
	var envelope struct {
		Data struct {
			Details errs.SafeDetails `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Details.Reason != action.InvalidReference || len(envelope.Data.Details.InvalidValues) != 1 {
		t.Fatalf("safe details missing from envelope: %s", recorder.Body.String())
	}
}

func containsJSONText(raw json.RawMessage, value string) bool {
	return json.Valid(raw) && strings.Contains(string(raw), value)
}

func TestDynamicReferenceToolEvidenceAndStaleConfirmation(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN not set")
	}
	schema := "ainp_dynamic_ref_" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000"), ".", "")
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
	if err = db.Exec(`INSERT INTO tenants(id) VALUES ('t1')`).Error; err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	actions := action.NewRegistry(nil, nil)
	readMeta := dynamicReferenceMeta("read")
	if err = actions.Register(action.Action{ID: "demo.catalog", Handler: func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"items": []string{"a"}}, nil
	}, Meta: action.Metadata{Capability: "demo", Name: "测试引用目录", Access: action.AccessAuthenticated, AI: readMeta}}); err != nil {
		t.Fatal(err)
	}
	writeMeta := dynamicReferenceMeta("write")
	writeMeta.DynamicReferences = []action.DynamicReference{{Field: "values[]", ResolverID: "demo.values", DiscoveryAction: "demo.catalog", RequireEvidence: true}}
	if err = actions.Register(action.Action{ID: "demo.write", Handler: func(context.Context, json.RawMessage) (any, error) { return map[string]any{"ok": true}, nil }, Meta: action.Metadata{Capability: "demo", Name: "测试引用写入", Access: action.AccessAuthenticated, AI: writeMeta}}); err != nil {
		t.Fatal(err)
	}
	var mu sync.RWMutex
	values := []string{"a"}
	if err = actions.RegisterReferenceResolver("demo.values", action.ReferenceResolverFunc(func(context.Context) (action.ReferenceSnapshot, error) {
		mu.RLock()
		defer mu.RUnlock()
		return action.ReferenceSnapshot{Values: append([]string(nil), values...)}, nil
	})); err != nil {
		t.Fatal(err)
	}
	registry := aitool.NewRegistry()
	if err = registry.RegisterProvider(&aitool.ActionProvider{Actions: actions}, &aitool.ActionExecutor{Actions: actions}); err != nil {
		t.Fatal(err)
	}
	if err = registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	signer, _ := aitool.NewHandleSigner(strings.Repeat("d", 32))
	service := ai.NewToolService(db, registry, signer, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", UserID: "u1", SessionID: "s1", AuthVersion: 1, Realm: auth.RealmTenant})
	tdb, err := tenantdata.TenantDB(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	if err = tenantdata.TenantExec(tdb, `INSERT INTO ai_tool_scopes
(tenant_id,id,user_id,realm,session_id,auth_version,allowed_sources_json,allowed_capabilities_json,allowed_tools_json,loaded_tools_json,schema_bytes,expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, 0, ?)`, "scope-ref", "u1", "tenant", "s1", 1, json.RawMessage(`["action"]`), json.RawMessage(`["demo"]`), json.RawMessage(`[]`), json.RawMessage(`[]`), expires); err != nil {
		t.Fatal(err)
	}
	loadedAny, err := service.Load(ctx, ai.ToolLoadRequest{ScopeID: "scope-ref", ToolIDs: []string{"action:demo.catalog", "action:demo.write"}})
	if err != nil {
		t.Fatal(err)
	}
	loaded := loadedAny.(ai.ToolLoadResponse).Tools
	byID := map[string]aitool.LoadedTool{}
	for _, tool := range loaded {
		byID[tool.Definition.ID] = tool
	}
	execute := func(toolID, callID, args string) *aitool.Result {
		value, execErr := service.Execute(ctx, ai.ToolExecuteRequest{Handle: byID[toolID].Handle, ToolCallID: callID, Arguments: json.RawMessage(args)})
		if execErr != nil {
			t.Fatal(execErr)
		}
		return value.(*aitool.Result)
	}
	if result := execute("action:demo.write", "call-required", `{"values":["a"]}`); result.ErrorCategory != action.ReferenceRequired {
		t.Fatalf("expected reference_required: %#v", result)
	}
	if result := execute("action:demo.catalog", "call-catalog", `{"values":[]}`); result.Status != aitool.ResultStatusCompleted {
		t.Fatalf("catalog failed: %#v", result)
	}
	if result := execute("action:demo.write", "call-invalid", `{"values":["z"]}`); result.ErrorCategory != action.InvalidReference || result.ErrorDetails == nil {
		t.Fatalf("expected invalid_reference: %#v", result)
	}
	pending := execute("action:demo.write", "call-confirm", `{"values":["a"]}`)
	if pending.Status != aitool.ResultStatusConfirmationRequired {
		t.Fatalf("confirmation missing: %#v", pending)
	}
	confirmation := pending.Data.(ai.ToolConfirmation)
	mu.Lock()
	values = []string{"b"}
	mu.Unlock()
	approved, err := service.Approve(ctx, ai.ToolConfirmationRequest{ID: confirmation.ID})
	if err != nil {
		t.Fatal(err)
	}
	if result := approved.(*aitool.Result); result.ErrorCategory != action.ReferenceStale || result.ErrorDetails == nil {
		t.Fatalf("expected stale result: %#v", result)
	}
	var stored ai.ToolConfirmation
	if err = tenantdata.GuardedDB(db).WithContext(ctx).Where("id=?", confirmation.ID).First(&stored).Error; err != nil || stored.Status != ai.ToolConfirmationConsumed {
		t.Fatalf("confirmation not consumed: %#v %v", stored, err)
	}
}
