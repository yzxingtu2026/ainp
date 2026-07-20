package ai

import (
	"encoding/json"
	"gorm.io/gorm"
	"time"
)

type Agent struct {
	ID                 string  `gorm:"primaryKey;size:64" json:"id"`
	TenantID           string  `gorm:"size:64;not null;uniqueIndex:ux_ai_agents_tenant_code" json:"-"`
	OwnerUserID        *string `gorm:"size:64;index" json:"owner_user_id,omitempty"`
	ContractKey        string  `gorm:"size:128;index" json:"contract_key,omitempty"`
	ContractVersion    string  `gorm:"size:32" json:"contract_version,omitempty"`
	Code               string  `gorm:"size:64;not null;uniqueIndex:ux_ai_agents_tenant_code" json:"code"`
	Name               string  `gorm:"size:128;not null" json:"name"`
	Description        string  `gorm:"size:512" json:"description"`
	AvatarAttachmentID *string `gorm:"size:64" json:"avatar_attachment_id,omitempty"`
	Source             string  `gorm:"size:16;not null;index" json:"source"`
	Visibility         string  `gorm:"size:16;not null;index" json:"visibility"`
	Status             string  `gorm:"size:16;not null;index" json:"status"`
	CurrentVersion     int     `gorm:"not null;default:0" json:"current_version"`
	// Builder 草稿：与元数据一同保存；Workspace/正式 Run 仍只消费 current Version。
	HasDraft                bool            `gorm:"not null;default:false" json:"has_draft"`
	DraftTenantInstructions string          `gorm:"type:text" json:"draft_tenant_instructions,omitempty"`
	DraftModelAlias         string          `gorm:"size:64" json:"draft_model_alias,omitempty"`
	DraftGenerationConfig   json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"draft_generation_config,omitempty"`
	DraftToolPolicy         json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"draft_tool_policy,omitempty"`
	DraftMemoryPolicy       json.RawMessage `gorm:"type:jsonb" json:"draft_memory_policy,omitempty"`
	Revision                uint64          `gorm:"not null;default:1" json:"revision"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

func (Agent) TableName() string { return "ai_agents" }

const (
	AgentVersionKindRelease = "release"
	AgentVersionKindPreview = "preview"
)

type AgentVersion struct {
	ID                 string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID           string          `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_version" json:"-"`
	AgentID            string          `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_version;index" json:"agent_id"`
	Version            int             `gorm:"not null;uniqueIndex:ux_ai_agent_version" json:"version"`
	Kind               string          `gorm:"size:16;not null;default:release;index" json:"kind"`
	ContentHash        string          `gorm:"size:64;not null;default:''" json:"content_hash,omitempty"`
	ModelAlias         string          `gorm:"size:64;not null" json:"model_alias"`
	TenantInstructions string          `gorm:"type:text" json:"tenant_instructions,omitempty"`
	GenerationConfig   json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"generation_config"`
	ToolPolicy         json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"tool_policy"`
	MemoryPolicy       json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"memory_policy"`
	MemoryPolicySource string          `gorm:"size:32;not null;default:'system_default'" json:"memory_policy_source"`
	CreatedBy          string          `gorm:"size:64;not null" json:"created_by"`
	CreatedAt          time.Time       `json:"created_at"`
}

func (AgentVersion) TableName() string { return "ai_agent_versions" }

type AgentToolBinding struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID  string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_tool" json:"-"`
	AgentID   string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_tool;index" json:"agent_id"`
	ToolID    string    `gorm:"size:255;not null;uniqueIndex:ux_ai_agent_tool" json:"tool_id"`
	Source    string    `gorm:"size:32;not null" json:"source"`
	Status    string    `gorm:"size:16;not null" json:"status"`
	CreatedBy string    `gorm:"size:64;not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (AgentToolBinding) TableName() string { return "ai_agent_tool_bindings" }

func AutoMigrateAgents(db *gorm.DB) error {
	return db.AutoMigrate(&Agent{}, &AgentVersion{}, &AgentToolBinding{}, &AgentSlotBinding{}, &AgentSlotPreference{})
}
