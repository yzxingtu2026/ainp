package notification

import (
	"context"
	"encoding/json"
	"time"
)

// ChannelProvider is the interface for sending notifications through a specific channel.
// Implementations must be idempotent when receiving the same delivery_id.
// Provider-specific config, error codes, and message IDs are encapsulated in the adapter/receipt.
type ChannelProvider interface {
	// Name returns the provider identifier, e.g. "in_app", "smtp", "aliyun_sms".
	Name() string

	// Channel returns the channel this provider handles.
	Channel() Channel

	// Send attempts to deliver a notification and returns a receipt.
	// The rendered content is provided as a generic map to keep the interface channel-agnostic.
	Send(ctx context.Context, input ProviderInput) (*ProviderReceipt, error)
}

// ProviderInput is the channel-agnostic input to a ChannelProvider.Send call.
type ProviderInput struct {
	DeliveryID     string          `json:"delivery_id"`
	RequestID      string          `json:"request_id"`
	DefinitionKey  string          `json:"definition_key"`
	RecipientType  RecipientType   `json:"recipient_type"`
	RecipientID    string          `json:"recipient_id"`
	Channel        Channel         `json:"channel"`
	Locale         string          `json:"locale"`
	Variables      map[string]any  `json:"variables"` // may include ephemeral for rendering
	Content        RenderedContent `json:"content"`
	IdempotencyKey string          `json:"idempotency_key"`
}

// RenderedContent holds the rendered output for a specific channel.
type RenderedContent struct {
	// Common
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`

	// InApp specific
	Title     string            `json:"title,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	Icon      string            `json:"icon,omitempty"`
	LocalIcon string            `json:"local_icon,omitempty"`
	Action    *ActionDescriptor `json:"action,omitempty"`

	// Email specific
	TextBody string `json:"text_body,omitempty"`
	HTMLBody string `json:"html_body,omitempty"`

	// SMS specific
	SMSText         string `json:"sms_text,omitempty"`
	SMSProviderCode string `json:"sms_provider_code,omitempty"`

	// Webhook specific
	WebhookPayload string `json:"webhook_payload,omitempty"`

	// IM specific
	IMText     string `json:"im_text,omitempty"`
	IMCardJSON string `json:"im_card_json,omitempty"`
}

// ProviderError represents a provider-specific error with retryable semantics.
type ProviderError struct {
	Code       string    `json:"code"`
	Message    string    `json:"message"`
	Retryable  bool      `json:"retryable"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e *ProviderError) Error() string {
	return e.Message
}

// NewRetryableError creates a retryable provider error.
func NewRetryableError(code, message string) *ProviderError {
	return &ProviderError{Code: code, Message: message, Retryable: true, OccurredAt: time.Now().UTC()}
}

// NewPermanentError creates a permanent (non-retryable) provider error.
func NewPermanentError(code, message string) *ProviderError {
	return &ProviderError{Code: code, Message: message, Retryable: false, OccurredAt: time.Now().UTC()}
}

// IsRetryable checks if a provider error is retryable.
func IsRetryable(err error) bool {
	if pe, ok := err.(*ProviderError); ok {
		return pe.Retryable
	}
	return false
}

// MarshalContent serializes RenderedContent to JSON.
func (c RenderedContent) Marshal() (json.RawMessage, error) {
	return json.Marshal(c)
}
