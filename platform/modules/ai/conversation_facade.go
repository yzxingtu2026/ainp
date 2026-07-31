package ai

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/conversation"
	"gorm.io/gorm"
)

type (
	Conversation                     = conversation.Conversation
	ConversationMessage              = conversation.ConversationMessage
	AssistantStream                  = conversation.AssistantStream
	AssistantStreamDelta             = conversation.AssistantStreamDelta
	ContextSnapshot                  = conversation.ContextSnapshot
	AgentRun                         = conversation.AgentRun
	AgentStep                        = conversation.AgentStep
	AgentRunEvent                    = conversation.AgentRunEvent
	ConversationService              = conversation.ConversationService
	ContextCompactionService         = conversation.ContextCompactionService
	AgentRuntimeService              = conversation.AgentRuntimeService
	ConversationListRequest          = conversation.ConversationListRequest
	ConversationCreateRequest        = conversation.ConversationCreateRequest
	ConversationCreateForSlotRequest = conversation.ConversationCreateForSlotRequest
	ConversationGetRequest           = conversation.ConversationGetRequest
	ConversationUpdateRequest        = conversation.ConversationUpdateRequest
	ConversationStateRequest         = conversation.ConversationStateRequest
	MessageListRequest               = conversation.MessageListRequest
	MessageGetRequest                = conversation.MessageGetRequest
	MessageCreateRequest             = conversation.MessageCreateRequest
	SnapshotCreateRequest            = conversation.SnapshotCreateRequest
	ModelContextBudget               = conversation.ModelContextBudget
	CompactionDiagnosticsRequest     = conversation.CompactionDiagnosticsRequest
	AgentRunStartRequest             = conversation.AgentRunStartRequest
	AgentRunIDRequest                = conversation.AgentRunIDRequest
	AgentRunListRequest              = conversation.AgentRunListRequest
	AgentRunEventsRequest            = conversation.AgentRunEventsRequest
)

const (
	ConversationPurposeWorkspace      = conversation.ConversationPurposeWorkspace
	ConversationPurposeBuilderPreview = conversation.ConversationPurposeBuilderPreview
	CodeSetAgentRunStatus             = conversation.CodeSetAgentRunStatus
	CodeSetAgentStepStatus            = conversation.CodeSetAgentStepStatus
	StateMachineAgentRun              = conversation.StateMachineAgentRun
	AgentRunJobType                   = conversation.AgentRunJobType
)

const agentRunTerminalEvent = conversation.AgentRunTerminalEvent

func NewConversationService(db *gorm.DB, assets aimodelruntime.AssetResolver) *ConversationService {
	return conversation.NewConversationService(db, assets)
}

func NewContextCompactionService(db *gorm.DB, service *ConversationService, models aimodel.Resolver, invocations *InvocationService, tools *aitool.Registry) *ContextCompactionService {
	return conversation.NewContextCompactionService(db, service, models, invocations, tools)
}

func NewAgentRuntimeService(db *gorm.DB, service *ConversationService, invocations *InvocationService, tools *ToolService, compactor aiconversation.ContextCompactor, writer outbox.Writer, authorization auth.AuthorizationProvider, pages *pagecontext.Registry, providers ...runtimecontext.Provider) *AgentRuntimeService {
	return conversation.NewAgentRuntimeService(db, service, invocations, tools, compactor, writer, authorization, pages, providers...)
}

func ResolveModelContextBudget(limits aimodel.Limits) (ModelContextBudget, error) {
	return conversation.ResolveModelContextBudget(limits)
}
func AgentRunJobDefinition() job.Definition { return conversation.AgentRunJobDefinition() }
func RegisterAgentRuntime(a *app.App, service *AgentRuntimeService) error {
	return conversation.RegisterAgentRuntime(a, service)
}

func memoryRecallAdapter(service *MemoryRetrievalService) conversation.MemoryRecallFunc {
	return func(ctx context.Context, request conversation.MemoryRecallRequest) (conversation.MemoryRecallResponse, error) {
		response, err := service.Retrieve(ctx, MemoryRecallRequest{Query: request.Query, AgentID: request.AgentID, Types: request.Types, DateFrom: request.DateFrom, DateTo: request.DateTo, Limit: request.Limit, MaxTokens: request.MaxTokens, NormalOnly: request.NormalOnly})
		if err != nil {
			return conversation.MemoryRecallResponse{}, err
		}
		out := conversation.MemoryRecallResponse{Items: make([]conversation.MemoryRecallItem, len(response.Items))}
		for i, item := range response.Items {
			out.Items[i] = conversation.MemoryRecallItem{Type: item.Type, Summary: item.Summary, Content: item.Content, Score: item.Score, OccurredAt: item.OccurredAt, OccurredEndAt: item.OccurredEndAt, OccurredTimezone: item.OccurredTimezone, LocalDate: item.LocalDate, DatePrecision: item.DatePrecision, Provenance: conversation.MemoryProvenanceRef{ItemID: item.Provenance.ItemID, ItemRevision: item.Provenance.ItemRevision, EvidenceIDs: item.Provenance.EvidenceIDs}}
		}
		return out, nil
	}
}

func asGenerateResponse(value any) (aimodelruntime.GenerateResponse, bool) {
	response, ok := value.(aimodelruntime.GenerateResponse)
	if ok {
		return response, true
	}
	pointer, ok := value.(*aimodelruntime.GenerateResponse)
	if ok && pointer != nil {
		return *pointer, true
	}
	return aimodelruntime.GenerateResponse{}, false
}
