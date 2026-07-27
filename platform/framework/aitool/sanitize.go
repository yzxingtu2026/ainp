package aitool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	maxSafeResultDepth = 8
	maxSafeArrayItems  = 100
	maxTruncationFacts = 64
)

// SanitizeData normalizes arbitrary Go values to JSON values, redacts sensitive
// fields, and bounds data plus truncation diagnostics to one shared byte budget.
func SanitizeData(value any, budget int) (any, *Truncation) {
	if budget <= 0 {
		return nil, &Truncation{Entries: []TruncationEntry{{Path: "$", Kind: jsonKind(value), OmittedCount: 1, Reason: "byte_budget"}}}
	}
	normalized, ok := normalizeJSON(value)
	if !ok {
		return nil, &Truncation{Entries: []TruncationEntry{{Path: "$", Kind: jsonKind(value), OmittedCount: 1, Reason: "invalid_json_value"}}}
	}
	facts := map[string]*TruncationEntry{}
	normalized = sanitizeJSON(normalized, "$", 0, facts)
	reserve := min(1024, budget/4)
	dataBudget := budget - reserve
	for encodedSize(normalized, nil) > dataBudget {
		if !shrinkJSON(&normalized, "$", facts) {
			normalized = nil
			addFact(facts, "$", jsonKind(value), 0, 1, "byte_budget")
			break
		}
	}
	meta := truncationFrom(facts)
	for meta != nil && encodedSize(normalized, meta) > budget && len(meta.Entries) > 0 {
		meta.Entries = meta.Entries[:len(meta.Entries)-1]
	}
	if meta != nil && len(meta.Entries) == 0 {
		meta = nil
	}
	return normalized, meta
}

// SanitizeResult applies the budget to the complete encoded Result, including
// its protocol fields and truncation diagnostics.
func SanitizeResult(result *Result, budget int) {
	if result == nil {
		return
	}
	data := result.Data
	result.Data = nil
	result.Truncation = nil
	result.Truncated = false
	base, _ := json.Marshal(result)
	const envelopeBase = len(`{"data":null}`)
	dataBudget := budget - len(base) + envelopeBase
	result.Data, result.Truncation = SanitizeData(data, dataBudget)
	result.Truncated = result.Truncation != nil
}

func normalizeJSON(value any) (any, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var normalized any
	if decoder.Decode(&normalized) != nil {
		return nil, false
	}
	return normalized, true
}

func sanitizeJSON(value any, path string, depth int, facts map[string]*TruncationEntry) any {
	switch current := value.(type) {
	case map[string]any:
		if depth >= maxSafeResultDepth {
			addFact(facts, path, "object", 0, len(current), "depth_limit")
			return map[string]any{}
		}
		keys := sortedKeys(current)
		out := make(map[string]any, len(current))
		for _, key := range keys {
			if sensitiveJSONKey(key) {
				out[key] = redactedJSON(current[key])
				continue
			}
			out[key] = sanitizeJSON(current[key], childPath(path, key), depth+1, facts)
		}
		return out
	case []any:
		if depth >= maxSafeResultDepth {
			addFact(facts, path, "array", 0, len(current), "depth_limit")
			return []any{}
		}
		limit := len(current)
		if limit > maxSafeArrayItems {
			limit = maxSafeArrayItems
			addFact(facts, path, "array", limit, len(current)-limit, "item_limit")
		}
		out := make([]any, 0, limit)
		for i := 0; i < limit; i++ {
			out = append(out, sanitizeJSON(current[i], indexPath(path, i), depth+1, facts))
		}
		return out
	default:
		return value
	}
}

func redactedJSON(value any) any {
	switch value.(type) {
	case map[string]any:
		return map[string]any{}
	case []any:
		return []any{}
	default:
		return "[REDACTED]"
	}
}

func shrinkJSON(value *any, path string, facts map[string]*TruncationEntry) bool {
	switch current := (*value).(type) {
	case map[string]any:
		keys := sortedKeys(current)
		for i := len(keys) - 1; i >= 0; i-- {
			key := keys[i]
			child := current[key]
			if isJSONCollection(child) && shrinkJSON(&child, childPath(path, key), facts) {
				current[key] = child
				return true
			}
		}
		if len(keys) > 0 {
			remove := max(1, len(keys)/2)
			for _, key := range keys[len(keys)-remove:] {
				delete(current, key)
			}
			addFact(facts, path, "object", len(current), remove, "byte_budget")
			return true
		}
	case []any:
		if len(current) > 0 {
			last := len(current) - 1
			child := current[last]
			if isJSONCollection(child) && shrinkJSON(&child, indexPath(path, last), facts) {
				current[last] = child
				return true
			}
		}
		if len(current) > 0 {
			remove := max(1, len(current)/2)
			kept := len(current) - remove
			clear(current[kept:])
			*value = current[:kept]
			addFact(facts, path, "array", kept, remove, "byte_budget")
			return true
		}
	case string:
		const marker = "[truncated]"
		if len(current) > len(marker)+1 {
			kept := (len(current) - len(marker)) / 2
			*value = current[:kept] + marker
			addFact(facts, path, "string", kept, len(current)-kept, "byte_budget")
			return true
		}
	}
	return false
}

func isJSONCollection(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func addFact(facts map[string]*TruncationEntry, path, kind string, returned, omitted int, reason string) {
	key := path + "\x00" + reason
	if fact := facts[key]; fact != nil {
		fact.ReturnedCount = returned
		fact.OmittedCount += omitted
		return
	}
	if len(facts) >= maxTruncationFacts {
		return
	}
	facts[key] = &TruncationEntry{Path: path, Kind: kind, ReturnedCount: returned, OmittedCount: omitted, Reason: reason}
}

func truncationFrom(facts map[string]*TruncationEntry) *Truncation {
	if len(facts) == 0 {
		return nil
	}
	entries := make([]TruncationEntry, 0, len(facts))
	for _, fact := range facts {
		entries = append(entries, *fact)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].Reason < entries[j].Reason
		}
		return entries[i].Path < entries[j].Path
	})
	return &Truncation{Entries: entries}
}

func encodedSize(data any, truncation *Truncation) int {
	encoded, err := json.Marshal(struct {
		Data       any         `json:"data"`
		Truncation *Truncation `json:"truncation,omitempty"`
	}{Data: data, Truncation: truncation})
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return len(encoded)
}

func sensitiveJSONKey(key string) bool {
	lower := strings.ToLower(key)
	for _, marker := range []string{"password", "secret", "token", "authorization", "credential"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func childPath(parent, key string) string { return parent + "." + key }
func indexPath(parent string, index int) string {
	return fmt.Sprintf("%s[%d]", parent, index)
}
func jsonKind(value any) string {
	if value == nil {
		return "scalar"
	}
	current := reflect.ValueOf(value)
	for current.IsValid() && (current.Kind() == reflect.Pointer || current.Kind() == reflect.Interface) {
		if current.IsNil() {
			return "scalar"
		}
		current = current.Elem()
	}
	if !current.IsValid() {
		return "scalar"
	}
	switch current.Kind() {
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Array, reflect.Slice:
		return "array"
	default:
		return "scalar"
	}
}
