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
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&tenant.Tenant{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range []model.User{{ID: "user-a", TenantID: "tenant-a", Username: "alice", PasswordHash: "x", DisplayName: "Alice", Status: status.ActivationEnabled}, {ID: "user-b", TenantID: "tenant-a", Username: "bob", PasswordHash: "x", DisplayName: "Bob", Status: status.ActivationEnabled}} {
		if err := testDB.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewUserSettingService(testDB, application.Settings)
	ctx := func(user string) context.Context {
		return auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", UserID: user, Realm: auth.RealmTenant, Authenticated: true})
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

	userService := system.NewUserService(testDB, nil)
	updatedProfile, err := userService.UpdateSelfProfile(ctx("user-a"), json.RawMessage(`{"display_name":"Alice Updated"}`))
	if err != nil {
		t.Fatal(err)
	}
	if updatedProfile.(system.SelfProfileView).DisplayName != "Alice Updated" {
		t.Fatalf("updated self profile=%#v", updatedProfile)
	}
	var alice, bob model.User
	if err := testDB.Where("tenant_id = ? AND id = ?", "tenant-a", "user-a").First(&alice).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Where("tenant_id = ? AND id = ?", "tenant-a", "user-b").First(&bob).Error; err != nil {
		t.Fatal(err)
	}
	if alice.DisplayName != "Alice Updated" || bob.DisplayName != "Bob" {
		t.Fatalf("self update escaped owner boundary: alice=%q bob=%q", alice.DisplayName, bob.DisplayName)
	}
}
