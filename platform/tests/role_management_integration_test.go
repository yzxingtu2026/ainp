package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestRoleManagementActionsAndMenuAreRegistered(t *testing.T) {
	application := app.New(&config.Config{}, nil, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	for _, actionID := range []string{
		system.ActionRoleList, system.ActionRoleGet, system.ActionRoleCreate, system.ActionRoleUpdate,
		system.ActionRoleDelete, system.ActionRolePermissionsUpdate, system.ActionPermissionCatalog,
	} {
		registered, ok := application.Actions.Get(actionID)
		if !ok {
			t.Fatalf("action not registered: %s", actionID)
		}
		if registered.Meta.PermissionGroup != "system.role_management" {
			t.Fatalf("role action permission group = %q", registered.Meta.PermissionGroup)
		}
		if registered.Meta.AI == nil {
			t.Fatalf("role action %s must be explicitly classified for AI", actionID)
		}
		shouldExpose := actionID == system.ActionRoleList || actionID == system.ActionRoleGet || actionID == system.ActionPermissionCatalog
		if registered.Meta.AI.ExposeTool != shouldExpose {
			t.Fatalf("role action %s expose_tool = %v", actionID, registered.Meta.AI.ExposeTool)
		}
		if !shouldExpose && registered.Meta.AI.ExclusionReason == "" {
			t.Fatalf("role action %s must record an AI exclusion reason", actionID)
		}
	}
	for _, actionID := range []string{
		"system.auth_me", "system.auth_permissions",
	} {
		registered, ok := application.Actions.Get(actionID)
		if !ok || registered.Meta.AccessMode() != action.AccessAuthenticated {
			t.Fatalf("basic action %s must be authenticated", actionID)
		}
	}
	foundMenu := false
	foundHiddenProfile := false
	for _, registered := range application.Menus.List() {
		if registered.ID == system.MenuRoleManagement && registered.Permission == system.ActionRoleList {
			foundMenu = true
		}
		if (registered.ID == "system.profile" || registered.ID == "system.profile_refresh") && registered.Permission != "" {
			t.Fatalf("authenticated profile entry must not require permission: %#v", registered)
		}
		if registered.ID == "system.profile" {
			foundHiddenProfile = registered.HideInMenu
		}
	}
	if !foundMenu {
		t.Fatal("role management menu was not registered")
	}
	if !foundHiddenProfile {
		t.Fatal("profile route must remain registered but hidden from the sidebar")
	}
}

func TestRoleManagementTenantIsolationAndProtection(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_role_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}

	for _, row := range []tenant.Tenant{
		{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled},
		{ID: "tenant-b", Name: "B", Status: status.ActivationEnabled},
	} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	users := []model.User{
		{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "user-b", TenantID: "tenant-b", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled},
	}
	for i := range users {
		if err := testDB.Create(&users[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	admin := model.Role{ID: "admin-a", TenantID: "tenant-a", Code: system.BuiltinAdminCode, Name: "Admin", Status: status.ActivationEnabled, IsBuiltin: true}
	if err := testDB.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}

	actions := action.NewRegistry(permission.NewChecker(), nil)
	capabilities := capability.NewRegistry()
	if err := capabilities.Register(capability.Capability{ID: "system", Name: "System"}); err != nil {
		t.Fatal(err)
	}
	dummy := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	for _, definition := range []struct {
		id         string
		permission string
		access     action.AccessMode
	}{
		{id: "system.role_list"},
		{id: "system.role_update"},
		{id: "system.shared_a", permission: "system.shared"},
		{id: "system.shared_b", permission: "system.shared"},
		{id: "system.public", access: action.AccessPublic},
		{id: "system.authenticated", access: action.AccessAuthenticated},
	} {
		if err := actions.Register(action.Action{ID: definition.id, Handler: dummy, Meta: action.Metadata{
			Capability: "system", Permission: definition.permission, Name: definition.id, Access: definition.access,
			HTTP: &action.HTTPMeta{Method: "POST", Path: "/" + definition.id},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	authService := system.NewAuthService(testDB)
	service := system.NewRoleService(testDB, authService, actions, capabilities)
	ctxA := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true})
	ctxB := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-b", UserID: "user-b", Realm: auth.RealmTenant, Authenticated: true})

	createdA, err := service.Create(ctxA, json.RawMessage(`{"code":"operator","name":"Operator A","status":"enabled","permissions":["system.role_list","system.shared"]}`))
	if err != nil {
		t.Fatal(err)
	}
	roleA := createdA.(system.RoleView)
	createdB, err := service.Create(ctxB, json.RawMessage(`{"code":"operator","name":"Operator B","status":"enabled","permissions":["system.role_list"]}`))
	if err != nil {
		t.Fatal(err)
	}
	roleB := createdB.(system.RoleView)
	if roleA.ID == roleB.ID {
		t.Fatal("roles in different tenants must have different ids")
	}
	if _, err := service.Create(ctxA, json.RawMessage(`{"code":"operator","name":"Duplicate","status":"enabled"}`)); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("same-tenant duplicate role error = %v", err)
	}
	listed, err := service.List(ctxA, json.RawMessage(`{"page":1,"page_size":20,"keyword":"operator"}`))
	if err != nil {
		t.Fatal(err)
	}
	if listed.(pagination.Result[system.RoleView]).Total != 1 {
		t.Fatalf("tenant role list leaked data: %#v", listed)
	}

	if _, err := service.Get(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q}`, roleB.ID))); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("cross-tenant role get error = %v", err)
	}
	if _, err := service.Create(ctxA, json.RawMessage(`{"code":"invalid","name":"Invalid","status":"enabled","permissions":["unknown.permission"]}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("invalid permission error = %v", err)
	}
	if _, err := service.Create(ctxA, json.RawMessage(`{"code":"base-access","name":"Base Access","status":"enabled","permissions":["system.authenticated"]}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("authenticated action permission must be rejected: %v", err)
	}

	if err := testDB.Create(&model.UserRole{ID: "ur-a", TenantID: "tenant-a", UserID: "user-a", RoleID: roleA.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdatePermissions(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q,"permissions":["system.role_update"]}`, roleA.ID))); err != nil {
		t.Fatal(err)
	}
	var userA model.User
	if err := testDB.First(&userA, "id = ? AND tenant_id = ?", "user-a", "tenant-a").Error; err != nil {
		t.Fatal(err)
	}
	if userA.AuthVersion != 2 {
		t.Fatalf("auth_version = %d, want 2", userA.AuthVersion)
	}
	if _, err := service.Delete(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q}`, roleA.ID))); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("in-use role delete error = %v", err)
	}
	if _, err := service.Update(ctxA, json.RawMessage(`{"id":"admin-a","name":"Admin","status":"disabled"}`)); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("built-in role disable error = %v", err)
	}
	if _, err := service.Delete(ctxA, json.RawMessage(`{"id":"admin-a"}`)); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("built-in role delete error = %v", err)
	}

	catalogAny, err := service.PermissionCatalog(ctxA, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	catalog := catalogAny.(map[string]any)["groups"].([]system.PermissionCatalogGroup)
	if len(catalog) != 1 || len(catalog[0].Items) != 3 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	for _, item := range catalog[0].Items {
		if item.Group != "system.other" {
			t.Fatalf("fallback permission group = %q", item.Group)
		}
		if item.Permission == "system.public" {
			t.Fatal("public action must not be assignable")
		}
		if item.Permission == "system.authenticated" {
			t.Fatal("authenticated action must not be assignable")
		}
		if item.Permission == "system.shared" && len(item.ActionIDs) != 2 {
			t.Fatalf("shared permission was not deduplicated: %#v", item)
		}
	}
}

func appErrorCode(err error) int {
	if appErr, ok := errs.AsAppError(err); ok {
		return appErr.Code
	}
	return 0
}
