package outbox

import (
	"context"
	"fmt"
)

// Transport delivers a committed Envelope to the consumption side.
// Phase 1: InProcessTransport runs registered handlers via DeliveryStore.
// Later: Redis Streams adapter publishes via XADD; consumers ack separately.
// Transport-specific IDs (stream key, Redis message ID) must not leak into Envelope payload.
type Transport interface {
	Name() string
	Publish(ctx context.Context, env Envelope) error
}

// InProcessTransport dispatches to HandlerRegistry with DeliveryStore idempotency.
type InProcessTransport struct {
	Handlers   *HandlerRegistry
	Deliveries DeliveryStore
}

func NewInProcessTransport(handlers *HandlerRegistry, deliveries DeliveryStore) *InProcessTransport {
	return &InProcessTransport{Handlers: handlers, Deliveries: deliveries}
}

func (t *InProcessTransport) Name() string { return "postgres" }

func (t *InProcessTransport) Publish(ctx context.Context, env Envelope) error {
	if t.Handlers == nil {
		return fmt.Errorf("outbox: handler registry is nil")
	}
	if t.Deliveries == nil {
		return fmt.Errorf("outbox: delivery store is nil")
	}
	handlers := t.Handlers.HandlersFor(env.EventType)
	if len(handlers) == 0 {
		// No handlers is a valid publish (event recorded; nothing to do).
		return nil
	}
	var firstErr error
	for _, h := range handlers {
		if err := t.Deliveries.RunHandler(ctx, env, h); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RecordingTransport is a fake transport for contract tests.
type RecordingTransport struct {
	NameValue string
	Published []Envelope
	Err       error
}

func (t *RecordingTransport) Name() string {
	if t.NameValue == "" {
		return "fake"
	}
	return t.NameValue
}

func (t *RecordingTransport) Publish(_ context.Context, env Envelope) error {
	if t.Err != nil {
		return t.Err
	}
	t.Published = append(t.Published, env)
	return nil
}
