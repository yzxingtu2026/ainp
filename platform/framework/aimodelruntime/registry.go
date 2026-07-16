package aimodelruntime

import (
	"fmt"
	"sync"
)

type AdapterRegistry struct {
	mu    sync.RWMutex
	items map[string]ModelAdapter
}

func NewAdapterRegistry() *AdapterRegistry { return &AdapterRegistry{items: map[string]ModelAdapter{}} }
func (r *AdapterRegistry) Register(protocol string, a ModelAdapter) error {
	if protocol == "" || a == nil {
		return fmt.Errorf("protocol and adapter are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[protocol]; ok {
		return fmt.Errorf("adapter already registered: %s", protocol)
	}
	r.items[protocol] = a
	return nil
}
func (r *AdapterRegistry) Get(protocol string) (ModelAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.items[protocol]
	return a, ok
}
