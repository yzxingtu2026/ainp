package modelruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const InvocationFinishedEvent = "ai.model_invocation.finished.v1"

const invocationFinishedEvent = InvocationFinishedEvent

type InvocationService struct {
	db                tenantdata.GuardedDatabase
	trustedRecoveryDB *gorm.DB
	resolver          aimodel.Resolver
	adapters          *aimodelruntime.AdapterRegistry
	embeddingAdapters *aimodelruntime.EmbeddingAdapterRegistry
	writer            outbox.Writer
	tools             ToolProjector
	assets            aimodelruntime.AssetResolver
}

type ToolProjector interface {
	ProjectModelTools(context.Context, string) (string, []aimodelruntime.ToolDefinition, error)
}

func (s *InvocationService) SetToolProjector(v ToolProjector)                { s.tools = v }
func (s *InvocationService) SetAssetResolver(v aimodelruntime.AssetResolver) { s.assets = v }
func (s *InvocationService) SetEmbeddingAdapters(v *aimodelruntime.EmbeddingAdapterRegistry) {
	s.embeddingAdapters = v
}

type invocationFinishedPayload struct {
	InvocationID        string `json:"invocation_id"`
	Status              string `json:"status"`
	ModelAlias          string `json:"model_alias"`
	Mode                string `json:"mode"`
	LatencyMS           int64  `json:"latency_ms"`
	FirstTokenLatencyMS *int64 `json:"first_token_latency_ms,omitempty"`
	InputTokens         *int64 `json:"input_tokens,omitempty"`
	OutputTokens        *int64 `json:"output_tokens,omitempty"`
	TotalTokens         *int64 `json:"total_tokens,omitempty"`
	UsageReported       bool   `json:"usage_reported"`
	FinishReason        string `json:"finish_reason,omitempty"`
	ErrorCategory       string `json:"error_category,omitempty"`
	EmbeddingPurpose    string `json:"embedding_purpose,omitempty"`
	EmbeddingItemCount  int    `json:"embedding_item_count,omitempty"`
	EmbeddingTextBytes  int64  `json:"embedding_text_bytes,omitempty"`
	EmbeddingDimensions *int   `json:"embedding_dimensions,omitempty"`
	EmbeddingSubBatches *int   `json:"embedding_sub_batches,omitempty"`
}

func NewInvocationService(db *gorm.DB, resolver aimodel.Resolver, adapters *aimodelruntime.AdapterRegistry, writer outbox.Writer) *InvocationService {
	if err := registerModelInvocationTenantModel(); err != nil {
		panic(err)
	}
	return &InvocationService{db: tenantdata.GuardedDB(db), trustedRecoveryDB: db, resolver: resolver, adapters: adapters, writer: writer}
}

func (s *InvocationService) Generate(ctx context.Context, req aimodelruntime.GenerateRequest) (any, error) {
	prepared, err := s.prepare(ctx, req, "generate")
	if err != nil {
		return nil, err
	}
	defer prepared.releaseImages()
	resp, err := prepared.adapter.Generate(prepared.ctx, prepared.request)
	prepared.cancel()
	if err != nil {
		_ = s.finish(ctx, prepared.row, nil, aimodelruntime.FinishError, runtimeErrorCategory(err), "")
		return nil, err
	}
	if resp == nil {
		_ = s.finish(ctx, prepared.row, nil, aimodelruntime.FinishError, aimodelruntime.ErrorProviderUnavailable, "")
		return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorProviderUnavailable, Message: "empty model response"}
	}
	if prepared.request.Request.StructuredOutput != nil {
		if err := aimodelruntime.ValidateStructuredResponse(resp, prepared.request.Request.StructuredOutput); err != nil {
			_ = s.finish(ctx, prepared.row, &resp.Usage, aimodelruntime.FinishError, runtimeErrorCategory(err), resp.ProviderRequestID)
			return nil, err
		}
	}
	if err := s.finish(ctx, prepared.row, &resp.Usage, resp.FinishReason, "", resp.ProviderRequestID); err != nil {
		return nil, err
	}
	resp.InvocationID = prepared.row.ID
	// 返回值类型，避免 Agent Runtime 等内部调用方对 *GenerateResponse 断言失败。
	return *resp, nil
}

