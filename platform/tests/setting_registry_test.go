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

func TestSettingMultiSelectCustomValues(t *testing.T) {
	options := []setting.FieldOption{{Value: ".pdf", Label: ".pdf"}}
	strict := setting.Definition{
		Key: "attachment.strict", Fields: []setting.Field{{
			Key: "allowed_ext", Type: setting.FieldMultiSelect, Label: "Allowed", Options: options,
		}},
	}
	if err := setting.ValidateValues(strict, map[string]any{"allowed_ext": []any{".custom"}}); err == nil {
		t.Fatal("normal multi_select must reject custom values")
	}

	custom := strict
	custom.Key = "attachment.custom"
	custom.Fields[0].AllowCustomValues = true
	custom.Fields[0].CustomValuePattern = `^\.?[A-Za-z0-9][A-Za-z0-9._+-]{0,31}$`
	if err := setting.ValidateValues(custom, map[string]any{"allowed_ext": []any{"custom", ".pdf"}}); err != nil {
		t.Fatalf("enabled multi_select should accept valid custom values: %v", err)
	}
	if err := setting.ValidateValues(custom, map[string]any{"allowed_ext": []any{"../invalid"}}); err == nil {
		t.Fatal("custom value that does not match pattern must fail")
	}
}

func TestSettingCustomValuesSchemaConstraints(t *testing.T) {
	category := map[string]setting.Category{
		"general": {Key: "general", Title: "General", Icon: "mdi:cog-outline", Surface: auth.RealmTenant},
	}
	base := setting.Definition{
		Key: "general.preferences", CategoryKey: "general", Title: "Prefs",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
	}
	base.Fields = []setting.Field{{
		Key: "name", Type: setting.FieldText, Label: "Name", Default: "x", AllowCustomValues: true,
	}}
	if err := setting.ValidateDefinition(base, category); err == nil {
		t.Fatal("allow_custom_values on a non-multi_select field must fail")
	}
	base.Fields = []setting.Field{{
		Key: "tags", Type: setting.FieldMultiSelect, Label: "Tags",
		Options:            []setting.FieldOption{{Value: "a", Label: "A"}},
		CustomValuePattern: "[",
	}}
	if err := setting.ValidateDefinition(base, category); err == nil {
		t.Fatal("custom_value_pattern without allow_custom_values must fail")
	}
}

func TestSettingRemoteSelectSchemaConstraints(t *testing.T) {
	categories := map[string]setting.Category{"general": {Key: "general", Title: "General", Icon: "mdi:cog", Surface: auth.RealmTenant}}
	definition := setting.Definition{Key: "general.remote", CategoryKey: "general", Title: "Remote", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1, Fields: []setting.Field{{Key: "alias", Type: setting.FieldRemoteSelect, Label: "Alias", Default: "", RemoteSource: &setting.RemoteOptionSource{Endpoint: "/api/v1/ai/aliases/list", LabelField: "code", ValueField: "code"}}}}
	if err := setting.ValidateDefinition(definition, categories); err != nil {
		t.Fatalf("valid remote select rejected: %v", err)
	}
	definition.Fields[0].RemoteSource.Endpoint = "https://example.com/options"
	if err := setting.ValidateDefinition(definition, categories); err == nil {
		t.Fatal("external remote source endpoint must fail")
	}
	definition.Fields[0].Type = setting.FieldText
	if err := setting.ValidateDefinition(definition, categories); err == nil {
		t.Fatal("remote source on non-remote field must fail")
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
				Options:     []setting.FieldOption{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}},
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
	attachStorage(t, application)
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
	if _, ok := application.Settings.GetDefinition("general.visibility_demo"); !ok {
		t.Fatal("visibility demo setting definition was not registered")
	}
	runtimePrefs, ok := application.Settings.GetDefinition(system.RuntimePreferencesKey)
	if !ok || runtimePrefs.Scope != setting.ScopeUser {
		t.Fatal("user-scope runtime preferences definition was not registered")
	}
	for _, actionID := range []string{system.ActionProfileGet, system.ActionProfileUpdate, system.ActionUserPreferencesGet, system.ActionUserPreferencesUpdate} {
		registered, ok := application.Actions.Get(actionID)
		if !ok || registered.Meta.Access != "authenticated" {
			t.Fatalf("authenticated profile action not registered: %s", actionID)
		}
	}
	attachmentDef, ok := application.Settings.GetDefinition(system.SettingKeyAttachmentUploadPolicy)
	if !ok {
		t.Fatal("attachment upload policy setting definition was not registered")
	}
	for _, fieldKey := range []string{"allowed_ext", "allowed_mime"} {
		found := false
		for _, field := range attachmentDef.Fields {
			if field.Key != fieldKey {
				continue
			}
			found = true
			if !field.AllowCustomValues || field.CustomValuePattern == "" {
				t.Fatalf("attachment field %s must validate custom values", fieldKey)
			}
		}
		if !found {
			t.Fatalf("attachment field not registered: %s", fieldKey)
		}
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
