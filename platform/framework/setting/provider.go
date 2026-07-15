package setting

import (
	"context"
	"fmt"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/revision"
)

// StoredValue is the persisted tenant override for one definition.
type StoredValue struct {
	SchemaVersion int
	Revision      int64
	Values        map[string]any
	UpdatedBy     string
}

// ValueStore loads and mutates tenant setting overrides.
type ValueStore interface {
	Get(ctx context.Context, tenantID, settingKey string) (*StoredValue, error)
	Put(ctx context.Context, tenantID, settingKey string, expectedRevision int64, schemaVersion int, values map[string]any, updatedBy string) (*StoredValue, error)
	Delete(ctx context.Context, tenantID, settingKey string, expectedRevision int64) error
}

// ErrRevisionConflict indicates optimistic lock failure.
// Aliased from the public revision package for backward compatibility.
var ErrRevisionConflict = revision.ErrRevisionConflict

// ErrSchemaVersionMismatch indicates stored schema no longer matches code definition.
var ErrSchemaVersionMismatch = fmt.Errorf("setting schema version mismatch")

// Effective is the merged view for one definition.
type Effective struct {
	Definition    Definition
	Values        map[string]any
	Revision      int64
	SchemaVersion int
	Source        string // "default" | "tenant"
}

// Provider merges code defaults with tenant overrides.
type Provider struct {
	registry *Registry
	store    ValueStore
	mu       sync.RWMutex
	cache    map[string]*Effective // key: tenantID + "\x00" + settingKey
}

func NewProvider(registry *Registry, store ValueStore) *Provider {
	return &Provider{
		registry: registry,
		store:    store,
		cache:    make(map[string]*Effective),
	}
}

func cacheKey(tenantID, settingKey string) string {
	return tenantID + "\x00" + settingKey
}

func (p *Provider) Invalidate(tenantID, settingKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.cache, cacheKey(tenantID, settingKey))
}

func (p *Provider) Get(ctx context.Context, tenantID, settingKey string) (*Effective, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant id required")
	}
	def, ok := p.registry.GetDefinition(settingKey)
	if !ok {
		return nil, fmt.Errorf("setting not found: %s", settingKey)
	}
	p.mu.RLock()
	if cached, ok := p.cache[cacheKey(tenantID, settingKey)]; ok {
		cp := *cached
		cp.Values = cloneMap(cached.Values)
		p.mu.RUnlock()
		return &cp, nil
	}
	p.mu.RUnlock()

	stored, err := p.store.Get(ctx, tenantID, settingKey)
	if err != nil {
		return nil, err
	}
	eff := &Effective{Definition: def, SchemaVersion: def.SchemaVersion}
	if stored == nil {
		eff.Values = DefaultValues(def)
		eff.Revision = 0
		eff.Source = "default"
	} else {
		if stored.SchemaVersion != def.SchemaVersion {
			return nil, ErrSchemaVersionMismatch
		}
		eff.Values = cloneMap(stored.Values)
		eff.Revision = stored.Revision
		eff.SchemaVersion = stored.SchemaVersion
		eff.Source = "tenant"
	}
	p.mu.Lock()
	p.cache[cacheKey(tenantID, settingKey)] = &Effective{
		Definition: def, Values: cloneMap(eff.Values), Revision: eff.Revision,
		SchemaVersion: eff.SchemaVersion, Source: eff.Source,
	}
	p.mu.Unlock()
	return eff, nil
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneJSONValue(v)
	}
	return out
}