func (s *InvocationService) Stream(ctx context.Context, req aimodelruntime.GenerateRequest) (any, error) {
	prepared, err := s.prepare(ctx, req, "stream")
	if err != nil {
		return nil, err
	}
	source, err := prepared.adapter.Stream(prepared.ctx, prepared.request)
	if err != nil {
		prepared.cancel()
		prepared.releaseImages()
		_ = s.finish(ctx, prepared.row, nil, aimodelruntime.FinishError, runtimeErrorCategory(err), "")
		return nil, err
	}
	events := make(chan aimodelruntime.StreamEvent, 16)
	terminal := make(chan error, 1)
	go s.forwardStream(ctx, prepared, source, events, terminal)
	return aimodelruntime.ResultStream{InvocationID: prepared.row.ID, Events: events, Terminal: terminal, Cancel: prepared.cancel}, nil
}

func (s *InvocationService) Embed(ctx context.Context, req aimodelruntime.EmbeddingRequest) (*aimodelruntime.EmbeddingResponse, error) {
	prepared, err := s.prepareEmbedding(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := prepared.adapter.Embed(prepared.ctx, prepared.request)
	prepared.cancel()
	if err != nil {
		_ = s.finish(ctx, prepared.row, nil, aimodelruntime.FinishError, runtimeErrorCategory(err), "")
		return nil, err
	}
	dims := resp.Dimensions
	batches := resp.SubBatchCount
	prepared.row.EmbeddingDimensions = &dims
	prepared.row.EmbeddingSubBatches = &batches
	if err := s.finish(ctx, prepared.row, &resp.Usage, aimodelruntime.FinishCompleted, "", resp.ProviderRequestID); err != nil {
		return nil, err
	}
	return resp, nil
}

type preparedEmbedding struct {
	ctx     context.Context
	cancel  context.CancelFunc
	adapter aimodelruntime.EmbeddingAdapter
	request aimodelruntime.EmbeddingAdapterRequest
	row     *ModelInvocation
}

func (s *InvocationService) prepareEmbedding(ctx context.Context, req aimodelruntime.EmbeddingRequest) (*preparedEmbedding, error) {
	req.Normalize()
	if err := req.Validate(ctx); err != nil {
		if re, ok := err.(*aimodelruntime.RuntimeError); ok {
			return nil, re
		}
		return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: err.Error()}
	}
	if s.embeddingAdapters == nil {
		return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorCapabilityNotSupported, Message: "embedding adapter registry is not configured"}
	}
	requirements := aimodel.CapabilityRequirements{
		ModelType:       aimodel.ModelTypeEmbedding,
		InputModalities: []aimodel.Modality{aimodel.ModalityText},
		Features:        []aimodel.Feature{aimodel.FeatureEmbeddings},
	}
	resolved, err := s.resolver.Resolve(ctx, req.ModelAlias, requirements)
	if err != nil {
		category := aimodelruntime.ErrorInvalidRequest
		if appErr, ok := errs.AsAppError(err); ok && appErr.Code == errs.CodeValidation && strings.Contains(appErr.Key(), "capability") {
			category = aimodelruntime.ErrorCapabilityNotSupported
		}
		return nil, &aimodelruntime.RuntimeError{Category: category, Message: "embedding model configuration is unavailable", Retryable: false, Cause: err}
	}
	if err := aimodelruntime.ValidateResolvedEmbeddingDimensions(resolved, req.Dimensions); err != nil {
		return nil, err
	}
	adapter, ok := s.embeddingAdapters.Get(resolved.Protocol)
	if !ok {
		return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorCapabilityNotSupported, Message: "embedding provider protocol is not supported"}
	}
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" {
		return nil, fmt.Errorf("authenticated tenant context is required")
	}
	itemCount, textBytes := aimodelruntime.EmbeddingInputStats(req.Inputs)
	modalities, _ := json.Marshal(requirements.InputModalities)
	now := time.Now().UTC()
	row := &ModelInvocation{
		ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.PrincipalID, SessionID: ac.SessionID,
		ModelAlias: req.ModelAlias, ProviderID: resolved.ProviderID, DeploymentID: resolved.DeploymentID, RemoteModelID: resolved.RemoteModelID,
		Mode: "embedding", Status: "running", StartedAt: now, RequestID: ac.RequestID, TraceID: ac.TraceID,
		InputModalitiesJSON: modalities, EmbeddingPurpose: string(req.Purpose), EmbeddingItemCount: itemCount, EmbeddingTextBytes: textBytes,
	}
	if req.Dimensions != nil {
		dims := *req.Dimensions
		row.EmbeddingDimensions = &dims
	}
	if v := strings.TrimSpace(req.Metadata["agent_id"]); v != "" {
		row.AgentID = &v
	}
	if v := strings.TrimSpace(req.Metadata["agent_run_id"]); v != "" {
		row.AgentRunID = &v
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond)
	return &preparedEmbedding{
		ctx: runCtx, cancel: cancel, adapter: adapter,
		request: aimodelruntime.EmbeddingAdapterRequest{Request: req, Model: resolved},
		row:     row,
	}, nil
}

