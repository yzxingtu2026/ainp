package notification

import (
	"context"
	"fmt"
	"strings"
)

// TemplateOverride represents a tenant-level template override loaded from DB.
type TemplateOverride struct {
	TenantID      string `json:"tenant_id"`
	DefinitionKey string `json:"definition_key"`
	Channel       string `json:"channel"`
	Locale        string `json:"locale"`
	Version       int    `json:"version"`
	Subject       string `json:"subject,omitempty"`
	Body          string `json:"body,omitempty"`
	ConfigJSON    string `json:"config_json,omitempty"` // channel-specific JSON config
	Enabled       bool   `json:"enabled"`
}

// OverrideProvider loads template overrides from storage.
// Injected by the module layer to avoid framework importing modules.
type OverrideProvider interface {
	// GetOverride returns the override for the given tenant/definition/channel/locale.
	// Returns nil if no override exists.
	GetOverride(ctx context.Context, tenantID, definitionKey, channel, locale string) (*TemplateOverride, error)
}

// TemplateResolver resolves templates using three-tier priority:
// 1. Tenant override (from DB)
// 2. Code default template
// 3. Locale fallback (strip region, e.g. "zh-CN" → "zh")
type TemplateResolver struct {
	renderer  *Renderer
	overrides OverrideProvider
}

// NewTemplateResolver creates a resolver with the given renderer and override provider.
// OverrideProvider can be nil if no tenant overrides are configured.
func NewTemplateResolver(renderer *Renderer, overrides OverrideProvider) *TemplateResolver {
	return &TemplateResolver{
		renderer:  renderer,
		overrides: overrides,
	}
}

// ResolveResult holds the resolved template content ready for rendering.
type ResolveResult struct {
	Channel    Channel
	Locale     string
	Source     string // "override", "default", "fallback"
	Version    int
	Subject    string
	Body       string
	InApp      *InAppTemplate
	Email      *EmailTemplate
	SMS        *SMSTemplate
	Webhook    *WebhookTemplate
	IM         *IMTemplate
	ConfigJSON string
}

// Resolve finds the best template for the given definition, channel, locale, and tenant.
func (r *TemplateResolver) Resolve(ctx context.Context, tenantID string, def Definition, ch Channel, locale string) (*ResolveResult, error) {
	// 1. Try tenant override
	if r.overrides != nil && tenantID != "" {
		override, err := r.overrides.GetOverride(ctx, tenantID, def.Key, string(ch), locale)
		if err != nil {
			return nil, fmt.Errorf("notification: override lookup failed: %w", err)
		}
		if override != nil && override.Enabled {
			return r.buildFromOverride(ch, locale, override, def), nil
		}
	}

	// 2. Try code default with exact locale
	if def.Templates != nil {
		if ct, ok := def.Templates.GetVariant(ch, locale); ok {
			return r.buildFromCodeDefault(ch, locale, ct, def), nil
		}
	}

	// 3. Try locale fallback (strip region: "zh-CN" → "zh", "en-US" → "en")
	fallbackLocale := stripLocaleRegion(locale)
	if fallbackLocale != locale && def.Templates != nil {
		if ct, ok := def.Templates.GetVariant(ch, fallbackLocale); ok {
			return r.buildFromCodeDefault(ch, fallbackLocale, ct, def), nil
		}
	}

	return nil, fmt.Errorf("notification: no template found for definition=%q channel=%q locale=%q", def.Key, ch, locale)
}

func (r *TemplateResolver) buildFromOverride(ch Channel, locale string, o *TemplateOverride, def Definition) *ResolveResult {
	result := &ResolveResult{
		Channel:    ch,
		Locale:     locale,
		Source:     "override",
		Version:    o.Version,
		Subject:    o.Subject,
		Body:       o.Body,
		ConfigJSON: o.ConfigJSON,
	}
	// For in-app, also try to get the code default for channel-specific fields
	if def.Templates != nil {
		if ct, ok := def.Templates.GetVariant(ch, locale); ok {
			result.InApp = ct.InApp
			result.Email = ct.Email
			result.SMS = ct.SMS
			result.Webhook = ct.Webhook
			result.IM = ct.IM
		}
	}
	return result
}

