package outbox

import (
	"context"
	"fmt"
	"sync"
)

// Handler consumes a stable Envelope. Implementations must be idempotent;
// at-least-once delivery may invoke Handle more than once for the same event.
type Handler interface {
	Name() string
	EventTypes() []string
	Handle(ctx context.Context, env Envelope) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc struct {
	HandlerName string
	Types       []string
	Fn          func(ctx context.Context, env Envelope) error
}

func (h HandlerFunc) Name() string              { return h.HandlerName }
func (h HandlerFunc) EventTypes() []string      { return h.Types }
func (h HandlerFunc) Handle(ctx context.Context, env Envelope) error {
	if h.Fn == nil {
		return fmt.Errorf("outbox: handler %q has nil Fn", h.HandlerName)
	}
	return h.Fn(ctx, env)
}

// HandlerRegistry indexes handlers by name and event type.
type HandlerRegistry struct {
	mu       sync.RWMutex
	byName   map[string]Handler
	byType   map[string][]Handler
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		byName: make(map[string]Handler),
		byType: make(map[string][]Handler),
	}
}

func (r *HandlerRegistry) Register(h Handler) error {
	if h == nil {
		return fmt.Errorf("outbox: handler is nil")
	}
	name := h.Name()
	if name == "" {
		return fmt.Errorf("outbox: handler name is required")
	}
	types := h.EventTypes()
	if len(types) == 0 {
		return fmt.Errorf("outbox: handler %q must declare at least one event type", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[name]; ok {
		return fmt.Errorf("outbox: duplicate handler name %q", name)
	}
	r.byName[name] = h
	for _, eventType := range types {
		if eventType == "" {
			return fmt.Errorf("outbox: handler %q has empty event type", name)
		}
		r.byType[eventType] = append(r.byType[eventType], h)
	}
	return nil
}

func (r *HandlerRegistry) HandlersFor(eventType string) []Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	src := r.byType[eventType]
	out := make([]Handler, len(src))
	copy(out, src)
	return out
}

func (r *HandlerRegistry) Get(name string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.byName[name]
	return h, ok
}

func (r *HandlerRegistry) List() []Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Handler, 0, len(r.byName))
	for _, h := range r.byName {
		out = append(out, h)
	}
	return out
}
