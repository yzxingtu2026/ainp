package aiagent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
)

type SingletonScope string

const (
	SingletonNone   SingletonScope = "none"
	SingletonTenant SingletonScope = "tenant"
)

type Contract struct {
	Key                       string          `json:"key"`
	Version                   string          `json:"version"`
	Name                      string          `json:"name"`
	Description               string          `json:"description"`
	BaseInstructions          string          `json:"-"`
	BaseInstructionsVersion   string          `json:"base_instructions_version,omitempty"`
	RequiredModelCapabilities []string        `json:"required_model_capabilities"`
	RequiredToolSelectors     []string        `json:"required_tool_selectors"`
	AllowedToolSources        []string        `json:"allowed_tool_sources,omitempty"`
	AllowedToolCapabilities   []string        `json:"allowed_tool_capabilities,omitempty"`
	ConfigSchema              json.RawMessage `json:"config_schema"`
	SingletonScope            SingletonScope  `json:"singleton_scope"`
	ManagedFields             []string        `json:"managed_fields"`
	UpgradePolicy             string          `json:"upgrade_policy"`
}

type ContractContribution struct {
	Owner                     string
	TargetKey                 string
	TargetVersion             string
	AllowedToolCapabilities   []string
	BaseInstructionsPromptKey string
	BaseInstructionsVersion   string
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9_-]*)+$`)
var keySegmentPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var ownerPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
var versionPattern = regexp.MustCompile(`^[1-9][0-9]*\.[0-9]+\.[0-9]+$`)
var toolPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*:[a-z][a-z0-9_.-]*$`)
var capabilities = map[string]bool{"chat": true, "streaming": true, "tools": true, "vision": true, "json_mode": true}
var managed = map[string]bool{"code": true, "source": true, "contract_key": true, "contract_version": true, "base_instructions": true, "visibility": true}

func (c Contract) Validate() error {
	if !keyPattern.MatchString(c.Key) || !versionPattern.MatchString(c.Version) || strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.BaseInstructions) == "" {
		return fmt.Errorf("invalid agent contract identity")
	}
	if c.BaseInstructionsVersion != "" && !versionPattern.MatchString(c.BaseInstructionsVersion) {
		return fmt.Errorf("invalid base instructions version")
	}
	if !json.Valid(c.ConfigSchema) {
		return fmt.Errorf("invalid config schema")
	}
	var schema map[string]any
	if json.Unmarshal(c.ConfigSchema, &schema) != nil || schema["type"] != "object" {
		return fmt.Errorf("config schema must describe an object")
	}
	for _, capability := range c.RequiredModelCapabilities {
		if !capabilities[capability] {
			return fmt.Errorf("unknown model capability: %s", capability)
		}
	}
	for _, selector := range c.RequiredToolSelectors {
		if !toolPattern.MatchString(selector) {
			return fmt.Errorf("invalid tool selector: %s", selector)
		}
	}
	for _, source := range c.AllowedToolSources {
		if source != "action" && source != "builtin" && source != "plugin" && source != "http" && source != "mcp" {
			return fmt.Errorf("invalid allowed tool source: %s", source)
		}
	}
	for _, capability := range c.AllowedToolCapabilities {
		if !keySegmentPattern.MatchString(capability) {
			return fmt.Errorf("invalid allowed tool capability: %s", capability)
		}
	}
	if c.SingletonScope != SingletonNone && c.SingletonScope != SingletonTenant {
		return fmt.Errorf("invalid singleton scope")
	}
	for _, field := range c.ManagedFields {
		if !managed[field] {
			return fmt.Errorf("unsafe managed field: %s", field)
		}
	}
	if c.UpgradePolicy != "manual" && c.UpgradePolicy != "managed-fields" {
		return fmt.Errorf("unsafe upgrade policy")
	}
	return nil
}

type Registry struct {
	mu        sync.RWMutex
	contracts map[string]map[string]Contract
}

const TenantSupplementSeparator = "--- tenant supplemental instructions ---"