func (r *TemplateResolver) buildFromCodeDefault(ch Channel, locale string, ct ChannelTemplate, def Definition) *ResolveResult {
	return &ResolveResult{
		Channel: ch,
		Locale:  locale,
		Source:  "default",
		Version: def.Templates.Version,
		Subject: ct.Subject,
		Body:    ct.Body,
		InApp:   ct.InApp,
		Email:   ct.Email,
		SMS:     ct.SMS,
		Webhook: ct.Webhook,
		IM:      ct.IM,
	}
}

// RenderResult holds rendered strings for all applicable fields.
type RenderResult struct {
	Subject   string
	Body      string
	Title     string
	Summary   string
	Icon      string
	LocalIcon string
	Action    *ActionDescriptor

	// Email
	TextBody string
	HTMLBody string

	// SMS
	SMSText string

	// Webhook
	WebhookPayload string

	// IM
	IMText     string
	IMCardJSON string
}

// Render renders a resolved template with the given variables.
func (r *TemplateResolver) Render(result *ResolveResult, variables map[string]any) (*RenderResult, error) {
	out := &RenderResult{
		Icon:      "",
		LocalIcon: "",
	}

	// Render common fields
	if result.Subject != "" {
		rendered, err := r.renderer.Render(result.Subject, variables, false)
		if err != nil {
			return nil, fmt.Errorf("notification: render subject: %w", err)
		}
		out.Subject = rendered
	}
	if result.Body != "" {
		rendered, err := r.renderer.Render(result.Body, variables, true)
		if err != nil {
			return nil, fmt.Errorf("notification: render body: %w", err)
		}
		out.Body = rendered
	}

	// Render channel-specific fields
	if result.InApp != nil {
		if result.InApp.Title != "" {
			rendered, err := r.renderer.Render(result.InApp.Title, variables, true)
			if err != nil {
				return nil, fmt.Errorf("notification: render in_app title: %w", err)
			}
			out.Title = rendered
		}
		if result.InApp.Summary != "" {
			rendered, err := r.renderer.Render(result.InApp.Summary, variables, true)
			if err != nil {
				return nil, fmt.Errorf("notification: render in_app summary: %w", err)
			}
			out.Summary = rendered
		}
		out.Icon = result.InApp.Icon
		out.LocalIcon = result.InApp.LocalIcon
		out.Action = result.InApp.Action
	}

	if result.Email != nil {
		if result.Email.Subject != "" {
			rendered, err := r.renderer.Render(result.Email.Subject, variables, false)
			if err != nil {
				return nil, fmt.Errorf("notification: render email subject: %w", err)
			}
			out.Subject = rendered // email subject overrides common subject
		}
		if result.Email.TextBody != "" {
			rendered, err := r.renderer.Render(result.Email.TextBody, variables, false)
			if err != nil {
				return nil, fmt.Errorf("notification: render email text: %w", err)
			}
			out.TextBody = rendered
		}
		if result.Email.HTMLBody != "" {
			rendered, err := r.renderer.Render(result.Email.HTMLBody, variables, true)
			if err != nil {
				return nil, fmt.Errorf("notification: render email html: %w", err)
			}
			out.HTMLBody = rendered
		}
	}

	if result.SMS != nil && result.SMS.Text != "" {
		rendered, err := r.renderer.Render(result.SMS.Text, variables, false)
		if err != nil {
			return nil, fmt.Errorf("notification: render sms text: %w", err)
		}
		out.SMSText = rendered
	}

	if result.Webhook != nil && result.Webhook.PayloadTpl != "" {
		rendered, err := r.renderer.RenderJSON(result.Webhook.PayloadTpl, variables)
		if err != nil {
			return nil, fmt.Errorf("notification: render webhook payload: %w", err)
		}
		out.WebhookPayload = rendered
	}

	if result.IM != nil {
		if result.IM.Text != "" {
			rendered, err := r.renderer.Render(result.IM.Text, variables, false)
			if err != nil {
				return nil, fmt.Errorf("notification: render im text: %w", err)
			}
			out.IMText = rendered
		}
		out.IMCardJSON = result.IM.CardJSON
	}

	return out, nil
}

// stripLocaleRegion strips the region part from a locale string.
// e.g. "zh-CN" → "zh", "en-US" → "en", "en" → "en"
func stripLocaleRegion(locale string) string {
	if idx := strings.IndexAny(locale, "-_"); idx > 0 {
		return locale[:idx]
	}
	return locale
}
