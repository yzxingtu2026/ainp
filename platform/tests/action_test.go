package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
)

func TestRegistryDefaultsSurfaceAndRejectsWrongRealm(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	called := false
	err := r.Register(action.Action{ID: "demo.run", Meta: action.Metadata{Capability: "demo", Access: action.AccessPublic}, Handler: func(context.Context, json.RawMessage) (any, error) {
		called = true
		return nil, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := r.Get("demo.run")
	if a.Meta.Surface != auth.RealmTenant {
		t.Fatalf("expected tenant default, got %q", a.Meta.Surface)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmPlatform, Authenticated: true})
	if _, err := r.Invoke(ctx, "demo.run", nil); err == nil || called {
		t.Fatal("platform realm must not invoke tenant action")
	}
}

func TestRegistryAccessModes(t *testing.T) {
	r := action.NewRegistry(permission.NewChecker(), nil)
	handler := func(context.Context, json.RawMessage) (any, error) { return "ok", nil }
	for _, item := range []struct {
		id     string
		access action.AccessMode
	}{
		{id: "demo.public", access: action.AccessPublic},
		{id: "demo.authenticated", access: action.AccessAuthenticated},
		{id: "demo.permission"},
	} {
		if err := r.Register(action.Action{ID: item.id, Meta: action.Metadata{Capability: "demo", Access: item.access}, Handler: handler}); err != nil {
			t.Fatal(err)
		}
	}
	defaultAction, _ := r.Get("demo.permission")
	if defaultAction.Meta.Access != action.AccessPermission {
		t.Fatalf("empty access normalized to %q", defaultAction.Meta.Access)
	}
	anonymous := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmTenant})
	authed := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmTenant, Authenticated: true})
	allowed := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{"demo.permission"}})
	if _, err := r.Invoke(anonymous, "demo.public", nil); err != nil {
		t.Fatalf("public: %v", err)
	}
	if _, err := r.Invoke(anonymous, "demo.authenticated", nil); errorCode(err) != errs.CodeUnauthorized {
		t.Fatalf("anonymous authenticated action: %v", err)
	}
	if _, err := r.Invoke(authed, "demo.authenticated", nil); err != nil {
		t.Fatalf("authenticated without permission: %v", err)
	}
	if _, err := r.Invoke(anonymous, "demo.permission", nil); errorCode(err) != errs.CodeUnauthorized {
		t.Fatalf("anonymous permission action: %v", err)
	}
	if _, err := r.Invoke(authed, "demo.permission", nil); errorCode(err) != errs.CodeForbidden {
		t.Fatalf("missing permission: %v", err)
	}
	if _, err := r.Invoke(allowed, "demo.permission", nil); err != nil {
		t.Fatalf("permission allowed: %v", err)
	}
}

