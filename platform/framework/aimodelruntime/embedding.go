package aimodelruntime

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
)

type EmbeddingPurpose string
type EmbeddingPreprocessing string

const (
	EmbeddingPurposeToolIndex           EmbeddingPurpose = "tool_index"
	EmbeddingPurposeKnowledgeIngestion  EmbeddingPurpose = "knowledge_ingestion"
	EmbeddingPurposeKnowledgeQuery      EmbeddingPurpose = "knowledge_query"
	EmbeddingPurposeMemory              EmbeddingPurpose = "memory"
	EmbeddingPurposeOther               EmbeddingPurpose = "other"
	EmbeddingPreprocessingNone          EmbeddingPreprocessing = "none"
	EmbeddingPreprocessingCosineNorm    EmbeddingPreprocessing = "cosine_normalize"
	EmbeddingPreprocessingVersionNone   = "none"
	EmbeddingPreprocessingVersionCosine = "cosine_normalize.v1"

	MaxEmbeddingItems          = 2048
	MaxEmbeddingItemBytes      = 32 * 1024
	MaxEmbeddingBatchBytes     = 2 * 1024 * 1024
	MaxEmbeddingEstimatedTokens = 300_000
	MaxEmbeddingVectorElements = 16 * 1024 * 1024
	DefaultEmbeddingTimeoutMS  = 60_000
	OpenAIEmbeddingBatchItems  = 2048
	OpenAICompatibleBatchItems = 10
)

type EmbeddingItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type EmbeddingRequest struct {
	ModelAlias     string                  `json:"model_alias"`
	Inputs         []EmbeddingItem         `json:"inputs"`
	Dimensions     *int                    `json:"dimensions,omitempty"`
	Purpose        EmbeddingPurpose        `json:"purpose"`
	TimeoutMS      int64                   `json:"timeout_ms,omitempty"`
	Preprocessing  EmbeddingPreprocessing  `json:"preprocessing,omitempty"`
	Metadata       map[string]string       `json:"metadata,omitempty"`
}

type EmbeddingVector struct {
	ID     string    `json:"id"`
	Index  int       `json:"index"`
	Vector []float32 `json:"vector"`
}

type EmbeddingResponse struct {
	Model                 ModelSummary `json:"model"`
	Vectors               []EmbeddingVector `json:"vectors"`
	Dimensions            int          `json:"dimensions"`
	Usage                 Usage        `json:"usage"`
	ProviderRequestID     string       `json:"-"`
	SubBatchCount         int          `json:"sub_batch_count"`
	InputItemCount        int          `json:"input_item_count"`
	InputTextBytes        int64        `json:"input_text_bytes"`
	Preprocessing         EmbeddingPreprocessing `json:"preprocessing"`
	PreprocessingVersion  string       `json:"preprocessing_version"`
}

type EmbeddingAdapterRequest struct {
	Request EmbeddingRequest
	Model   *aimodel.ResolvedModel
}

type EmbeddingAdapter interface {
	Embed(context.Context, EmbeddingAdapterRequest) (*EmbeddingResponse, error)
}

type EmbeddingRuntime interface {
	Embed(context.Context, EmbeddingRequest) (*EmbeddingResponse, error)
}

type EmbeddingAdapterRegistry struct {
	mu    sync.RWMutex
	items map[string]EmbeddingAdapter
}

func NewEmbeddingAdapterRegistry() *EmbeddingAdapterRegistry {
	return &EmbeddingAdapterRegistry{items: map[string]EmbeddingAdapter{}}
}

