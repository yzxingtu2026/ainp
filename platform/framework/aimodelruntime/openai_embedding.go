package aimodelruntime

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
)

// OpenAIEmbeddingAdapter implements /v1/embeddings for OpenAI and OpenAI-compatible providers.
type OpenAIEmbeddingAdapter struct {
	Client         HTTPClient
	DefaultBaseURL string
	MaxBatchItems  int
}

func NewOpenAIEmbeddingAdapter(client HTTPClient) *OpenAIEmbeddingAdapter {
	return newOpenAIFamilyEmbeddingAdapter(client, "https://api.openai.com")
}

func NewOpenAICompatibleEmbeddingAdapter(client HTTPClient) *OpenAIEmbeddingAdapter {
	adapter := newOpenAIFamilyEmbeddingAdapter(client, "")
	adapter.MaxBatchItems = OpenAICompatibleBatchItems
	return adapter
}

func newOpenAIFamilyEmbeddingAdapter(client HTTPClient, defaultBase string) *OpenAIEmbeddingAdapter {
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	return &OpenAIEmbeddingAdapter{Client: client, DefaultBaseURL: defaultBase, MaxBatchItems: OpenAIEmbeddingBatchItems}
}

func (a *OpenAIEmbeddingAdapter) Embed(ctx context.Context, in EmbeddingAdapterRequest) (*EmbeddingResponse, error) {
	if in.Model == nil {
		return nil, &RuntimeError{Category: ErrorInvalidRequest, Message: "resolved model is required"}
	}
	if strings.TrimSpace(in.Model.Credential) == "" {
		return nil, &RuntimeError{Category: ErrorAuthenticationFailed, Message: "provider credential is required"}
	}
	base := strings.TrimSpace(in.Model.BaseURL)
	if base == "" {
		base = a.DefaultBaseURL
	}
	if base == "" {
		return nil, &RuntimeError{Category: ErrorInvalidRequest, Message: "provider endpoint is required"}
	}
	batchSize := a.MaxBatchItems
	if batchSize <= 0 {
		batchSize = OpenAIEmbeddingBatchItems
	}
	// Custom OpenAI-compatible endpoints commonly cap input arrays at 10 items.
	// Keep official OpenAI throughput while using the conservative interoperable limit for proxies.
	if a.DefaultBaseURL == "" || !strings.EqualFold(strings.TrimRight(base, "/"), strings.TrimRight(a.DefaultBaseURL, "/")) {
		if batchSize > OpenAICompatibleBatchItems {
			batchSize = OpenAICompatibleBatchItems
		}
	}
	inputs := in.Request.Inputs
	out := &EmbeddingResponse{
		Model:          embeddingModelSummary(in.Request.ModelAlias, in.Model),
		Vectors:        make([]EmbeddingVector, 0, len(inputs)),
		InputItemCount: len(inputs),
	}
	_, out.InputTextBytes = EmbeddingInputStats(inputs)
	var providerRequestID string
	var usage Usage
	dimensions := 0
	for start := 0; start < len(inputs); start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, classifyTransportError(ctx, err)
		}
		end := start + batchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		batch := inputs[start:end]
		part, err := a.embedBatch(ctx, in, base, batch, start)
		if err != nil {
			return nil, err
		}
		out.SubBatchCount++
		if providerRequestID == "" {
			providerRequestID = part.ProviderRequestID
		}
		if dimensions == 0 {
			dimensions = part.Dimensions
		} else if part.Dimensions != dimensions {
			return nil, &RuntimeError{Category: ErrorInvalidVector, Message: "inconsistent embedding dimensions across sub-batches"}
		}
		usage = mergeEmbeddingUsage(usage, part.Usage)
		out.Vectors = append(out.Vectors, part.Vectors...)
	}
	out.Dimensions = dimensions
	out.Usage = usage
	out.ProviderRequestID = providerRequestID
	if err := ValidateEmbeddingVectors(inputs, out.Vectors, out.Dimensions); err != nil {
		return nil, err
	}
	if err := ApplyEmbeddingPreprocessing(out, in.Request.Preprocessing); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *OpenAIEmbeddingAdapter) embedBatch(ctx context.Context, in EmbeddingAdapterRequest, base string, batch []EmbeddingItem, offset int) (*EmbeddingResponse, error) {
	texts := make([]string, len(batch))
	for i, item := range batch {
		texts[i] = item.Text
	}
	payload := map[string]any{"model": in.Model.RemoteModelID, "input": texts}
	if in.Request.Dimensions != nil {
		payload["dimensions"] = *in.Request.Dimensions
	}
	resp, err := postJSON(ctx, a.Client, endpoint(base, "/v1/embeddings"), map[string]string{"Authorization": "Bearer " + in.Model.Credential}, payload)
	if err != nil {
		return nil, err
	}
	var raw openAIEmbeddingResponse
	if err := decodeBody(resp, &raw); err != nil {
		return nil, err
	}
	if len(raw.Data) == 0 {
		return nil, &RuntimeError{Category: ErrorResponseMismatch, Message: "embedding provider returned no vectors"}
	}
	// Restore input order: providers may return by index but we never trust order alone.
	byIndex := map[int]openAIEmbeddingDatum{}
	for _, item := range raw.Data {
		if _, ok := byIndex[item.Index]; ok {
			return nil, &RuntimeError{Category: ErrorResponseMismatch, Message: "duplicate embedding provider index"}
		}
		byIndex[item.Index] = item
	}
	vectors := make([]EmbeddingVector, 0, len(batch))
	dimensions := 0
	for i := range batch {
		item, ok := byIndex[i]
		if !ok {
			return nil, &RuntimeError{Category: ErrorResponseMismatch, Message: fmt.Sprintf("missing embedding provider index %d", i)}
		}
		vec, err := toFloat32Vector(item.Embedding)
		if err != nil {
			return nil, err
		}
		if dimensions == 0 {
			dimensions = len(vec)
		} else if len(vec) != dimensions {
			return nil, &RuntimeError{Category: ErrorInvalidVector, Message: "inconsistent embedding dimensions"}
		}
		vectors = append(vectors, EmbeddingVector{ID: batch[i].ID, Index: offset + i, Vector: vec})
	}
	out := &EmbeddingResponse{Vectors: vectors, Dimensions: dimensions, ProviderRequestID: raw.ID}
	if raw.Usage != nil {
		out.Usage = Usage{Reported: true}
		if raw.Usage.PromptTokens > 0 || raw.Usage.TotalTokens > 0 {
			out.Usage.InputTokens = int64ptr(raw.Usage.PromptTokens)
			out.Usage.TotalTokens = int64ptr(raw.Usage.TotalTokens)
		} else {
			out.Usage.Reported = false
		}
	}
	return out, nil
}

