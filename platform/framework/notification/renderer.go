package notification

import (
	"bytes"
	"fmt"
	"html"
	"strings"
	"text/template"
)

// Renderer renders a template string with variables using a controlled function whitelist.
// Missing variables cause an error; no arbitrary code execution is allowed.
type Renderer struct {
	funcMap template.FuncMap
}

// NewRenderer creates a Renderer with the safe function whitelist.
func NewRenderer() *Renderer {
	return &Renderer{
		funcMap: safeFuncMap(),
	}
}

// Render executes a Go text/template with the given variables.
// It uses "missingkey=error" to fail on missing variables instead of outputting <no value>.
// For HTML output, auto-escaping is applied.
func (r *Renderer) Render(tpl string, variables map[string]any, escapeHTML bool) (string, error) {
	t, err := template.New("notification").
		Option("missingkey=error").
		Funcs(r.funcMap).
		Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("notification: template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, variables); err != nil {
		return "", fmt.Errorf("notification: template render error: %w", err)
	}

	result := buf.String()
	if escapeHTML {
		result = html.EscapeString(result)
	}
	return result, nil
}

// RenderJSON renders a template and validates the output is valid JSON.
func (r *Renderer) RenderJSON(tpl string, variables map[string]any) (string, error) {
	result, err := r.Render(tpl, variables, false)
	if err != nil {
		return "", err
	}
	// Basic JSON validation
	trimmed := strings.TrimSpace(result)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return "", fmt.Errorf("notification: rendered webhook payload is not valid JSON object")
	}
	return result, nil
}

// ValidateVariables checks that all required fields in the schema are present in variables.
func ValidateVariables(schema VariableSchema, variables map[string]any) error {
	for _, name := range schema.RequiredFields() {
		if _, ok := variables[name]; !ok {
			return fmt.Errorf("notification: required variable %q is missing", name)
		}
	}
	return nil
}

// SanitizeVariables removes ephemeral/secret fields from variables for persistence.
func SanitizeVariables(schema VariableSchema, variables map[string]any) map[string]any {
	ephemeral := make(map[string]bool)
	for _, name := range schema.EphemeralFields() {
		ephemeral[name] = true
	}
	if len(ephemeral) == 0 {
		return variables
	}
	out := make(map[string]any, len(variables))
	for k, v := range variables {
		if !ephemeral[k] {
			out[k] = v
		}
	}
	return out
}

// RedactVariables replaces sensitive field values with "***" for logging.
func RedactVariables(schema VariableSchema, variables map[string]any) map[string]any {
	sensitive := make(map[string]bool)
	for _, name := range schema.SensitiveFields() {
		sensitive[name] = true
	}
	if len(sensitive) == 0 {
		return variables
	}
	out := make(map[string]any, len(variables))
	for k, v := range variables {
		if sensitive[k] {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}
