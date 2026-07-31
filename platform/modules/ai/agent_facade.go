package ai

import (
	"github.com/yzxingtu2026/ainp/platform/framework/agentslot"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/agent"
	"gorm.io/gorm"
)

type (
	Agent                          = agent.Agent
	AgentVersion                   = agent.AgentVersion
	AgentToolBinding               = agent.AgentToolBinding
	AgentSlotBinding               = agent.AgentSlotBinding
	AgentSlotPreference            = agent.AgentSlotPreference
	AgentService                   = agent.AgentService
	AgentSlotService               = agent.AgentSlotService
	AgentListRequest               = agent.AgentListRequest
	AgentCreateRequest             = agent.AgentCreateRequest
	AgentUpdateRequest             = agent.AgentUpdateRequest
	AgentVersionCreateRequest      = agent.AgentVersionCreateRequest
	AgentPreviewMaterializeRequest = agent.AgentPreviewMaterializeRequest
	AgentBindRequest               = agent.AgentBindRequest
	AgentUnbindRequest             = agent.AgentUnbindRequest
	AgentStatusRequest             = agent.AgentStatusRequest
	AgentDiagnosticIssue           = agent.AgentDiagnosticIssue
	AgentDiagnostics               = agent.AgentDiagnostics
	AgentToolSearchRequest         = agent.AgentToolSearchRequest
	AgentEmptyRequest              = agent.AgentEmptyRequest
	AgentDetail                    = agent.AgentDetail
	AgentDuplicateRequest          = agent.AgentDuplicateRequest
	AgentSlotListRequest           = agent.AgentSlotListRequest
	AgentSlotGetRequest            = agent.AgentSlotGetRequest
	AgentSlotMutationRequest       = agent.AgentSlotMutationRequest
	AgentSlotResetRequest          = agent.AgentSlotResetRequest
	AgentSlotResolution            = agent.AgentSlotResolution
)

const (
	AgentAvatarResourceType = agent.AgentAvatarResourceType
	AgentAvatarField        = agent.AgentAvatarField
	AgentVersionKindRelease = agent.AgentVersionKindRelease
	AgentVersionKindPreview = agent.AgentVersionKindPreview
	WorkspaceDefaultSlot    = agent.WorkspaceDefaultSlot
)

func NewAgentService(db *gorm.DB, contracts *aiagent.Registry, models aimodel.Resolver, tools *aitool.Registry) *AgentService {
	return agent.NewAgentService(db, contracts, models, tools)
}

func NewAgentSlotService(db *gorm.DB, slots *agentslot.Registry, agents *AgentService) *AgentSlotService {
	return agent.NewAgentSlotService(db, slots, agents)
}

func AutoMigrateAgents(db *gorm.DB) error { return agent.AutoMigrateAgents(db) }

func newAgentAvatarAttachmentPolicy(service *AgentService) attachment.AccessPolicy {
	return agent.NewAvatarAttachmentPolicy(service)
}
