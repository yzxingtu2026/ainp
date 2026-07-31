package modelruntime

import (
	"encoding/json"
	"time"
)

type Provider struct {
	ID                   string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID             string          `gorm:"size:64;not null;uniqueIndex:ux_ai_providers_tenant_code" json:"-"`
	Code                 string          `gorm:"size:64;not null;uniqueIndex:ux_ai_providers_tenant_code" json:"code"`
	Name                 string          `gorm:"size:128;not null" json:"name"`
	Protocol             string          `gorm:"size:64;not null" json:"protocol"`
	BaseURL              string          `gorm:"size:512" json:"base_url"`
	CredentialCiphertext string          `gorm:"type:text;not null" json:"-"`
	CredentialConfigured bool            `gorm:"-" json:"credential_configured"`
	Status               string          `gorm:"size:32;not null;default:enabled" json:"status"`
	ConfigJSON           json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"config_json"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (Provider) TableName() string { return "ai_providers" }

type Deployment struct {
	ID               string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID         string          `gorm:"size:64;not null;uniqueIndex:ux_ai_deployments_tenant_code" json:"-"`
	ProviderID       string          `gorm:"size:64;not null;uniqueIndex:ux_ai_deployments_provider_remote;index" json:"provider_id"`
	Code             string          `gorm:"size:64;not null;uniqueIndex:ux_ai_deployments_tenant_code" json:"code"`
	RemoteModelID    string          `gorm:"size:255;not null;uniqueIndex:ux_ai_deployments_provider_remote" json:"remote_model_id"`
	DisplayName      string          `gorm:"size:128;not null" json:"display_name"`
	ModelType        string          `gorm:"size:32;not null" json:"model_type"`
	CapabilitiesJSON json.RawMessage `gorm:"type:jsonb;not null" json:"capabilities_json"`
	LimitsJSON       json.RawMessage `gorm:"type:jsonb;not null" json:"limits_json"`
	Status           string          `gorm:"size:32;not null;default:enabled" json:"status"`
	ConfigJSON       json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"config_json"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (Deployment) TableName() string { return "ai_model_deployments" }

type Alias struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID     string    `gorm:"size:64;not null;uniqueIndex:ux_ai_aliases_tenant_code" json:"-"`
	Code         string    `gorm:"size:64;not null;uniqueIndex:ux_ai_aliases_tenant_code" json:"code"`
	DeploymentID string    `gorm:"size:64;not null;index" json:"deployment_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Alias) TableName() string { return "ai_model_aliases" }

type ModelInvocation struct {
	ID                       string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID                 string          `gorm:"size:64;not null;index" json:"-"`
	UserID                   string          `gorm:"size:64;not null;index" json:"user_id"`
	SessionID                string          `gorm:"size:64" json:"session_id,omitempty"`
	AgentID                  *string         `gorm:"size:64" json:"agent_id,omitempty"`
	AgentRunID               *string         `gorm:"size:64" json:"agent_run_id,omitempty"`
	ModelAlias               string          `gorm:"size:64;not null" json:"model_alias"`
	ProviderID               string          `gorm:"size:64;not null" json:"provider_id"`
	DeploymentID             string          `gorm:"size:64;not null" json:"deployment_id"`
	RemoteModelID            string          `gorm:"size:255;not null" json:"remote_model_id"`
	Mode                     string          `gorm:"size:16;not null" json:"mode"`
	StructuredOutputStrategy string          `gorm:"size:32" json:"structured_output_strategy,omitempty"`
	Status                   string          `gorm:"size:16;not null;index" json:"status"`
	StartedAt                time.Time       `gorm:"not null" json:"started_at"`
	FirstTokenAt             *time.Time      `json:"first_token_at,omitempty"`
	FinishedAt               *time.Time      `json:"finished_at,omitempty"`
	LatencyMS                *int64          `json:"latency_ms,omitempty"`
	FirstTokenLatencyMS      *int64          `json:"first_token_latency_ms,omitempty"`
	InputTokens              *int64          `json:"input_tokens,omitempty"`
	OutputTokens             *int64          `json:"output_tokens,omitempty"`
	TotalTokens              *int64          `json:"total_tokens,omitempty"`
	CachedInputTokens        *int64          `json:"cached_input_tokens,omitempty"`
	ReasoningTokens          *int64          `json:"reasoning_tokens,omitempty"`
	UsageReported            bool            `gorm:"not null;default:false" json:"usage_reported"`
	FinishReason             string          `gorm:"size:32" json:"finish_reason,omitempty"`
	ErrorCategory            string          `gorm:"size:64" json:"error_category,omitempty"`
	ProviderRequestID        string          `gorm:"size:255" json:"provider_request_id,omitempty"`
	RequestID                string          `gorm:"size:64;index" json:"request_id,omitempty"`
	TraceID                  string          `gorm:"size:64;index" json:"trace_id,omitempty"`
	InputModalitiesJSON      json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"input_modalities"`
	InputAttachmentCount     int             `gorm:"not null;default:0" json:"input_attachment_count"`
	InputAttachmentBytes     int64           `gorm:"not null;default:0" json:"input_attachment_bytes"`
	EmbeddingPurpose         string          `gorm:"size:32" json:"embedding_purpose,omitempty"`
	EmbeddingItemCount       int             `gorm:"not null;default:0" json:"embedding_item_count"`
	EmbeddingTextBytes       int64           `gorm:"not null;default:0" json:"embedding_text_bytes"`
	EmbeddingDimensions      *int            `json:"embedding_dimensions,omitempty"`
	EmbeddingSubBatches      *int            `json:"embedding_sub_batches,omitempty"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

func (ModelInvocation) TableName() string { return "ai_model_invocations" }
