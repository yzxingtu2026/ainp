package setting

import (
	"encoding/json"
	"fmt"
	"reflect"
)

func validateVisibleWhenGraph(def Definition, fieldKeys map[string]struct{}) error {
	edges := make(map[string]string, len(def.Fields))
	for _, field := range def.Fields {
		when := field.VisibleWhen
		if when == nil {
			continue
		}
		if when.Field == "" {
			return fmt.Errorf("visible_when.field required: %s.%s", def.Key, field.Key)
		}
		if when.Field == field.Key {
			return fmt.Errorf("visible_when cannot reference self: %s.%s", def.Key, field.Key)
		}
		if _, ok := fieldKeys[when.Field]; !ok {
			return fmt.Errorf("visible_when references unknown field %q: %s.%s", when.Field, def.Key, field.Key)
		}
		hasEq := when.Eq != nil
		hasIn := len(when.In) > 0
		if hasEq == hasIn {
			return fmt.Errorf("visible_when requires exactly one of eq or in: %s.%s", def.Key, field.Key)
		}
		edges[field.Key] = when.Field
	}
	for start := range edges {
		seen := map[string]struct{}{start: {}}
		cur := start
		for {
			next, ok := edges[cur]
			if !ok {
				break
			}
			if _, loop := seen[next]; loop {
				return fmt.Errorf("visible_when cycle detected involving %s.%s", def.Key, start)
			}
			seen[next] = struct{}{}
			cur = next
		}
	}
	return nil
}

// IsFieldVisible reports whether fieldKey should be shown given sibling values.
// If a dependency field is itself hidden, the field is hidden (transitive).
func IsFieldVisible(def Definition, values map[string]any, fieldKey string) bool {
	byKey := make(map[string]Field, len(def.Fields))
	for _, field := range def.Fields {
		byKey[field.Key] = field
	}
	return isFieldVisible(byKey, values, fieldKey, map[string]bool{})
}

func isFieldVisible(byKey map[string]Field, values map[string]any, fieldKey string, visiting map[string]bool) bool {
	field, ok := byKey[fieldKey]
	if !ok {
		return false
	}
	when := field.VisibleWhen
	if when == nil {
		return true
	}
	if visiting[fieldKey] {
		return false
	}
	visiting[fieldKey] = true
	if !isFieldVisible(byKey, values, when.Field, visiting) {
		return false
	}
	actual, exists := values[when.Field]
	if !exists {
		if dep, ok := byKey[when.Field]; ok && dep.Default != nil {
			actual = dep.Default
		} else {
			actual = nil
		}
	}
	if when.Eq != nil {
		return valuesEqual(actual, when.Eq)
	}
	for _, candidate := range when.In {
		if valuesEqual(actual, candidate) {
			return true
		}
	}
	return false
}

func valuesEqual(left, right any) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if lf, lok := asFloat(left); lok {
		if rf, rok := asFloat(right); rok {
			return lf == rf
		}
	}
	if lb, ok := left.(bool); ok {
		if rb, ok := right.(bool); ok {
			return lb == rb
		}
	}
	if ls, ok := left.(string); ok {
		if rs, ok := right.(string); ok {
			return ls == rs
		}
	}
	if la, ok := asStringSlice(left); ok {
		if ra, ok := asStringSlice(right); ok {
			if len(la) != len(ra) {
				return false
			}
			for i := range la {
				if la[i] != ra[i] {
					return false
				}
			}
			return true
		}
	}
	// Fallback for JSON-decoded shapes (e.g. []any vs typed).
	lb, errL := json.Marshal(left)
	rb, errR := json.Marshal(right)
	if errL == nil && errR == nil && string(lb) == string(rb) {
		return true
	}
	return reflect.DeepEqual(left, right)
}
