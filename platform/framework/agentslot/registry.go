package agentslot

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
)

type FallbackPolicy string
type UpgradePolicy string

const (
	FallbackFailClosed FallbackPolicy = "fail_closed"
	UpgradePinned      UpgradePolicy  = "pinned"
	UpgradeCompatible  UpgradePolicy  = "compatible"
)

type Slot struct {
	Key                    string         `json:"key"`
	Version                string         `json:"version"`
	Module                 string         `json:"module"`
	Title                  string         `json:"title"`
	Description            string         `json:"description"`
	DefaultContractKey     string         `json:"default_contract_key"`
	DefaultContractVersion string         `json:"default_contract_version"`
	RequiredCapabilities   []string       `json:"required_capabilities"`
	TenantOverride         bool           `json:"tenant_override"`
	UserOverride           bool           `json:"user_override"`
	FallbackPolicy         FallbackPolicy `json:"fallback_policy"`
	UpgradePolicy          UpgradePolicy  `json:"upgrade_policy"`
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9_-]*)+$`)
var versionPattern = regexp.MustCompile(`^[1-9][0-9]*\.[0-9]+\.[0-9]+$`)
var capabilities = map[string]bool{"chat": true, "streaming": true, "tools": true, "vision": true, "json_mode": true}

func (s Slot) Validate(contracts *aiagent.Registry) error {
	if !keyPattern.MatchString(s.Key) || !versionPattern.MatchString(s.Version) || strings.TrimSpace(s.Module) == "" || strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("invalid agent slot identity")
	}
	if s.DefaultContractKey == "" || s.DefaultContractVersion == "" {
		return fmt.Errorf("agent slot default contract is required")
	}
	contract, ok := contracts.Get(s.DefaultContractKey, s.DefaultContractVersion)
	if !ok {
		return fmt.Errorf("unknown default agent contract: %s@%s", s.DefaultContractKey, s.DefaultContractVersion)
	}
	available := map[string]bool{}
	for _, capability := range contract.RequiredModelCapabilities {
		available[capability] = true
	}
	for _, capability := range s.RequiredCapabilities {
		if !capabilities[capability] {
			return fmt.Errorf("unknown model capability: %s", capability)
		}
		if !available[capability] {
			return fmt.Errorf("default contract does not satisfy capability: %s", capability)
		}
	}
	if s.UserOverride && !s.TenantOverride {
		return fmt.Errorf("user override requires tenant override support")
	}
	if s.FallbackPolicy != FallbackFailClosed {
		return fmt.Errorf("unsafe fallback policy")
	}
	if s.UpgradePolicy != UpgradePinned && s.UpgradePolicy != UpgradeCompatible {
		return fmt.Errorf("unsafe upgrade policy")
	}
	return nil
}

type Registry struct {
	mu        sync.RWMutex
	slots     map[string]map[string]Slot
	contracts *aiagent.Registry
}

func NewRegistry(contracts *aiagent.Registry) *Registry {
	return &Registry{slots: map[string]map[string]Slot{}, contracts: contracts}
}
func (r *Registry) Register(slot Slot) error {
	if err := slot.Validate(r.contracts); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.slots[slot.Key]
	if versions == nil {
		versions = map[string]Slot{}
		r.slots[slot.Key] = versions
	}
	if _, ok := versions[slot.Version]; ok {
		return fmt.Errorf("agent slot already registered: %s@%s", slot.Key, slot.Version)
	}
	versions[slot.Version] = slot
	return nil
}
func (r *Registry) Get(key, version string) (Slot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.slots[key][version]
	return s, ok
}
func (r *Registry) Current(key string) (Slot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.slots[key]
	if len(versions) == 0 {
		return Slot{}, false
	}
	keys := make([]string, 0, len(versions))
	for version := range versions {
		keys = append(keys, version)
	}
	sort.Strings(keys)
	return versions[keys[len(keys)-1]], true
}

// AcceptsContract allows registered versions of the same contract only for compatible slots.
func (r *Registry) AcceptsContract(slot Slot, key, version string) bool {
	if key != slot.DefaultContractKey {
		return false
	}
	if version == slot.DefaultContractVersion {
		return true
	}
	if slot.UpgradePolicy != UpgradeCompatible {
		return false
	}
	contract, ok := r.contracts.Get(key, version)
	if !ok {
		return false
	}
	available := map[string]bool{}
	for _, capability := range contract.RequiredModelCapabilities {
		available[capability] = true
	}
	for _, capability := range slot.RequiredCapabilities {
		if !available[capability] {
			return false
		}
	}
	return true
}
func (r *Registry) List() []Slot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []Slot{}
	for _, versions := range r.slots {
		for _, s := range versions {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Version < out[j].Version
		}
		return out[i].Key < out[j].Key
	})
	return out
}