func TestActionHTTPRouteProjectsCanonicalActionWithoutAlias(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	if err := r.Register(action.Action{ID: "demo.route", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: action.Metadata{Capability: "demo", Access: action.AccessPublic, HTTP: &action.HTTPMeta{Method: "GET", Path: "/canonical"}}}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterHTTPRoute(action.HTTPRoute{ActionID: "demo.route", HTTP: &action.HTTPMeta{Method: "GET", Path: "/surface/route"}}); err != nil {
		t.Fatal(err)
	}
	if len(r.HTTPRoutes()) != 1 || len(r.HTTPAliases()) != 0 {
		t.Fatalf("route projection changed action identity: routes=%#v aliases=%#v", r.HTTPRoutes(), r.HTTPAliases())
	}
	if err := r.RegisterHTTPRoute(action.HTTPRoute{ActionID: "demo.route", HTTP: &action.HTTPMeta{Method: "GET", Path: "/surface/route"}}); err == nil {
		t.Fatal("duplicate projected route must fail")
	}
	if err := r.RegisterHTTPRoute(action.HTTPRoute{ActionID: "demo.route", HTTP: &action.HTTPMeta{Method: "GET", Path: "/canonical"}}); err == nil {
		t.Fatal("projected route must not conflict with canonical route")
	}
	if err := r.RegisterHTTPRoute(action.HTTPRoute{ActionID: "missing", HTTP: &action.HTTPMeta{Method: "GET", Path: "/missing"}}); err == nil {
		t.Fatal("unknown action route must fail")
	}
	handler := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	if err := r.Register(action.Action{ID: "demo.conflict", Handler: handler, Meta: action.Metadata{Capability: "demo", HTTP: &action.HTTPMeta{Method: "GET", Path: "/surface/route"}}}); err == nil {
		t.Fatal("canonical route registered after a projected route must fail")
	}
	if err := r.RegisterAlias(action.Alias{ID: "demo.alias", TargetID: "demo.route", HTTP: &action.HTTPMeta{Method: "GET", Path: "/surface/route"}}); err == nil {
		t.Fatal("alias route registered after a projected route must fail")
	}
}

func TestRegistryAliasInvokesCanonicalWithoutDiscoveryProjection(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	called := 0
	if err := r.Register(action.Action{
		ID:   "system.account_list",
		Meta: action.Metadata{Capability: "demo", Access: action.AccessPublic},
		Handler: func(context.Context, json.RawMessage) (any, error) {
			called++
			return "ok", nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterAlias(action.Alias{
		ID:       "system.user_list",
		TargetID: "system.account_list",
		HTTP:     &action.HTTPMeta{Method: "POST", Path: "/api/v1/system/users/list"},
	}); err != nil {
		t.Fatal(err)
	}

	ctx := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmTenant})
	if result, err := r.Invoke(ctx, "system.user_list", nil); err != nil || result != "ok" || called != 1 {
		t.Fatalf("alias invocation failed: result=%v called=%d err=%v", result, called, err)
	}
	if _, ok := r.Get("system.user_list"); ok {
		t.Fatal("alias must not be returned by canonical lookup")
	}
	items := r.List()
	if len(items) != 1 || items[0].ID != "system.account_list" {
		t.Fatalf("alias leaked into canonical list: %#v", items)
	}
	aliases := r.HTTPAliases()
	if len(aliases) != 1 || aliases[0].TargetID != "system.account_list" {
		t.Fatalf("expected explicit HTTP alias: %#v", aliases)
	}
	if err := r.Register(action.Action{ID: "system.user_list", Meta: action.Metadata{Capability: "demo"}, Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }}); err == nil {
		t.Fatal("canonical action must not overwrite an alias")
	}
}

func TestHTTPActionAliasUsesCanonicalAuditAndIdempotencyPipeline(t *testing.T) {
	auditor := &identityAuditRecorder{}
	executor := &identityIdempotencyExecutor{}
	registry := action.NewRegistry(nil, auditor)
	registry.SetIdempotencyExecutor(executor)
	called := 0
	if err := registry.Register(action.Action{
		ID: "system.account_list",
		Meta: action.Metadata{
			Capability: "demo", Access: action.AccessPublic,
			HTTP:        &action.HTTPMeta{Method: http.MethodPost, Path: "/api/v1/system/accounts/list"},
			Idempotency: &action.IdempotencyMeta{Mode: "optional"},
		},
		Handler: func(context.Context, json.RawMessage) (any, error) {
			called++
			return map[string]any{"ok": true}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAlias(action.Alias{ID: "system.user_list", TargetID: "system.account_list", HTTP: &action.HTTPMeta{Method: http.MethodPost, Path: "/api/v1/system/users/list"}}); err != nil {
		t.Fatal(err)
	}
	installed := true
	router := httpserver.New(httpserver.Options{Actions: registry, Installed: &installed})
	for _, path := range []string{"/api/v1/system/accounts/list", "/api/v1/system/users/list"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if response.Code != http.StatusOK {
			t.Fatalf("POST %s status = %d, body=%s", path, response.Code, response.Body.String())
		}
	}
	if called != 2 {
		t.Fatalf("canonical handler calls = %d, want 2", called)
	}
	if len(executor.actionIDs) != 2 || executor.actionIDs[0] != "system.account_list" || executor.actionIDs[1] != "system.account_list" {
		t.Fatalf("idempotency action IDs = %#v", executor.actionIDs)
	}
	if len(auditor.records) != 2 {
		t.Fatalf("audit records = %d, want 2", len(auditor.records))
	}
	for _, record := range auditor.records {
		if record.ActionID != "system.account_list" || record.Permission != "system.account_list" {
			t.Fatalf("alias leaked into audit contract: %#v", record)
		}
	}
}

type identityAuditRecorder struct{ records []audit.Record }

func (r *identityAuditRecorder) Write(_ context.Context, record audit.Record) error {
	r.records = append(r.records, record)
	return nil
}

type identityIdempotencyExecutor struct{ actionIDs []string }

func (e *identityIdempotencyExecutor) Execute(ctx context.Context, actionID string, input json.RawMessage, handler action.Handler, _ string, _ bool, _, _ time.Duration) (any, bool, error) {
	e.actionIDs = append(e.actionIDs, actionID)
	result, err := handler(ctx, input)
	return result, false, err
}

func TestRegistryRejectsInvalidAccessMetadata(t *testing.T) {
	handler := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	for _, meta := range []action.Metadata{
		{Capability: "demo", Access: "invalid"},
		{Capability: "demo", Access: action.AccessPublic, Permission: "demo.read"},
		{Capability: "demo", Access: action.AccessAuthenticated, Permission: "demo.read"},
	} {
		r := action.NewRegistry(nil, nil)
		if err := r.Register(action.Action{ID: "demo.run", Meta: meta, Handler: handler}); err == nil {
			t.Fatalf("expected invalid metadata rejection: %#v", meta)
		}
	}
}

func errorCode(err error) int {
	if appErr, ok := errs.AsAppError(err); ok {
		return appErr.Code
	}
	return 0
}

func TestRegistryRejectsInvalidSurface(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	err := r.Register(action.Action{ID: "demo.run", Meta: action.Metadata{Capability: "demo", Surface: "other"}, Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }})
	if err == nil {
		t.Fatal("expected invalid surface registration error")
	}
}

func TestAIClassificationIsMutuallyExclusiveAndFailClosed(t *testing.T) {
	exposed := &action.AIMeta{ExposeTool: true, ExclusionReason: action.AIExcludePendingSafetyReview}
	if err := action.ValidateAIClassification("demo.exposed", exposed); err == nil {
		t.Fatal("exposed action must not also declare an exclusion reason")
	}
	if err := action.ValidateAIClassification("demo.excluded", &action.AIMeta{}); err == nil {
		t.Fatal("excluded action without a reason must fail")
	}
	if err := action.ValidateAIClassification("demo.excluded", action.ExcludeFromAI(action.AIExcludePendingSafetyReview)); err != nil {
		t.Fatalf("valid exclusion rejected: %v", err)
	}
}

func TestRegistryRequiresExplicitAIClassificationOutsideDemo(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	handler := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	if err := r.Register(action.Action{ID: "foundation.profile_get", Handler: handler, Meta: action.Metadata{Capability: "foundation"}}); err == nil || !strings.Contains(err.Error(), "AI classification is required") {
		t.Fatalf("production Action without AI classification must fail: %v", err)
	}
	if err := r.Register(action.Action{ID: "foundation.profile_update", Handler: handler, Meta: action.Metadata{Capability: "foundation", AI: action.ExcludeFromAI(action.AIExcludePendingSafetyReview)}}); err != nil {
		t.Fatalf("explicit production exclusion must pass: %v", err)
	}
	if err := r.Register(action.Action{ID: "demo.unclassified", Handler: handler, Meta: action.Metadata{Capability: "demo"}}); err != nil {
		t.Fatalf("demo Action must retain the migration exemption: %v", err)
	}
}

func TestSideEffectingAIActionRequiresConfirmation(t *testing.T) {
	meta := &action.AIMeta{
		ExposeTool: true, Risk: "medium", SideEffect: "write",
		ToolTitle: "更新资源", Summary: "更新资源", HumanDescription: "更新指定资源", ResultSummary: "资源已更新",
		InputSchema: action.InputSchemaFor[struct{}](),
	}
	if err := action.ValidateAIClassification("demo.update", meta); err == nil {
		t.Fatal("side-effecting AI action without confirmation must fail")
	}
	meta.RequiresConfirmation = true
	if err := action.ValidateAIClassification("demo.update", meta); err != nil {
		t.Fatalf("confirmed side-effecting AI action rejected: %v", err)
	}
}

func TestExposedAIActionRequiresToolTitle(t *testing.T) {
	meta := &action.AIMeta{
		ExposeTool: true, Risk: "low", SideEffect: "read",
		Summary: "读取指定资源", HumanDescription: "读取当前用户有权访问的资源", ResultSummary: "资源已读取",
		InputSchema: action.InputSchemaFor[struct{}](),
	}
	if err := action.ValidateAIClassification("demo.read", meta); err == nil || !strings.Contains(err.Error(), "tool title") {
		t.Fatalf("missing tool title must fail with a useful error: %v", err)
	}
	meta.ToolTitle = "   "
	if err := action.ValidateAIClassification("demo.read", meta); err == nil {
		t.Fatal("blank tool title must fail")
	}
}
