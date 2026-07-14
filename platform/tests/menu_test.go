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
