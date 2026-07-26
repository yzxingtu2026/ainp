package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/demo"
	"github.com/yzxingtu2026/ainp/platform/modules/operations"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestPermissionCatalogUsesCompactToolDiscoveryAndRichHTTPProjection(t *testing.T) {
	actions := action.NewRegistry(permission.NewChecker(), nil)
	capabilities := capability.NewRegistry()
	for _, definition := range []capability.Capability{
		{ID: "operations", Name: "运维", Description: "任务、事件和审计"},
		{ID: "system", Name: "系统", Description: "系统管理"},
	} {
		if err := capabilities.Register(definition); err != nil {
			t.Fatal(err)
		}
	}
	dummy := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	register := func(id, capabilityID, group, name string) {
		t.Helper()
		if err := actions.Register(action.Action{ID: id, Handler: dummy, Meta: action.Metadata{
			Capability: capabilityID, PermissionGroup: group, PermissionGroupName: group,
			Name: name, Description: name + " description", HTTP: &action.HTTPMeta{Method: "POST", Path: "/" + id},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 151; i++ {
		group := "operations.jobs"
		if i >= 76 {
			group = "operations.audit"
		}
		register(fmt.Sprintf("operations.permission_%03d", i), "operations", group, fmt.Sprintf("运维权限 %03d", i))
	}
	register("system.role_list", "system", "system.roles", "角色列表")
	if err := actions.Register(action.Action{ID: "system.authenticated", Handler: dummy, Meta: action.Metadata{
		Capability: "system", Access: action.AccessAuthenticated, Name: "Authenticated", HTTP: &action.HTTPMeta{Method: "POST", Path: "/system.authenticated"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := actions.RegisterReferenceResolver("system.assignable_permissions", action.ReferenceResolverFunc(func(context.Context) (action.ReferenceSnapshot, error) {
		return action.ReferenceSnapshot{Values: system.AssignablePermissions(actions)}, nil
	})); err != nil {
		t.Fatal(err)
	}
	service := system.NewRoleService(nil, nil, actions, capabilities)
	httpCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "t1", UserID: "u1", Language: "zh-CN"})
	toolCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "t1", UserID: "u1", Language: "zh-CN", ToolInvocationID: "inv-1"})

	richValue, err := service.PermissionCatalog(httpCtx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	rich := richValue.(map[string]any)
	richGroups := rich["groups"].([]system.PermissionCatalogGroup)
	if rich["has_more"].(bool) || rich["total"].(int) != 152 || rich["returned_count"].(int) != 152 {
		t.Fatalf("unexpected rich metadata: %#v", rich)
	}
	if len(richGroups) != 2 || len(richGroups[0].Items) == 0 || len(richGroups[0].Items[0].ActionIDs) == 0 {
		t.Fatalf("HTTP rich projection lost catalog detail: %#v", richGroups)
	}

	searchValue, err := service.PermissionCatalog(toolCtx, json.RawMessage(`{"query":"运维"}`))
	if err != nil {
		t.Fatal(err)
	}
	search := searchValue.(map[string]any)
	if !search["has_more"].(bool) || search["total"].(int) != 151 || search["returned_count"].(int) != 150 {
		t.Fatalf("keyword search did not return a bounded operations catalog: %#v", search)
	}
	if got := search["available_capabilities"].([]string); !slices.Equal(got, []string{"operations"}) {
		t.Fatalf("available capabilities = %#v", got)
	}
	if !strings.Contains(search["hint"].(string), "available_capabilities") {
		t.Fatalf("capability narrowing hint missing: %#v", search["hint"])
	}
	if encoded, marshalErr := json.Marshal(search["groups"]); marshalErr != nil || len(encoded) >= 32<<10 {
		t.Fatalf("compact groups exceed effective tool budget: bytes=%d err=%v", len(encoded), marshalErr)
	}

	capabilityValue, err := service.PermissionCatalog(toolCtx, json.RawMessage(`{"capabilities":["operations"]}`))
	if err != nil {
		t.Fatal(err)
	}
	capabilityResult := capabilityValue.(map[string]any)
	if got := capabilityResult["available_groups"].([]string); !slices.Equal(got, []string{"operations.audit", "operations.jobs"}) {
		t.Fatalf("available groups = %#v", got)
	}
	if !strings.Contains(capabilityResult["hint"].(string), "available_groups") {
		t.Fatalf("group narrowing hint missing: %#v", capabilityResult["hint"])
	}
	if capabilityResult["catalog_version"] != search["catalog_version"] {
		t.Fatal("catalog version changed across projections of the same resolver snapshot")
	}

	groupValue, err := service.PermissionCatalog(toolCtx, json.RawMessage(`{"groups":["operations.jobs"]}`))
	if err != nil {
		t.Fatal(err)
	}
	groupResult := groupValue.(map[string]any)
	if groupResult["has_more"].(bool) || groupResult["total"].(int) != 76 || groupResult["returned_count"].(int) != 76 {
		t.Fatalf("group narrowing is incomplete: %#v", groupResult)
	}
	compactGroups := groupResult["groups"].([]system.PermissionCatalogGroup)
	for _, group := range compactGroups {
		for _, item := range group.Items {
			if len(item.ActionIDs) != 0 || item.GroupName != "" || item.Description != "" || item.NameKey != "" || item.Audit {
				t.Fatalf("compact item leaked rich fields: %#v", item)
			}
		}
	}
	if _, err := service.PermissionCatalog(toolCtx, json.RawMessage(`{"view":"rich"}`)); err == nil {
		t.Fatal("permission catalog must not expose a model-controlled view switch")
	}

	discovered := []string{}
	for _, group := range []string{"operations.audit", "operations.jobs", "system.roles"} {
		value, groupErr := service.PermissionCatalog(toolCtx, json.RawMessage(fmt.Sprintf(`{"groups":[%q]}`, group)))
		if groupErr != nil {
			t.Fatal(groupErr)
		}
		for _, capabilityGroup := range value.(map[string]any)["groups"].([]system.PermissionCatalogGroup) {
			for _, item := range capabilityGroup.Items {
				discovered = append(discovered, item.Permission)
			}
		}
	}
	slices.Sort(discovered)
	want := system.AssignablePermissions(actions)
	if !slices.Equal(discovered, want) {
		t.Fatalf("group narrowing union differs from resolver snapshot: got=%d want=%d", len(discovered), len(want))
	}

	previousVersion := search["catalog_version"].(string)
	register("system.role_get", "system", "system.roles", "角色详情")
	changedValue, err := service.PermissionCatalog(toolCtx, json.RawMessage(`{"groups":["system.roles"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if changedValue.(map[string]any)["catalog_version"] == previousVersion {
		t.Fatal("catalog version did not change after assignable Action registration")
	}
}

func TestRegisteredPermissionGroupsStayWithinCompactDiscoveryLimit(t *testing.T) {
	application := app.New(&config.Config{
		Auth: config.AuthConfig{JWT: config.JWTConfig{Secret: "permission-catalog-test-secret"}},
		AI: config.AIConfig{
			CredentialEncryptionKey: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		},
	}, openOutboxTestDB(t), nil, nil)
	var err error
	application.Outbox, err = outbox.NewRuntime(application.DB, config.OutboxConfig{})
	if err != nil {
		t.Fatal(err)
	}
	application.Jobs, err = job.NewRuntime(application.DB, job.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ai.AutoMigrate(application.DB); err != nil {
		t.Fatal(err)
	}
	if err := prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	if err := operations.Register(application, nil); err != nil {
		t.Fatal(err)
	}
	if err := ai.Register(application); err != nil {
		t.Fatal(err)
	}
	if err := demo.Register(application); err != nil {
		t.Fatal(err)
	}
	catalogAction, ok := application.Actions.Get(system.ActionPermissionCatalog)
	if !ok {
		t.Fatal("permission catalog Action is not registered")
	}
	inputSchema := string(catalogAction.Meta.AI.InputSchema)
	for _, field := range []string{`"query"`, `"capabilities"`, `"groups"`} {
		if !strings.Contains(inputSchema, field) {
			t.Fatalf("permission catalog input schema missing %s: %s", field, inputSchema)
		}
	}
	if strings.Contains(inputSchema, `"view"`) {
		t.Fatalf("permission catalog input schema exposes view switch: %s", inputSchema)
	}
	toolCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "t1", UserID: "u1", Language: "zh-CN", ToolInvocationID: "inv-current"})
	currentValue, err := catalogAction.Handler(toolCtx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	current := currentValue.(map[string]any)
	if encoded, marshalErr := json.Marshal(current["groups"]); marshalErr != nil || len(encoded) >= 32<<10 {
		t.Fatalf("current compact catalog exceeds effective tool budget: bytes=%d err=%v", len(encoded), marshalErr)
	}
	operationsValue, err := catalogAction.Handler(toolCtx, json.RawMessage(`{"query":"运维"}`))
	if err != nil {
		t.Fatal(err)
	}
	operationsGroups := operationsValue.(map[string]any)["groups"].([]system.PermissionCatalogGroup)
	if len(operationsGroups) == 0 {
		t.Fatal("localized capability keyword did not discover operations permissions")
	}
	for _, group := range operationsGroups {
		if group.Capability != "operations" {
			t.Fatalf("operations keyword leaked unrelated capability %s", group.Capability)
		}
	}
	assignable := map[string]bool{}
	for _, value := range system.AssignablePermissions(application.Actions) {
		assignable[value] = true
	}
	groups := map[string]map[string]bool{}
	permissionScopes := map[string]string{}
	for _, registered := range application.Actions.List() {
		permissionCode := registered.Meta.PermissionCode(registered.ID)
		if !assignable[permissionCode] {
			continue
		}
		group := registered.Meta.PermissionGroup
		if group == "" {
			group = registered.Meta.Capability + ".other"
		}
		scope := registered.Meta.Capability + "/" + group
		if previous := permissionScopes[permissionCode]; previous != "" && previous != scope {
			t.Fatalf("permission %s crosses catalog scopes %s and %s", permissionCode, previous, scope)
		}
		permissionScopes[permissionCode] = scope
		if groups[group] == nil {
			groups[group] = map[string]bool{}
		}
		groups[group][permissionCode] = true
	}
	for group, permissions := range groups {
		if len(permissions) > 150 {
			t.Fatalf("permission group %s has %d permissions and no further narrowing level", group, len(permissions))
		}
	}
}
