package logcenter

import (
	"encoding/json"
	"regexp"
	"strings"
)

const MaxMessageLength = 4096
const MaxAttributeLength = 1024
const Redacted = "[REDACTED]"

var sensitiveKey = regexp.MustCompile(`(?i)(password|passwd|token|authorization|api[_-]?key|secret|credential|connection[_-]?string|dsn|id[_-]?card|bank[_-]?card)`)
var sensitiveValue = regexp.MustCompile(`(?i)(authorization:\s*(?:bearer\s+)?\S+|bearer\s+[A-Za-z0-9._~+/=-]+|password\s*[=:]\s*\S+|api[_-]?key\s*[=:]\s*\S+)`)

func Sanitize(entry Entry) Entry {
	entry.Message = truncate(sensitiveValue.ReplaceAllString(entry.Message, Redacted), MaxMessageLength)
	entry.Attributes = sanitizeMap(entry.Attributes)
	return entry
}

func sanitizeMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveKey.MatchString(key) {
			out[key] = Redacted
			continue
		}
		out[key] = sanitizeValue(value)
	}
	return out
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case string:
		return truncate(sensitiveValue.ReplaceAllString(typed, Redacted), MaxAttributeLength)
	case map[string]any:
		return sanitizeMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = sanitizeValue(typed[i])
		}
		return out
	default:
		encoded, _ := json.Marshal(value)
		if len(encoded) > MaxAttributeLength {
			return truncate(string(encoded), MaxAttributeLength)
		}
		return value
	}
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "...[truncated]"
}
