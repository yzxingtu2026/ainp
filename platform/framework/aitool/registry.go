package aitool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
)

type Registry struct {
	mu          sync.RWMutex
	tools       map[string]Definition
	providers   map[Source]Provider
	executors   map[Source]Executor
	slots       map[string]RuntimeSlot
	runtimeRefs map[string]string
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Definition{}, providers: map[Source]Provider{}, executors: map[Source]Executor{}, slots: map[string]RuntimeSlot{}, runtimeRefs: map[string]string{}}
}
func (r *Registry) RegisterSlot(slot RuntimeSlot) error {
	if slot == nil {
		return fmt.Errorf("runtime tool slot is required")
	}
	d := slot.Definition()
	if err := d.Validate(); err != nil {
		return fmt.Errorf("runtime tool slot %s: %w", d.ID, err)
	}
	if slot.DiscoveryMode() != DiscoveryDirect && slot.DiscoveryMode() != DiscoveryProgressive {
		return fmt.Errorf("runtime tool slot %s has invalid discovery mode", d.ID)
	}
	if err := slot.Metadata().Validate(); err != nil {
		return fmt.Errorf("runtime tool slot %s: %w", d.ID, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.slots[d.ID]; ok {
		return fmt.Errorf("runtime tool slot already registered: %s", d.ID)
	}
	if existing, ok := r.runtimeRefs[d.RuntimeRef]; ok {
		return fmt.Errorf("runtime ref already registered by %s: %s", existing, d.RuntimeRef)
	}
	r.slots[d.ID] = slot
	r.runtimeRefs[d.RuntimeRef] = d.ID
	return nil
}
func (r *Registry) RegisterProvider(p Provider, e Executor) error {
	if p == nil || e == nil {
		return fmt.Errorf("provider and executor are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[p.Source()]; ok {
		return fmt.Errorf("tool provider already registered: %s", p.Source())
	}
	r.providers[p.Source()] = p
	r.executors[p.Source()] = e
	return nil
}
func (r *Registry) Refresh(ctx context.Context) error {
	r.mu.RLock()
	ps := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		ps = append(ps, p)
	}
	r.mu.RUnlock()
	sort.Slice(ps, func(i, j int) bool { return ps[i].Source() < ps[j].Source() })
	next := map[string]Definition{}
	refs := map[string]string{}
	r.mu.RLock()
	slots := make([]RuntimeSlot, 0, len(r.slots))
	for _, slot := range r.slots {
		slots = append(slots, slot)
	}
	r.mu.RUnlock()
	sort.Slice(slots, func(i, j int) bool { return slots[i].Definition().ID < slots[j].Definition().ID })
	for _, slot := range slots {
		d := slot.Definition()
		d.SchemaHash = schemaHash(d)
		if err := d.Validate(); err != nil {
			return fmt.Errorf("runtime tool slot %s: %w", d.ID, err)
		}
		next[d.ID] = d
		refs[d.RuntimeRef] = d.ID
	}
	for _, p := range ps {
		items, err := p.List(ctx)
		if err != nil {
			return err
		}
		for _, d := range items {
			if d.Source != p.Source() {
				return fmt.Errorf("provider source mismatch")
			}
			d.SchemaHash = schemaHash(d)
			if err := d.Validate(); err != nil {
				return fmt.Errorf("tool %s: %w", d.ID, err)
			}
			if _, ok := next[d.ID]; ok {
				return fmt.Errorf("duplicate tool: %s", d.ID)
			}
			if existing, ok := refs[d.RuntimeRef]; ok {
				return fmt.Errorf("duplicate runtime ref %s: %s and %s", d.RuntimeRef, existing, d.ID)
			}
			next[d.ID] = d
			refs[d.RuntimeRef] = d.ID
		}
	}
	r.mu.Lock()
	r.tools = next
	r.mu.Unlock()
	return nil
}
func (r *Registry) Get(id string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.tools[id]
	return d, ok
}
func (r *Registry) Executor(source Source) (Executor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.executors[source]
	return e, ok
}
func (r *Registry) ExecutorFor(d Definition) (Executor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if slot, ok := r.slots[d.ID]; ok {
		return slot, true
	}
	e, ok := r.executors[d.Source]
	return e, ok
}
func (r *Registry) Project(ctx context.Context, scope RuntimeScope, d Definition) (Definition, bool, error) {
	r.mu.RLock()
	slot, ok := r.slots[d.ID]
	r.mu.RUnlock()
	if !ok {
		return d, true, nil
	}
	return slot.Project(ctx, scope, d)
}
func (r *Registry) SlotMetadata(id string) (RuntimeMetadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slot, ok := r.slots[id]
	if !ok {
		return RuntimeMetadata{}, false
	}
	return slot.Metadata(), true
}
func (r *Registry) DirectSlotIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.slots))
	for id, slot := range r.slots {
		if slot.DiscoveryMode() == DiscoveryDirect {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
func (r *Registry) SlotIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.slots))
	for id := range r.slots {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.tools))
	for _, d := range r.tools {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func schemaHash(d Definition) string {
	copy := d
	copy.SchemaHash = ""
	b, _ := json.Marshal(copy)
	b = append(b, []byte(d.RuntimeRef)...)
	v := sha256.Sum256(b)
	return hex.EncodeToString(v[:])
}

// Search 面向模型常驻 tool.search：默认 5、硬顶 10，避免把全量目录塞进上下文。
func Search(ctx context.Context, definitions []Definition, query, capability string, source Source, limit int, visible func(Definition) bool) ([]Candidate, error) {
	return search(ctx, definitions, query, capability, source, limit, 5, 10, visible)
}

// Browse 面向管理端工具选择器：默认 50、硬顶 100，便于在弹窗中浏览已注册工具。
func Browse(ctx context.Context, definitions []Definition, query, capability string, source Source, limit int, visible func(Definition) bool) ([]Candidate, error) {
	return search(ctx, definitions, query, capability, source, limit, 50, 100, visible)
}

func search(ctx context.Context, definitions []Definition, query, capability string, source Source, limit, defaultLimit, maxLimit int, visible func(Definition) bool) ([]Candidate, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	q := strings.ToLower(strings.TrimSpace(query))
	eligible := []Definition{}
	for _, d := range definitions {
		if d.Status != "enabled" || (capability != "" && d.Capability != capability) || (source != "" && d.Source != source) || !visible(d) {
			continue
		}
		eligible = append(eligible, d)
	}
	out := make([]Candidate, 0, len(eligible))
	for _, d := range eligible {
		text := strings.ToLower(strings.Join(append([]string{d.ID, d.Name, d.DescriptionForLLM, d.Capability}, d.Tags...), " "))
		score := 0.0
		if q == "" {
			score += 0.1
		} else {
			if strings.EqualFold(d.ID, q) || strings.EqualFold(d.Name, q) {
				score += 1
			} else if strings.Contains(text, q) {
				score += 0.7
			} else {
				for _, term := range LexicalTerms(q) {
					if strings.Contains(text, term) {
						score += 0.12
					}
				}
			}
		}
		if score <= 0 {
			continue
		}
		out = append(out, Candidate{ID: d.ID, Name: d.Name, Summary: d.DescriptionForLLM, Source: d.Source, Capability: d.Capability, Risk: d.Risk, SideEffect: d.SideEffect, InputHints: InputHints(d.InputSchema), Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func LexicalTerms(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	seen := map[string]bool{}
	terms := make([]string, 0, 16)
	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" || seen[term] || len(terms) >= 32 {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	for _, field := range strings.Fields(value) {
		add(field)
		runes := []rune(field)
		for i := 0; i+1 < len(runes); i++ {
			if unicode.Is(unicode.Han, runes[i]) && unicode.Is(unicode.Han, runes[i+1]) {
				add(string(runes[i : i+2]))
			}
		}
	}
	return terms
}

func SearchDocumentText(definition Definition) string {
	parts := append([]string{definition.ID, definition.Name, definition.DescriptionForLLM, definition.Capability}, definition.Tags...)
	terms := make([]string, 0, len(parts)*2)
	for _, part := range parts {
		terms = append(terms, part)
		terms = append(terms, LexicalTerms(part)...)
	}
	return strings.Join(terms, " ")
}
func InputHints(schema json.RawMessage) []string {
	var v struct {
		Properties map[string]any `json:"properties"`
	}
	if json.Unmarshal(schema, &v) != nil {
		return nil
	}
	out := make([]string, 0, len(v.Properties))
	for k := range v.Properties {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}
