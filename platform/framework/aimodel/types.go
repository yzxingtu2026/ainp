package aimodel

import (
	"context"
	"fmt"
	"sort"
)

type ModelType string
type Modality string
type Feature string

const (
	ModelTypeChat            ModelType = "chat"
	ModelTypeEmbedding       ModelType = "embedding"
	ModelTypeRerank          ModelType = "rerank"
	ModelTypeImage           ModelType = "image"
	ModelTypeSpeech          ModelType = "speech"
	ModelTypeTranscription   ModelType = "transcription"
	ModalityText             Modality  = "text"
	ModalityImage            Modality  = "image"
	ModalityAudio            Modality  = "audio"
	ModalityFile             Modality  = "file"
	FeatureStreaming         Feature   = "streaming"
	FeatureToolCalling       Feature   = "tool_calling"
	FeatureParallelToolCalls Feature   = "parallel_tool_calls"
	FeatureStructuredOutput  Feature   = "structured_output"
	FeatureVision            Feature   = "vision"
	FeatureEmbeddings        Feature   = "embeddings"
	FeatureImageGeneration   Feature   = "image_generation"
	FeatureAudioInput        Feature   = "audio_input"
	FeatureAudioOutput       Feature   = "audio_output"
)

var validModelTypes = set(ModelTypeChat, ModelTypeEmbedding, ModelTypeRerank, ModelTypeImage, ModelTypeSpeech, ModelTypeTranscription)
var validModalities = set(ModalityText, ModalityImage, ModalityAudio, ModalityFile)
var validFeatures = set(FeatureStreaming, FeatureToolCalling, FeatureParallelToolCalls, FeatureStructuredOutput, FeatureVision, FeatureEmbeddings, FeatureImageGeneration, FeatureAudioInput, FeatureAudioOutput)

func set[T comparable](values ...T) map[T]struct{} {
	out := make(map[T]struct{}, len(values))
	for _, v := range values {
		out[v] = struct{}{}
	}
	return out
}
func (v ModelType) Valid() bool { _, ok := validModelTypes[v]; return ok }
func (v Modality) Valid() bool  { _, ok := validModalities[v]; return ok }
func (v Feature) Valid() bool   { _, ok := validFeatures[v]; return ok }

type Capabilities struct {
	InputModalities  []Modality `json:"input_modalities"`
	OutputModalities []Modality `json:"output_modalities"`
	Features         []Feature  `json:"features"`
}

type Limits struct {
	ContextWindow   int64 `json:"context_window,omitempty"`
	MaxOutputTokens int64 `json:"max_output_tokens,omitempty"`
}

func (c Capabilities) Validate() error {
	for _, v := range c.InputModalities {
		if !v.Valid() {
			return fmt.Errorf("unknown input modality: %s", v)
		}
	}
	for _, v := range c.OutputModalities {
		if !v.Valid() {
			return fmt.Errorf("unknown output modality: %s", v)
		}
	}
	for _, v := range c.Features {
		if !v.Valid() {
			return fmt.Errorf("unknown feature: %s", v)
		}
	}
	return nil
}
func (l Limits) Validate() error {
	if l.ContextWindow < 0 || l.MaxOutputTokens < 0 {
		return fmt.Errorf("limits must be non-negative")
	}
	return nil
}

type CapabilityRequirements struct {
	ModelType        ModelType  `json:"model_type"`
	InputModalities  []Modality `json:"input_modalities"`
	OutputModalities []Modality `json:"output_modalities"`
	Features         []Feature  `json:"features"`
}

func (r CapabilityRequirements) Validate() error {
	if !r.ModelType.Valid() {
		return fmt.Errorf("invalid model type: %s", r.ModelType)
	}
	return (Capabilities{r.InputModalities, r.OutputModalities, r.Features}).Validate()
}

func Intersect(left, right Capabilities) Capabilities {
	return Capabilities{InputModalities: intersect(left.InputModalities, right.InputModalities), OutputModalities: intersect(left.OutputModalities, right.OutputModalities), Features: intersect(left.Features, right.Features)}
}
func Satisfies(have Capabilities, req CapabilityRequirements) bool {
	return contains(have.InputModalities, req.InputModalities) && contains(have.OutputModalities, req.OutputModalities) && contains(have.Features, req.Features)
}
func intersect[T ~string](a, b []T) []T {
	s := map[T]bool{}
	for _, v := range a {
		s[v] = true
	}
	out := []T{}
	for _, v := range b {
		if s[v] {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func contains[T comparable](have, need []T) bool {
	s := map[T]bool{}
	for _, v := range have {
		s[v] = true
	}
	for _, v := range need {
		if !s[v] {
			return false
		}
	}
	return true
}

type ResolvedModel struct {
	ProviderID    string
	Protocol      string
	BaseURL       string
	CredentialRef string
	DeploymentID  string
	RemoteModelID string
	ModelType     ModelType
	Capabilities  Capabilities
	Limits        Limits
	Config        map[string]any
}
type Resolver interface {
	Resolve(context.Context, string, CapabilityRequirements) (*ResolvedModel, error)
}
