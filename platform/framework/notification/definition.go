package notification

import "fmt"

// Definition is the registration contract for a notification type.
// Definitions are code-level metadata; they are not created dynamically by tenants.
type Definition struct {
	Key             string              `json:"key"`             // stable name, e.g. "user.password_reset"
	Category        string              `json:"category"`        // grouping, e.g. "security", "approval"
	Surface         Surface             `json:"surface"`         // tenant or platform
	NameKey         string              `json:"name_key"`        // i18n key for display name
	DescriptionKey  string              `json:"description_key"` // i18n key for description
	Variables       VariableSchema      `json:"variables"`
	AllowedChannels []Channel           `json:"allowed_channels"` // channels this definition can deliver to
	DefaultChannels []Channel           `json:"default_channels"` // channels used when caller doesn't specify
	Mandatory       bool                `json:"mandatory"`        // bypasses user preferences (future)
	Priority        Priority            `json:"priority"`
	Templates       *TemplateDefinition `json:"templates"`
}

// Validate performs full validation of the definition at registration time.
func (d Definition) Validate() error {
	if d.Key == "" {
		return fmt.Errorf("notification: definition key is required")
	}
	if !isValidDefinitionKey(d.Key) {
		return fmt.Errorf("notification: definition key %q must be domain.verb format (e.g. user.password_reset)", d.Key)
	}
	if d.Category == "" {
		return fmt.Errorf("notification: definition %q category is required", d.Key)
	}
	if !d.Surface.Valid() {
		return fmt.Errorf("notification: definition %q surface must be tenant or platform", d.Key)
	}
	if d.NameKey == "" {
		return fmt.Errorf("notification: definition %q name_key is required", d.Key)
	}
	if !d.Priority.Valid() {
		return fmt.Errorf("notification: definition %q priority is invalid: %q", d.Key, d.Priority)
	}
	if err := d.Variables.Validate(); err != nil {
		return fmt.Errorf("notification: definition %q: %w", d.Key, err)
	}
	if len(d.AllowedChannels) == 0 {
		return fmt.Errorf("notification: definition %q must declare at least one allowed channel", d.Key)
	}
	for _, ch := range d.AllowedChannels {
		if !ch.Valid() {
			return fmt.Errorf("notification: definition %q has invalid allowed channel %q", d.Key, ch)
		}
	}
	if len(d.DefaultChannels) == 0 {
		return fmt.Errorf("notification: definition %q must declare at least one default channel", d.Key)
	}
	for _, ch := range d.DefaultChannels {
		if !ch.Valid() {
			return fmt.Errorf("notification: definition %q has invalid default channel %q", d.Key, ch)
		}
		if !d.isAllowed(ch) {
			return fmt.Errorf("notification: definition %q default channel %q is not in allowed channels", d.Key, ch)
		}
	}
	if d.Templates == nil {
		return fmt.Errorf("notification: definition %q templates are required", d.Key)
	}
	if len(d.Templates.Variants) == 0 {
		return fmt.Errorf("notification: definition %q must have at least one template variant", d.Key)
	}
	return nil
}

func (d Definition) isAllowed(ch Channel) bool {
	for _, c := range d.AllowedChannels {
		if c == ch {
			return true
		}
	}
	return false
}

// IsChannelAllowed checks if a channel is in the allowed list.
func (d Definition) IsChannelAllowed(ch Channel) bool {
	return d.isAllowed(ch)
}

// isValidDefinitionKey checks the domain.verb format.
func isValidDefinitionKey(key string) bool {
	dotFound := false
	for i, c := range key {
		if c == '.' {
			if i == 0 || i == len(key)-1 {
				return false
			}
			dotFound = true
		}
	}
	return dotFound
}