func (r *EmbeddingAdapterRegistry) Register(protocol string, a EmbeddingAdapter) error {
	if protocol == "" || a == nil {
		return fmt.Errorf("protocol and adapter are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[protocol]; ok {
		return fmt.Errorf("embedding adapter already registered: %s", protocol)
	}
	r.items[protocol] = a
	return nil
}

func (r *EmbeddingAdapterRegistry) Get(protocol string) (EmbeddingAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.items[protocol]
	return a, ok
}

func (r *EmbeddingRequest) Normalize() {
	if r.TimeoutMS == 0 {
		r.TimeoutMS = DefaultEmbeddingTimeoutMS
	}
	if r.Purpose == "" {
		r.Purpose = EmbeddingPurposeOther
	}
	if r.Preprocessing == "" {
		r.Preprocessing = EmbeddingPreprocessingNone
	}
	for i := range r.Inputs {
		r.Inputs[i].ID = strings.TrimSpace(r.Inputs[i].ID)
	}
}

func (r EmbeddingRequest) Validate(context.Context) error {
	if strings.TrimSpace(r.ModelAlias) == "" {
		return fmt.Errorf("model_alias is required")
	}
	if !validEmbeddingPurpose(r.Purpose) {
		return fmt.Errorf("invalid embedding purpose: %s", r.Purpose)
	}
	if r.Preprocessing != EmbeddingPreprocessingNone && r.Preprocessing != EmbeddingPreprocessingCosineNorm {
		return fmt.Errorf("unsupported embedding preprocessing: %s", r.Preprocessing)
	}
	if r.TimeoutMS < 1000 || r.TimeoutMS > 300000 {
		return fmt.Errorf("timeout_ms must be between 1000 and 300000")
	}
	if len(r.Inputs) == 0 {
		return fmt.Errorf("inputs are required")
	}
	if len(r.Inputs) > MaxEmbeddingItems {
		return &RuntimeError{Category: ErrorBatchTooLarge, Message: fmt.Sprintf("at most %d embedding inputs are allowed", MaxEmbeddingItems)}
	}
	if r.Dimensions != nil && *r.Dimensions < 1 {
		return &RuntimeError{Category: ErrorDimensionsUnsupported, Message: "dimensions must be positive"}
	}
	seen := map[string]struct{}{}
	totalBytes := 0
	estimatedTokens := 0
	for _, item := range r.Inputs {
		if item.ID == "" {
			return fmt.Errorf("embedding item id is required")
		}
		if _, ok := seen[item.ID]; ok {
			return fmt.Errorf("duplicate embedding item id: %s", item.ID)
		}
		seen[item.ID] = struct{}{}
		if strings.TrimSpace(item.Text) == "" {
			return fmt.Errorf("embedding item text must not be blank")
		}
		itemBytes := len(item.Text)
		if itemBytes > MaxEmbeddingItemBytes {
			return &RuntimeError{Category: ErrorInputTooLarge, Message: fmt.Sprintf("embedding item exceeds %d bytes", MaxEmbeddingItemBytes)}
		}
		totalBytes += itemBytes
		estimatedTokens += estimateEmbeddingTokens(item.Text)
	}
	if totalBytes > MaxEmbeddingBatchBytes {
		return &RuntimeError{Category: ErrorBatchTooLarge, Message: fmt.Sprintf("embedding batch exceeds %d bytes", MaxEmbeddingBatchBytes)}
	}
	if estimatedTokens > MaxEmbeddingEstimatedTokens {
		return &RuntimeError{Category: ErrorBatchTooLarge, Message: "embedding batch estimated tokens exceed platform limit"}
	}
	return nil
}

func validEmbeddingPurpose(v EmbeddingPurpose) bool {
	switch v {
	case EmbeddingPurposeToolIndex, EmbeddingPurposeKnowledgeIngestion, EmbeddingPurposeKnowledgeQuery, EmbeddingPurposeMemory, EmbeddingPurposeOther:
		return true
	default:
		return false
	}
}

func estimateEmbeddingTokens(text string) int {
	runes := utf8.RuneCountInString(text)
	if runes == 0 {
		return 0
	}
	byBytes := (len(text) + 3) / 4
	if byBytes > runes {
		return byBytes
	}
	return runes
}

func EmbeddingInputStats(inputs []EmbeddingItem) (count int, bytes int64) {
	count = len(inputs)
	for _, item := range inputs {
		bytes += int64(len(item.Text))
	}
	return count, bytes
}

func ValidateResolvedEmbeddingDimensions(model *aimodel.ResolvedModel, requested *int) error {
	if model == nil {
		return &RuntimeError{Category: ErrorInvalidRequest, Message: "resolved model is required"}
	}
	if requested == nil {
		return nil
	}
	dims := int64(*requested)
	if model.Limits.MaxDimensions > 0 && dims > model.Limits.MaxDimensions {
		return &RuntimeError{Category: ErrorDimensionsUnsupported, Message: "requested dimensions exceed deployment max_dimensions"}
	}
	if model.Limits.MaxDimensions == 0 && model.Limits.Dimensions > 0 && dims != model.Limits.Dimensions {
		return &RuntimeError{Category: ErrorDimensionsUnsupported, Message: "requested dimensions are not supported by deployment"}
	}
	return nil
}

func ValidateEmbeddingVectors(expected []EmbeddingItem, vectors []EmbeddingVector, dimensions int) error {
	if dimensions < 1 {
		return &RuntimeError{Category: ErrorInvalidVector, Message: "embedding dimensions must be positive"}
	}
	if len(vectors) != len(expected) {
		return &RuntimeError{Category: ErrorResponseMismatch, Message: "embedding response count mismatch"}
	}
	totalElements := 0
	seenIndex := map[int]struct{}{}
	seenID := map[string]struct{}{}
	byID := map[string]EmbeddingVector{}
	for _, v := range vectors {
		if _, ok := seenIndex[v.Index]; ok {
			return &RuntimeError{Category: ErrorResponseMismatch, Message: "duplicate embedding response index"}
		}
		seenIndex[v.Index] = struct{}{}
		if v.ID == "" {
			return &RuntimeError{Category: ErrorResponseMismatch, Message: "embedding response item id is required"}
		}
		if _, ok := seenID[v.ID]; ok {
			return &RuntimeError{Category: ErrorResponseMismatch, Message: "duplicate embedding response item id"}
		}
		seenID[v.ID] = struct{}{}
		if len(v.Vector) == 0 {
			return &RuntimeError{Category: ErrorInvalidVector, Message: "empty embedding vector"}
		}
		if len(v.Vector) != dimensions {
			return &RuntimeError{Category: ErrorInvalidVector, Message: "inconsistent embedding dimensions"}
		}
		for _, f := range v.Vector {
			if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
				return &RuntimeError{Category: ErrorInvalidVector, Message: "embedding vector contains NaN or Inf"}
			}
		}
		totalElements += len(v.Vector)
		byID[v.ID] = v
	}
	if totalElements > MaxEmbeddingVectorElements {
		return &RuntimeError{Category: ErrorBatchTooLarge, Message: "embedding vector element count exceeds platform limit"}
	}
	for i, item := range expected {
		v, ok := byID[item.ID]
		if !ok {
			return &RuntimeError{Category: ErrorResponseMismatch, Message: "missing embedding response item"}
		}
		if v.Index != i {
			return &RuntimeError{Category: ErrorResponseMismatch, Message: "embedding response order mismatch"}
		}
	}
	return nil
}

