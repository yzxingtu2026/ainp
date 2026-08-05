package tests

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestDefaultConfirmationPresentationIsHumanReadableAndSafe(t *testing.T) {
	registry := aitool.NewRegistry()
	definition := toolFixture("action:demo.update", "更新记录", "更新一条记录")
	definition.InputSchema = json.RawMessage(`{
		"type":"object",
		"properties":{
			"display_name":{"type":"string","title":"显示名称"},
			"enabled":{"type":"boolean","title":"是否启用"},
			"id":{"type":"string","title":"记录 ID"},
			"password":{"type":"string","title":"密码","writeOnly":true},
			"role_ids":{"type":"array","title":"角色"}
		}
	}`)
	presentation := registry.PresentConfirmation(context.Background(), definition, json.RawMessage(`{
		"display_name":"测试用户",
		"enabled":true,
		"id":"7c15e0fe-8027-4565-8396-d04efe89e528",
		"password":"top-secret",
		"role_ids":["role-1"]
	}`))
	if presentation.Tool.Title != "更新记录" || presentation.Summary != "请求执行「更新记录」" {
		t.Fatalf("unexpected default presentation: %#v", presentation)
	}
	encoded, _ := json.Marshal(presentation)
	text := string(encoded)
	if !strings.Contains(text, "测试用户") || !strings.Contains(text, "是否启用") {
		t.Fatalf("safe scalar details missing: %s", text)
	}
	for _, leaked := range []string{"top-secret", "7c15e0fe", "role-1", "[complex value]", "password", "role_ids"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("unsafe argument leaked into presentation (%s): %s", leaked, text)
		}
	}
}

func TestConfirmationPresenterFailureFallsBackSafely(t *testing.T) {
	registry := aitool.NewRegistry()
	definition := toolFixture("action:demo.update", "更新记录", "更新一条记录")
	if err := registry.RegisterConfirmationPresenter(definition.ID, aitool.ConfirmationPresenterFunc(func(context.Context, aitool.ConfirmationPresentationRequest) (aitool.ConfirmationPresentation, error) {
		return aitool.ConfirmationPresentation{}, errors.New("lookup failed")
	})); err != nil {
		t.Fatal(err)
	}
	presentation := registry.PresentConfirmation(context.Background(), definition, json.RawMessage(`{"secret":"must-not-leak"}`))
	encoded, _ := json.Marshal(presentation)
	if presentation.Summary != "请求执行「更新记录」" || strings.Contains(string(encoded), "must-not-leak") {
		t.Fatalf("presenter failure did not use safe fallback: %s", encoded)
	}
}

func TestContentPartAcceptsToolPresentationSnapshot(t *testing.T) {
	raw := []byte(`{"type":"tool_call","tool_call_id":"call-1","tool_name":"action__demo_update","arguments":{},"presentation":{"title":"更新记录","description":"更新一条记录"}}`)
	var part aimodelruntime.ContentPart
	if err := json.Unmarshal(raw, &part); err != nil {
		t.Fatal(err)
	}
	if err := aimodelruntime.ValidateMessage(aimodelruntime.Message{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{part}}); err != nil || part.Presentation == nil || part.Presentation.Title != "更新记录" {
		t.Fatalf("tool presentation snapshot was not preserved: %#v %v", part.Presentation, err)
	}
}

func TestSystemAccountRolesConfirmationPresenterResolvesTenantNames(t *testing.T) {
	db := openOutboxTestDB(t)
	if err := db.AutoMigrate(&tenant.Tenant{}); err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	for _, row := range []tenant.Tenant{{ID: "t1", Name: "Tenant 1", Status: status.ActivationEnabled}, {ID: "t2", Name: "Tenant 2", Status: status.ActivationEnabled}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []model.TenantAccount{
		{ID: "user-t1", TenantID: "t1", Username: "xiaowang", DisplayName: "测试小王", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "user-t2", TenantID: "t2", Username: "other", DisplayName: "其他租户用户", PasswordHash: "x", Status: status.ActivationEnabled},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []model.Role{
		{ID: "role-admin", TenantID: "t1", Code: "admin", Name: "管理员", Status: status.ActivationEnabled},
		{ID: "role-audit", TenantID: "t1", Code: "audit", Name: "审计员", Status: status.ActivationEnabled},
	} {
		if err := db.Create(&role).Error; err != nil {
			t.Fatal(err)
		}
	}
	application := app.New(&config.Config{}, db, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	definitions, err := (&aitool.ActionProvider{Actions: application.Actions}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var definition aitool.Definition
	for _, candidate := range definitions {
		if candidate.RuntimeRef == system.ActionAccountRolesUpdate {
			definition = candidate
			break
		}
	}
	if definition.ID == "" {
		t.Fatal("account roles Action Tool definition missing")
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", PrincipalID: "operator", Realm: auth.RealmTenant, Permissions: []string{system.ActionAccountRolesUpdate}})
	presentation := application.ToolRegistry.PresentConfirmation(ctx, definition, json.RawMessage(`{"id":"user-t1","role_ids":["role-admin","role-audit"]}`))
	if presentation.Summary != "将账号「测试小王」的角色调整为「审计员、管理员」" || len(presentation.Details) != 2 {
		t.Fatalf("domain presentation did not resolve names: %#v", presentation)
	}
	crossTenant := application.ToolRegistry.PresentConfirmation(ctx, definition, json.RawMessage(`{"id":"user-t2","role_ids":["role-admin"]}`))
	encoded, _ := json.Marshal(crossTenant)
	if crossTenant.Summary != "请求执行「更新后台账号角色」" || strings.Contains(string(encoded), "其他租户用户") {
		t.Fatalf("cross-tenant lookup did not fail safely: %s", encoded)
	}
}
