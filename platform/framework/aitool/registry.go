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
)

type Registry struct {
	mu        sync.RWMutex
	tools     map[string]Definition
	providers map[Source]Provider
	executors map[Source]Executor
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Definition{}, providers: map[Source]Provider{}, executors: map[Source]Executor{}}
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
	next := map[string]Definition{}
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
			next[d.ID] = d
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
func Search(ctx context.Context, definitions []Definition, query, capability string, source Source, limit int, visible func(Definition) bool, embeddings EmbeddingService) ([]Candidate, error) {
	return search(ctx, definitions, query, capability, source, limit, 5, 10, visible, embeddings)
}

// Browse 面向管理端工具选择器：默认 50、硬顶 100，便于在弹窗中浏览已注册工具。
func Browse(ctx context.Context, definitions []Definition, query, capability string, source Source, limit int, visible func(Definition) bool) ([]Candidate, error) {
	return search(ctx, definitions, query, capability, source, limit, 50, 100, visible, nil)
}

func search(ctx context.Context, definitions []Definition, query, capability string, source Source, limit, defaultLimit, maxLimit int, visible func(Definition) bool, embeddings EmbeddingService) ([]Candidate, error) {
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
	vector := map[string]float64{}
	if embeddings != nil {
		scores, err := embeddings.Scores(ctx, q, eligible)
		if err == nil {
			vector = scores
		}
	}
	out := make([]Candidate, 0, len(eligible))
	for _, d := range eligible {
		text := strings.ToLower(strings.Join(append([]string{d.ID, d.Name, d.DescriptionForLLM, d.Capability}, d.Tags...), " "))
		score := vector[d.ID] * 0.25
		if q == "" {
			score += 0.1
		} else {
			if strings.EqualFold(d.ID, q) || strings.EqualFold(d.Name, q) {
				score += 1
			} else if strings.Contains(text, q) {
				score += 0.7
			} else {
				for _, term := range strings.Fields(q) {
					if strings.Contains(text, term) {
						score += 0.12
					}
				}
			}
		}
		if score <= 0 {
			continue
		}
		out = append(out, Candidate{ID: d.ID, Name: d.Name, Summary: d.DescriptionForLLM, Source: d.Source, Capability: d.Capability, Risk: d.Risk, SideEffect: d.SideEffect, InputHints: inputHints(d.InputSchema), Score: score})
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
func inputHints(schema json.RawMessage) []string {
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
