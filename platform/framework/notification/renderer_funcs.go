package notification

import (
	"fmt"
	"strings"
	"text/template"
	"time"
)

// safeFuncMap returns the controlled whitelist of template functions.
// No function here can access DB, files, network, or environment variables.
func safeFuncMap() template.FuncMap {
	return template.FuncMap{
		// String manipulation
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,
		"truncate": truncateStr,
		"replace":  strings.ReplaceAll,
		"trim":     strings.TrimSpace,
		"join":     strings.Join,
		"split":    strings.Split,
		"contains": strings.Contains,

		// Date formatting
		"dateFormat": dateFormat,
		"now":        func() string { return time.Now().UTC().Format(time.RFC3339) },

		// Number formatting
		"currency": currencyFormat,

		// Logic helpers
		"default": defaultVal,
		"eq":      func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
		"ne":      func(a, b any) bool { return fmt.Sprint(a) != fmt.Sprint(b) },

		// Conditional
		"ternary": func(trueVal, falseVal any, cond bool) any {
			if cond {
				return trueVal
			}
			return falseVal
		},
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func dateFormat(layout, value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value // return as-is if not parseable
	}
	return t.Format(layout)
}

func currencyFormat(amount any, currency string) string {
	return fmt.Sprintf("%v %s", amount, currency)
}

func defaultVal(defaultValue, val any) any {
	if val == nil || val == "" {
		return defaultValue
	}
	return val
}
