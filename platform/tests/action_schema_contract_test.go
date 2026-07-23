package tests

import (
	"encoding/json"
	"reflect"
	"testing"

	. "github.com/yzxingtu2026/ainp/platform/framework/action"
)

func TestSchemaForRequiresOnlyExplicitRequiredValidation(t *testing.T) {
	type request struct {
		ID            string  `json:"id,omitempty" validate:"required"`
		DisplayName   string  `json:"display_name" validate:"max=128"`
		Organization  string  `json:"organization,omitempty"`
		Department    *string `json:"department"`
		ConditionalID string  `json:"conditional_id" validate:"required_if=DisplayName admin"`
	}

	var schema map[string]any
	if err := json.Unmarshal(SchemaFor[request](), &schema); err != nil {
		t.Fatal(err)
	}
	required, _ := schema["required"].([]any)
	if got, want := required, []any{"id"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required = %#v, want %#v", got, want)
	}
}

func TestSchemaForMergesExplicitRequiredFromEmbeddedDTO(t *testing.T) {
	type pagination struct {
		Page     int `json:"page" validate:"omitempty,min=1"`
		PageSize int `json:"page_size" validate:"omitempty,min=1,max=100"`
	}
	type request struct {
		pagination
		Query string `json:"query" validate:"required,max=128"`
	}

	var schema map[string]any
	if err := json.Unmarshal(SchemaFor[request](), &schema); err != nil {
		t.Fatal(err)
	}
	required, _ := schema["required"].([]any)
	if got, want := required, []any{"query"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required = %#v, want %#v", got, want)
	}
}

func TestInputSchemaForRejectsAmbiguousPresence(t *testing.T) {
	type ambiguous struct {
		Name string `json:"name" validate:"max=128"`
	}
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("InputSchemaFor should reject fields without required or omitempty")
		}
	}()
	_ = InputSchemaFor[ambiguous]()
}

func TestInputSchemaForAcceptsExplicitPresence(t *testing.T) {
	type request struct {
		ID    string `json:"id" validate:"required"`
		Query string `json:"query,omitempty" validate:"omitempty,max=128"`
	}
	var schema map[string]any
	if err := json.Unmarshal(InputSchemaFor[request](), &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema["required"]; !reflect.DeepEqual(got, []any{"id"}) {
		t.Fatalf("required = %#v", got)
	}
	if !HasExplicitInputContract(InputSchemaFor[request]()) {
		t.Fatal("explicit input contract marker is missing")
	}
	var projected map[string]any
	if err := json.Unmarshal(ToolInputSchema(InputSchemaFor[request]()), &projected); err != nil {
		t.Fatal(err)
	}
	if _, exists := projected["x-ainp-input-contract"]; exists {
		t.Fatal("internal input contract marker leaked to tool schema")
	}
}

func TestSchemaForDoesNotTreatDiveRequiredAsFieldRequired(t *testing.T) {
	type request struct {
		RoleIDs []string `json:"role_ids,omitempty" validate:"omitempty,max=20,dive,required"`
	}
	var schema map[string]any
	if err := json.Unmarshal(InputSchemaFor[request](), &schema); err != nil {
		t.Fatal(err)
	}
	if required, ok := schema["required"]; ok {
		t.Fatalf("optional slice unexpectedly required: %#v", required)
	}
}
