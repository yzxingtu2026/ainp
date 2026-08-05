package notification

import (
	"fmt"
	"time"
)

// Channel represents a notification delivery channel.
type Channel string

const (
	ChannelInApp   Channel = "in_app"
	ChannelEmail   Channel = "email"
	ChannelSMS     Channel = "sms"
	ChannelWebhook Channel = "webhook"
	ChannelIM      Channel = "im"
)

// Valid returns true if the channel is a known channel type.
func (c Channel) Valid() bool {
	switch c {
	case ChannelInApp, ChannelEmail, ChannelSMS, ChannelWebhook, ChannelIM:
		return true
	default:
		return false
	}
}

// AllChannels returns all known channels.
func AllChannels() []Channel {
	return []Channel{ChannelInApp, ChannelEmail, ChannelSMS, ChannelWebhook, ChannelIM}
}

// Priority represents notification priority.
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

func (p Priority) Valid() bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical:
		return true
	default:
		return false
	}
}

// Surface represents where the notification applies.
type Surface string

const (
	SurfaceTenant   Surface = "tenant"
	SurfacePlatform Surface = "platform"
)

func (s Surface) Valid() bool {
	return s == SurfaceTenant || s == SurfacePlatform
}

// RecipientType classifies the recipient.
type RecipientType string

const (
	RecipientUser RecipientType = "user"
	RecipientRole RecipientType = "role"
)

func (r RecipientType) Valid() bool {
	return r == RecipientUser || r == RecipientRole
}

// RecipientSpec identifies a notification recipient.
type RecipientSpec struct {
	Type RecipientType `json:"type"`
	ID   string        `json:"id"` // account_id or role_code
}

func (r RecipientSpec) Validate() error {
	if !r.Type.Valid() {
		return fmt.Errorf("notification: invalid recipient type %q", r.Type)
	}
	if r.ID == "" {
		return fmt.Errorf("notification: recipient id is required")
	}
	return nil
}

// ProviderReceipt is returned by a ChannelProvider after attempting delivery.
type ProviderReceipt struct {
	Provider   string    `json:"provider"`    // e.g. "in_app", "smtp", "aliyun_sms"
	ExternalID string    `json:"external_id"` // provider-specific message ID
	AcceptedAt time.Time `json:"accepted_at"` // when provider accepted
	Retryable  bool      `json:"retryable"`   // true if failure is transient
}

// ActionDescriptor describes a controlled navigation action for in-app notifications.
// Only internal routes or known action IDs are allowed; never arbitrary URLs or javascript:.
type ActionDescriptor struct {
	Type   string `json:"type"`            // "route" | "action"
	Target string `json:"target"`          // e.g. "/orders/123" or "order.view"
	Label  string `json:"label,omitempty"` // display label
}

// Validate checks that the action descriptor is safe.
func (a ActionDescriptor) Validate() error {
	if a.Type != "route" && a.Type != "action" {
		return fmt.Errorf("notification: action type must be 'route' or 'action', got %q", a.Type)
	}
	if a.Target == "" {
		return fmt.Errorf("notification: action target is required")
	}
	// Reject dangerous schemes
	if len(a.Target) > 11 && a.Target[:11] == "javascript:" {
		return fmt.Errorf("notification: javascript: URLs are forbidden in action target")
	}
	// Reject external URLs (http/https)
	if len(a.Target) > 7 && (a.Target[:7] == "http://" || a.Target[:8] == "https://") {
		return fmt.Errorf("notification: external URLs are forbidden in action target")
	}
	return nil
}
