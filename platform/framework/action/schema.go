package action

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const inputContractMarker = "explicit-v1"

// SchemaFor derives the AI input contract from the same Go DTO used by action.JSON.
func SchemaFor[T any]() json.RawMessage {
	t := reflect.TypeOf((*T)(nil)).Elem()
	schema := schemaForType(t)
	b, _ := json.Marshal(schema)
	return b
}

// InputSchemaFor derives an Action input schema and rejects fields whose
// required/optional presence semantics are not explicitly declared.
func InputSchemaFor[T any]() json.RawMessage {
	t := reflect.TypeOf((*T)(nil)).Elem()
	if err := validateInputContract(t, ""); err != nil {
		panic(err)
	}
	var schema map[string]any
	_ = json.Unmarshal(SchemaFor[T](), &schema)
	schema["x-ainp-input-contract"] = inputContractMarker
	b, _ := json.Marshal(schema)
	return b
}

func HasExplicitInputContract(schema json.RawMessage) bool {
	var value map[string]any
	return json.Unmarshal(schema, &value) == nil && value["x-ainp-input-contract"] == inputContractMarker
}

// ToolInputSchema removes Action-internal contract metadata before projection
// to model-provider JSON Schema.
func ToolInputSchema(schema json.RawMessage) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(schema, &value) != nil {
		return schema
	}
	delete(value, "x-ainp-input-contract")
	b, _ := json.Marshal(value)
	return b
}

func validateInputContract(t reflect.Type, path string) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _ := jsonField(field)
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			if err := validateInputContract(field.Type, path); err != nil {
				return err
			}
			continue
		}
		if name == "" {
			name = field.Name
		}
		fieldPath := name
		if path != "" {
			fieldPath = path + "." + name
		}
		rules := fieldValidationRules(field.Tag.Get("validate"))
		required := hasValidationRule(rules, "required")
		optional := hasValidationRule(rules, "omitempty")
		if required == optional {
			return fmt.Errorf("action input field %s must declare exactly one of validate required or omitempty", fieldPath)
		}
	}
	return nil
}

func schemaForType(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, _ := jsonField(f)
			if name == "-" {
				continue
			}
			if f.Anonymous && name == "" {
				embedded := schemaForType(f.Type)
				if embeddedProperties, ok := embedded["properties"].(map[string]any); ok {
					for embeddedName, embeddedSchema := range embeddedProperties {
						properties[embeddedName] = embeddedSchema
					}
				}
				if embeddedRequired, ok := embedded["required"].([]string); ok {
					required = append(required, embeddedRequired...)
				}
				continue
			}
			if name == "" {
				name = f.Name
			}
			fieldSchema := schemaForType(f.Type)
			if description := strings.TrimSpace(f.Tag.Get("description")); description != "" {
				fieldSchema["description"] = description
			}
			properties[name] = fieldSchema
			validateRules := fieldValidationRules(f.Tag.Get("validate"))
			applyValidation(fieldSchema, validateRules, f.Type)
			if hasValidationRule(validateRules, "required") {
				required = append(required, name)
			}
		}
		out := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			out["required"] = required
		}
		return out
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaForType(t.Elem())}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Map, reflect.Interface:
		return map[string]any{"type": "object"}
	default:
		return map[string]any{"type": "string"}
	}
}

func validationRules(tag string) []string {
	if strings.TrimSpace(tag) == "" {
		return nil
	}
	rules := strings.Split(tag, ",")
	for i := range rules {
		rules[i] = strings.TrimSpace(rules[i])
	}
	return rules
}

func fieldValidationRules(tag string) []string {
	rules := validationRules(tag)
	for i, rule := range rules {
		if rule == "dive" {
			return rules[:i]
		}
	}
	return rules
}

func hasValidationRule(rules []string, expected string) bool {
	for _, rule := range rules {
		if rule == expected {
			return true
		}
	}
	return false
}

func applyValidation(schema map[string]any, rules []string, t reflect.Type) {
	for _, rule := range rules {
		parts := strings.SplitN(rule, "=", 2)
		switch parts[0] {
		case "min":
			if len(parts) == 2 {
				if t.Kind() == reflect.String {
					schema["minLength"] = number(parts[1])
				} else if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
					schema["minItems"] = number(parts[1])
				} else {
					schema["minimum"] = number(parts[1])
				}
			}
		case "max":
			if len(parts) == 2 {
				if t.Kind() == reflect.String {
					schema["maxLength"] = number(parts[1])
				} else if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
					schema["maxItems"] = number(parts[1])
				} else {
					schema["maximum"] = number(parts[1])
				}
			}
		case "oneof":
			if len(parts) == 2 {
				schema["enum"] = strings.Fields(parts[1])
			}
		}
	}
}
func number(v string) any {
	n := json.Number(v)
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return v
}

func jsonField(f reflect.StructField) (string, string) {
	parts := strings.Split(f.Tag.Get("json"), ",")
	if len(parts) == 0 {
		return "", ""
	}
	return parts[0], strings.Join(parts[1:], ",")
}
