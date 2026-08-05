package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestRuntimeContextProviderPrecedenceAndRunSnapshotReuse(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_runtime_context_" + uuid.NewString()[:8]
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
	if err := testDB.Create(&model.TenantAccount{ID: "user-a", TenantID: "tenant-a", Username: "alice", DisplayName: "Alice", PasswordHash: "x", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`CREATE TABLE ai_memory_current_profiles (tenant_id text NOT NULL,user_id text NOT NULL,agent_id text,status text,source_revision bigint,profile_json jsonb NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`INSERT INTO ai_memory_current_profiles VALUES ('tenant-a','user-a',NULL,'ready',1,'{"preferred_address":"同学"}')`).Error; err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	prefs := system.NewUserSettingService(testDB, application.Settings)
	tenantSettings := system.NewSettingService(testDB, application.Settings)
	clockNow := time.Date(2026, 7, 19, 15, 59, 0, 0, time.UTC)
	provider := system.NewRuntimeContextProvider(testDB, prefs, tenantSettings.Provider(), runtimecontext.ClockFunc(func() time.Time { return clockNow }))
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{system.ActionSettingList, system.ActionSettingUpdate}})
	if _, err := tenantSettings.Update(ctx, json.RawMessage(`{"key":"general.runtime_defaults","revision":0,"values":{"timezone":"Asia/Shanghai","locale":"zh-CN"}}`)); err != nil {
		t.Fatal(err)
	}
	req := runtimecontext.BuildRequest{TenantID: "tenant-a", PrincipalID: "user-a", ConversationID: "c1", ConversationCreatedAt: clockNow, AgentID: "a1", AgentVersionID: "v1", RunID: "r1"}
	first, err := provider.Build(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.PreferredAddress != "同学" || first.PreferredAddressSource != "memory_profile" || first.Time.Timezone != "Asia/Shanghai" || first.Time.TimezoneSource != "tenant_default" || first.Time.CurrentLocalDateTime != "2026-07-19T23:59:00+08:00" {
		t.Fatalf("default context=%#v", first)
	}
	_, err = prefs.Update(ctx, json.RawMessage(`{"revision":0,"values":{"preferred_address_override":"老板","timezone":"America/New_York","locale":"en-US","response_style":"standard","date_format":"YYYY-MM-DD","time_format":"24h","auto_memory_enabled":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Build(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if second.PreferredAddress != "老板" || second.PreferredAddressSource != "user_preference" || second.Time.Timezone != "America/New_York" || second.Time.CurrentLocalDate != "2026-07-19" {
		t.Fatalf("user context=%#v", second)
	}
	clockNow = clockNow.Add(12 * time.Hour)
	req.TimeSnapshot = &second.Time
	restored, err := provider.Build(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Time.EvaluatedAtUTC.Equal(second.Time.EvaluatedAtUTC) || restored.Time.CurrentLocalDate != second.Time.CurrentLocalDate {
		t.Fatalf("run snapshot changed: before=%#v after=%#v", second.Time, restored.Time)
	}
}
