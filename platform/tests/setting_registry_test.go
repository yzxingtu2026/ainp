package tests

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestSettingRegistryRejectsInvalidDefinitions(t *testing.T) {
	reg := setting.NewRegistry()
	if err := reg.RegisterCategory(setting.Category{
		Key: "general", Title: "General", Icon: "mdi:cog-outline", Surface: auth.RealmTenant,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterCategory(setting.Category{
		Key: "general", Title: "Dup", Icon: "mdi:cog-outline", Surface: auth.RealmTenant,
	}); err == nil {
		t.Fatal("duplicate category must fail")
	}
	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.preferences", CategoryKey: "missing", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{{Key: "name", Type: setting.FieldText, Label: "Name", Default: "x"}},
	}); err == nil {
		t.Fatal("unknown category must fail")
	}
	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.preferences", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{
			{Key: "name", Type: setting.FieldText, Label: "Name", Default: "x"},
			{Key: "name", Type: setting.FieldText, Label: "Dup", Default: "y"},
		},
	}); err == nil {
		t.Fatal("duplicate field key must fail")
	}
	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.preferences", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{{Key: "name", Type: setting.FieldType("blob"), Label: "Name", Default: "x"}},
	}); err == nil {
		t.Fatal("invalid field type must fail")
	}
	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.preferences", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{{Key: "name", Type: setting.FieldText, Label: "Name", Default: 12}},
	}); err == nil {
		t.Fatal("invalid default must fail")
	}
	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.preferences", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{{Key: "name", Type: setting.FieldText, Label: "Name", Required: true, Default: "AINP"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSettingActionsAndMenuAreRegistered(t *testing.T) {
	application := app.New(&config.Config{}, nil, nil, nil)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	for _, actionID := range []string{
		system.ActionSettingList, system.ActionSettingGet, system.ActionSettingUpdate, system.ActionSettingReset,
	} {
		registered, ok := application.Actions.Get(actionID)
		if !ok {
			t.Fatalf("action not registered: %s", actionID)
		}
		if registered.Meta.PermissionGroup != "system.setting_management" {
			t.Fatalf("setting action permission group = %q", registered.Meta.PermissionGroup)
		}
	}
	if _, ok := application.Settings.GetDefinition("general.preferences"); !ok {
		t.Fatal("example setting definition was not registered")
	}
	foundMenu := false
	for _, registered := range application.Menus.List() {
		if registered.ID == system.MenuSettingManagement && registered.Permission == system.ActionSettingList {
			foundMenu = true
		}
	}
	if !foundMenu {
		t.Fatal("settings menu was not registered")
	}
	for _, permission := range []string{system.ActionSettingList, system.ActionSettingGet, system.ActionSettingUpdate, system.ActionSettingReset} {
		found := false
		for _, minimum := range system.AdminMinimumPermissions() {
			if minimum == permission {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("admin minimum permissions missing %s", permission)
		}
	}
}
