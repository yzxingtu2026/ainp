package setting

import (
	"fmt"
	"sort"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type Registry struct {
	mu          sync.RWMutex
	categories  map[string]Category
	definitions map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{
		categories:  make(map[string]Category),
		definitions: make(map[string]Definition),
	}
}

func (r *Registry) RegisterCategory(c Category) error {
	if err := ValidateCategory(c); err != nil {
		return err
	}
	c.Surface = auth.NormalizeRealm(c.Surface)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.categories[c.Key]; exists {
		return fmt.Errorf("setting category already registered: %s", c.Key)
	}
	r.categories[c.Key] = c
	return nil
}

func (r *Registry) RegisterDefinition(def Definition) error {
	r.mu.RLock()
	cats := cloneCategories(r.categories)
	r.mu.RUnlock()
	if def.Scope == "" {
		def.Scope = ScopeTenant
	}
	def.Surface = auth.NormalizeRealm(def.Surface)
	if err := ValidateDefinition(def, cats); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.definitions[def.Key]; exists {
		return fmt.Errorf("setting definition already registered: %s", def.Key)
	}
	if _, ok := r.categories[def.CategoryKey]; !ok {
		return fmt.Errorf("setting definition references unknown category %q: %s", def.CategoryKey, def.Key)
	}
	r.definitions[def.Key] = cloneDefinition(def)
	return nil
}

func (r *Registry) GetCategory(key string) (Category, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.categories[key]
	return c, ok
}

func (r *Registry) GetDefinition(key string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.definitions[key]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(d), true
}

func (r *Registry) ListCategories(surface auth.Realm) []Category {
	r.mu.RLock()
	defer r.mu.RUnlock()
	surface = auth.NormalizeRealm(surface)
	out := make([]Category, 0, len(r.categories))
	for _, c := range r.categories {
		if auth.NormalizeRealm(c.Surface) == surface {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].Key < out[j].Key
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func (r *Registry) ListDefinitions(surface auth.Realm) []Definition {
	return r.ListDefinitionsByScope(surface, "")
}

func (r *Registry) ListDefinitionsByScope(surface auth.Realm, scope Scope) []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	surface = auth.NormalizeRealm(surface)
	out := make([]Definition, 0, len(r.definitions))
	for _, d := range r.definitions {
		if auth.NormalizeRealm(d.Surface) == surface && (scope == "" || d.Scope == scope) {
			out = append(out, cloneDefinition(d))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].Key < out[j].Key
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func cloneCategories(in map[string]Category) map[string]Category {
	out := make(map[string]Category, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneDefinition(def Definition) Definition {
	fields := make([]Field, len(def.Fields))
	copy(fields, def.Fields)
	for i := range fields {
		if len(fields[i].Options) > 0 {
			opts := make([]FieldOption, len(fields[i].Options))
			copy(opts, fields[i].Options)
			fields[i].Options = opts
		}
		if fields[i].Default != nil {
			fields[i].Default = cloneJSONValue(fields[i].Default)
		}
	}
	def.Fields = fields
	return def
}
