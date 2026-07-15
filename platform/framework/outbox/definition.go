package outbox

import (
	"fmt"
	"sync"
)

// Definition registers a stable event type + schema version contract.
type Definition struct {
	EventType     string
	SchemaVersion int
	Description   string
}

// DefinitionRegistry maps event_type → Definition. Duplicate registration fails at boot.
type DefinitionRegistry struct {
	mu   sync.RWMutex
	byType map[string]Definition
}

func NewDefinitionRegistry() *DefinitionRegistry {
	return &DefinitionRegistry{byType: make(map[string]Definition)}
}

func (r *DefinitionRegistry) Register(def Definition) error {
	if def.EventType == "" {
		return fmt.Errorf("outbox: event type is required")
	}
	if def.SchemaVersion < 1 {
		return fmt.Errorf("outbox: schema_version must be >= 1 for %s", def.EventType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.byType[def.EventType]; ok {
		return fmt.Errorf("outbox: duplicate event type %q (existing v%d)", def.EventType, existing.SchemaVersion)
	}
	r.byType[def.EventType] = def
	return nil
}

func (r *DefinitionRegistry) Get(eventType string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.byType[eventType]
	return def, ok
}

func (r *DefinitionRegistry) MustGet(eventType string) (Definition, error) {
	def, ok := r.Get(eventType)
	if !ok {
		return Definition{}, fmt.Errorf("outbox: unknown event type %q", eventType)
	}
	return def, nil
}

func (r *DefinitionRegistry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.byType))
	for _, def := range r.byType {
		out = append(out, def)
	}
	return out
}
