package aimodelruntime

import (
	"context"
	"hash/fnv"
)

// FakeEmbeddingAdapter returns deterministic vectors without network access.
type FakeEmbeddingAdapter struct {
	Dimensions int
	Fail       error
}

func NewFakeEmbeddingAdapter(dimensions int) *FakeEmbeddingAdapter {
	if dimensions <= 0 {
		dimensions = 8
	}
	return &FakeEmbeddingAdapter{Dimensions: dimensions}
}

func (a *FakeEmbeddingAdapter) Embed(ctx context.Context, in EmbeddingAdapterRequest) (*EmbeddingResponse, error) {
	if a.Fail != nil {
		return nil, a.Fail
	}
	if err := ctx.Err(); err != nil {
		return nil, classifyTransportError(ctx, err)
	}
	dims := a.Dimensions
	if in.Request.Dimensions != nil {
		dims = *in.Request.Dimensions
	}
	if dims <= 0 {
		return nil, &RuntimeError{Category: ErrorDimensionsUnsupported, Message: "dimensions must be positive"}
	}
	vectors := make([]EmbeddingVector, 0, len(in.Request.Inputs))
	for i, item := range in.Request.Inputs {
		vectors = append(vectors, EmbeddingVector{ID: item.ID, Index: i, Vector: deterministicEmbedding(item.Text, dims)})
	}
	count, bytes := EmbeddingInputStats(in.Request.Inputs)
	tokens := int64(0)
	for _, item := range in.Request.Inputs {
		tokens += int64(estimateEmbeddingTokens(item.Text))
	}
	out := &EmbeddingResponse{
		Model:          embeddingModelSummary(in.Request.ModelAlias, in.Model),
		Vectors:        vectors,
		Dimensions:     dims,
		Usage:          Usage{InputTokens: &tokens, TotalTokens: &tokens, Reported: true},
		SubBatchCount:  1,
		InputItemCount: count,
		InputTextBytes: bytes,
	}
	if err := ValidateEmbeddingVectors(in.Request.Inputs, out.Vectors, out.Dimensions); err != nil {
		return nil, err
	}
	if err := ApplyEmbeddingPreprocessing(out, in.Request.Preprocessing); err != nil {
		return nil, err
	}
	return out, nil
}

func deterministicEmbedding(text string, dimensions int) []float32 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	seed := h.Sum64()
	out := make([]float32, dimensions)
	for i := range out {
		seed = seed*6364136223846793005 + 1
		// Map to (-1, 1) without NaN/Inf.
		out[i] = float32(int64(seed%2001)-1000) / 1000
	}
	return out
}
