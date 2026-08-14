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
	cipher   SecretCipher
	mu       sync.RWMutex
	cache    map[string]*Effective // key: tenantID + "\x00" + settingKey
}

func NewProvider(registry *Registry, store ValueStore, ciphers ...SecretCipher) *Provider {
	provider := &Provider{
		registry: registry,
		store:    store,
		cache:    make(map[string]*Effective),
	}
	if len(ciphers) > 0 {
		provider.cipher = ciphers[0]
	}
	return provider
}

func cacheKey(tenantID, settingKey string) string {
	return tenantID + "\x00" + settingKey
}

func (p *Provider) Invalidate(tenantID, settingKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.cache, cacheKey(tenantID, settingKey))
}

func (p *Provider) SecretCipher() SecretCipher { return p.cipher }

func (p *Provider) Get(ctx context.Context, tenantID, settingKey string) (*Effective, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant id required")
	}
	def, ok := p.registry.GetDefinition(settingKey)
	if !ok {
		return nil, fmt.Errorf("setting not found: %s", settingKey)
	}
	if def.Scope != ScopeTenant {
		return nil, fmt.Errorf("setting scope %q is not supported by tenant provider: %s", def.Scope, settingKey)
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
		for _, field := range def.Fields {
			if field.Type != FieldSecret {
				continue
			}
			raw, exists := eff.Values[field.Key]
			if !exists || raw == nil || raw == "" {
				continue
			}
			ciphertext, ok := raw.(string)
			if !ok || p.cipher == nil {
				return nil, ErrSecretUnavailable
			}
			plaintext, decryptErr := p.cipher.Decrypt(ctx, ciphertext, SecretAAD(tenantID, settingKey, field.Key, stored.SchemaVersion))
			if decryptErr != nil || plaintext == "" {
				return nil, ErrSecretUnavailable
			}
			eff.Values[field.Key] = plaintext
		}
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
