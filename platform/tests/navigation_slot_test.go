package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	navigation "github.com/yzxingtu2026/ainp/platform/modules/ai/experience/navigation"
)

func TestSlotProjectsAndSearchesFromTrustedAuthContext(t *testing.T) {
	registry := menu.NewRegistry()
	for _, item := range []menu.Menu{
		{ID: "system.root", Title: "系统", Type: menu.TypeDirectory},
		{ID: "system.accounts", ParentID: "system.root", Title: "用户管理", Type: menu.TypeMenu, Path: "/system/accounts", Permission: "user.list"},
		{ID: "system.roles", ParentID: "system.root", Title: "角色管理", Type: menu.TypeMenu, Path: "/system/roles", Permission: "role.list"},
	} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	slot := navigation.New(registry)
	ac := &auth.Context{TenantID: "tenant-1", PrincipalID: "user-1", SessionID: "session-1", Realm: auth.RealmTenant, AuthVersion: 4, Permissions: []string{"user.list"}, Authenticated: true}
	ctx := auth.WithContext(context.Background(), ac)
	scope := aitool.RuntimeScope{TenantID: ac.TenantID, PrincipalID: ac.PrincipalID, SessionID: ac.SessionID, Realm: string(ac.Realm), AuthVersion: ac.AuthVersion}
	if _, visible, err := slot.Project(ctx, scope, slot.Definition()); err != nil || !visible {
		t.Fatalf("projection failed: visible=%v err=%v", visible, err)
	}

	result, err := slot.Execute(ctx, aitool.LoadedTool{}, json.RawMessage(`{"query":"管理","limit":3}`))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["returned_count"] != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	items, ok := data["items"].([]menu.SearchResult)
	if !ok || len(items) != 1 || items[0].MenuID != "system.accounts" || items[0].Path != "/system/accounts" {
		t.Fatalf("unauthorized result projection: %#v", data)
	}

	mismatched := scope
	mismatched.PrincipalID = "other"
	if _, visible, err := slot.Project(ctx, mismatched, slot.Definition()); err != nil || visible {
		t.Fatalf("mismatched scope must be hidden: visible=%v err=%v", visible, err)
	}
}

func TestSlotRejectsModelSuppliedInvalidBudget(t *testing.T) {
	slot := navigation.New(menu.NewRegistry())
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-1", PrincipalID: "user-1", Realm: auth.RealmTenant, Authenticated: true})
	result, err := slot.Execute(ctx, aitool.LoadedTool{}, json.RawMessage(`{"query":"users","limit":4}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.ErrorCategory != "invalid_request" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSlotDefaultsToThreeNavigationCandidates(t *testing.T) {
	registry := menu.NewRegistry()
	if err := registry.Register(menu.Menu{ID: "system.root", Title: "系统", Type: menu.TypeDirectory}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []menu.Menu{
		{ID: "system.accounts", Title: "用户管理", Path: "/system/accounts"},
		{ID: "system.roles", Title: "角色管理", Path: "/system/roles"},
		{ID: "system.settings", Title: "系统管理", Path: "/system/settings"},
		{ID: "system.audit", Title: "审计管理", Path: "/system/audit"},
	} {
		item.ParentID = "system.root"
		item.Type = menu.TypeMenu
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	slot := navigation.New(registry)
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-1", PrincipalID: "user-1", Realm: auth.RealmTenant, Authenticated: true})
	result, err := slot.Execute(ctx, aitool.LoadedTool{}, json.RawMessage(`{"query":"管理"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["returned_count"] != 3 {
		t.Fatalf("navigation candidate count must default to 3: %#v", result)
	}
}

func TestHelpSlotReauthorizesAndRejectsArbitraryPaths(t *testing.T) {
	registry := menu.NewRegistry()
	for _, item := range []menu.Menu{{ID: "system.root", Title: "系统", Type: menu.TypeDirectory}, {ID: "system.accounts", ParentID: "system.root", Title: "用户管理", Type: menu.TypeMenu, Path: "/system/accounts", Permission: "user.list"}} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	document, err := menu.ParseHelpDocument("system.accounts.zh-CN.md", []byte("---\nmenu_id: system.accounts\nversion: 1.0.0\nlocale: zh-CN\nuse_cases: [新增用户]\npermission_refs: [user.list]\n---\n# 用户管理\n\n## 新增用户\n1. 新增"))
	if err != nil {
		t.Fatal(err)
	}
	help := menu.NewHelpRegistry()
	if err = help.Register(document); err != nil {
		t.Fatal(err)
	}
	slot := navigation.NewHelp(registry, help)
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-1", PrincipalID: "user-1", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{"user.list"}, Language: "zh-CN"})
	result, err := slot.Execute(ctx, aitool.LoadedTool{}, json.RawMessage(`{"menu_id":"system.accounts","use_case":"新增用户"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("unexpected result: %#v", result)
	}
	result, _ = slot.Execute(ctx, aitool.LoadedTool{}, json.RawMessage(`{"menu_id":"../../system.accounts"}`))
	if result.ErrorCategory != "invalid_request" {
		t.Fatalf("path input must fail: %#v", result)
	}
	revoked := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-1", PrincipalID: "user-1", Realm: auth.RealmTenant, Authenticated: true})
	result, _ = slot.Execute(revoked, aitool.LoadedTool{}, json.RawMessage(`{"menu_id":"system.accounts"}`))
	if result.ErrorCategory != "authorization_changed" {
		t.Fatalf("revoked permission must fail: %#v", result)
	}
}
