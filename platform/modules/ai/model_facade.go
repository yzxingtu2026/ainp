package ai

import (
	"github.com/yzxingtu2026/ainp/platform/modules/ai/agent"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/conversation"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/experience/navigation"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/experience/personalization"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/knowledge"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/memory"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/modelruntime"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/tooling"
	"gorm.io/gorm"
)

// Deprecated: new code should import the capability-owned model types directly.
type (
	Provider        = modelruntime.Provider
	Deployment      = modelruntime.Deployment
	Alias           = modelruntime.Alias
	ModelInvocation = modelruntime.ModelInvocation

	ToolCatalog                = tooling.ToolCatalog
	ToolSearchDocument         = tooling.ToolSearchDocument
	ToolScope                  = tooling.ToolScope
	ConversationToolWorkingSet = tooling.ConversationToolWorkingSet
	ToolConfirmation           = tooling.ToolConfirmation
	ToolInvocation             = tooling.ToolInvocation

	NavigationSearchDocument = navigation.SearchDocument
)

const (
	ToolConfirmationPending   = tooling.ToolConfirmationPending
	ToolConfirmationApproved  = tooling.ToolConfirmationApproved
	ToolConfirmationRejected  = tooling.ToolConfirmationRejected
	ToolConfirmationExpired   = tooling.ToolConfirmationExpired
	ToolConfirmationConsumed  = tooling.ToolConfirmationConsumed
	ToolConfirmationCancelled = tooling.ToolConfirmationCancelled

	ToolInvocationRunning             = tooling.ToolInvocationRunning
	ToolInvocationWaitingConfirmation = tooling.ToolInvocationWaitingConfirmation
	ToolInvocationCompleted           = tooling.ToolInvocationCompleted
	ToolInvocationFailed              = tooling.ToolInvocationFailed
)

func AutoMigrate(db *gorm.DB) error {
	if err := personalization.RegisterTenantModels(); err != nil {
		return err
	}
	if err := agent.RegisterTenantModels(); err != nil {
		return err
	}
	if err := conversation.RegisterTenantModels(); err != nil {
		return err
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public").Error; err != nil {
		return err
	}
	if err := memory.RegisterTenantModels(); err != nil {
		return err
	}
	if err := navigation.RegisterTenantModels(); err != nil {
		return err
	}
	if db.Migrator().HasTable(&Provider{}) && db.Migrator().HasColumn(&Provider{}, "credential_ref") && !db.Migrator().HasColumn(&Provider{}, "credential_ciphertext") {
		if err := db.Exec(`ALTER TABLE ai_providers ADD COLUMN credential_ciphertext text NOT NULL DEFAULT ''`).Error; err != nil {
			return err
		}
		if err := db.Exec(`UPDATE ai_providers SET status = 'disabled' WHERE credential_ciphertext = ''`).Error; err != nil {
			return err
		}
		if err := db.Migrator().DropColumn(&Provider{}, "credential_ref"); err != nil {
			return err
		}
		if err := db.Exec(`ALTER TABLE ai_providers ALTER COLUMN credential_ciphertext DROP DEFAULT`).Error; err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(&Provider{}, &Deployment{}, &Alias{}, &ModelInvocation{}, &ToolCatalog{}, &ToolSearchDocument{}, &NavigationSearchDocument{}, &ToolScope{}, &ConversationToolWorkingSet{}, &ToolConfirmation{}, &ToolInvocation{}, &Agent{}, &AgentVersion{}, &AgentToolBinding{}, &AgentSlotBinding{}, &AgentSlotPreference{}, &Conversation{}, &ConversationMessage{}, &AssistantStream{}, &AssistantStreamDelta{}, &ContextSnapshot{}, &AgentRun{}, &AgentStep{}, &AgentRunEvent{}, &MemoryItem{}, &MemoryItemVersion{}, &MemoryEvidence{}, &MemoryProjection{}, &MemoryDaySummary{}, &MemoryCurrentProfile{}, &MemoryEvaluation{}, &MemoryGovernanceDecision{}, &MemoryRelation{}, &MemoryConsolidationBatch{}, &AssistantPersonalizationSnapshot{}); err != nil {
		return err
	}
	return knowledge.AutoMigrateKnowledge(db)
}
