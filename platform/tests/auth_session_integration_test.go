package tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestSessionRefreshRotationReplayAndTenantScope(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_auth_" + uuid.NewString()[:8]
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
	for _, row := range []tenant.Tenant{{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}, {ID: "tenant-b", Name: "B", Status: status.ActivationEnabled}} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	user := model.User{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled, AuthVersion: 1}
	if err := testDB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	authService := system.NewAuthService(testDB)
	sessions := system.NewSessionService(testDB, tokens, authService)
	created, err := sessions.Create(context.Background(), &user, "web", "test")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.Parse(created.AccessToken)
	if err != nil || claims.SessionID != created.SessionID {
		t.Fatalf("claims/session mismatch: %#v %v", claims, err)
	}
	rotated, err := sessions.Refresh(context.Background(), created.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == created.RefreshToken {
		t.Fatal("refresh token must rotate")
	}
	if _, err := sessions.Refresh(context.Background(), created.RefreshToken); err == nil {
		t.Fatal("replayed refresh must fail")
	}
	if _, err := sessions.Refresh(context.Background(), rotated.RefreshToken); err == nil {
		t.Fatal("replay must revoke the session family")
	}
	if _, err := authService.LoadAuthorization(context.Background(), auth.AuthorizationRequest{TenantID: "tenant-b", UserID: user.ID, SessionID: created.SessionID, Realm: auth.RealmTenant, AuthVersion: 1}); err == nil {
		t.Fatal("cross-tenant session must fail")
	}
}
