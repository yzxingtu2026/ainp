package conversation

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
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/agent"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/modelruntime"
	"gorm.io/gorm"
)

type ContextCompactionService struct {
	db            *gorm.DB
	conversations *ConversationService
	models        aimodel.Resolver
	invocations   *modelruntime.InvocationService
	tokenizer     aiconversation.Tokenizer
	memory        aiconversation.DurableMemoryFlushHook
	tools         *aitool.Registry
}

func NewContextCompactionService(db *gorm.DB, conversations *ConversationService, models aimodel.Resolver, invocations *modelruntime.InvocationService, tools *aitool.Registry) *ContextCompactionService {
	return &ContextCompactionService{db: db, conversations: conversations, models: models, invocations: invocations, tokenizer: aiconversation.JSONTokenizer{}, memory: aiconversation.NoopMemoryFlushHook{}, tools: tools}
}
func (s *ContextCompactionService) SetMemoryFlushHook(h aiconversation.DurableMemoryFlushHook) {
	if h != nil {
		s.memory = h
	}
}

type ModelContextBudget struct {
	ContextWindow        int
	ReservedOutputTokens int
	SafetyMarginTokens   int
	AvailableTokens      int
}

func ResolveModelContextBudget(limits aimodel.Limits) (ModelContextBudget, error) {
	if limits.ContextWindow <= 0 {
		return ModelContextBudget{}, fmt.Errorf("model context window unavailable")
	}
	reserved := int(limits.MaxOutputTokens)
	if reserved <= 0 {
		reserved = 2048
	}
	margin := int(limits.ContextWindow / 10)
	budget := ModelContextBudget{
		ContextWindow:        int(limits.ContextWindow),
		ReservedOutputTokens: reserved,
		SafetyMarginTokens:   margin,
		AvailableTokens:      int(limits.ContextWindow) - reserved - margin,
	}
	if budget.AvailableTokens <= 0 {
		return ModelContextBudget{}, fmt.Errorf("fixed context exceeds model window")
	}
	return budget, nil
}

func (s *ContextCompactionService) ContextTokenLimit(ctx context.Context, modelAlias string) (int, error) {
	budget, err := s.ModelTurnBudget(ctx, modelAlias)
	if err != nil {
		return 0, err
	}
	return budget.AvailableTokens, nil
}

func (s *ContextCompactionService) ModelTurnBudget(ctx context.Context, modelAlias string) (ModelContextBudget, error) {
	resolved, err := s.models.Resolve(ctx, modelAlias, aimodel.CapabilityRequirements{
		ModelType:        aimodel.ModelTypeChat,
		InputModalities:  []aimodel.Modality{aimodel.ModalityText},
		OutputModalities: []aimodel.Modality{aimodel.ModalityText},
	})
	if err != nil {
		return ModelContextBudget{}, err
	}
	budget, err := ResolveModelContextBudget(resolved.Limits)
	if err != nil {
		return ModelContextBudget{}, err
	}
	return budget, nil
}

func (s *ContextCompactionService) EstimateRequestTokens(ctx context.Context, request aimodelruntime.GenerateRequest) (int, error) {
	request.ToolScopeID = ""
	request.EnableTools = len(request.Tools) > 0
	tokens, err := s.tokenizer.Count(ctx, request)
	if err != nil {
		return 0, err
	}
	for _, message := range request.Messages {
		for _, part := range message.Content {
			if part.ImageAttachmentID != "" {
				tokens += 1024
			}
		}
	}
	return tokens, nil
}

type runtimeCompactionProvider struct {
	invocations *modelruntime.InvocationService
	modelAlias  string
	agentRunID  string
	usageInput  int64
	usageOutput int64
}