type preparedInvocation struct {
	ctx       context.Context
	cancel    context.CancelFunc
	adapter   aimodelruntime.ModelAdapter
	request   aimodelruntime.AdapterRequest
	row       *ModelInvocation
	imageData [][]byte
}

func (s *InvocationService) prepare(ctx context.Context, req aimodelruntime.GenerateRequest, mode string) (*preparedInvocation, error) {
	req.Normalize()
	if err := req.Validate(ctx); err != nil {
		return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: err.Error()}
	}
	if mode == "stream" && req.StructuredOutput != nil {
		return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorCapabilityNotSupported, Message: "validated structured output is not supported in stream mode"}
	}
	if s.tools != nil {
		if len(req.Tools) > 0 {
			return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: "tools must be loaded through tool_scope_id"}
		}
		if req.EnableTools || req.ToolScopeID != "" {
			scopeID, definitions, err := s.tools.ProjectModelTools(ctx, req.ToolScopeID)
			if err != nil {
				return nil, err
			}
			req.ToolScopeID = scopeID
			req.Tools = definitions
		}
	}
	requirements := requirementsFor(req, mode == "stream")
	resolved, err := s.resolver.Resolve(ctx, req.ModelAlias, requirements)
	if err != nil {
		return nil, err
	}
	adapter, ok := s.adapters.Get(resolved.Protocol)
	if !ok {
		return nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorCapabilityNotSupported, Message: "model provider protocol is not supported"}
	}
	strategy, err := aimodelruntime.SelectStructuredOutputStrategy(resolved, adapter, req.StructuredOutput)
	if err != nil {
		return nil, err
	}
	imageCount, imageBytes, imageData, err := s.resolveImages(ctx, &req)
	if err != nil {
		return nil, err
	}
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" {
		return nil, fmt.Errorf("authenticated tenant context is required")
	}
	now := time.Now().UTC()
	modalities, _ := json.Marshal(requirements.InputModalities)
	documentCount, documentBytes := derivedDocumentStats(req.Messages)
	row := &ModelInvocation{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.PrincipalID, SessionID: ac.SessionID, ModelAlias: req.ModelAlias, ProviderID: resolved.ProviderID, DeploymentID: resolved.DeploymentID, RemoteModelID: resolved.RemoteModelID, Mode: mode, StructuredOutputStrategy: string(strategy), Status: "running", StartedAt: now, RequestID: ac.RequestID, TraceID: ac.TraceID, InputModalitiesJSON: modalities, InputAttachmentCount: imageCount + documentCount, InputAttachmentBytes: imageBytes + documentBytes}
	if v := strings.TrimSpace(req.Metadata["agent_id"]); v != "" {
		row.AgentID = &v
	}
	if v := strings.TrimSpace(req.Metadata["agent_run_id"]); v != "" {
		row.AgentRunID = &v
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond)
	return &preparedInvocation{ctx: runCtx, cancel: cancel, adapter: adapter, request: aimodelruntime.AdapterRequest{Request: req, Model: resolved, StructuredOutputStrategy: strategy, DisableReasoning: strategy == aimodelruntime.StructuredOutputResponseFunction}, row: row, imageData: imageData}, nil
}

