package setting

import (
	"fmt"
	"regexp"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type FieldType string

const (
	FieldText        FieldType = "text"
	FieldTextarea    FieldType = "textarea"
	FieldNumber      FieldType = "number"
	FieldBoolean     FieldType = "boolean"
	FieldSelect      FieldType = "select"
	FieldMultiSelect FieldType = "multi_select"
	FieldColor       FieldType = "color"
)

func (t FieldType) Valid() bool {
	switch t {
	case FieldText, FieldTextarea, FieldNumber, FieldBoolean, FieldSelect, FieldMultiSelect, FieldColor:
		return true
	default:
		return false
	}
}

type Scope string

const (
	ScopeTenant   Scope = "tenant"
	ScopePlatform Scope = "platform" // reserved; not enabled in Phase 1
)

func (s Scope) Valid() bool {
	return s == ScopeTenant || s == ScopePlatform
}

var (
	keyPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	fieldKeyPat = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	iconPattern = regexp.MustCompile(`^[a-z0-9-]+:[a-z0-9][a-z0-9-]*$`)
	colorPat    = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
)

// ValidIconifyIcon reports whether icon is collection:name (Iconify).
func ValidIconifyIcon(icon string) bool {
	return iconPattern.MatchString(icon)
}

type FieldOption struct {
	Value    string `json:"value"`
	Label    string `json:"label,omitempty"`
	LabelKey string `json:"label_key,omitempty"`
}

// VisibleWhen declares declarative sibling-field visibility (no expression language).
// Exactly one of Eq or In must be set. Field must reference another field in the same Definition.
type VisibleWhen struct {
	Field string `json:"field"`
	Eq    any    `json:"eq,omitempty"`
	In    []any  `json:"in,omitempty"`
}

type Field struct {
	Key                string        `json:"key"`
	Type               FieldType     `json:"type"`
	Label              string        `json:"label,omitempty"`
	LabelKey           string        `json:"label_key,omitempty"`
	Description        string        `json:"description,omitempty"`
	DescriptionKey     string        `json:"description_key,omitempty"`
	Required           bool          `json:"required"`
	Default            any           `json:"default,omitempty"`
	MinLength          *int          `json:"min_length,omitempty"`
	MaxLength          *int          `json:"max_length,omitempty"`
	Min                *float64      `json:"min,omitempty"`
	Max                *float64      `json:"max,omitempty"`
	Step               *float64      `json:"step,omitempty"`
	Options            []FieldOption `json:"options,omitempty"`
	AllowCustomValues  bool          `json:"allow_custom_values,omitempty"`
	CustomValuePattern string        `json:"custom_value_pattern,omitempty"`
	VisibleWhen        *VisibleWhen  `json:"visible_when,omitempty"`
}

type Category struct {
	Key            string     `json:"key"`
	Title          string     `json:"title,omitempty"`
	TitleKey       string     `json:"title_key,omitempty"`
	Description    string     `json:"description,omitempty"`
	DescriptionKey string     `json:"description_key,omitempty"`
	Icon           string     `json:"icon,omitempty"`
	LocalIcon      string     `json:"local_icon,omitempty"`
	Order          int        `json:"order"`
	Surface        auth.Realm `json:"surface"`
}

type Definition struct {
	Key            string     `json:"key"`
	CategoryKey    string     `json:"category_key"`
	Title          string     `json:"title,omitempty"`
	TitleKey       string     `json:"title_key,omitempty"`
	Description    string     `json:"description,omitempty"`
	DescriptionKey string     `json:"description_key,omitempty"`
	Scope          Scope      `json:"scope"`
	Surface        auth.Realm `json:"surface"`
	Permission     string     `json:"permission,omitempty"`
	Order          int        `json:"order"`
	SchemaVersion  int        `json:"schema_version"`
	Fields         []Field    `json:"fields"`
}

func ValidateCategory(c Category) error {
	if !keyPattern.MatchString(c.Key) {
		return fmt.Errorf("invalid setting category key: %s", c.Key)
	}
	if c.Title == "" && c.TitleKey == "" {
		return fmt.Errorf("setting category title or title_key required: %s", c.Key)
	}
	if c.Order < 0 {
		return fmt.Errorf("setting category order must be non-negative: %s", c.Key)
	}
	surface := auth.NormalizeRealm(c.Surface)
	if !surface.Valid() {
		return fmt.Errorf("invalid setting category surface: %s", c.Surface)
	}
	if c.Icon == "" && c.LocalIcon == "" {
		return fmt.Errorf("setting category icon or local_icon required: %s", c.Key)
	}
	if c.Icon != "" && !ValidIconifyIcon(c.Icon) {
		return fmt.Errorf("invalid setting category icon %q: %s", c.Icon, c.Key)
	}
	return nil
}

func ValidateDefinition(def Definition, categories map[string]Category) error {
	if !keyPattern.MatchString(def.Key) || !stringsContainsDot(def.Key) {
		return fmt.Errorf("invalid setting definition key: %s", def.Key)
	}
	if def.CategoryKey == "" {
		return fmt.Errorf("setting definition category_key required: %s", def.Key)
	}
	if _, ok := categories[def.CategoryKey]; !ok {
		return fmt.Errorf("setting definition references unknown category %q: %s", def.CategoryKey, def.Key)
	}
	if def.Title == "" && def.TitleKey == "" {
		return fmt.Errorf("setting definition title or title_key required: %s", def.Key)
	}
	if def.SchemaVersion < 1 {
		return fmt.Errorf("setting definition schema_version must be >= 1: %s", def.Key)
	}
	if def.Order < 0 {
		return fmt.Errorf("setting definition order must be non-negative: %s", def.Key)
	}
	scope := def.Scope
	if scope == "" {
		scope = ScopeTenant
	}
	if !scope.Valid() {
		return fmt.Errorf("invalid setting definition scope: %s", def.Scope)
	}
	// Phase 1 only enables tenant scope/surface registrations.
	if scope != ScopeTenant {
		return fmt.Errorf("setting definition scope %q is not enabled in phase 1: %s", scope, def.Key)
	}
	surface := auth.NormalizeRealm(def.Surface)
	if !surface.Valid() {
		return fmt.Errorf("invalid setting definition surface: %s", def.Surface)
	}
	if surface != auth.RealmTenant {
		return fmt.Errorf("setting definition surface %q is not enabled in phase 1: %s", surface, def.Key)
	}
	if len(def.Fields) == 0 {
		return fmt.Errorf("setting definition fields required: %s", def.Key)
	}
	seen := make(map[string]struct{}, len(def.Fields))
	for i := range def.Fields {
		if err := validateField(def.Key, &def.Fields[i]); err != nil {
			return err
		}
		if _, ok := seen[def.Fields[i].Key]; ok {
			return fmt.Errorf("duplicate field key %q in %s", def.Fields[i].Key, def.Key)
		}
		seen[def.Fields[i].Key] = struct{}{}
	}
	return validateVisibleWhenGraph(def, seen)
}

func stringsContainsDot(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return true
		}
	}
	return false
}