func (p *runtimeCompactionProvider) Summarize(ctx context.Context, r aiconversation.SummaryRequest) (*aiconversation.CompactionSummary, error) {
	messages := r.Messages
	for i := range messages {
		messages[i].Content = append([]aimodelruntime.ContentPart(nil), messages[i].Content...)
		for j := range messages[i].Content {
			messages[i].Content[j].DerivedSource = nil
		}
	}
	body, err := json.Marshal(map[string]any{"schema_version": r.SchemaVersion, "messages": messages})
	if err != nil {
		return nil, err
	}
	max := int64(r.MaxTokens)
	metadata := map[string]string{"purpose": "context_compaction"}
	if p.agentRunID != "" {
		metadata["agent_run_id"] = p.agentRunID
	}
	req := aimodelruntime.GenerateRequest{ModelAlias: p.modelAlias, Instructions: prompt.MustGet("ai/context-compaction"), Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: string(body)}}}}, StructuredOutput: &aimodelruntime.StructuredOutput{Name: "context_compaction", Strict: true, Schema: compactionSummarySchema()}, MaxOutputTokens: &max, Metadata: metadata}
	value, err := p.invocations.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	response, ok := asGenerateResponse(value)
	if !ok {
		return nil, fmt.Errorf("invalid compaction response: %T", value)
	}
	if response.Usage.InputTokens != nil {
		p.usageInput += *response.Usage.InputTokens
	}
	if response.Usage.OutputTokens != nil {
		p.usageOutput += *response.Usage.OutputTokens
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
	tdb, scopeErr := tenantdata.TenantDB(ctx, s.db)
	if scopeErr != nil {
		return nil, scopeErr
	}
	existing, err := tenantdata.TenantRaw[ContextSnapshot](tdb, `SELECT * FROM ai_context_snapshots WHERE tenant_id=? AND conversation_id=? AND idempotency_key=? LIMIT 1`, r.ConversationID, r.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if len(existing) == 1 {
		snapshot := existing[0]
		return &aiconversation.CompactionResult{Status: "compacted", SnapshotID: snapshot.ID, OutputTokens: snapshot.TokenEstimate, RetainedFrom: snapshot.SourceSequenceFrom, RetainedTo: snapshot.SourceSequenceTo, CompactionDepth: snapshot.CompactionDepth}, nil
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
	contextBudget, budgetErr := ResolveModelContextBudget(resolved.Limits)
	if budgetErr != nil {
		return &aiconversation.CompactionResult{Status: "failed", Reason: "model_context_window_unavailable"}, nil
	}
	version, err := agent.GetVersion(ctx, s.db, visible.AgentVersionID)
	if err != nil {
		return nil, errs.New(errs.CodeNotFound, "agent version not found")
	}
	fixedTokens, _ := s.tokenizer.Count(ctx, version.TenantInstructions)
	toolSchemaBytes := r.ToolSchemaBytes
	if !r.DisableTools {
		bindings, err := agent.ListEnabledToolBindings(ctx, s.db, visible.AgentID)
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
	}
	reserved := r.ReservedOutputTokens
	if reserved <= 0 {
		reserved = contextBudget.ReservedOutputTokens
	}
	margin := r.SafetyMarginTokens
	if margin <= 0 {
		margin = contextBudget.SafetyMarginTokens
	}
	available := int(resolved.Limits.ContextWindow) - reserved - margin - fixedTokens - (toolSchemaBytes+3)/4 - r.AttachmentTokens
	if r.TargetInputTokens > 0 && r.TargetInputTokens < available {
		available = r.TargetInputTokens
	}
	if available <= 0 {
		return &aiconversation.CompactionResult{Status: "cannot_fit", Reason: "fixed_context_exceeds_model_window"}, nil
	}
	assembly, err := s.conversations.Assemble(ctx, aiconversation.AssemblyRequest{ConversationID: r.ConversationID, TriggerMessageID: r.TriggerMessageID, MaxTokens: available, MaxBytes: available * 8})
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
	provider := &runtimeCompactionProvider{invocations: s.invocations, modelAlias: r.ModelAlias, agentRunID: r.AgentRunID}
	engine := aiconversation.Engine{Tokenizer: s.tokenizer, Provider: provider, Memory: s.memory, Config: aiconversation.EngineConfig{PolicyVersion: "hybrid-v1", PromptVersion: "summary-v1", MaxDepth: 3, RecentMessages: 6}}
	payload, status, err := engine.Compact(ctx, r.ConversationID, *assembly, available, depth)
	if err != nil {
		return &aiconversation.CompactionResult{Status: "failed", Reason: "compaction_failed", CompactionDepth: depth}, err
	}
	result := &aiconversation.CompactionResult{Status: status, InputTokens: assembly.Budget.EstimatedTokens, CompactionDepth: depth, UsageInputTokens: provider.usageInput, UsageOutputTokens: provider.usageOutput}
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
	if !retainsAllDocumentDerivations(assembly.Messages, payload.RetainedMessages) {
		return &aiconversation.CompactionResult{Status: "cannot_fit", Reason: "attachments_exceed_model_window", InputTokens: payload.InputTokenEstimate, OutputTokens: payload.OutputTokenEstimate, CompactionDepth: depth}, nil
	}
	if r.Reason != "" {
		payload.Reason = r.Reason
	}
	payload.ProtectedFacts = map[string]any{"tenant_id": ac.TenantID, "actor_id": ac.PrincipalID, "auth_version": ac.AuthVersion, "agent_version_id": visible.AgentVersionID, "conversation_revision": assembly.ConversationRevision, "pending_confirmations": []string{}, "side_effect_facts": []string{}}
	finalTokens, countErr := s.tokenizer.Count(ctx, payload)
	if countErr != nil {
		return nil, countErr
	}
	if finalTokens >= payload.InputTokenEstimate || finalTokens > available {
		return &aiconversation.CompactionResult{Status: "cannot_fit", Reason: "protected_facts_exceed_budget", InputTokens: payload.InputTokenEstimate, OutputTokens: finalTokens, CompactionDepth: depth}, nil
	}
	payload.OutputTokenEstimate = finalTokens
	result.OutputTokens = payload.OutputTokenEstimate
	payload.RetainedMessages = canonicalizeDocumentDerivations(payload.RetainedMessages)
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

func retainsAllDocumentDerivations(source, retained []aiconversation.Message) bool {
	want := documentDerivationKeys(source)
	if len(want) == 0 {
		return true
	}
	got := documentDerivationKeys(retained)
	for key := range want {
		if !got[key] {
			return false
		}
	}
	return true
}

func documentDerivationKeys(messages []aiconversation.Message) map[string]bool {
	keys := map[string]bool{}
	for _, message := range messages {
		for _, part := range message.Content {
			if part.Type == aimodelruntime.PartDerivedText && part.DerivedSource != nil && part.DerivedSource.DerivationType == "document_parse" {
				keys[part.DerivedSource.AttachmentID+"\x00"+part.DerivedSource.Revision] = true
			}
		}
	}
	return keys
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
		contextBudget, budgetErr := ResolveModelContextBudget(resolved.Limits)
		if budgetErr != nil {
			result["status"] = "failed"
			result["reason"] = "model_context_window_unavailable"
			return result, nil
		}
		reserved := contextBudget.ReservedOutputTokens
		margin := contextBudget.SafetyMarginTokens
		available := contextBudget.AvailableTokens
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
