package tests

import (
	"testing"

	. "github.com/yzxingtu2026/ainp/platform/framework/setting"
)

func TestConditionalRequiredFieldAllowsHiddenEmptyDefault(t *testing.T) {
	def := Definition{Key: "ai.example", CategoryKey: "ai", Title: "Example", Scope: ScopeTenant, SchemaVersion: 1, Fields: []Field{
		{Key: "enabled", Type: FieldBoolean, Label: "Enabled", Default: false},
		{Key: "model_alias", Type: FieldRemoteSelect, Label: "Model alias", Default: "", Required: true, VisibleWhen: &VisibleWhen{Field: "enabled", Eq: true}, RemoteSource: &RemoteOptionSource{Endpoint: "/api/v1/ai/aliases/list", LabelField: "code", ValueField: "code"}},
	}}
	registry := NewRegistry()
	if err := registry.RegisterCategory(Category{Key: "ai", Title: "AI", Icon: "mdi:brain"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterDefinition(def); err != nil {
		t.Fatalf("hidden empty default should be valid: %v", err)
	}
	if err := ValidateValues(def, map[string]any{"enabled": true, "model_alias": ""}); err == nil {
		t.Fatal("visible model alias must remain required")
	}
	if err := ValidateValues(def, map[string]any{"enabled": false, "model_alias": ""}); err != nil {
		t.Fatalf("hidden field should skip required validation: %v", err)
	}
}
