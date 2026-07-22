package setting

import "testing"

func TestConditionalRequiredFieldAllowsHiddenEmptyDefault(t *testing.T) {
	def := Definition{Key: "ai.example", CategoryKey: "ai", Fields: []Field{
		{Key: "enabled", Type: FieldBoolean, Label: "Enabled", Default: false},
		{Key: "model_alias", Type: FieldRemoteSelect, Label: "Model alias", Default: "", Required: true, VisibleWhen: &VisibleWhen{Field: "enabled", Eq: true}, RemoteSource: &RemoteOptionSource{Endpoint: "/api/v1/ai/aliases/list", LabelField: "code", ValueField: "code"}},
	}}
	if err := validateField(def.Key, &def.Fields[1]); err != nil {
		t.Fatalf("hidden empty default should be valid: %v", err)
	}
	if err := ValidateValues(def, map[string]any{"enabled": true, "model_alias": ""}); err == nil {
		t.Fatal("visible model alias must remain required")
	}
	if err := ValidateValues(def, map[string]any{"enabled": false, "model_alias": ""}); err != nil {
		t.Fatalf("hidden field should skip required validation: %v", err)
	}
}
