package action

import (
	"encoding/json"
	"reflect"
	"strings"
)

// SchemaFor derives the AI input contract from the same Go DTO used by action.JSON.
func SchemaFor[T any]() json.RawMessage {
	t := reflect.TypeOf((*T)(nil)).Elem()
	schema := schemaForType(t)
	b, _ := json.Marshal(schema)
	return b
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
			name, options := jsonField(f)
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
			validateTag := f.Tag.Get("validate")
			applyValidation(fieldSchema, validateTag, f.Type)
			if strings.Contains(validateTag, "required") || (!strings.Contains(options, "omitempty") && f.Type.Kind() != reflect.Pointer) {
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

func applyValidation(schema map[string]any, tag string, t reflect.Type) {
	for _, rule := range strings.Split(tag, ",") {
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
