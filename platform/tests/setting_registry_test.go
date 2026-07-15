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

func TestSettingVisibleWhenRegistrationAndValidation(t *testing.T) {
	reg := setting.NewRegistry()
	if err := reg.RegisterCategory(setting.Category{
		Key: "general", Title: "General", Icon: "mdi:cog-outline", Surface: auth.RealmTenant,
	}); err != nil {
		t.Fatal(err)
	}

	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.prefs", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{
			{Key: "enabled", Type: setting.FieldBoolean, Label: "Enabled", Default: false},
			{Key: "title", Type: setting.FieldText, Label: "Title", Required: true, Default: "x",
				VisibleWhen: &setting.VisibleWhen{Field: "missing", Eq: true}},
		},
	}); err == nil {
		t.Fatal("unknown visible_when field must fail")
	}

	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.prefs", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{
			{Key: "title", Type: setting.FieldText, Label: "Title", Required: true, Default: "x",
				VisibleWhen: &setting.VisibleWhen{Field: "title", Eq: "x"}},
		},
	}); err == nil {
		t.Fatal("self-referencing visible_when must fail")
	}

	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.prefs", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{
			{Key: "enabled", Type: setting.FieldBoolean, Label: "Enabled", Default: false},
			{Key: "title", Type: setting.FieldText, Label: "Title", Required: true, Default: "x",
				VisibleWhen: &setting.VisibleWhen{Field: "enabled"}},
		},
	}); err == nil {
		t.Fatal("visible_when without eq/in must fail")
	}

	if err := reg.RegisterDefinition(setting.Definition{
		Key: "general.prefs", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{
			{Key: "a", Type: setting.FieldBoolean, Label: "A", Default: true,
				VisibleWhen: &setting.VisibleWhen{Field: "b", Eq: true}},
			{Key: "b", Type: setting.FieldBoolean, Label: "B", Default: true,
				VisibleWhen: &setting.VisibleWhen{Field: "a", Eq: true}},
		},
	}); err == nil {
		t.Fatal("visible_when cycle must fail")
	}

	def := setting.Definition{
		Key: "general.prefs", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{
			{Key: "enabled", Type: setting.FieldBoolean, Label: "Enabled", Default: false},
			{Key: "mode", Type: setting.FieldSelect, Label: "Mode", Required: true, Default: "a",
				Options: []setting.FieldOption{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}},
				VisibleWhen: &setting.VisibleWhen{Field: "enabled", Eq: true}},
			{Key: "note", Type: setting.FieldText, Label: "Note", Required: true, Default: "n",
				VisibleWhen: &setting.VisibleWhen{Field: "mode", In: []any{"b"}}},
		},
	}
	if err := reg.RegisterDefinition(def); err != nil {
		t.Fatal(err)
	}

	if setting.IsFieldVisible(def, map[string]any{"enabled": false}, "mode") {
		t.Fatal("mode must be hidden when enabled=false")
	}
	if !setting.IsFieldVisible(def, map[string]any{"enabled": true}, "mode") {
		t.Fatal("mode must be visible when enabled=true")
	}
	if setting.IsFieldVisible(def, map[string]any{"enabled": true, "mode": "a"}, "note") {
		t.Fatal("note must be hidden when mode=a")
	}
	if !setting.IsFieldVisible(def, map[string]any{"enabled": true, "mode": "b"}, "note") {
		t.Fatal("note must be visible when mode=b")
	}
	if setting.IsFieldVisible(def, map[string]any{"enabled": false, "mode": "b"}, "note") {
		t.Fatal("note must be hidden transitively when enabled=false")
	}

	if err := setting.ValidateValues(def, map[string]any{"enabled": false}); err != nil {
		t.Fatalf("hidden required fields must be skipped: %v", err)
	}
	if err := setting.ValidateValues(def, map[string]any{"enabled": true}); err == nil {
		t.Fatal("visible required mode must fail when missing")
	}
	if err := setting.ValidateValues(def, map[string]any{"enabled": true, "mode": "a"}); err != nil {
		t.Fatalf("visible mode=a should pass without note: %v", err)
	}
	if err := setting.ValidateValues(def, map[string]any{"enabled": true, "mode": "b"}); err == nil {
		t.Fatal("visible required note must fail when missing")
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