func validateField(defKey string, field *Field) error {
	if !fieldKeyPat.MatchString(field.Key) {
		return fmt.Errorf("invalid field key %q in %s", field.Key, defKey)
	}
	if !field.Type.Valid() {
		return fmt.Errorf("invalid field type %q in %s.%s", field.Type, defKey, field.Key)
	}
	if field.Label == "" && field.LabelKey == "" {
		return fmt.Errorf("field label or label_key required: %s.%s", defKey, field.Key)
	}
	if field.AllowCustomValues && field.Type != FieldMultiSelect {
		return fmt.Errorf("allow_custom_values requires multi_select: %s.%s", defKey, field.Key)
	}
	if field.CustomValuePattern != "" {
		if !field.AllowCustomValues {
			return fmt.Errorf("custom_value_pattern requires allow_custom_values: %s.%s", defKey, field.Key)
		}
		if _, err := regexp.Compile(field.CustomValuePattern); err != nil {
			return fmt.Errorf("invalid custom_value_pattern in %s.%s: %w", defKey, field.Key, err)
		}
	}
	switch field.Type {
	case FieldSelect, FieldMultiSelect:
		if len(field.Options) == 0 {
			return fmt.Errorf("field options required: %s.%s", defKey, field.Key)
		}
		optSeen := make(map[string]struct{}, len(field.Options))
		for _, opt := range field.Options {
			if opt.Value == "" {
				return fmt.Errorf("empty option value in %s.%s", defKey, field.Key)
			}
			if _, ok := optSeen[opt.Value]; ok {
				return fmt.Errorf("duplicate option %q in %s.%s", opt.Value, defKey, field.Key)
			}
			optSeen[opt.Value] = struct{}{}
		}
	case FieldNumber:
		if field.Min != nil && field.Max != nil && *field.Min > *field.Max {
			return fmt.Errorf("field min > max in %s.%s", defKey, field.Key)
		}
	case FieldText, FieldTextarea:
		if field.MinLength != nil && field.MaxLength != nil && *field.MinLength > *field.MaxLength {
			return fmt.Errorf("field min_length > max_length in %s.%s", defKey, field.Key)
		}
	}
	if field.Default != nil {
		if err := validateFieldValue(defKey, *field, field.Default, true); err != nil {
			return fmt.Errorf("invalid default for %s.%s: %w", defKey, field.Key, err)
		}
	} else if field.Required {
		return fmt.Errorf("required field must declare default: %s.%s", defKey, field.Key)
	}
	return nil
}
