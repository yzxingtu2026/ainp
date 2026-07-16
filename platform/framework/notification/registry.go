package notification

import (
	"fmt"
	"sync"
)

// DefinitionRegistry maps definition key → Definition. Duplicate registration fails at boot.
type DefinitionRegistry struct {
	mu   sync.RWMutex
	defs map[string]Definition
}

func NewDefinitionRegistry() *DefinitionRegistry {
	return &DefinitionRegistry{defs: make(map[string]Definition)}
}

// Register adds a notification definition. Returns error for duplicates or invalid definitions.
func (r *DefinitionRegistry) Register(def Definition) error {
	if err := def.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.defs[def.Key]; ok {
		return fmt.Errorf("notification: duplicate definition key %q (existing category=%q)", def.Key, existing.Category)
	}
	r.defs[def.Key] = def
	return nil
}

// Get returns a definition by key.
func (r *DefinitionRegistry) Get(key string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.defs[key]
	return def, ok
}

// MustGet returns a definition or an error if not found.
func (r *DefinitionRegistry) MustGet(key string) (Definition, error) {
	def, ok := r.Get(key)
	if !ok {
		return Definition{}, fmt.Errorf("notification: unknown definition key %q", key)
	}
	return def, nil
}

// List returns all registered definitions.
func (r *DefinitionRegistry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.defs))
	for _, def := range r.defs {
		out = append(out, def)
	}
	return out
}

// Keys returns all registered definition keys.
func (r *DefinitionRegistry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.defs))
	for key := range r.defs {
		out = append(out, key)
	}
	return out
}
