package notification

import "fmt"

// VariableField defines a single variable in a notification template.
type VariableField struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "string", "int", "float", "bool", "datetime", "json"
	Required  bool   `json:"required"`
	Sensitive bool   `json:"sensitive,omitempty"` // redact from logs/audit
	Ephemeral bool   `json:"ephemeral,omitempty"` // never persist to variables_json
	Secret    bool   `json:"secret,omitempty"`    // alias for sensitive+ephemeral
}

// Validate checks the variable field definition.
func (v VariableField) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("notification: variable name is required")
	}
	switch v.Type {
	case "string", "int", "float", "bool", "datetime", "json":
		// ok
	default:
		return fmt.Errorf("notification: variable %q has unknown type %q", v.Name, v.Type)
	}
	return nil
}

// VariableSchema describes the variables a notification definition accepts.
type VariableSchema struct {
	Fields []VariableField `json:"fields"`
}

// Validate checks all fields in the schema.
func (s VariableSchema) Validate() error {
	seen := make(map[string]bool)
	for _, f := range s.Fields {
		if err := f.Validate(); err != nil {
			return err
		}
		if seen[f.Name] {
			return fmt.Errorf("notification: duplicate variable %q", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}

// RequiredFields returns the names of all required fields.
func (s VariableSchema) RequiredFields() []string {
	var out []string
	for _, f := range s.Fields {
		if f.Required {
			out = append(out, f.Name)
		}
	}
	return out
}

// EphemeralFields returns the names of fields that must not be persisted.
func (s VariableSchema) EphemeralFields() []string {
	var out []string
	for _, f := range s.Fields {
		if f.Ephemeral || f.Secret {
			out = append(out, f.Name)
		}
	}
	return out
}

// SensitiveFields returns the names of fields that should be redacted from logs/audit.
func (s VariableSchema) SensitiveFields() []string {
	var out []string
	for _, f := range s.Fields {
		if f.Sensitive || f.Secret {
			out = append(out, f.Name)
		}
	}
	return out
}
