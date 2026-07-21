package navigation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

func TestSlotProjectsAndSearchesFromTrustedAuthContext(t *testing.T) {
	registry := menu.NewRegistry()
	for _, item := range []menu.Menu{
		{ID: "system.root", Title: "系统", Type: menu.TypeDirectory},
		{ID: "system.users", ParentID: "system.root", Title: "用户管理", Type: menu.TypeMenu, Path: "/system/users", Permission: "user.list"},
		{ID: "system.roles", ParentID: "system.root", Title: "角色管理", Type: menu.TypeMenu, Path: "/system/roles", Permission: "role.list"},
	} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	slot := New(registry)
	ac := &auth.Context{TenantID: "tenant-1", UserID: "user-1", SessionID: "session-1", Realm: auth.RealmTenant, AuthVersion: 4, Permissions: []string{"user.list"}, Authenticated: true}
	ctx := auth.WithContext(context.Background(), ac)
	scope := aitool.RuntimeScope{TenantID: ac.TenantID, UserID: ac.UserID, SessionID: ac.SessionID, Realm: string(ac.Realm), AuthVersion: ac.AuthVersion}
	if _, visible, err := slot.Project(ctx, scope, slot.Definition()); err != nil || !visible {
		t.Fatalf("projection failed: visible=%v err=%v", visible, err)
	}

	result, err := slot.Execute(ctx, aitool.LoadedTool{}, json.RawMessage(`{"query":"管理","limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["returned_count"] != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	items, ok := data["items"].([]menu.SearchResult)
	if !ok || len(items) != 1 || items[0].MenuID != "system.users" || items[0].Path != "/system/users" {
		t.Fatalf("unauthorized result projection: %#v", data)
	}

	mismatched := scope
	mismatched.UserID = "other"
	if _, visible, err := slot.Project(ctx, mismatched, slot.Definition()); err != nil || visible {
		t.Fatalf("mismatched scope must be hidden: visible=%v err=%v", visible, err)
	}
}

func TestSlotRejectsModelSuppliedInvalidBudget(t *testing.T) {
	slot := New(menu.NewRegistry())
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-1", UserID: "user-1", Realm: auth.RealmTenant, Authenticated: true})
	result, err := slot.Execute(ctx, aitool.LoadedTool{}, json.RawMessage(`{"query":"users","limit":6}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.ErrorCategory != "invalid_request" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
