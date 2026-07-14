package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestUserTenantUsernameMigration(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_test_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}

	// Reproduce the pre-migration global unique index.
	if err := testDB.Exec(`CREATE TABLE users (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, username varchar(64) NOT NULL, password_hash varchar(255) NOT NULL, status varchar(32) NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`CREATE UNIQUE INDEX idx_users_username ON users (username)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`INSERT INTO users (id, tenant_id, username, password_hash, status) VALUES ('legacy', 'legacy-tenant', 'legacy', 'x', 'enabled')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	if testDB.Migrator().HasIndex(&model.User{}, "idx_users_username") {
		t.Fatal("legacy global username index still exists")
	}
	var legacy model.User
	if err := testDB.First(&legacy, "id = ?", "legacy").Error; err != nil {
		t.Fatal(err)
	}
	if legacy.AuthVersion != 1 {
		t.Fatalf("legacy user auth_version = %d, want 1", legacy.AuthVersion)
	}

	users := []model.User{
		{ID: "u1", TenantID: "t1", Username: "same", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "u2", TenantID: "t2", Username: "same", PasswordHash: "x", Status: status.ActivationEnabled},
	}
	for i := range users {
		if err := testDB.Create(&users[i]).Error; err != nil {
			t.Fatalf("cross-tenant duplicate username must be allowed: %v", err)
		}
	}
	duplicate := model.User{ID: "u3", TenantID: "t1", Username: "same", PasswordHash: "x", Status: status.ActivationEnabled}
	if err := testDB.Create(&duplicate).Error; err == nil {
		t.Fatal("same-tenant duplicate username must be rejected")
	}
	invalid := model.User{ID: "u4", TenantID: "t3", Username: "invalid", PasswordHash: "x", Status: status.ActivationStatus("pending")}
	if err := testDB.Create(&invalid).Error; err == nil {
		t.Fatal("database CHECK must reject invalid activation status")
	}
	if err := testDB.Exec(`INSERT INTO users (id, tenant_id, username, password_hash, status, auth_version) VALUES ('u5', 't3', 'invalid-version', 'x', 'enabled', 0)`).Error; err == nil {
		t.Fatal("database CHECK must reject invalid auth_version")
	}

	if err := testDB.Create(&tenant.Tenant{ID: "t-bump", Name: "bump", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	bumpUser := model.User{ID: "u-bump", TenantID: "t-bump", Username: "bump", PasswordHash: "x", Status: status.ActivationEnabled}
	if err := testDB.Create(&bumpUser).Error; err != nil {
		t.Fatal(err)
	}
	role := model.Role{ID: "r-bump", TenantID: "t-bump", Code: "operator", Name: "Operator", Status: status.ActivationEnabled}
	if err := testDB.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&model.UserRole{ID: "ur-bump", TenantID: "t-bump", UserID: "u-bump", RoleID: "r-bump"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&model.RolePermission{ID: "rp-bump", RoleID: "r-bump", Permission: "demo.read"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := system.NewAuthService(testDB)
	snapshot, err := svc.LoadAuthorization(context.Background(), auth.AuthorizationRequest{
		TenantID: "t-bump", UserID: "u-bump", Realm: auth.RealmTenant, AuthVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Roles) != 1 || snapshot.Roles[0] != "operator" || len(snapshot.Permissions) != 1 || snapshot.Permissions[0] != "demo.read" {
		t.Fatalf("unexpected authorization snapshot: %#v", snapshot)
	}
	if _, err := svc.LoadAuthorization(context.Background(), auth.AuthorizationRequest{
		TenantID: "legacy-tenant", UserID: "u-bump", Realm: auth.RealmTenant, AuthVersion: 1,
	}); !errors.Is(err, auth.ErrAuthorizationInvalid) {
		t.Fatalf("cross-tenant authorization error = %v", err)
	}
	if _, err := svc.LoadAuthorization(context.Background(), auth.AuthorizationRequest{
		TenantID: "t-bump", UserID: "u-bump", Realm: auth.RealmPlatform, AuthVersion: 1,
	}); !errors.Is(err, auth.ErrAuthorizationInvalid) {
		t.Fatalf("wrong realm authorization error = %v", err)
	}
	if err := svc.BumpAuthVersion(context.Background(), testDB, "t-bump", "u-bump"); err != nil {
		t.Fatal(err)
	}
	if err := testDB.First(&bumpUser, "id = ? AND tenant_id = ?", "u-bump", "t-bump").Error; err != nil {
		t.Fatal(err)
	}
	if bumpUser.AuthVersion != 2 {
		t.Fatalf("auth_version after bump = %d, want 2", bumpUser.AuthVersion)
	}
	if _, err := svc.LoadAuthorization(context.Background(), auth.AuthorizationRequest{
		TenantID: "t-bump", UserID: "u-bump", Realm: auth.RealmTenant, AuthVersion: 1,
	}); !errors.Is(err, auth.ErrAuthorizationStale) {
		t.Fatalf("stale authorization error = %v", err)
	}
	if err := svc.BumpAuthVersionForRole(context.Background(), testDB, "t-bump", "r-bump"); err != nil {
		t.Fatal(err)
	}
	if err := testDB.First(&bumpUser, "id = ? AND tenant_id = ?", "u-bump", "t-bump").Error; err != nil {
		t.Fatal(err)
	}
	if bumpUser.AuthVersion != 3 {
		t.Fatalf("auth_version after role bump = %d, want 3", bumpUser.AuthVersion)
	}
	if err := testDB.Model(&model.User{}).Where("id = ? AND tenant_id = ?", "u-bump", "t-bump").Update("status", status.ActivationDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LoadAuthorization(context.Background(), auth.AuthorizationRequest{
		TenantID: "t-bump", UserID: "u-bump", Realm: auth.RealmTenant, AuthVersion: 3,
	}); !errors.Is(err, auth.ErrAuthorizationInvalid) {
		t.Fatalf("disabled user authorization error = %v", err)
	}
	if err := testDB.Model(&model.User{}).Where("id = ? AND tenant_id = ?", "u-bump", "t-bump").Update("status", status.ActivationEnabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Model(&tenant.Tenant{}).Where("id = ?", "t-bump").Update("status", status.ActivationDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LoadAuthorization(context.Background(), auth.AuthorizationRequest{
		TenantID: "t-bump", UserID: "u-bump", Realm: auth.RealmTenant, AuthVersion: 3,
	}); !errors.Is(err, auth.ErrAuthorizationInvalid) {
		t.Fatalf("disabled tenant authorization error = %v", err)
	}
}