func derivedDocumentStats(messages []aimodelruntime.Message) (int, int64) {
	seen := map[string]bool{}
	count := 0
	total := int64(0)
	for _, message := range messages {
		for _, part := range message.Content {
			source := part.DerivedSource
			if part.Type != aimodelruntime.PartDerivedText || source == nil || source.DerivationType != "document_parse" {
				continue
			}
			key := source.AttachmentID + "\x00" + source.Revision
			if seen[key] {
				continue
			}
			seen[key] = true
			count++
			total += source.SourceBytes
		}
	}
	return count, total
}

func (s *InvocationService) resolveImages(ctx context.Context, req *aimodelruntime.GenerateRequest) (int, int64, [][]byte, error) {
	count := 0
	total := int64(0)
	data := [][]byte{}
	for mi := range req.Messages {
		for pi := range req.Messages[mi].Content {
			part := &req.Messages[mi].Content[pi]
			if part.Type != aimodelruntime.PartInputImage {
				continue
			}
			count++
			if count > aimodelruntime.MaxNativeImagesPerRequest {
				releaseResolvedImageData(data)
				return 0, 0, nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: fmt.Sprintf("at most %d images are allowed", aimodelruntime.MaxNativeImagesPerRequest)}
			}
			if s.assets == nil {
				releaseResolvedImageData(data)
				return 0, 0, nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorCapabilityNotSupported, Message: "image asset resolver is not configured"}
			}
			asset, err := s.assets.Resolve(ctx, aimodelruntime.AssetResolveRequest{AttachmentID: part.ImageAttachmentID, Purpose: "model_input"})
			if err != nil {
				releaseResolvedImageData(data)
				return 0, 0, nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: "image attachment is unavailable", Cause: err}
			}
			if part.AttachmentRevision == "" || asset.Revision != part.AttachmentRevision {
				clear(asset.Data)
				releaseResolvedImageData(data)
				return 0, 0, nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: "image attachment revision changed"}
			}
			total += asset.Size
			if total > 20*1024*1024 {
				clear(asset.Data)
				releaseResolvedImageData(data)
				return 0, 0, nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: "total image size exceeds model input limit"}
			}
			part.ResolvedImage = asset
			data = append(data, asset.Data)
		}
	}
	return count, total, data, nil
}

func releaseResolvedImageData(items [][]byte) {
	for _, data := range items {
		clear(data)
	}
}

func requirementsFor(req aimodelruntime.GenerateRequest, stream bool) aimodel.CapabilityRequirements {
	features := []aimodel.Feature{}
	if stream {
		features = append(features, aimodel.FeatureStreaming)
	}
	if len(req.Tools) > 0 {
		features = append(features, aimodel.FeatureToolCalling)
	}
	if req.StructuredOutput != nil {
		features = append(features, aimodel.FeatureStructuredOutput)
	}
	inputs := []aimodel.Modality{aimodel.ModalityText}
	for _, m := range req.Messages {
		for _, p := range m.Content {
			if p.Type == aimodelruntime.PartInputImage {
				inputs = append(inputs, aimodel.ModalityImage)
				features = append(features, aimodel.FeatureVision)
				goto done
			}
		}
	}
done:
	return aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat, InputModalities: inputs, OutputModalities: []aimodel.Modality{aimodel.ModalityText}, Features: features}
}

