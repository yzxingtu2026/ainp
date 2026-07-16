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
