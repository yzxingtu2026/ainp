package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
)

func TestUserManagementActionsAndMenuAreRegistered(t *testing.T) {
	application := app.New(&config.Config{}, nil, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	for _, actionID := range []string{
		system.ActionAccountList, system.ActionAccountGet, system.ActionAccountCreate, system.ActionAccountUpdate,
		system.ActionAccountStatusUpdate, system.ActionAccountRolesUpdate, system.ActionAccountPasswordReset,
	} {
		registered, ok := application.Actions.Get(actionID)
		if !ok {
			t.Fatalf("action not registered: %s", actionID)
		}
		if registered.Meta.PermissionGroup != "system.account_management" {
			t.Fatalf("user action permission group = %q", registered.Meta.PermissionGroup)
		}
		shouldExpose := actionID != system.ActionAccountCreate && actionID != system.ActionAccountPasswordReset
		if shouldExpose && (registered.Meta.AI == nil || !registered.Meta.AI.ExposeTool) {
			t.Fatalf("user action %s must expose a complete AI contract", actionID)
		}
		if !shouldExpose && registered.Meta.AI != nil && registered.Meta.AI.ExposeTool {
			t.Fatalf("user action %s must remain unavailable to AI", actionID)
		}
		if shouldExpose && actionID != system.ActionAccountList && actionID != system.ActionAccountGet && !registered.Meta.AI.RequiresConfirmation {
			t.Fatalf("user mutation %s must require confirmation", actionID)
		}
	}
	foundMenu := false
	for _, registered := range application.Menus.List() {
		if registered.ID == system.MenuAccountManagement && registered.Permission == system.ActionAccountList {
			foundMenu = true
		}
	}
	if !foundMenu {
		t.Fatal("user management menu was not registered")
	}
	for _, permission := range []string{
		system.ActionAccountList, system.ActionAccountGet, system.ActionAccountCreate, system.ActionAccountUpdate,
		system.ActionAccountStatusUpdate, system.ActionAccountRolesUpdate, system.ActionAccountPasswordReset,
	} {
		found := false
		for _, minimum := range system.AdminMinimumPermissions() {
			if minimum == permission {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("admin minimum permissions missing %s", permission)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := system.ValidatePassword("Admin@123"); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	for _, password := range []string{"short1", "allletters", "12345678", ""} {
		if appErrorCode(system.ValidatePassword(password)) != errs.CodeValidation {
			t.Fatalf("weak password %q should fail validation", password)
		}
	}
}

func TestUserManagementTenantIsolationAndProtection(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_user_" + uuid.NewString()[:8]
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

	adminRoleA := model.Role{ID: "admin-a", TenantID: "tenant-a", Code: system.BuiltinAdminCode, Name: "Admin", Status: status.ActivationEnabled, IsBuiltin: true}
	adminRoleB := model.Role{ID: "admin-b", TenantID: "tenant-b", Code: system.BuiltinAdminCode, Name: "Admin", Status: status.ActivationEnabled, IsBuiltin: true}
	operatorA := model.Role{ID: "operator-a", TenantID: "tenant-a", Code: "operator", Name: "Operator", Status: status.ActivationEnabled}
	disabledRole := model.Role{ID: "disabled-a", TenantID: "tenant-a", Code: "disabled_role", Name: "Disabled", Status: status.ActivationDisabled}
	for _, role := range []model.Role{adminRoleA, adminRoleB, operatorA, disabledRole} {
		if err := testDB.Create(&role).Error; err != nil {
			t.Fatal(err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("Admin@123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	seedUsers := []model.TenantAccount{
		{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: string(hash), DisplayName: "Admin A", Status: status.ActivationEnabled, AuthVersion: 1},
		{ID: "user-b", TenantID: "tenant-b", Username: "admin", PasswordHash: string(hash), DisplayName: "Admin B", Status: status.ActivationEnabled, AuthVersion: 1},
	}
	for i := range seedUsers {
		if err := testDB.Create(&seedUsers[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.Create(&model.AccountRole{ID: "ur-a", TenantID: "tenant-a", AccountID: "user-a", RoleID: "admin-a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&model.AccountRole{ID: "ur-b", TenantID: "tenant-b", AccountID: "user-b", RoleID: "admin-b"}).Error; err != nil {
		t.Fatal(err)
	}

	authService := system.NewAuthService(testDB)
	service := system.NewAccountService(testDB, authService)
	ctxA := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true})
	ctxB := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-b", PrincipalID: "user-b", Realm: auth.RealmTenant, Authenticated: true})

	createdA, err := service.Create(ctxA, json.RawMessage(`{"username":"alice","display_name":"Alice","password":"Alice@123","status":"enabled","role_ids":["operator-a"],"org_id":"org-a","dept_id":"dept-a"}`))
	if err != nil {
		t.Fatal(err)
	}
	userA := createdA.(system.AccountView)
	updatedA, err := service.Update(ctxA, json.RawMessage(`{"id":"`+userA.ID+`","display_name":"Alice Updated"}`))
	if err != nil {
		t.Fatal(err)
	}
	if view := updatedA.(system.AccountView); view.DisplayName != "Alice Updated" || view.OrgID != "org-a" || view.DeptID != "dept-a" {
		t.Fatalf("partial user update overwrote omitted fields: %#v", view)
	}
	clearedA, err := service.Update(ctxA, json.RawMessage(`{"id":"`+userA.ID+`","display_name":"Alice Updated","org_id":"","dept_id":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if view := clearedA.(system.AccountView); view.OrgID != "" || view.DeptID != "" {
		t.Fatalf("explicit user field clear was ignored: %#v", view)
	}
	if err := testDB.Model(&model.TenantAccount{}).Where("tenant_id=? AND id=?", "tenant-a", userA.ID).Updates(map[string]any{"org_id": "org-a", "dept_id": "dept-a"}).Error; err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	actionCtx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", PrincipalID: "user-a", SessionID: "session-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{system.ActionAccountUpdate}})
	minimalHTTPPayload := json.RawMessage(`{"id":"` + userA.ID + `","display_name":"HTTP Updated"}`)
	if _, err := application.Actions.Invoke(actionCtx, system.ActionAccountUpdate, minimalHTTPPayload); err != nil {
		t.Fatalf("minimal HTTP Action payload failed: %v", err)
	}
	provider := &aitool.ActionProvider{Actions: application.Actions}
	definitions, err := provider.List(actionCtx)
	if err != nil {
		t.Fatal(err)
	}
	var userUpdateTool aitool.Definition
	for _, definition := range definitions {
		if definition.RuntimeRef == system.ActionAccountUpdate {
			userUpdateTool = definition
			break
		}
	}
	if userUpdateTool.ID == "" {
		t.Fatal("system.account_update Action Tool was not projected")
	}
	minimalToolPayload := json.RawMessage(`{"id":"` + userA.ID + `","display_name":"Tool Updated"}`)
	if _, err := (&aitool.ActionExecutor{Actions: application.Actions}).Execute(actionCtx, aitool.LoadedTool{Definition: userUpdateTool}, minimalToolPayload); err != nil {
		t.Fatalf("minimal Tool payload failed: %v", err)
	}
	var persisted model.TenantAccount
	if err := testDB.Where("tenant_id=? AND id=?", "tenant-a", userA.ID).First(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.DisplayName != "Tool Updated" || persisted.OrgID != "org-a" || persisted.DeptID != "dept-a" {
		t.Fatalf("HTTP/Tool update semantics diverged: %#v", persisted)
	}
	createdB, err := service.Create(ctxB, json.RawMessage(`{"username":"alice","display_name":"Alice B","password":"Alice@123","status":"enabled","role_ids":["admin-b"]}`))
	if err != nil {
		t.Fatal(err)
	}
	userB := createdB.(system.AccountView)
	if userA.ID == userB.ID {
		t.Fatal("users in different tenants must have different ids")
	}
	if _, err := service.Create(ctxA, json.RawMessage(`{"username":"alice","display_name":"Dup","password":"Alice@123","status":"enabled"}`)); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("same-tenant duplicate username error = %v", err)
	}
	if _, err := service.Create(ctxA, json.RawMessage(`{"username":"bob","display_name":"Bob","password":"weak","status":"enabled"}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("weak password error = %v", err)
	}
	if _, err := service.Create(ctxA, json.RawMessage(`{"username":"carol","display_name":"Carol","password":"Carol@123","status":"enabled","role_ids":["admin-b"]}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("cross-tenant role injection error = %v", err)
	}
	if _, err := service.Create(ctxA, json.RawMessage(`{"username":"dave","display_name":"Dave","password":"Dave@123","status":"enabled","role_ids":["disabled-a"]}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("disabled role assignment error = %v", err)
	}

	listed, err := service.List(ctxA, json.RawMessage(`{"page":1,"page_size":20,"keyword":"alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	if listed.(pagination.Result[system.AccountView]).Total != 1 {
		t.Fatalf("tenant user list leaked data: %#v", listed)
	}
	filtered, err := service.List(ctxA, json.RawMessage(`{"page":1,"page_size":20,"role_id":"operator-a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if filtered.(pagination.Result[system.AccountView]).Total != 1 {
		t.Fatalf("role filter failed: %#v", filtered)
	}
	if _, err := service.Get(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q}`, userB.ID))); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("cross-tenant user get error = %v", err)
	}

	if _, err := service.UpdateStatus(ctxA, json.RawMessage(`{"id":"user-a","status":"disabled"}`)); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("self disable error = %v", err)
	}
	if _, err := service.UpdateRoles(ctxA, json.RawMessage(`{"id":"user-a","role_ids":["operator-a"]}`)); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("last admin role removal error = %v", err)
	}
	if _, err := service.UpdateStatus(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q,"status":"disabled"}`, userA.ID))); err != nil {
		t.Fatal(err)
	}
	var disabled model.TenantAccount
	if err := testDB.First(&disabled, "id = ?", userA.ID).Error; err != nil {
		t.Fatal(err)
	}
	if disabled.Status != status.ActivationDisabled || disabled.AuthVersion != 2 {
		t.Fatalf("disabled user state = %#v", disabled)
	}
	if _, err := authService.LoadAuthorization(ctxA, auth.AuthorizationRequest{
		TenantID: "tenant-a", PrincipalID: userA.ID, Realm: auth.RealmTenant, AuthVersion: 1,
	}); err != auth.ErrAuthorizationInvalid && err != auth.ErrAuthorizationStale {
		t.Fatalf("disabled user auth must fail, got %v", err)
	}

	secondAdmin, err := service.Create(ctxA, json.RawMessage(`{"username":"admin2","display_name":"Admin 2","password":"Admin2@12","status":"enabled","role_ids":["admin-a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	second := secondAdmin.(system.AccountView)
	if _, err := service.UpdateRoles(ctxA, json.RawMessage(`{"id":"user-a","role_ids":["operator-a"]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateRoles(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q,"role_ids":["operator-a"]}`, second.ID))); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("removing last remaining admin role error = %v", err)
	}

	if _, err := service.ResetPassword(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q,"password":"Reset@123"}`, second.ID))); err != nil {
		t.Fatal(err)
	}
	var resetUser model.TenantAccount
	if err := testDB.First(&resetUser, "id = ?", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resetUser.AuthVersion < 2 {
		t.Fatalf("password reset auth_version = %d", resetUser.AuthVersion)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(resetUser.PasswordHash), []byte("Reset@123")); err != nil {
		t.Fatal("password hash was not updated")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(resetUser.PasswordHash), []byte("Admin2@12")); err == nil {
		t.Fatal("old password must not remain valid")
	}

	service.SetOutboxWriter(failingOutboxWriter{})
	beforeHash := resetUser.PasswordHash
	beforeVersion := resetUser.AuthVersion
	if _, err := service.ResetPassword(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q,"password":"Rollback@123"}`, second.ID))); err == nil {
		t.Fatal("password reset should fail when the outbox append fails")
	}
	var rolledBack model.TenantAccount
	if err := testDB.First(&rolledBack, "id = ?", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rolledBack.PasswordHash != beforeHash || rolledBack.AuthVersion != beforeVersion {
		t.Fatalf("outbox failure did not roll back user mutation: version=%d", rolledBack.AuthVersion)
	}
}