func (s *InvocationService) forwardStream(parent context.Context, p *preparedInvocation, source aimodelruntime.EventStream, out chan<- aimodelruntime.StreamEvent, result chan<- error) {
	defer close(out)
	defer close(result)
	defer p.cancel()
	defer source.Close()
	defer p.releaseImages()
	var usage *aimodelruntime.Usage
	finishReason := aimodelruntime.FinishError
	errorCategory := aimodelruntime.ErrorStreamProtocol
	providerRequestID := ""
	terminal := false
	var streamErr error
	lastSequence := int64(0)
	for {
		event, err := source.Recv(p.ctx)
		if err != nil {
			if errors.Is(err, io.EOF) && terminal {
				break
			}
			errorCategory = runtimeErrorCategory(err)
			if p.ctx.Err() != nil {
				errorCategory = runtimeErrorCategory(p.ctx.Err())
				finishReason = aimodelruntime.FinishCancelled
			}
			runtimeErr := &aimodelruntime.RuntimeError{Category: errorCategory, Message: "model stream failed"}
			streamErr = runtimeErr
			if p.ctx.Err() == nil {
				lastSequence++
				failed := aimodelruntime.StreamEvent{Type: aimodelruntime.EventResponseFailed, Sequence: lastSequence, ResponseID: providerRequestID, FinishReason: aimodelruntime.FinishError, Error: runtimeErr, At: time.Now().UTC()}
				select {
				case out <- failed:
					terminal = true
				case <-p.ctx.Done():
				}
			}
			break
		}
		lastSequence = event.Sequence
		if p.row.FirstTokenAt == nil && (event.Type == aimodelruntime.EventOutputTextDelta || event.Type == aimodelruntime.EventToolArgumentsDelta) {
			now := time.Now().UTC()
			latency := now.Sub(p.row.StartedAt).Milliseconds()
			p.row.FirstTokenAt = &now
			p.row.FirstTokenLatencyMS = &latency
		}
		if event.Usage != nil {
			usage = event.Usage
		}
		if event.ResponseID != "" {
			providerRequestID = event.ResponseID
		}
		if event.Type == aimodelruntime.EventResponseCompleted {
			terminal = true
			finishReason = event.FinishReason
			errorCategory = ""
		}
		if event.Type == aimodelruntime.EventResponseFailed {
			terminal = true
			finishReason = aimodelruntime.FinishError
			if event.Error != nil {
				errorCategory = event.Error.Category
				streamErr = event.Error
			} else {
				errorCategory = aimodelruntime.ErrorProviderUnavailable
				streamErr = &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorProviderUnavailable, Message: "model stream failed"}
			}
		}
		// Prefer an already recognized event when the provider terminal and the
		// request deadline become ready together. The fallback remains cancellable
		// so an exited consumer cannot strand the producer.
		delivered := false
		select {
		case out <- event:
			delivered = true
		default:
		}
		if !delivered {
			select {
			case out <- event:
				delivered = true
			case <-p.ctx.Done():
				errorCategory = runtimeErrorCategory(p.ctx.Err())
				finishReason = aimodelruntime.FinishCancelled
				terminal = false
			}
		}
		if p.ctx.Err() != nil {
			break
		}
		if terminal {
			break
		}
	}
	status := "failed"
	if terminal && errorCategory == "" {
		status = "completed"
	}
	if errorCategory == aimodelruntime.ErrorCancelled {
		status = "cancelled"
	}
	if errorCategory == aimodelruntime.ErrorTimeout {
		status = "failed"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	finishErr := s.finish(finishCtx, p.row, usage, finishReason, errorCategory, providerRequestID, status)
	result <- errors.Join(streamErr, finishErr)
}

func (p *preparedInvocation) releaseImages() {
	releaseResolvedImageData(p.imageData)
	p.imageData = nil
	for mi := range p.request.Request.Messages {
		for pi := range p.request.Request.Messages[mi].Content {
			p.request.Request.Messages[mi].Content[pi].ResolvedImage = nil
		}
	}
}

func runtimeErrorCategory(err error) aimodelruntime.ErrorCategory {
	var re *aimodelruntime.RuntimeError
	if errors.As(err, &re) {
		return re.Category
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return aimodelruntime.ErrorTimeout
	}
	if errors.Is(err, context.Canceled) {
		return aimodelruntime.ErrorCancelled
	}
	return aimodelruntime.ErrorProviderUnavailable
}

