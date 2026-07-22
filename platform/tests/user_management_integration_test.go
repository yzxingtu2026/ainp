package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
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
		system.ActionUserList, system.ActionUserGet, system.ActionUserCreate, system.ActionUserUpdate,
		system.ActionUserStatusUpdate, system.ActionUserRolesUpdate, system.ActionUserPasswordReset,
	} {
		registered, ok := application.Actions.Get(actionID)
		if !ok {
			t.Fatalf("action not registered: %s", actionID)
		}
		if registered.Meta.PermissionGroup != "system.user_management" {
			t.Fatalf("user action permission group = %q", registered.Meta.PermissionGroup)
		}
		shouldExpose := actionID != system.ActionUserCreate && actionID != system.ActionUserPasswordReset
		if shouldExpose && (registered.Meta.AI == nil || !registered.Meta.AI.ExposeTool) {
			t.Fatalf("user action %s must expose a complete AI contract", actionID)
		}
		if !shouldExpose && registered.Meta.AI != nil && registered.Meta.AI.ExposeTool {
			t.Fatalf("user action %s must remain unavailable to AI", actionID)
		}
		if shouldExpose && actionID != system.ActionUserList && actionID != system.ActionUserGet && !registered.Meta.AI.RequiresConfirmation {
			t.Fatalf("user mutation %s must require confirmation", actionID)
		}
	}
	foundMenu := false
	for _, registered := range application.Menus.List() {
		if registered.ID == system.MenuUserManagement && registered.Permission == system.ActionUserList {
			foundMenu = true
		}
	}
	if !foundMenu {
		t.Fatal("user management menu was not registered")
	}
	for _, permission := range []string{
		system.ActionUserList, system.ActionUserGet, system.ActionUserCreate, system.ActionUserUpdate,
		system.ActionUserStatusUpdate, system.ActionUserRolesUpdate, system.ActionUserPasswordReset,
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
	seedUsers := []model.User{
		{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: string(hash), DisplayName: "Admin A", Status: status.ActivationEnabled, AuthVersion: 1},
		{ID: "user-b", TenantID: "tenant-b", Username: "admin", PasswordHash: string(hash), DisplayName: "Admin B", Status: status.ActivationEnabled, AuthVersion: 1},
	}
	for i := range seedUsers {
		if err := testDB.Create(&seedUsers[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := testDB.Create(&model.UserRole{ID: "ur-a", TenantID: "tenant-a", UserID: "user-a", RoleID: "admin-a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&model.UserRole{ID: "ur-b", TenantID: "tenant-b", UserID: "user-b", RoleID: "admin-b"}).Error; err != nil {
		t.Fatal(err)
	}

	authService := system.NewAuthService(testDB)
	service := system.NewUserService(testDB, authService)
	ctxA := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true})
	ctxB := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-b", UserID: "user-b", Realm: auth.RealmTenant, Authenticated: true})

	createdA, err := service.Create(ctxA, json.RawMessage(`{"username":"alice","display_name":"Alice","password":"Alice@123","status":"enabled","role_ids":["operator-a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	userA := createdA.(system.UserView)
	createdB, err := service.Create(ctxB, json.RawMessage(`{"username":"alice","display_name":"Alice B","password":"Alice@123","status":"enabled","role_ids":["admin-b"]}`))
	if err != nil {
		t.Fatal(err)
	}
	userB := createdB.(system.UserView)
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
	if listed.(pagination.Result[system.UserView]).Total != 1 {
		t.Fatalf("tenant user list leaked data: %#v", listed)
	}
	filtered, err := service.List(ctxA, json.RawMessage(`{"page":1,"page_size":20,"role_id":"operator-a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if filtered.(pagination.Result[system.UserView]).Total != 1 {
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
	var disabled model.User
	if err := testDB.First(&disabled, "id = ?", userA.ID).Error; err != nil {
		t.Fatal(err)
	}
	if disabled.Status != status.ActivationDisabled || disabled.AuthVersion != 2 {
		t.Fatalf("disabled user state = %#v", disabled)
	}
	if _, err := authService.LoadAuthorization(ctxA, auth.AuthorizationRequest{
		TenantID: "tenant-a", UserID: userA.ID, Realm: auth.RealmTenant, AuthVersion: 1,
	}); err != auth.ErrAuthorizationInvalid && err != auth.ErrAuthorizationStale {
		t.Fatalf("disabled user auth must fail, got %v", err)
	}

	secondAdmin, err := service.Create(ctxA, json.RawMessage(`{"username":"admin2","display_name":"Admin 2","password":"Admin2@12","status":"enabled","role_ids":["admin-a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	second := secondAdmin.(system.UserView)
	if _, err := service.UpdateRoles(ctxA, json.RawMessage(`{"id":"user-a","role_ids":["operator-a"]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateRoles(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q,"role_ids":["operator-a"]}`, second.ID))); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("removing last remaining admin role error = %v", err)
	}

	if _, err := service.ResetPassword(ctxA, json.RawMessage(fmt.Sprintf(`{"id":%q,"password":"Reset@123"}`, second.ID))); err != nil {
		t.Fatal(err)
	}
	var resetUser model.User
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
	var rolledBack model.User
	if err := testDB.First(&rolledBack, "id = ?", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rolledBack.PasswordHash != beforeHash || rolledBack.AuthVersion != beforeVersion {
		t.Fatalf("outbox failure did not roll back user mutation: version=%d", rolledBack.AuthVersion)
	}
}
