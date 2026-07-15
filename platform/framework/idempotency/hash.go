package idempotency

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// HashRequest computes a stable SHA-256 hash of the request input.
// For JSON objects, keys are sorted to ensure deterministic output.
func HashRequest(input json.RawMessage) string {
	if len(input) == 0 {
		return sha256Hex([]byte("{}"))
	}
	// Try to normalize by re-marshalling with sorted keys.
	var obj any
	if err := json.Unmarshal(input, &obj); err != nil {
		// Fallback: hash raw bytes.
		return sha256Hex(input)
	}
	normalized, err := json.Marshal(sortJSON(obj))
	if err != nil {
		return sha256Hex(input)
	}
	return sha256Hex(normalized)
}

// HashSummary returns the first 16 characters of a hash for safe logging.
func HashSummary(hash string) string {
	if len(hash) <= 16 {
		return hash
	}
	return hash[:16]
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// sortJSON recursively sorts map keys for deterministic serialization.
func sortJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sorted := make(map[string]any, len(val))
		for _, k := range keys {
			sorted[k] = sortJSON(val[k])
		}
		return sorted
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = sortJSON(item)
		}
		return out
	default:
		return v
	}
}
