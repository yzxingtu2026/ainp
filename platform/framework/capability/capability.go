package capability

import "fmt"

type Capability struct {
	ID             string
	Name           string // 兜底文案
	NameKey        string // i18n key，如 capability.system.name
	Description    string
	DescriptionKey string // i18n key，如 capability.system.description
	Module         string
	Order          int
}

type Registry struct {
	items map[string]Capability
}

func NewRegistry() *Registry {
	return &Registry{items: make(map[string]Capability)}
}

func (r *Registry) Register(c Capability) error {
	if c.ID == "" {
		return fmt.Errorf("capability id is required")
	}
	if _, exists := r.items[c.ID]; exists {
		return fmt.Errorf("capability already registered: %s", c.ID)
	}
	r.items[c.ID] = c
	return nil
}

func (r *Registry) Get(id string) (Capability, bool) {
	c, ok := r.items[id]
	return c, ok
}

func (r *Registry) List() []Capability {
	out := make([]Capability, 0, len(r.items))
	for _, c := range r.items {
		out = append(out, c)
	}
	return out
}