func ApplyEmbeddingPreprocessing(resp *EmbeddingResponse, policy EmbeddingPreprocessing) error {
	if resp == nil {
		return fmt.Errorf("embedding response is required")
	}
	switch policy {
	case "", EmbeddingPreprocessingNone:
		resp.Preprocessing = EmbeddingPreprocessingNone
		resp.PreprocessingVersion = EmbeddingPreprocessingVersionNone
		return nil
	case EmbeddingPreprocessingCosineNorm:
		for i := range resp.Vectors {
			if err := cosineNormalize(resp.Vectors[i].Vector); err != nil {
				return err
			}
		}
		resp.Preprocessing = EmbeddingPreprocessingCosineNorm
		resp.PreprocessingVersion = EmbeddingPreprocessingVersionCosine
		return nil
	default:
		return &RuntimeError{Category: ErrorInvalidRequest, Message: "unsupported embedding preprocessing"}
	}
}

func cosineNormalize(v []float32) error {
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	if sum == 0 {
		return &RuntimeError{Category: ErrorInvalidVector, Message: "cannot normalize zero vector"}
	}
	norm := math.Sqrt(sum)
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
	return nil
}

func embeddingModelSummary(alias string, model *aimodel.ResolvedModel) ModelSummary {
	return ModelSummary{Alias: alias, DeploymentID: model.DeploymentID, Protocol: model.Protocol, RemoteModelID: model.RemoteModelID}
}