type InstructionError struct {
	Code string
}

func (e *InstructionError) Error() string { return e.Code }

type EffectiveInstructions struct {
	Text                string
	ContractKey         string
	ContractVersion     string
	HasTenantSupplement bool
}

// ResolveInstructions is the single composition path for preview and release runs.
func ResolveInstructions(registry *Registry, contractKey, contractVersion, tenantInstructions string) (EffectiveInstructions, error) {
	key := strings.TrimSpace(contractKey)
	version := strings.TrimSpace(contractVersion)
	supplement := strings.TrimSpace(tenantInstructions)
	if key == "" && version == "" {
		return EffectiveInstructions{Text: supplement, HasTenantSupplement: supplement != ""}, nil
	}
	if key == "" || version == "" || registry == nil {
		return EffectiveInstructions{}, &InstructionError{Code: "agent.contract_unavailable"}
	}
	contract, ok := registry.Get(key, version)
	if !ok || strings.TrimSpace(contract.BaseInstructions) == "" {
		return EffectiveInstructions{}, &InstructionError{Code: "agent.contract_unavailable"}
	}
	base := strings.TrimSpace(contract.BaseInstructions)
	result := EffectiveInstructions{Text: base, ContractKey: key, ContractVersion: version, HasTenantSupplement: supplement != ""}
	if supplement != "" {
		result.Text += "\n\n" + TenantSupplementSeparator + "\n\n" + supplement
	}
	return result, nil
}

func NewRegistry() *Registry { return &Registry{contracts: map[string]map[string]Contract{}} }

func (r *Registry) Register(contract Contract) error {
	if err := contract.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.contracts[contract.Key]
	if versions == nil {
		versions = map[string]Contract{}
		r.contracts[contract.Key] = versions
	}
	if _, exists := versions[contract.Version]; exists {
		return fmt.Errorf("agent contract already registered: %s@%s", contract.Key, contract.Version)
	}
	versions[contract.Version] = contract
	return nil
}

func (r *Registry) ApplyContributions(contributions []ContractContribution) error {
	ordered := append([]ContractContribution(nil), contributions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Owner < ordered[j].Owner })
	r.mu.RLock()
	working := make(map[string]map[string]Contract, len(r.contracts))
	for key, versions := range r.contracts {
		working[key] = make(map[string]Contract, len(versions))
		for version, contract := range versions {
			working[key][version] = contract
		}
	}
	r.mu.RUnlock()
	seenOwners := map[string]struct{}{}
	for _, contribution := range ordered {
		owner := strings.TrimSpace(contribution.Owner)
		if !ownerPattern.MatchString(owner) {
			return fmt.Errorf("invalid Agent Contract contribution owner: %q", owner)
		}
		if _, exists := seenOwners[owner]; exists {
			return fmt.Errorf("duplicate Agent Contract contribution owner: %s", owner)
		}
		seenOwners[owner] = struct{}{}
		key := strings.TrimSpace(contribution.TargetKey)
		version := strings.TrimSpace(contribution.TargetVersion)
		contract, ok := working[key][version]
		if !ok {
			return fmt.Errorf("Agent Contract contribution owner %s targets unknown contract %s@%s", owner, key, version)
		}
		capabilities := append([]string(nil), contribution.AllowedToolCapabilities...)
		if len(capabilities) == 0 && strings.TrimSpace(contribution.BaseInstructionsPromptKey) == "" {
			return fmt.Errorf("Agent Contract contribution owner %s is empty", owner)
		}
		for _, capability := range capabilities {
			if !keySegmentPattern.MatchString(capability) {
				return fmt.Errorf("Agent Contract contribution owner %s has invalid capability %q", owner, capability)
			}
		}
		contract.AllowedToolCapabilities = append(contract.AllowedToolCapabilities, capabilities...)
		contract.AllowedToolCapabilities = sortedUnique(contract.AllowedToolCapabilities)
		promptKey := strings.TrimSpace(contribution.BaseInstructionsPromptKey)
		if promptKey != "" {
			if !strings.HasPrefix(promptKey, owner+"/") {
				return fmt.Errorf("Agent Contract contribution owner %s cannot use Prompt %s", owner, promptKey)
			}
			metadata, ok := prompt.MetadataFor(promptKey)
			if !ok || strings.TrimSpace(contribution.BaseInstructionsVersion) == "" || metadata.Version != contribution.BaseInstructionsVersion {
				return fmt.Errorf("Agent Contract contribution owner %s requires Prompt %s@%s", owner, promptKey, contribution.BaseInstructionsVersion)
			}
			instructions := strings.TrimSpace(prompt.Get(promptKey))
			if instructions == "" {
				return fmt.Errorf("Agent Contract contribution owner %s Prompt %s is empty", owner, promptKey)
			}
			contract.BaseInstructions += "\n\n--- downstream module instructions: " + owner + " ---\n\n" + instructions
		}
		if err := contract.Validate(); err != nil {
			return fmt.Errorf("Agent Contract contribution owner %s produced invalid contract: %w", owner, err)
		}
		working[key][version] = contract
	}
	r.mu.Lock()
	r.contracts = working
	r.mu.Unlock()
	return nil
}

