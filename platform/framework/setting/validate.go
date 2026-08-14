package setting

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// DefaultValues returns a map of field defaults for a definition.
func DefaultValues(def Definition) map[string]any {
	out := make(map[string]any, len(def.Fields))
	for _, field := range def.Fields {
		if field.Default != nil {
			out[field.Key] = cloneJSONValue(field.Default)
		}
	}
	return out
}

// ValidateValues checks submitted values against the definition schema.
// unknown fields are rejected; types are strict (no silent coercion).
// Fields hidden by visible_when are skipped (required and type checks).
func ValidateValues(def Definition, values map[string]any) error {
	if values == nil {
		values = map[string]any{}
	}
	allowed := make(map[string]Field, len(def.Fields))
	for _, field := range def.Fields {
		allowed[field.Key] = field
	}
	for key := range values {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown field: %s", key)
		}
	}
	for _, field := range def.Fields {
		if !IsFieldVisible(def, values, field.Key) {
			continue
		}
		raw, ok := values[field.Key]
		if !ok || raw == nil {
			if field.Required {
				return fmt.Errorf("field required: %s", field.Key)
			}
			continue
		}
		if err := validateFieldValue(def.Key, field, raw, false); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldValue(defKey string, field Field, raw any, asDefault bool) error {
	switch field.Type {
	case FieldText, FieldTextarea, FieldColor, FieldSelect, FieldRemoteSelect, FieldSecret:
		s, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string")
		}
		if field.Required && strings.TrimSpace(s) == "" {
			return fmt.Errorf("field required: %s", field.Key)
		}
		if field.Type == FieldSecret && strings.TrimSpace(s) == "" {
			return fmt.Errorf("secret value required: %s", field.Key)
		}
		if field.MinLength != nil && len([]rune(s)) < *field.MinLength {
			return fmt.Errorf("min_length %d", *field.MinLength)
		}
		if field.MaxLength != nil && len([]rune(s)) > *field.MaxLength {
			return fmt.Errorf("max_length %d", *field.MaxLength)
		}
		if field.Type == FieldColor && s != "" && !colorPat.MatchString(s) {
			return fmt.Errorf("invalid color")
		}
		if field.Type == FieldSelect && s != "" && !optionAllowed(field.Options, s) {
			return fmt.Errorf("invalid option")
		}
		return nil
	case FieldBoolean:
		if _, ok := raw.(bool); !ok {
			return fmt.Errorf("expected boolean")
		}
		return nil
	case FieldNumber:
		n, ok := asFloat(raw)
		if !ok {
			return fmt.Errorf("expected number")
		}
		if field.Min != nil && n < *field.Min {
			return fmt.Errorf("min %v", *field.Min)
		}
		if field.Max != nil && n > *field.Max {
			return fmt.Errorf("max %v", *field.Max)
		}
		if field.Step != nil && *field.Step > 0 {
			base := 0.0
			if field.Min != nil {
				base = *field.Min
			}
			steps := (n - base) / *field.Step
			if math.Abs(steps-math.Round(steps)) > 1e-9 {
				return fmt.Errorf("step %v", *field.Step)
			}
		}
		_ = asDefault
		_ = defKey
		return nil
	case FieldMultiSelect:
		arr, ok := asStringSlice(raw)
		if !ok {
			return fmt.Errorf("expected string array")
		}
		if field.Required && len(arr) == 0 {
			return fmt.Errorf("field required: %s", field.Key)
		}
		for _, item := range arr {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("empty custom value")
			}
			if optionAllowed(field.Options, item) {
				continue
			}
			if !field.AllowCustomValues {
				return fmt.Errorf("invalid option %q", item)
			}
			if field.CustomValuePattern != "" {
				pattern, err := regexp.Compile(field.CustomValuePattern)
				if err != nil || !pattern.MatchString(item) {
					return fmt.Errorf("invalid custom value %q", item)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported type")
	}
}

func optionAllowed(options []FieldOption, value string) bool {
	for _, opt := range options {
		if opt.Value == value {
			return true
		}
	}
	return false
}

func asFloat(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	default:
		return 0, false
	}
}

func asStringSlice(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func cloneJSONValue(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}
