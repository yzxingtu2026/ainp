package job

import (
	"fmt"
	"sync"
	"time"
)

// Definition describes a registered job type contract.
type Definition struct {
	Type           string
	SchemaVersion  int
	Queue          string
	MaxAttempts    int
	Timeout        time.Duration
	UniqueFor      time.Duration // optional uniqueness window for transport TaskID
	MaxPayloadSize int           // bytes; 0 = use runtime default
	Description    string
}

func (d Definition) Validate() error {
	if d.Type == "" {
		return fmt.Errorf("job: type is required")
	}
	if d.SchemaVersion < 1 {
		return fmt.Errorf("job: schema_version must be >= 1 for %s", d.Type)
	}
	if !ValidQueue(d.Queue) {
		return fmt.Errorf("job: invalid queue %q for %s", d.Queue, d.Type)
	}
	if d.MaxAttempts < 1 {
		return fmt.Errorf("job: max_attempts must be >= 1 for %s", d.Type)
	}
	if d.Timeout <= 0 {
		return fmt.Errorf("job: timeout must be > 0 for %s", d.Type)
	}
	return nil
}

// Registry maps job type → Definition + Handler.
type Registry struct {
	mu       sync.RWMutex
	defs     map[string]Definition
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		defs:     make(map[string]Definition),
		handlers: make(map[string]Handler),
	}
}

func (r *Registry) Register(def Definition, h Handler) error {
	if err := def.Validate(); err != nil {
		return err
	}
	if h == nil {
		return fmt.Errorf("job: handler is required for %s", def.Type)
	}
	if h.Type() != def.Type {
		return fmt.Errorf("job: handler type %q != definition %q", h.Type(), def.Type)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.defs[def.Type]; ok {
		return fmt.Errorf("job: duplicate job type %q", def.Type)
	}
	r.defs[def.Type] = def
	r.handlers[def.Type] = h
	return nil
}

func (r *Registry) Definition(jobType string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.defs[jobType]
	return d, ok
}

func (r *Registry) Handler(jobType string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[jobType]
	return h, ok
}

func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}
