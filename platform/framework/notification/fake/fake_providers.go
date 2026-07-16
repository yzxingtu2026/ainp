package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/notification"
)

// CallRecord captures a single Send invocation for test assertions.
type CallRecord struct {
	Input     notification.ProviderInput
	Timestamp time.Time
}

// baseProvider is the shared fake implementation.
type baseProvider struct {
	name    string
	channel notification.Channel
	mu      sync.Mutex
	Calls   []CallRecord
	// StubError, when set, makes Send return this error.
	StubError error
	// StubReceipt, when set, overrides the default receipt.
	StubReceipt *notification.ProviderReceipt
}

func (b *baseProvider) Name() string                  { return b.name }
func (b *baseProvider) Channel() notification.Channel { return b.channel }

func (b *baseProvider) Send(_ context.Context, input notification.ProviderInput) (*notification.ProviderReceipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Calls = append(b.Calls, CallRecord{Input: input, Timestamp: time.Now().UTC()})
	if b.StubError != nil {
		return nil, b.StubError
	}
	if b.StubReceipt != nil {
		return b.StubReceipt, nil
	}
	return &notification.ProviderReceipt{
		Provider:   b.name,
		ExternalID: fmt.Sprintf("fake-%s-%s", b.channel, input.DeliveryID),
		AcceptedAt: time.Now().UTC(),
		Retryable:  false,
	}, nil
}

// Reset clears recorded calls and stub overrides.
func (b *baseProvider) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Calls = nil
	b.StubError = nil
	b.StubReceipt = nil
}

// CallCount returns the number of recorded Send calls.
func (b *baseProvider) CallCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.Calls)
}

// LastCall returns the most recent CallRecord, or zero value if none.
func (b *baseProvider) LastCall() CallRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.Calls) == 0 {
		return CallRecord{}
	}
	return b.Calls[len(b.Calls)-1]
}

// EmailFakeProvider is a fake email provider for testing.
type EmailFakeProvider struct{ baseProvider }

func NewEmailFakeProvider() *EmailFakeProvider {
	return &EmailFakeProvider{baseProvider{name: "fake_email", channel: notification.ChannelEmail}}
}

// SmsFakeProvider is a fake SMS provider for testing.
type SmsFakeProvider struct{ baseProvider }

func NewSmsFakeProvider() *SmsFakeProvider {
	return &SmsFakeProvider{baseProvider{name: "fake_sms", channel: notification.ChannelSMS}}
}

// WebhookFakeProvider is a fake webhook provider for testing.
type WebhookFakeProvider struct{ baseProvider }

func NewWebhookFakeProvider() *WebhookFakeProvider {
	return &WebhookFakeProvider{baseProvider{name: "fake_webhook", channel: notification.ChannelWebhook}}
}

// IMFakeProvider is a fake IM provider for testing.
type IMFakeProvider struct{ baseProvider }

func NewIMFakeProvider() *IMFakeProvider {
	return &IMFakeProvider{baseProvider{name: "fake_im", channel: notification.ChannelIM}}
}

// Compile-time interface checks.
var (
	_ notification.ChannelProvider = (*EmailFakeProvider)(nil)
	_ notification.ChannelProvider = (*SmsFakeProvider)(nil)
	_ notification.ChannelProvider = (*WebhookFakeProvider)(nil)
	_ notification.ChannelProvider = (*IMFakeProvider)(nil)
)
