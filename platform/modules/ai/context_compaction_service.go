package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

type ContextCompactionService struct {
	db            *gorm.DB
	conversations *ConversationService
	models        aimodel.Resolver
	invocations   *InvocationService
	tokenizer     aiconversation.Tokenizer
	memory        aiconversation.DurableMemoryFlushHook
	tools         *aitool.Registry
}

func NewContextCompactionService(db *gorm.DB, conversations *ConversationService, models aimodel.Resolver, invocations *InvocationService, tools *aitool.Registry) *ContextCompactionService {
	return &ContextCompactionService{db: db, conversations: conversations, models: models, invocations: invocations, tokenizer: aiconversation.JSONTokenizer{}, memory: aiconversation.NoopMemoryFlushHook{}, tools: tools}
}

type runtimeCompactionProvider struct {
	invocations *InvocationService
	modelAlias  string
}

func (p runtimeCompactionProvider) Summarize(ctx context.Context, r aiconversation.SummaryRequest) (*aiconversation.CompactionSummary, error) {
	body, err := json.Marshal(map[string]any{"schema_version": r.SchemaVersion, "messages": r.Messages})
	if err != nil {
		return nil, err
	}
	max := int64(r.MaxTokens)
	req := aimodelruntime.GenerateRequest{ModelAlias: p.modelAlias, Instructions: "Summarize only supplied facts into the required JSON schema. Preserve every source_message_id exactly. Never invent identifiers, approvals, side effects, or resources.", Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: string(body)}}}}, StructuredOutput: &aimodelruntime.StructuredOutput{Name: "context_compaction", Strict: true, Schema: compactionSummarySchema()}, MaxOutputTokens: &max, Metadata: map[string]string{"purpose": "context_compaction"}}
	value, err := p.invocations.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	response, ok := asGenerateResponse(value)
	if !ok {
		return nil, fmt.Errorf("invalid compaction response: %T", value)
	}
	var text string
	for _, item := range response.Output {
		if item.Type == aimodelruntime.PartOutputText {
			text += item.Text
		}
	}
	var summary aiconversation.CompactionSummary
	if text == "" || json.Unmarshal([]byte(text), &summary) != nil {
		return nil, fmt.Errorf("invalid structured compaction output")
	}
	return &summary, nil
}

func compactionSummarySchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["goal","decisions","constraints","completed_actions","side_effect_results","active_resources","unresolved_questions","pending_confirmations","failure_state","source_message_ids","source_sequence_from","source_sequence_to"],"properties":{"goal":{"type":"string"},"decisions":{"type":"array","items":{"type":"string"}},"constraints":{"type":"array","items":{"type":"string"}},"completed_actions":{"type":"array","items":{"type":"string"}},"side_effect_results":{"type":"array","items":{"type":"string"}},"active_resources":{"type":"array","items":{"type":"string"}},"unresolved_questions":{"type":"array","items":{"type":"string"}},"pending_confirmations":{"type":"array","items":{"type":"string"}},"failure_state":{"type":"string"},"source_message_ids":{"type":"array","items":{"type":"string"}},"source_sequence_from":{"type":"integer"},"source_sequence_to":{"type":"integer"}}}`)
}

func (s *ContextCompactionService) Compact(ctx context.Context, r aiconversation.CompactionRequest) (*aiconversation.CompactionResult, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	if r.ConversationID == "" || r.ModelAlias == "" || r.IdempotencyKey == "" {
		return nil, errs.New(errs.CodeValidation, "invalid compaction request")
	}
	visible, _, err := s.conversations.get(ctx, r.ConversationID, false)
	if err != nil {
		return nil, err
	}
	// 预算判断只需 chat + 上下文窗口；structured_output 仅在真正摘要时由摘要调用再校验。
	resolved, err := s.models.Resolve(ctx, r.ModelAlias, aimodel.CapabilityRequirements{
		ModelType:        aimodel.ModelTypeChat,
		InputModalities:  []aimodel.Modality{aimodel.ModalityText},
		OutputModalities: []aimodel.Modality{aimodel.ModalityText},
	})
	if err != nil {
		return nil, err
	}
	if resolved.Limits.ContextWindow <= 0 {
		return &aiconversation.CompactionResult{Status: "failed", Reason: "model_context_window_unavailable"}, nil
	}
	repos, repoErr := newAgentRepositories(ctx, s.db)
	if repoErr != nil {
		return nil, repoErr
	}
	version, err := repos.versions.GetByID(visible.AgentVersionID)
	if err != nil {
		return nil, errs.New(errs.CodeNotFound, "agent version not found")
	}
	fixedTokens, _ := s.tokenizer.Count(ctx, version.TenantInstructions)
	toolSchemaBytes := r.ToolSchemaBytes
	bindings, err := repos.tools.Find("agent_id=? AND status=?", visible.AgentID, "enabled")
	if err != nil {
		return nil, err
	}
	if s.tools != nil {
		for _, binding := range bindings {
			if definition, ok := s.tools.Get(binding.ToolID); ok {
				encoded, _ := json.Marshal(definition.InputSchema)
				toolSchemaBytes += len(encoded)
			}
		}
	}
	reserved := r.ReservedOutputTokens
	if reserved <= 0 {
		reserved = int(resolved.Limits.MaxOutputTokens)
		if reserved <= 0 {
			reserved = 2048
		}
	}
	margin := r.SafetyMarginTokens
	if margin <= 0 {
		margin = int(resolved.Limits.ContextWindow / 10)
	}
	available := int(resolved.Limits.ContextWindow) - reserved - margin - fixedTokens - (toolSchemaBytes+3)/4 - r.AttachmentTokens
	if available <= 0 {
		return &aiconversation.CompactionResult{Status: "cannot_fit", Reason: "fixed_context_exceeds_model_window"}, nil
	}
	assembly, err := s.conversations.Assemble(ctx, aiconversation.AssemblyRequest{ConversationID: r.ConversationID, MaxTokens: available, MaxBytes: available * 8})
	if err != nil {
		return nil, err
	}
	attachmentTokens := 0
	for _, message := range assembly.Messages {
		for _, part := range message.Content {
			if part.ImageAttachmentID != "" {
				attachmentTokens += 1024
			}
		}
	}
	if attachmentTokens > r.AttachmentTokens {
		available -= attachmentTokens - r.AttachmentTokens
	}
	if available <= 0 {
		return &aiconversation.CompactionResult{Status: "cannot_fit", Reason: "attachments_exceed_model_window"}, nil
	}
	depth := 0
	if r.ParentSnapshotID != nil {
		tdb, scopeErr := tenantdata.TenantDB(ctx, s.db)
		if scopeErr != nil {
			return nil, scopeErr
		}
		parents, queryErr := tenantdata.TenantRaw[ContextSnapshot](tdb,
			`SELECT * FROM ai_context_snapshots WHERE tenant_id=? AND conversation_id=? AND id=? LIMIT 1`, r.ConversationID, *r.ParentSnapshotID)
		if queryErr != nil || len(parents) != 1 {
			return nil, errs.New(errs.CodeNotFound, "parent snapshot not found")
		}
		depth = parents[0].CompactionDepth + 1
	}
	engine := aiconversation.Engine{Tokenizer: s.tokenizer, Provider: runtimeCompactionProvider{invocations: s.invocations, modelAlias: r.ModelAlias}, Memory: s.memory, Config: aiconversation.EngineConfig{PolicyVersion: "hybrid-v1", PromptVersion: "summary-v1", MaxDepth: 3, RecentMessages: 6}}
	payload, status, err := engine.Compact(ctx, r.ConversationID, *assembly, available, depth)
	if err != nil {
		return &aiconversation.CompactionResult{Status: "failed", Reason: "compaction_failed", CompactionDepth: depth}, err
	}
	result := &aiconversation.CompactionResult{Status: status, InputTokens: assembly.Budget.EstimatedTokens, CompactionDepth: depth}
	if payload == nil {
		if status == "cannot_fit" {
			result.Reason = "compaction_cannot_reduce_context"
		}
		return result, nil
	}
	if status == "within_budget" {
		result.OutputTokens = payload.OutputTokenEstimate
		return result, nil
	}
	payload.ProtectedFacts = map[string]any{"tenant_id": ac.TenantID, "actor_id": ac.UserID, "auth_version": ac.AuthVersion, "agent_version_id": visible.AgentVersionID, "conversation_revision": assembly.ConversationRevision, "pending_confirmations": []string{}, "side_effect_facts": []string{}}
	finalTokens, countErr := s.tokenizer.Count(ctx, payload)
	if countErr != nil {
		return nil, countErr
	}
	if finalTokens >= payload.InputTokenEstimate || finalTokens > available {
		return &aiconversation.CompactionResult{Status: "cannot_fit", Reason: "protected_facts_exceed_budget", InputTokens: payload.InputTokenEstimate, OutputTokens: finalTokens, CompactionDepth: depth}, nil
	}
	payload.OutputTokenEstimate = finalTokens
	result.OutputTokens = payload.OutputTokenEstimate
	encoded, _ := json.Marshal(payload)
	snap, err := s.conversations.CreateSnapshot(ctx, SnapshotCreateRequest{ConversationID: r.ConversationID, ModelDeploymentID: resolved.DeploymentID, Kind: "compacted", PolicyVersion: payload.PolicyVersion, SchemaVersion: payload.SchemaVersion, PromptVersion: payload.PromptVersion, SummarizerDeploymentID: resolved.DeploymentID, Reason: payload.Reason, QualityStatus: "valid", CompactionDepth: depth, IdempotencyKey: r.IdempotencyKey, ParentSnapshotID: r.ParentSnapshotID, Payload: encoded, TokenEstimate: payload.OutputTokenEstimate, SourceSequenceFrom: assembly.SourceSequenceFrom, SourceSequenceTo: assembly.SourceSequenceTo})
	if err != nil {
		return nil, err
	}
	result.SnapshotID = snap.ID
	result.RetainedFrom = payload.RetainedMessages[0].Sequence
	result.RetainedTo = payload.RetainedMessages[len(payload.RetainedMessages)-1].Sequence
	return result, nil
}

type CompactionDiagnosticsRequest struct {
	ConversationID string `json:"conversation_id" validate:"required"`
	ModelAlias     string `json:"model_alias"`
}

func (s *ContextCompactionService) Diagnostics(ctx context.Context, r CompactionDiagnosticsRequest) (any, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	if _, _, err := s.conversations.get(ctx, r.ConversationID, false); err != nil {
		return nil, err
	}
	tdb, err := tenantdata.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	rows, err := tenantdata.TenantRaw[ContextSnapshot](tdb, `SELECT id, tenant_id, conversation_id, conversation_revision,
