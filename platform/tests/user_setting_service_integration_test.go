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
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestUserSettingsAreSelfOnlyAndRevisioned(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_user_setting_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	fixtureDB := func() *gorm.DB {
		db, migrationErr := tenant.MigrationDB(testDB)
		if migrationErr != nil {
			t.Fatal(migrationErr)
		}
		return db
	}
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	if err := fixtureDB().Create(&tenant.Tenant{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range []model.TenantAccount{{ID: "user-a", TenantID: "tenant-a", Username: "alice", PasswordHash: "x", DisplayName: "Alice", Status: status.ActivationEnabled}, {ID: "user-b", TenantID: "tenant-a", Username: "bob", PasswordHash: "x", DisplayName: "Bob", Status: status.ActivationEnabled}} {
		if err := fixtureDB().Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []model.Attachment{
		{ID: "avatar-a", TenantID: "tenant-a", Provider: "local", ObjectKey: "tenant-a/avatar-a", OriginalName: "avatar.png", ContentType: "image/png", Size: 10, Status: status.AttachmentReady, UploaderID: "user-a"},
		{ID: "avatar-b", TenantID: "tenant-a", Provider: "local", ObjectKey: "tenant-a/avatar-b", OriginalName: "other.png", ContentType: "image/png", Size: 10, Status: status.AttachmentReady, UploaderID: "user-b"},
	} {
		if err := fixtureDB().Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := fixtureDB().Create(&model.AttachmentLink{ID: "avatar-link-a", TenantID: "tenant-a", AttachmentID: "avatar-a", ResourceType: system.AccountProfileAttachmentResourceType, ResourceID: "user-a", Field: system.AccountProfileAvatarField}).Error; err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewUserSettingService(testDB, application.Settings)
	ctx := func(user string) context.Context {
		return auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", PrincipalID: user, Realm: auth.RealmTenant, Authenticated: true})
	}

	defaults, err := svc.Get(ctx("user-a"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.(system.SettingDefinitionView).Revision != 0 || defaults.(system.SettingDefinitionView).Source != "default" {
		t.Fatalf("defaults=%#v", defaults)
	}
	payload := json.RawMessage(`{"revision":0,"values":{"preferred_address_override":"老板","timezone":"Asia/Shanghai","locale":"zh-CN","response_style":"concise","date_format":"YYYY-MM-DD","time_format":"24h","auto_memory_enabled":false}}`)
	updated, err := svc.Update(ctx("user-a"), payload)
	if err != nil {
		t.Fatal(err)
	}
	if updated.(system.SettingDefinitionView).Revision != 1 {
		t.Fatalf("updated=%#v", updated)
	}
	if _, err := svc.Update(ctx("user-a"), payload); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("stale revision=%v", err)
	}
	other, err := svc.Get(ctx("user-b"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if other.(system.SettingDefinitionView).Values["preferred_address_override"] != "" {
		t.Fatal("private preference leaked to another user")
	}
	badTZ := json.RawMessage(`{"revision":1,"values":{"preferred_address_override":"","timezone":"Mars/Base","locale":"zh-CN","response_style":"standard","date_format":"YYYY-MM-DD","time_format":"24h","auto_memory_enabled":true}}`)
	if _, err := svc.Update(ctx("user-a"), badTZ); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("invalid timezone=%v", err)
	}
	prefs, err := svc.Preferences(ctx("user-a"))
	if err != nil {
		t.Fatal(err)
	}
	if prefs.PreferredAddressOverride != "老板" || prefs.AutoMemoryEnabled {
		t.Fatalf("typed prefs=%#v", prefs)
	}

	userService := system.NewAccountService(testDB, system.NewAuthService(testDB))
	updatedProfile, err := userService.UpdateSelfProfile(ctx("user-a"), json.RawMessage(`{"display_name":"Alice Updated"}`))
	if err != nil {
		t.Fatal(err)
	}
	if updatedProfile.(system.AccountSelfProfileView).DisplayName != "Alice Updated" {
		t.Fatalf("updated self profile=%#v", updatedProfile)
	}
	if updatedProfile.(system.AccountSelfProfileView).AvatarAttachmentID != nil {
		t.Fatalf("legacy profile update unexpectedly changed avatar: %#v", updatedProfile)
	}
	updatedProfile, err = userService.UpdateSelfProfile(ctx("user-a"), json.RawMessage(`{"display_name":"Alice Updated","avatar_attachment_id":"avatar-a"}`))
	if err != nil {
		t.Fatal(err)
	}
	avatarID := updatedProfile.(system.AccountSelfProfileView).AvatarAttachmentID
	if avatarID == nil || *avatarID != "avatar-a" {
		t.Fatalf("avatar was not saved: %#v", updatedProfile)
	}
	if _, err := userService.UpdateSelfProfile(ctx("user-a"), json.RawMessage(`{"display_name":"Alice Updated","avatar_attachment_id":"avatar-b"}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("other user's avatar must be rejected: %v", err)
	}
	if err := application.AttachmentPolicies.Allow(ctx("user-a"), system.AccountProfileAttachmentResourceType, "tenant-a", "user-a", "user-a", system.AccountProfileAvatarField); err != nil {
		t.Fatalf("self avatar policy rejected owner: %v", err)
	}
	if err := application.AttachmentPolicies.Allow(ctx("user-a"), system.AccountProfileAttachmentResourceType, "tenant-a", "user-a", "user-b", system.AccountProfileAvatarField); err == nil {
		t.Fatal("self avatar policy allowed another user's resource")
	}
	var alice, bob model.TenantAccount
	if err := fixtureDB().Raw("SELECT * FROM tenant_accounts WHERE tenant_id = ? AND id = ?", "tenant-a", "user-a").Scan(&alice).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixtureDB().Raw("SELECT * FROM tenant_accounts WHERE tenant_id = ? AND id = ?", "tenant-a", "user-b").Scan(&bob).Error; err != nil {
		t.Fatal(err)
	}
	if alice.DisplayName != "Alice Updated" || bob.DisplayName != "Bob" {
		t.Fatalf("self update escaped owner boundary: alice=%q bob=%q", alice.DisplayName, bob.DisplayName)
	}
	currentHash, err := bcrypt.GenerateFromPassword([]byte("Current@123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixtureDB().Exec("UPDATE tenant_accounts SET password_hash = ? WHERE tenant_id = ? AND id = ?", string(currentHash), "tenant-a", "user-a").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := userService.UpdateSelfPassword(ctx("user-a"), json.RawMessage(`{"current_password":"wrong","new_password":"Next@1234"}`)); appErrorCode(err) != errs.CodeInvalidCredentials {
		t.Fatalf("wrong current password=%v", err)
	}
	if _, err := userService.UpdateSelfPassword(ctx("user-a"), json.RawMessage(`{"current_password":"Current@123","new_password":"weak"}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("weak new password=%v", err)
	}
	result, err := userService.UpdateSelfPassword(ctx("user-a"), json.RawMessage(`{"current_password":"Current@123","new_password":"Next@1234"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]any)["reauthentication_required"] != true {
		t.Fatalf("password update result=%#v", result)
	}
	if err := fixtureDB().Raw("SELECT * FROM tenant_accounts WHERE tenant_id = ? AND id = ?", "tenant-a", "user-a").Scan(&alice).Error; err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(alice.PasswordHash), []byte("Next@1234")) != nil {
		t.Fatal("new password was not persisted")
	}
	if alice.AuthVersion != 2 {
		t.Fatalf("auth_version=%d want 2", alice.AuthVersion)
	}
}