func sortedUnique(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (r *Registry) Get(key, version string) (Contract, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.contracts[key][version]
	return c, ok
}

func (r *Registry) List() []Contract {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Contract
	for _, versions := range r.contracts {
		for _, c := range versions {
			out = append(out, c)
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

// ValidateActionToolCapabilityCoverage ensures every exposed Action capability is
// deliberately reachable from at least one builtin Agent Contract. Exclusions are
// explicit safety decisions and are checked in both directions to prevent drift.
func (r *Registry) ValidateActionToolCapabilityCoverage(actions *action.Registry, exclusions []string) error {
	reachable := map[string]struct{}{}
	for _, contract := range r.List() {
		reachable[contract.Key+"@"+contract.Version] = struct{}{}
	}
	return r.ValidateActionToolCapabilityReachability(actions, reachable, exclusions)
}

func (r *Registry) ValidateActionToolCapabilityReachability(actions *action.Registry, reachableContracts map[string]struct{}, exclusions []string) error {
	if actions == nil {
		return fmt.Errorf("action registry is required for Agent Contract capability coverage")
	}
	exposed := map[string]struct{}{}
	for _, registered := range actions.List() {
		if registered.Meta.AI != nil && registered.Meta.AI.ExposeTool {
			exposed[registered.Meta.Capability] = struct{}{}
		}
	}
	covered := map[string]struct{}{}
	for _, contract := range r.List() {
		identity := contract.Key + "@" + contract.Version
		if _, reachable := reachableContracts[identity]; !reachable {
			continue
		}
		for _, capability := range contract.AllowedToolCapabilities {
			covered[capability] = struct{}{}
		}
	}
	excluded := map[string]struct{}{}
	for _, capability := range exclusions {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return fmt.Errorf("Agent Contract capability coverage exclusion must not be empty")
		}
		if _, exists := excluded[capability]; exists {
			return fmt.Errorf("duplicate Agent Contract capability coverage exclusion: %s", capability)
		}
		excluded[capability] = struct{}{}
	}
	var missing, stale []string
	for capability := range exposed {
		if _, ok := covered[capability]; ok {
			continue
		}
		if _, ok := excluded[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	for capability := range excluded {
		_, isExposed := exposed[capability]
		_, isCovered := covered[capability]
		if !isExposed || isCovered {
			stale = append(stale, capability)
		}
	}
	if len(missing) == 0 && len(stale) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(stale)
	return fmt.Errorf("Agent Contract capability reachability invalid: missing=%v stale_exclusions=%v reachable_contracts=%v; verify Contract -> builtin Agent -> Agent Slot -> Runtime Tool Scope", missing, stale, sortedMapKeys(reachableContracts))
}

func sortedMapKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
