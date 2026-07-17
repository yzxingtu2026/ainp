package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"gorm.io/gorm"
)

const invocationFinishedEvent = "ai.model_invocation.finished.v1"

type InvocationService struct {
	db       *gorm.DB
	resolver aimodel.Resolver
	adapters *aimodelruntime.AdapterRegistry
	writer   outbox.Writer
	tools    ToolProjector
	assets   aimodelruntime.AssetResolver
}

type ToolProjector interface {
	ProjectModelTools(context.Context, string) (string, []aimodelruntime.ToolDefinition, error)
}

func (s *InvocationService) SetToolProjector(v ToolProjector)                { s.tools = v }
func (s *InvocationService) SetAssetResolver(v aimodelruntime.AssetResolver) { s.assets = v }

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
}

func NewInvocationService(db *gorm.DB, resolver aimodel.Resolver, adapters *aimodelruntime.AdapterRegistry, writer outbox.Writer) *InvocationService {
	return &InvocationService{db: db, resolver: resolver, adapters: adapters, writer: writer}
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
	if err := s.finish(ctx, prepared.row, &resp.Usage, resp.FinishReason, "", resp.ProviderRequestID); err != nil {
		return nil, err
	}
	return resp, nil
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
	events := make(chan action.StreamEvent, 16)
	go s.forwardStream(ctx, prepared, source, events)
	return action.StreamResult{Events: events, Cancel: prepared.cancel}, nil
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
	imageCount, imageBytes, imageData, err := s.resolveImages(ctx, &req)
	if err != nil {
		return nil, err
	}
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" {
		return nil, fmt.Errorf("authenticated tenant context is required")
	}
	now := time.Now().UTC()
	modalities, _ := json.Marshal(requirements.InputModalities)
	row := &ModelInvocation{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.UserID, SessionID: ac.SessionID, ModelAlias: req.ModelAlias, ProviderID: resolved.ProviderID, DeploymentID: resolved.DeploymentID, RemoteModelID: resolved.RemoteModelID, Mode: mode, Status: "running", StartedAt: now, RequestID: ac.RequestID, TraceID: ac.TraceID, InputModalitiesJSON: modalities, InputAttachmentCount: imageCount, InputAttachmentBytes: imageBytes}
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
	return &preparedInvocation{ctx: runCtx, cancel: cancel, adapter: adapter, request: aimodelruntime.AdapterRequest{Request: req, Model: resolved}, row: row, imageData: imageData}, nil
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
			if count > 4 {
				releaseResolvedImageData(data)
				return 0, 0, nil, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: "at most 4 images are allowed"}
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

func (s *InvocationService) forwardStream(parent context.Context, p *preparedInvocation, source aimodelruntime.EventStream, out chan<- action.StreamEvent) {
	defer close(out)
	defer p.cancel()
	defer source.Close()
	defer p.releaseImages()
	var usage *aimodelruntime.Usage
	finishReason := aimodelruntime.FinishError
	errorCategory := aimodelruntime.ErrorStreamProtocol
	providerRequestID := ""
	terminal := false
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
			if p.ctx.Err() == nil {
				lastSequence++
				failed := aimodelruntime.StreamEvent{Type: aimodelruntime.EventResponseFailed, Sequence: lastSequence, ResponseID: providerRequestID, FinishReason: aimodelruntime.FinishError, Error: &aimodelruntime.RuntimeError{Category: errorCategory, Message: "model stream failed"}, At: time.Now().UTC()}
				select {
				case out <- action.StreamEvent{Event: string(failed.Type), Data: failed}:
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
			}
		}
		select {
		case out <- action.StreamEvent{Event: string(event.Type), Data: event}:
		case <-p.ctx.Done():
			errorCategory = runtimeErrorCategory(p.ctx.Err())
			finishReason = aimodelruntime.FinishCancelled
			terminal = false
		}
		if p.ctx.Err() != nil {
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
	_ = s.finish(finishCtx, p.row, usage, finishReason, errorCategory, providerRequestID, status)
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
	payload := invocationFinishedPayload{InvocationID: row.ID, Status: status, ModelAlias: row.ModelAlias, Mode: row.Mode, LatencyMS: latency, FirstTokenLatencyMS: row.FirstTokenLatencyMS, FinishReason: string(reason), ErrorCategory: string(category)}
	if usage != nil {
		payload.InputTokens = usage.InputTokens
		payload.OutputTokens = usage.OutputTokens
		payload.TotalTokens = usage.TotalTokens
		payload.UsageReported = usage.Reported
	}
	if s.writer == nil {
		return s.db.WithContext(ctx).Model(&ModelInvocation{}).Where("id = ? AND status = 'running'", row.ID).Updates(updates).Error
	}
	return outbox.WithinTx(ctx, s.db, s.writer, func(tx *gorm.DB, w outbox.Writer) error {
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
	var rows []ModelInvocation
	if err := s.db.WithContext(ctx).Where("status = 'running' AND started_at < ?", cutoff).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		recoveryCtx := auth.WithContext(ctx, &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: row.TenantID, UserID: row.UserID, SessionID: row.SessionID, RequestID: row.RequestID, TraceID: row.TraceID})
		if err := s.finish(recoveryCtx, row, nil, aimodelruntime.FinishError, aimodelruntime.ErrorProviderUnavailable, "", "unknown"); err != nil {
			return err
		}
	}
	return nil
}
