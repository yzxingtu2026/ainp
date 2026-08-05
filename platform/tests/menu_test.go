package tests

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

func TestRegistryFiltersMenusByRealm(t *testing.T) {
	r := menu.NewRegistry()
	for _, item := range []menu.Menu{{ID: "tenant", Type: menu.TypeMenu}, {ID: "platform", Type: menu.TypeMenu, Surface: auth.RealmPlatform}} {
		if err := r.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	items := r.TreeForUser(auth.RealmTenant, nil)
	if len(items) != 1 || items[0].ID != "tenant" || items[0].Surface != auth.RealmTenant {
		t.Fatalf("unexpected tenant menus: %#v", items)
	}
}

func TestRegistryPrunesDirectoryWithoutVisibleChildren(t *testing.T) {
	r := menu.NewRegistry()
	for _, item := range []menu.Menu{
		{ID: "operations.root", Type: menu.TypeDirectory},
		{ID: "operations.jobs", ParentID: "operations.root", Type: menu.TypeMenu, Permission: "operations.job_list"},
		{ID: "operations.outbox", ParentID: "operations.root", Type: menu.TypeMenu, Permission: "operations.outbox_list"},
	} {
		if err := r.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	items := r.TreeForUser(auth.RealmTenant, func(permission string) bool { return permission == "operations.job_list" })
	if len(items) != 2 || items[0].ID != "operations.jobs" || items[1].ID != "operations.root" {
		t.Fatalf("unexpected visible operations tree: %#v", items)
	}
	if items := r.TreeForUser(auth.RealmTenant, func(string) bool { return false }); len(items) != 0 {
		t.Fatalf("empty operations directory must be hidden: %#v", items)
	}
}

func TestRegistryValidatesShellPlacement(t *testing.T) {
	r := menu.NewRegistry()
	if err := r.Register(menu.Menu{ID: "workspace", Type: menu.TypeMenu, Placement: menu.PlacementGlobalAILauncher, Immersive: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(menu.Menu{ID: "bad-placement", Type: menu.TypeMenu, Placement: "sidebar_bottom"}); err == nil {
		t.Fatal("expected unsupported placement to fail")
	}
	if err := r.Register(menu.Menu{ID: "bad-directory", Type: menu.TypeDirectory, Placement: menu.PlacementGlobalAILauncher}); err == nil {
		t.Fatal("expected directory placement to fail")
	}
}

func TestRegistryValidatesNavigationMetadata(t *testing.T) {
	r := menu.NewRegistry()
	yes := true
	if err := r.Register(menu.Menu{ID: "hidden", Type: menu.TypeMenu, Path: "/hidden", HideInMenu: true, Navigation: &menu.NavigationMetadata{Searchable: &yes}}); err != nil {
		t.Fatal(err)
	}
	invalid := []menu.Menu{
		{ID: "directory", Type: menu.TypeDirectory, Navigation: &menu.NavigationMetadata{Searchable: &yes}},
		{ID: "parameterized", Type: menu.TypeMenu, Path: "/users/:id", Navigation: &menu.NavigationMetadata{Searchable: &yes}},
		{ID: "bad-key", Type: menu.TypeMenu, Path: "/bad", Navigation: &menu.NavigationMetadata{DescriptionKey: "Bad Key"}},
	}
	for _, item := range invalid {
		if err := r.Register(item); err == nil {
			t.Fatalf("expected navigation validation to fail for %s", item.ID)
		}
	}
}

func TestNavigationSearchFiltersAndMatchesRegistryProjection(t *testing.T) {
	r := menu.NewRegistry()
	yes, no := true, false
	items := []menu.Menu{
		{ID: "system.root", Title: "系统", Type: menu.TypeDirectory},
		{ID: "system.accounts", ParentID: "system.root", Title: "用户管理", Type: menu.TypeMenu, Path: "/system/accounts", Permission: "user.list", Order: 1, Navigation: &menu.NavigationMetadata{Description: "管理租户账号", Keywords: []string{"成员"}, UseCases: []string{"停用用户"}}},
		{ID: "system.roles", ParentID: "system.root", Title: "角色管理", Type: menu.TypeMenu, Path: "/system/roles", Permission: "role.list", Order: 2, Navigation: &menu.NavigationMetadata{Keywords: []string{"授权"}}},
		{ID: "system.hidden", ParentID: "system.root", Title: "隐藏页面", Type: menu.TypeMenu, Path: "/system/hidden", HideInMenu: true},
		{ID: "system.hidden_search", ParentID: "system.root", Title: "快捷资料", Type: menu.TypeMenu, Path: "/system/quick-profile", HideInMenu: true, Navigation: &menu.NavigationMetadata{Searchable: &yes}},
		{ID: "system.disabled", ParentID: "system.root", Title: "禁用搜索", Type: menu.TypeMenu, Path: "/system/disabled", Navigation: &menu.NavigationMetadata{Searchable: &no}},
		{ID: "platform.users", Title: "平台用户", Type: menu.TypeMenu, Path: "/platform/users", Surface: auth.RealmPlatform},
		{ID: "system.edit", Title: "编辑用户", Type: menu.TypeMenu, Path: "/system/accounts/:id"},
		{ID: "system.create", Title: "新增用户", Type: menu.TypeButton, Permission: "user.create"},
		{ID: "restricted.root", Title: "受限目录", Type: menu.TypeDirectory, Permission: "restricted.root"},
		{ID: "restricted.child", ParentID: "restricted.root", Title: "孤立页面", Type: menu.TypeMenu, Path: "/restricted/child", Permission: "restricted.child"},
	}
	for _, item := range items {
		if err := r.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	service := menu.NewSearchService(r)
	search := func(query string, hasPerm func(string) bool) []menu.SearchResult {
		results, err := service.Search(menu.SearchOptions{Realm: auth.RealmTenant, Query: query, HasPermission: hasPerm})
		if err != nil {
			t.Fatal(err)
		}
		return results
	}
	allowAll := func(string) bool { return true }
	for _, query := range []string{"用户管理", "系统 用户", "成员", "租户账号", "停用用户"} {
		results := search(query, allowAll)
		if len(results) == 0 || results[0].MenuID != "system.accounts" || results[0].Path != "/system/accounts" {
			t.Fatalf("query %q returned unexpected results: %#v", query, results)
		}
	}
	if results := search("隐藏页面", allowAll); len(results) != 0 {
		t.Fatalf("hidden menu leaked: %#v", results)
	}
	if results := search("快捷资料", allowAll); len(results) != 1 || results[0].MenuID != "system.hidden_search" {
		t.Fatalf("explicit hidden menu missing: %#v", results)
	}
	if results := search("用户", func(permission string) bool { return permission == "role.list" }); len(results) != 0 {
		t.Fatalf("revoked permission leaked: %#v", results)
	}
	if results := search("孤立页面", func(permission string) bool { return permission == "restricted.child" }); len(results) != 0 {
		t.Fatalf("menu with inaccessible parent leaked: %#v", results)
	}
}

func TestNavigationSearchValidatesBudgets(t *testing.T) {
	service := menu.NewSearchService(menu.NewRegistry())
	for _, opts := range []menu.SearchOptions{{Query: " "}, {Query: "valid", Limit: 21}, {Query: string(make([]rune, menu.MaxSearchQueryRunes+1))}} {
		if _, err := service.Search(opts); err == nil {
			t.Fatalf("expected invalid options to fail: %#v", opts)
		}
	}
}

func TestNavigationSearchFailsClosedWithoutPermissionChecker(t *testing.T) {
	r := menu.NewRegistry()
	if err := r.Register(menu.Menu{ID: "protected", Title: "受保护页面", Type: menu.TypeMenu, Path: "/protected", Permission: "protected.list"}); err != nil {
		t.Fatal(err)
	}
	results, err := menu.NewSearchService(r).Search(menu.SearchOptions{Query: "受保护"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("protected menu leaked without permission checker: %#v", results)
	}
}
