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

func TestSettingServiceTenantIsolationAndRevision(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_setting_" + uuid.NewString()[:8]
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
	for _, row := range []model.User{
		{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "user-b", TenantID: "tenant-b", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled},
	} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	application := app.New(&config.Config{}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	svc := system.NewSettingService(testDB, application.Settings)
	ctxA := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", UserID: "user-a", Realm: auth.RealmTenant, Authenticated: true,
		Permissions: []string{system.ActionSettingList, system.ActionSettingUpdate, system.ActionSettingReset, system.ActionSettingGet},
	})
	ctxB := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-b", UserID: "user-b", Realm: auth.RealmTenant, Authenticated: true,
		Permissions: []string{system.ActionSettingList, system.ActionSettingUpdate, system.ActionSettingReset, system.ActionSettingGet},
	})

	listed, err := svc.List(ctxA, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	categories := listed.(map[string]any)["categories"].([]system.SettingCategoryView)
	if len(categories) < 1 {
		t.Fatalf("unexpected list: %#v", listed)
	}
	var general *system.SettingCategoryView
	for i := range categories {
		if categories[i].Key == "general" {
			general = &categories[i]
			break
		}
	}
	if general == nil || len(general.Definitions) < 2 {
		t.Fatalf("unexpected general category: %#v", listed)
	}
	var def system.SettingDefinitionView
	for _, item := range general.Definitions {
		if item.Key == "general.preferences" {
			def = item
			break
		}
	}
	if def.Key == "" {
		t.Fatalf("preferences definition missing: %#v", listed)
	}
	if def.Source != "default" || def.Revision != 0 || def.Values["display_name"] != "AINP" {
		t.Fatalf("default effective values = %#v", def)
	}

	updated, err := svc.Update(ctxA, json.RawMessage(`{"key":"general.preferences","revision":0,"values":{"display_name":"Tenant A","page_size":30,"dark_mode":true,"language":"en-US","tags":["biz"],"brand_color":"#112233","notes":"hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	viewA := updated.(system.SettingDefinitionView)
	if viewA.Revision != 1 || viewA.Source != "tenant" || viewA.Values["display_name"] != "Tenant A" {
		t.Fatalf("updated view = %#v", viewA)
	}
	if _, err := svc.Update(ctxA, json.RawMessage(`{"key":"general.preferences","revision":0,"values":{"display_name":"stale","page_size":30,"dark_mode":true,"language":"en-US","tags":["biz"],"brand_color":"#112233","notes":""}}`)); appErrorCode(err) != errs.CodeConflict {
		t.Fatalf("stale revision error = %v", err)
	}

	_, err = svc.Update(ctxB, json.RawMessage(`{"key":"general.preferences","revision":0,"values":{"display_name":"Tenant B","page_size":10,"dark_mode":false,"language":"zh-CN","tags":["ops"],"brand_color":"#18A058","notes":""}}`))
	if err != nil {
		t.Fatal(err)
	}
	gotA, err := svc.Get(ctxA, json.RawMessage(`{"key":"general.preferences"}`))
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := svc.Get(ctxB, json.RawMessage(`{"key":"general.preferences"}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotA.(system.SettingDefinitionView).Values["display_name"] == gotB.(system.SettingDefinitionView).Values["display_name"] {
		t.Fatal("tenant settings leaked across tenants")
	}

	if _, err := svc.Update(ctxA, json.RawMessage(`{"key":"general.preferences","revision":1,"values":{"display_name":"Tenant A","page_size":200,"dark_mode":true,"language":"en-US","tags":["biz"],"brand_color":"#112233","notes":""}}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("out of range number error = %v", err)
	}
	if _, err := svc.Update(ctxA, json.RawMessage(`{"key":"general.preferences","revision":1,"values":{"display_name":"Tenant A","page_size":30,"dark_mode":true,"language":"fr-FR","tags":["biz"],"brand_color":"#112233","notes":""}}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("invalid select option error = %v", err)
	}

	prefs, err := svc.Preferences(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.DisplayName != "Tenant A" || prefs.PageSize != 30 {
		t.Fatalf("typed preferences = %#v", prefs)
	}

	reset, err := svc.Reset(ctxA, json.RawMessage(`{"key":"general.preferences","revision":1}`))
	if err != nil {
		t.Fatal(err)
	}
	resetView := reset.(system.SettingDefinitionView)
	if resetView.Source != "default" || resetView.Revision != 0 || resetView.Values["display_name"] != "AINP" {
		t.Fatalf("reset view = %#v", resetView)
	}
	var count int64
	if err := testDB.Model(&model.TenantSettingValue{}).Where("tenant_id = ? AND setting_key = ?", "tenant-a", "general.preferences").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("override row should be deleted, count=%d", count)
	}

	noTenant := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, UserID: "x"})
	if _, err := svc.List(noTenant, json.RawMessage(`{}`)); appErrorCode(err) != errs.CodeUnauthorized {
		t.Fatalf("missing tenant must fail closed: %v", err)
	}
}