type openAIEmbeddingResponse struct {
	ID    string                 `json:"id"`
	Data  []openAIEmbeddingDatum `json:"data"`
	Usage *struct {
		PromptTokens int64 `json:"prompt_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
}

type openAIEmbeddingDatum struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

func toFloat32Vector(values []float64) ([]float32, error) {
	if len(values) == 0 {
		return nil, &RuntimeError{Category: ErrorInvalidVector, Message: "empty embedding vector"}
	}
	out := make([]float32, len(values))
	for i, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, &RuntimeError{Category: ErrorInvalidVector, Message: "embedding vector contains NaN or Inf"}
		}
		out[i] = float32(v)
	}
	return out, nil
}

func mergeEmbeddingUsage(left, right Usage) Usage {
	if !left.Reported && !right.Reported {
		return Usage{}
	}
	out := Usage{Reported: true}
	if left.InputTokens != nil || right.InputTokens != nil {
		var sum int64
		if left.InputTokens != nil {
			sum += *left.InputTokens
		}
		if right.InputTokens != nil {
			sum += *right.InputTokens
		}
		out.InputTokens = &sum
	}
	if left.TotalTokens != nil || right.TotalTokens != nil {
		var sum int64
		if left.TotalTokens != nil {
			sum += *left.TotalTokens
		}
		if right.TotalTokens != nil {
			sum += *right.TotalTokens
		}
		out.TotalTokens = &sum
	}
	return out
}
