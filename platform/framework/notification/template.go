package notification

import "fmt"

// TemplateVariant identifies a channel+locale combination for a template.
type TemplateVariant struct {
	Channel Channel `json:"channel"`
	Locale  string  `json:"locale"` // e.g. "zh-CN", "en"
}

func (v TemplateVariant) Key() string {
	return string(v.Channel) + ":" + v.Locale
}

// InAppTemplate holds in-app specific template fields.
type InAppTemplate struct {
	Title     string            `json:"title"`
	Summary   string            `json:"summary,omitempty"`
	Body      string            `json:"body,omitempty"`
	Icon      string            `json:"icon,omitempty"`       // Iconify collection:name
	LocalIcon string            `json:"local_icon,omitempty"` // custom SVG
	Action    *ActionDescriptor `json:"action,omitempty"`
}

// EmailTemplate holds email specific template fields.
type EmailTemplate struct {
	Subject  string `json:"subject"`
	TextBody string `json:"text_body"`
	HTMLBody string `json:"html_body,omitempty"`
}

// SMSTemplate holds SMS specific template fields.
type SMSTemplate struct {
	Text         string `json:"text"`
	ProviderCode string `json:"provider_code,omitempty"` // supplier template code
}

// WebhookTemplate holds webhook specific template fields.
type WebhookTemplate struct {
	Version    string `json:"version"`
	PayloadTpl string `json:"payload_tpl"` // JSON template string
}

// IMTemplate holds IM specific template fields.
type IMTemplate struct {
	Text     string `json:"text"`
	CardJSON string `json:"card_json,omitempty"` // controlled markdown/card extension
}

// TemplateDefinition holds templates for all channels of a definition.
// Each channel's template is keyed by locale.
type TemplateDefinition struct {
	Version  int                        `json:"version"`  // template_version
	Variants map[string]ChannelTemplate `json:"variants"` // key = "channel:locale"
}

// ChannelTemplate holds the template for a specific channel+locale.
type ChannelTemplate struct {
	Channel Channel `json:"channel"`
	Locale  string  `json:"locale"`

	// Common fields (used by all channels)
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`

	// Channel-specific fields
	InApp   *InAppTemplate   `json:"in_app,omitempty"`
	Email   *EmailTemplate   `json:"email,omitempty"`
	SMS     *SMSTemplate     `json:"sms,omitempty"`
	Webhook *WebhookTemplate `json:"webhook,omitempty"`
	IM      *IMTemplate      `json:"im,omitempty"`
}

// Validate checks that the channel template is valid for its channel type.
func (t ChannelTemplate) Validate() error {
	if !t.Channel.Valid() {
		return fmt.Errorf("notification: template channel %q is invalid", t.Channel)
	}
	if t.Locale == "" {
		return fmt.Errorf("notification: template locale is required")
	}
	switch t.Channel {
	case ChannelInApp:
		if t.InApp == nil {
			return fmt.Errorf("notification: in_app template is required for channel in_app")
		}
		if t.InApp.Title == "" {
			return fmt.Errorf("notification: in_app title is required")
		}
		if t.InApp.Icon != "" && !isValidIconify(t.InApp.Icon) {
			return fmt.Errorf("notification: in_app icon must be Iconify collection:name format")
		}
		if t.InApp.Action != nil {
			if err := t.InApp.Action.Validate(); err != nil {
				return err
			}
		}
	case ChannelEmail:
		if t.Email == nil {
			return fmt.Errorf("notification: email template is required for channel email")
		}
		if t.Email.Subject == "" {
			return fmt.Errorf("notification: email subject is required")
		}
	case ChannelSMS:
		if t.SMS == nil {
			return fmt.Errorf("notification: sms template is required for channel sms")
		}
		if t.SMS.Text == "" {
			return fmt.Errorf("notification: sms text is required")
		}
	case ChannelWebhook:
		if t.Webhook == nil {
			return fmt.Errorf("notification: webhook template is required for channel webhook")
		}
	case ChannelIM:
		if t.IM == nil {
			return fmt.Errorf("notification: im template is required for channel im")
		}
	}
	return nil
}

// isValidIconify checks basic Iconify format "collection:name".
func isValidIconify(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c == ':' && i > 0 && i < len(s)-1 {
			return true
		}
	}
	return false
}

// NewTemplateDefinition creates a new TemplateDefinition with initialized map.
func NewTemplateDefinition(version int) *TemplateDefinition {
	return &TemplateDefinition{
		Version:  version,
		Variants: make(map[string]ChannelTemplate),
	}
}

// AddVariant registers a channel+locale template variant.
func (td *TemplateDefinition) AddVariant(ct ChannelTemplate) error {
	if err := ct.Validate(); err != nil {
		return err
	}
	key := string(ct.Channel) + ":" + ct.Locale
	td.Variants[key] = ct
	return nil
}

// GetVariant retrieves a template for the given channel and locale.
func (td *TemplateDefinition) GetVariant(ch Channel, locale string) (ChannelTemplate, bool) {
	key := string(ch) + ":" + locale
	ct, ok := td.Variants[key]
	return ct, ok
}