func (s *InvocationService) finish(ctx context.Context, row *ModelInvocation, usage *aimodelruntime.Usage, reason aimodelruntime.FinishReason, category aimodelruntime.ErrorCategory, providerRequestID string, statusOverride ...string) error {
	status := "completed"
	if category != "" {
		status = "failed"
	}
	if len(statusOverride) > 0 {
		status = statusOverride[0]
	}
	now := time.Now().UTC()
	latency := now.Sub(row.StartedAt).Milliseconds()
	updates := map[string]any{"status": status, "finished_at": now, "latency_ms": latency, "finish_reason": string(reason), "error_category": string(category), "provider_request_id": providerRequestID}
	if row.FirstTokenAt != nil {
		updates["first_token_at"] = row.FirstTokenAt
		updates["first_token_latency_ms"] = row.FirstTokenLatencyMS
	}
	if usage != nil {
		updates["input_tokens"] = usage.InputTokens
		updates["output_tokens"] = usage.OutputTokens
		updates["total_tokens"] = usage.TotalTokens
		updates["cached_input_tokens"] = usage.CachedInputTokens
		updates["reasoning_tokens"] = usage.ReasoningTokens
		updates["usage_reported"] = usage.Reported
	}
	if row.Mode == "embedding" {
		updates["embedding_purpose"] = row.EmbeddingPurpose
		updates["embedding_item_count"] = row.EmbeddingItemCount
		updates["embedding_text_bytes"] = row.EmbeddingTextBytes
		if row.EmbeddingDimensions != nil {
			updates["embedding_dimensions"] = row.EmbeddingDimensions
		}
		if row.EmbeddingSubBatches != nil {
			updates["embedding_sub_batches"] = row.EmbeddingSubBatches
		}
	}
	payload := invocationFinishedPayload{
		InvocationID: row.ID, Status: status, ModelAlias: row.ModelAlias, Mode: row.Mode, LatencyMS: latency,
		FirstTokenLatencyMS: row.FirstTokenLatencyMS, FinishReason: string(reason), ErrorCategory: string(category),
		EmbeddingPurpose: row.EmbeddingPurpose, EmbeddingItemCount: row.EmbeddingItemCount, EmbeddingTextBytes: row.EmbeddingTextBytes,
		EmbeddingDimensions: row.EmbeddingDimensions, EmbeddingSubBatches: row.EmbeddingSubBatches,
	}
	if usage != nil {
		payload.InputTokens = usage.InputTokens
		payload.OutputTokens = usage.OutputTokens
		payload.TotalTokens = usage.TotalTokens
		payload.UsageReported = usage.Reported
	}
	if s.writer == nil {
		return s.db.WithContext(ctx).Model(&ModelInvocation{}).Where("id = ? AND status = 'running'", row.ID).Updates(updates).Error
	}
	return outbox.WithinTx(ctx, s.db.WithContext(ctx), s.writer, func(tx *gorm.DB, w outbox.Writer) error {
		result := tx.Model(&ModelInvocation{}).Where("id = ? AND status = 'running'", row.ID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		_, err := w.Append(ctx, tx, outbox.AppendInput{EventType: invocationFinishedEvent, AggregateType: "ai_model_invocation", AggregateID: row.ID, AggregateVersion: 1, Payload: payload})
		return err
	})
}

func (s *InvocationService) RecoverStale(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().UTC().Add(-olderThan)
	// Startup recovery runs before a tenant AuthContext exists. Keep this fixed
	// cross-tenant scan private, then restore each row's trusted tenant context.
	type staleInvocation ModelInvocation
	var staleRows []staleInvocation
	if err := s.trustedRecoveryDB.WithContext(ctx).Raw("SELECT * FROM ai_model_invocations WHERE status = ? AND started_at < ? ORDER BY started_at", "running", cutoff).Scan(&staleRows).Error; err != nil {
		return err
	}
	for i := range staleRows {
		row := ModelInvocation(staleRows[i])
		recoveryCtx := auth.WithContext(ctx, &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: row.TenantID, PrincipalID: row.UserID, SessionID: row.SessionID, RequestID: row.RequestID, TraceID: row.TraceID})
		if err := s.finish(recoveryCtx, &row, nil, aimodelruntime.FinishError, aimodelruntime.ErrorProviderUnavailable, "", "unknown"); err != nil {
			return err
		}
	}
	return nil
}
