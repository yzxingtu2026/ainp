package tests

import (
	"context"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/framework/notification/fake"
)

// TestFakeProviderContract verifies that every fake provider satisfies
// the ChannelProvider interface contract:
//  1. Name() is non-empty
//  2. Channel() is a valid channel
//  3. Send() returns a valid ProviderReceipt with provider/external_id/accepted_at
//  4. Retryable/Permanent error semantics are consistent
func TestFakeProviderContract(t *testing.T) {
	providers := []notification.ChannelProvider{
		fake.NewEmailFakeProvider(),
		fake.NewSmsFakeProvider(),
		fake.NewWebhookFakeProvider(),
		fake.NewIMFakeProvider(),
	}

	expectedChannels := map[string]notification.Channel{
		"fake_email":   notification.ChannelEmail,
		"fake_sms":     notification.ChannelSMS,
		"fake_webhook": notification.ChannelWebhook,
		"fake_im":      notification.ChannelIM,
	}

	input := notification.ProviderInput{
		DeliveryID:    "test-delivery-001",
		RequestID:     "test-request-001",
		DefinitionKey: "test.notify",
		RecipientType: notification.RecipientUser,
		RecipientID:   "user-123",
		Channel:       notification.ChannelEmail,
		Locale:        "zh-CN",
		Variables:     map[string]any{"name": "test"},
		Content: notification.RenderedContent{
			Subject: "Test Subject",
			Body:    "Test Body",
		},
		IdempotencyKey: "test-idem-001",
	}

	for _, p := range providers {
		t.Run(p.Name(), func(t *testing.T) {
			// Contract: Name is non-empty
			if p.Name() == "" {
				t.Fatal("provider Name() must not be empty")
			}

			// Contract: Channel is valid
			if !p.Channel().Valid() {
				t.Fatalf("provider %s has invalid channel %q", p.Name(), p.Channel())
			}

			// Contract: Channel matches expected
			expected, ok := expectedChannels[p.Name()]
			if !ok {
				t.Fatalf("unexpected provider name %q", p.Name())
			}
			if p.Channel() != expected {
				t.Fatalf("provider %s: expected channel %q, got %q", p.Name(), expected, p.Channel())
			}

			// Contract: Send returns a valid receipt
			ctx := context.Background()
			receipt, err := p.Send(ctx, input)
			if err != nil {
				t.Fatalf("provider %s: Send must succeed, got: %v", p.Name(), err)
			}
			if receipt == nil {
				t.Fatalf("provider %s: receipt must not be nil", p.Name())
			}
			if receipt.Provider == "" {
				t.Fatalf("provider %s: receipt.Provider must not be empty", p.Name())
			}
			if receipt.ExternalID == "" {
				t.Fatalf("provider %s: receipt.ExternalID must not be empty", p.Name())
			}
			if receipt.AcceptedAt.IsZero() {
				t.Fatalf("provider %s: receipt.AcceptedAt must not be zero", p.Name())
			}
			if receipt.AcceptedAt.After(time.Now().Add(time.Second)) {
				t.Fatalf("provider %s: receipt.AcceptedAt must not be in the future", p.Name())
			}
		})
	}
}

// TestFakeProviderRetryableError verifies retryable error semantics.
func TestFakeProviderRetryableError(t *testing.T) {
	p := fake.NewEmailFakeProvider()
	p.StubError = notification.NewRetryableError("rate_limited", "too many requests")

	_, err := p.Send(context.Background(), notification.ProviderInput{
		DeliveryID: "d1",
		Content:    notification.RenderedContent{Body: "test"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !notification.IsRetryable(err) {
		t.Fatal("error must be retryable")
	}
}

// TestFakeProviderPermanentError verifies permanent error semantics.
func TestFakeProviderPermanentError(t *testing.T) {
	p := fake.NewSmsFakeProvider()
	p.StubError = notification.NewPermanentError("invalid_number", "phone number is invalid")

	_, err := p.Send(context.Background(), notification.ProviderInput{
		DeliveryID: "d1",
		Content:    notification.RenderedContent{SMSText: "test"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if notification.IsRetryable(err) {
		t.Fatal("permanent error must not be retryable")
	}
}

// TestFakeProviderCallRecording verifies that fake providers record calls.
func TestFakeProviderCallRecording(t *testing.T) {
	p := fake.NewWebhookFakeProvider()
	ctx := context.Background()

	if p.CallCount() != 0 {
		t.Fatal("initial call count must be 0")
	}

	input := notification.ProviderInput{
		DeliveryID: "d1",
		Content:    notification.RenderedContent{WebhookPayload: `{"event":"test"}`},
	}
	if _, err := p.Send(ctx, input); err != nil {
		t.Fatal(err)
	}
	if p.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", p.CallCount())
	}

	last := p.LastCall()
	if last.Input.DeliveryID != "d1" {
		t.Fatalf("expected delivery_id d1, got %s", last.Input.DeliveryID)
	}

	p.Reset()
	if p.CallCount() != 0 {
		t.Fatal("call count after reset must be 0")
	}
}

// TestFakeProviderCustomReceipt verifies stub receipt override.
func TestFakeProviderCustomReceipt(t *testing.T) {
	p := fake.NewIMFakeProvider()
	p.StubReceipt = &notification.ProviderReceipt{
		Provider:   "fake_im",
		ExternalID: "custom-ext-123",
		AcceptedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Retryable:  true,
	}

	receipt, err := p.Send(context.Background(), notification.ProviderInput{
		DeliveryID: "d1",
		Content:    notification.RenderedContent{IMText: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ExternalID != "custom-ext-123" {
		t.Fatalf("expected custom external_id, got %s", receipt.ExternalID)
	}
	if !receipt.Retryable {
		t.Fatal("expected retryable=true from stub receipt")
	}
}
