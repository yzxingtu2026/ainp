package aimodel

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	ProtocolOpenAI           = "openai"
	ProtocolAnthropic        = "anthropic"
	ProtocolOpenAICompatible = "openai_compatible"
)

var protocolPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

type ConfigField struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}
type ProviderType struct {
	Protocol       string        `json:"protocol"`
	Name           string        `json:"name"`
	ConfigSchema   []ConfigField `json:"config_schema"`
	ModelDiscovery bool          `json:"model_discovery"`
	Capabilities   Capabilities  `json:"capabilities"`
}
type ProviderTypeRegistry struct {
	mu     sync.RWMutex
	values map[string]ProviderType
}

func NewProviderTypeRegistry() *ProviderTypeRegistry {
	return &ProviderTypeRegistry{values: map[string]ProviderType{}}
}
func (r *ProviderTypeRegistry) Register(v ProviderType) error {
	v.Protocol = strings.TrimSpace(v.Protocol)
	v.Name = strings.TrimSpace(v.Name)
	if !protocolPattern.MatchString(v.Protocol) || v.Name == "" {
		return fmt.Errorf("invalid provider type")
	}
	if err := v.Capabilities.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, f := range v.ConfigSchema {
		if !protocolPattern.MatchString(f.Key) || (f.Type != "string" && f.Type != "boolean" && f.Type != "number") || seen[f.Key] {
			return fmt.Errorf("invalid config schema field: %s", f.Key)
		}
		seen[f.Key] = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.values[v.Protocol]; ok {
		return fmt.Errorf("provider type already registered: %s", v.Protocol)
	}
	r.values[v.Protocol] = v
	return nil
}
func (r *ProviderTypeRegistry) Get(protocol string) (ProviderType, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.values[protocol]
	return v, ok
}
func (r *ProviderTypeRegistry) List() []ProviderType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderType, 0, len(r.values))
	for _, v := range r.values {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Protocol < out[j].Protocol })
	return out
}

func RegisterBuiltinProviderTypes(r *ProviderTypeRegistry) error {
	chat := Capabilities{InputModalities: []Modality{ModalityText, ModalityImage, ModalityAudio, ModalityFile}, OutputModalities: []Modality{ModalityText, ModalityAudio}, Features: []Feature{FeatureStreaming, FeatureToolCalling, FeatureParallelToolCalls, FeatureStructuredOutput, FeatureVision, FeatureEmbeddings, FeatureImageGeneration, FeatureAudioInput, FeatureAudioOutput}}
	compatible := Capabilities{InputModalities: []Modality{ModalityText}, OutputModalities: []Modality{ModalityText}, Features: []Feature{FeatureToolCalling, FeatureStructuredOutput, FeatureEmbeddings}}
	for _, v := range []ProviderType{{ProtocolOpenAI, "OpenAI", nil, true, chat}, {ProtocolAnthropic, "Anthropic", nil, true, Capabilities{InputModalities: []Modality{ModalityText, ModalityImage}, OutputModalities: []Modality{ModalityText}, Features: []Feature{FeatureStreaming, FeatureToolCalling, FeatureStructuredOutput, FeatureVision}}}, {ProtocolOpenAICompatible, "OpenAI Compatible", []ConfigField{{Key: "models_path", Type: "string"}}, true, compatible}} {
		if err := r.Register(v); err != nil {
			return err
		}
	}
	return nil
}
