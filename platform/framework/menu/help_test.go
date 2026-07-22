package menu

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type semanticRecorder struct{ request SemanticSearchRequest }

func (s *semanticRecorder) SearchNavigation(_ context.Context, request SemanticSearchRequest) SemanticSearchResult {
	s.request = request
	return SemanticSearchResult{Candidates: []SemanticCandidate{{MenuID: "system.users", Score: .8}}}
}

func TestParseAndValidateHelpDocument(t *testing.T) {
	data := []byte("---\nmenu_id: system.users\nversion: 1.2.0\nlocale: zh-CN\nuse_cases: [新增用户]\npermission_refs: [system.user_list]\n---\n# 用户管理\n\n操作说明")
	document, err := ParseHelpDocument("users.md", data)
	if err != nil {
		t.Fatal(err)
	}
	if document.MenuID != "system.users" || document.Version != "1.2.0" || document.Content == "" {
		t.Fatalf("unexpected document: %#v", document)
	}
	registry := NewRegistry()
	if err := registry.Register(Menu{ID: "system.users", Type: TypeMenu, Path: "/system/users", Surface: auth.RealmTenant, Permission: "system.user_list"}); err != nil {
		t.Fatal(err)
	}
	help := NewHelpRegistry()
	if err := help.Register(document); err != nil {
		t.Fatal(err)
	}
	if err := help.ValidateMenus(registry); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticSearchReceivesOnlyAuthorizedMenusAndRegistryProjectsPath(t *testing.T) {
	registry := NewRegistry()
	for _, item := range []Menu{{ID: "root", Title: "系统", Type: TypeDirectory, Surface: auth.RealmTenant}, {ID: "system.users", ParentID: "root", Title: "用户管理", Type: TypeMenu, Path: "/trusted/users", Permission: "user.list", Surface: auth.RealmTenant}, {ID: "system.roles", ParentID: "root", Title: "角色管理", Type: TypeMenu, Path: "/system/roles", Permission: "role.list", Surface: auth.RealmTenant}} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	recorder := &semanticRecorder{}
	registry.SetSemanticSearcher(recorder)
	service := NewSearchService(registry)
	results, _, err := service.SearchContext(context.Background(), SearchOptions{TenantID: "tenant-1", Realm: auth.RealmTenant, Query: "完全不同的自然语言", HasPermission: func(permission string) bool { return permission == "user.list" }})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.request.AllowedMenuIDs) != 1 || recorder.request.AllowedMenuIDs[0] != "system.users" {
		t.Fatalf("semantic scope was not prefiltered: %#v", recorder.request)
	}
	if len(results) != 1 || results[0].Path != "/trusted/users" {
		t.Fatalf("path must be projected from registry: %#v", results)
	}
}

func TestHelpRegistryRejectsInvalidAssets(t *testing.T) {
	invalid := map[string]string{
		"missing-frontmatter.md": "# no metadata",
		"invalid-version.md":     "---\nmenu_id: system.users\nversion: latest\nlocale: zh-CN\nuse_cases: [新增用户]\n---\n# 用户管理",
		"duplicate-use-case.md":  "---\nmenu_id: system.users\nversion: 1.0.0\nlocale: zh-CN\nuse_cases: [新增用户, 新增用户]\n---\n# 用户管理",
		"forbidden-path.md":      "---\nmenu_id: system.users\nversion: 1.0.0\nlocale: zh-CN\npath: /system/users\nuse_cases: [新增用户]\n---\n# 用户管理",
	}
	for name, content := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseHelpDocument(name, []byte(content)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	help := NewHelpRegistry()
	err := help.Load(fstest.MapFS{"one.md": {Data: []byte("---\nmenu_id: missing\nversion: 1.0.0\nlocale: zh-CN\nuse_cases: [test]\n---\n# Missing")}}, "*.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := help.ValidateMenus(NewRegistry()); err == nil {
		t.Fatal("expected unknown menu error")
	}
}

func TestHelpRegistryRequiresDefaultLocaleForSearchableModuleMenus(t *testing.T) {
	registry := NewRegistry()
	for _, item := range []Menu{
		{ID: "system.users", Type: TypeMenu, Path: "/system/users", Module: "system", Surface: auth.RealmTenant},
		{ID: "system.hidden", Type: TypeMenu, Path: "/system/hidden", Module: "system", HideInMenu: true, Surface: auth.RealmTenant},
		{ID: "operations.jobs", Type: TypeMenu, Path: "/operations/jobs", Module: "operations", Surface: auth.RealmTenant},
	} {
		if err := registry.Register(item); err != nil {
			t.Fatal(err)
		}
	}
	help := NewHelpRegistry()
	if err := help.ValidateModuleCoverage(registry, "system", "zh-CN"); err == nil {
		t.Fatal("expected missing searchable system menu help")
	}
	document, err := ParseHelpDocument("system.users.zh-CN.md", []byte("---\nmenu_id: system.users\nversion: 1.0.0\nlocale: zh-CN\nuse_cases: [查询用户]\n---\n# 用户管理"))
	if err != nil {
		t.Fatal(err)
	}
	if err := help.Register(document); err != nil {
		t.Fatal(err)
	}
	if err := help.ValidateModuleCoverage(registry, "system", "zh-CN"); err != nil {
		t.Fatal(err)
	}
}