source_sequence_from, source_sequence_to, policy_version, compaction_depth, reason, quality_status, token_estimate, created_at
FROM ai_context_snapshots WHERE tenant_id=? AND conversation_id=? AND kind=? ORDER BY created_at DESC LIMIT 20`, r.ConversationID, "compacted")
	if err != nil {
		return nil, err
	}
	result := map[string]any{"snapshots": rows}
	if r.ModelAlias != "" {
		resolved, err := s.models.Resolve(ctx, r.ModelAlias, aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat, InputModalities: []aimodel.Modality{aimodel.ModalityText}, OutputModalities: []aimodel.Modality{aimodel.ModalityText}})
		if err != nil {
			return nil, err
		}
		if resolved.Limits.ContextWindow <= 0 {
			result["status"] = "failed"
			result["reason"] = "model_context_window_unavailable"
			return result, nil
		}
		reserved := int(resolved.Limits.MaxOutputTokens)
		if reserved <= 0 {
			reserved = 2048
		}
		margin := int(resolved.Limits.ContextWindow / 10)
		available := int(resolved.Limits.ContextWindow) - reserved - margin
		if available <= 0 {
			result["status"] = "cannot_fit"
			result["reason"] = "fixed_context_exceeds_model_window"
			return result, nil
		}
		assembly, err := s.conversations.Assemble(ctx, aiconversation.AssemblyRequest{ConversationID: r.ConversationID, MaxTokens: available, MaxBytes: available * 8})
		if err != nil {
			return nil, err
		}
		result["status"] = assembly.Status
		result["reason"] = assembly.Reason
		result["budget"] = map[string]any{"context_window": resolved.Limits.ContextWindow, "reserved_output_tokens": reserved, "safety_margin_tokens": margin, "available_tokens": available, "estimated_tokens": assembly.Budget.EstimatedTokens}
	}
	return result, nil
}
